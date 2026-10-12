package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/config"
	"github.com/jemang/headboard/internal/store"
)

// Recovery commands, run inside the container:
//
//	docker exec <container> headboard reset-password --email you@example.com
//	docker exec <container> headboard reset-pin --email you@example.com
//
// There is no web endpoint for either. Whoever can run them already has a
// shell next to the database, which is the boundary these commands rely on.
var resetCommands = map[string]string{
	"reset-password": "give a password account a new password, printed once",
	"reset-pin":      "clear an account's sudo PIN; they set a new one at their next sign-in",
}

// runReset opens the server's database without migrating it and runs one
// reset. It never starts the server.
func runReset(name string, args []string, stdout, stderr io.Writer) int {
	path, err := databasePath()
	if err != nil {
		fmt.Fprintln(stderr, err)

		return 1
	}

	ctx := context.Background()

	st, err := store.OpenExisting(ctx, path)
	if err != nil {
		fmt.Fprintln(stderr, err)

		return 1
	}
	defer st.Close()

	if err := reset(ctx, st, name, args, stdout); err != nil {
		fmt.Fprintln(stderr, err)

		return 1
	}

	return 0
}

func reset(ctx context.Context, st *store.Store, name string, args []string, out io.Writer) error {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(out)
	email := fs.String("email", "", "the account's email")
	id := fs.Int64("id", 0, "the account's id, when one email has several accounts")
	fs.Usage = func() {
		fmt.Fprintf(out, "usage: headboard %s --email <email> [--id <id>]\n%s\n", name, resetCommands[name])
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	u, err := pickAccount(ctx, st, name, *email, *id)
	if err != nil {
		return err
	}

	switch name {
	case "reset-password":
		return resetPassword(ctx, st, u, out)
	case "reset-pin":
		return resetPin(ctx, st, u, out)
	}

	return fmt.Errorf("unknown command %s", name)
}

// pickAccount finds exactly one account, refusing rather than guessing when an
// email is shared: a local account and an identity-provider one may have it.
func pickAccount(ctx context.Context, st *store.Store, name, email string, id int64) (store.User, error) {
	if id != 0 {
		u, err := st.UserByID(ctx, id)
		if err != nil {
			return store.User{}, fmt.Errorf("no account with id %d", id)
		}

		return u, nil
	}

	if email == "" {
		return store.User{}, errors.New("say which account: --email <email>, or --id <id>")
	}

	found, err := st.UsersByEmail(ctx, email)
	if err != nil {
		return store.User{}, err
	}

	// A password can only be reset on a password account, so that narrows it.
	if name == "reset-password" {
		var local []store.User
		for _, u := range found {
			if u.Local() {
				local = append(local, u)
			}
		}
		if len(local) == 0 && len(found) > 0 {
			return store.User{}, fmt.Errorf("%s signs in through the identity provider; its password lives there", email)
		}
		found = local
	}

	switch len(found) {
	case 0:
		return store.User{}, fmt.Errorf("no account with the email %s", email)
	case 1:
		return found[0], nil
	}

	lines := make([]string, len(found))
	for i, u := range found {
		how := "password"
		if !u.Local() {
			how = "identity provider " + u.OIDCIssuer
		}
		lines[i] = fmt.Sprintf("  --id %d  (%s, %s)", u.ID, how, u.Role)
	}

	return store.User{}, fmt.Errorf("%d accounts use %s; run again with one of:\n%s", len(found), email, strings.Join(lines, "\n"))
}

func resetPassword(ctx context.Context, st *store.Store, u store.User, out io.Writer) error {
	if !u.Local() {
		return fmt.Errorf("%s signs in through the identity provider; its password lives there", u.Email)
	}

	password, err := auth.GeneratePassword()
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	if err := st.ResetPassword(ctx, u.ID, hash, cliEntry(u, "account.password_reset")); err != nil {
		return err
	}

	fmt.Fprintf(out, "New password for %s\n\n  password: %s\n\nIt is shown once. Every session this account had is signed out.\n", u.Email, password)

	return nil
}

func resetPin(ctx context.Context, st *store.Store, u store.User, out io.Writer) error {
	if err := st.ResetPin(ctx, u.ID, cliEntry(u, "account.pin_reset")); err != nil {
		return err
	}

	fmt.Fprintf(out, "Cleared the sudo PIN for %s (id %d).\nEvery session this account had is signed out; they set a new PIN when they next sign in.\n", u.Email, u.ID)

	return nil
}

// cliEntry is the activity row for a reset, under "cli" since no account did
// it. The store writes it in the same transaction as the reset.
func cliEntry(u store.User, action string) store.AuditEntry {
	return store.AuditEntry{
		ActorLabel: "cli", Action: action, TargetType: "account",
		TargetID: strconv.FormatInt(u.ID, 10), TargetLabel: u.Email,
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

// databasePath is the server's own setting, read from the same environment the
// server runs with inside the container.
func databasePath() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}

	return cfg.DatabaseURL, nil
}
