package api

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/store"
)

// Sudo mode: dangerous actions need the caller's six-digit PIN, entered within
// the last five minutes. The PIN is set once per account and is mandatory for
// every role that can change things (auth.NeedsPin).
const (
	sudoWindow   = 5 * time.Minute
	reauthWindow = 5 * time.Minute

	// A wrong PIN is counted on the account, in the database: five lock sudo
	// for fifteen minutes whatever browser or address the guesses come from.
	pinMaxFailures = 5
	pinLock        = 15 * time.Minute

	// The per-address cap covers a spray across many accounts.
	ipMaxFailures = 20
	ipWindow      = 15 * time.Minute
)

// SudoSessions opens and closes sudo on the caller's session. *auth.Auth
// implements it; the API never touches the session store directly.
type SudoSessions interface {
	GrantSudo(ctx context.Context, userID int64, until time.Time) error
	EndSudo(ctx context.Context) error
}

// codedError is a problem+json error with a machine-readable code, so the SPA
// can tell "open the PIN prompt" from any other 403.
type codedError struct {
	Status int    `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Code   string `json:"code"`
}

func (e *codedError) Error() string             { return e.Detail }
func (e *codedError) GetStatus() int            { return e.Status }
func (e *codedError) ContentType(string) string { return "application/problem+json" }

func coded(status int, code, detail string) error {
	return &codedError{Status: status, Title: http.StatusText(status), Detail: detail, Code: code}
}

// requireSudo is require plus sudo, for the dangerous actions. Roles that need
// no PIN have no sudo to check: everything they may do is already harmless.
func requireSudo(ctx context.Context, cap auth.Capability) (auth.Principal, error) {
	p, err := require(ctx, cap)
	if err != nil {
		return auth.Principal{}, err
	}

	return p, checkSudo(p)
}

func checkSudo(p auth.Principal) error {
	if !auth.NeedsPin(p.User.Role) || p.SudoUntil.After(time.Now()) {
		return nil
	}

	return coded(http.StatusForbidden, "sudo_required", "confirm with your PIN to do this")
}

func reauthFresh(p auth.Principal) bool {
	return !p.ReauthAt.IsZero() && time.Since(p.ReauthAt) < reauthWindow
}

// weakPin refuses the PINs people pick first: one digit repeated, or a run
// up or down (123456, 987654).
func weakPin(pin string) bool {
	same, up, down := true, true, true
	for i := 1; i < len(pin); i++ {
		d := int(pin[i]) - int(pin[i-1])
		same = same && d == 0
		up = up && d == 1
		down = down && d == -1
	}

	return same || up || down
}

func validPin(pin string) error {
	if len(pin) != 6 {
		return huma.Error422UnprocessableEntity("the PIN is exactly six digits")
	}
	for _, c := range pin {
		if c < '0' || c > '9' {
			return huma.Error422UnprocessableEntity("the PIN is digits only")
		}
	}
	if weakPin(pin) {
		return huma.Error422UnprocessableEntity("that PIN is too easy to guess; avoid repeated digits and runs like 123456")
	}

	return nil
}

// accountLocks serialises PIN and password checks per account, so guesses
// sent at once are counted one after another and the five-try lock holds.
type accountLocks struct {
	mu    sync.Mutex
	locks map[int64]*sync.Mutex
}

func (a *accountLocks) lock(id int64) func() {
	a.mu.Lock()
	if a.locks == nil {
		a.locks = map[int64]*sync.Mutex{}
	}
	l, ok := a.locks[id]
	if !ok {
		l = &sync.Mutex{}
		a.locks[id] = l
	}
	a.mu.Unlock()

	l.Lock()

	return l.Unlock
}

// ipFailures caps wrong PINs per client address, in memory.
type ipFailures struct {
	mu   sync.Mutex
	seen map[string][]time.Time
}

func (f *ipFailures) blocked(ip string, now time.Time) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.recent(ip, now)) >= ipMaxFailures
}

func (f *ipFailures) add(ip string, now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.seen == nil {
		f.seen = map[string][]time.Time{}
	}
	f.seen[ip] = append(f.recent(ip, now), now)
}

// recent drops entries older than the window. Caller holds the lock.
func (f *ipFailures) recent(ip string, now time.Time) []time.Time {
	kept := f.seen[ip][:0]
	for _, t := range f.seen[ip] {
		if now.Sub(t) < ipWindow {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(f.seen, ip)

		return nil
	}
	f.seen[ip] = kept

	return kept
}

type pinInput struct {
	Body struct {
		PIN string `json:"pin"`
		// Password proves a local account's identity when it sets its
		// first PIN. Identity-provider accounts sign in again instead.
		Password string `json:"password,omitempty"`
	}
}

type sudoOutput struct {
	Body struct {
		SudoUntil time.Time `json:"sudoUntil"`
	}
}

func init() {
	register(func(api huma.API, deps Deps) {
		var (
			byIP   ipFailures
			byUser accountLocks
		)

		// check runs one secret check (the PIN, or the password when setting a
		// first PIN) under the account's lock and the shared failure count. It
		// returns the account as it is now, and nil only when the secret matched.
		check := func(ctx context.Context, p auth.Principal, kind string, ok func(store.User) bool) (auth.Principal, error) {
			defer byUser.lock(p.User.ID)()

			// Fresh from the database, inside the lock: a lock set by the
			// request just before this one must already apply.
			if fresh, err := deps.Store.UserByID(ctx, p.User.ID); err == nil {
				p.User = fresh
			}

			now := time.Now()
			if until := p.User.PinLockedUntil; until != nil && until.After(now) {
				return p, locked(*until, now)
			}
			if byIP.blocked(p.IP, now) {
				return p, coded(http.StatusTooManyRequests, "pin_locked", "too many wrong tries from this address; try again later")
			}

			if ok(p.User) {
				if err := deps.Store.ResetPinFailures(ctx, p.User.ID); err != nil {
					return p, statusFor(err, "could not check that")
				}

				return p, nil
			}

			byIP.add(p.IP, now)

			n, until, err := deps.Store.RecordPinFailure(ctx, p.User.ID, pinMaxFailures, pinLock, now)
			if err != nil {
				return p, statusFor(err, "could not check that")
			}

			audit(ctx, deps, p, "sudo.fail", "account", p.User.ID, p.User.Email, nil, map[string]any{"failures": n, "check": kind})
			if until != nil {
				audit(ctx, deps, p, "sudo.locked", "account", p.User.ID, p.User.Email, nil, map[string]time.Time{"until": *until})

				return p, locked(*until, now)
			}

			if kind == "password" {
				return p, coded(http.StatusUnauthorized, "password_wrong",
					fmt.Sprintf("that is not your password; %d more tries before this locks for 15 minutes", pinMaxFailures-n))
			}

			return p, coded(http.StatusUnauthorized, "pin_wrong",
				fmt.Sprintf("wrong PIN; %d more tries before sudo locks for 15 minutes", pinMaxFailures-n))
		}

		// grant opens sudo and says until when.
		grant := func(ctx context.Context, p auth.Principal) (*sudoOutput, error) {
			until := time.Now().Add(sudoWindow)
			if deps.Sessions != nil {
				if err := deps.Sessions.GrantSudo(ctx, p.User.ID, until); err != nil {
					return nil, huma.Error500InternalServerError("could not turn on sudo", err)
				}
			}
			out := &sudoOutput{}
			out.Body.SudoUntil = until

			return out, nil
		}

		huma.Register(api, huma.Operation{
			OperationID: "setPin",
			Method:      http.MethodPost,
			Path:        "/api/me/pin",
			Summary:     "Set or change your sudo PIN",
			Description: "The first PIN needs your password, or for an identity-provider account a sign-in " +
				"at the provider within the last five minutes. Changing it later needs sudo.",
			Tags: []string{"auth"},
		}, func(ctx context.Context, in *pinInput) (*sudoOutput, error) {
			p, err := requireAuthenticated(ctx)
			if err != nil {
				return nil, err
			}
			if !p.User.Active() {
				return nil, admissionDenied(p.User.Admission)
			}
			if !auth.NeedsPin(p.User.Role) {
				return nil, huma.Error409Conflict("your role does not use a sudo PIN")
			}

			action := "account.pin_set"
			switch {
			case p.User.PinHash != "":
				action = "account.pin_change"
				if err := checkSudo(p); err != nil {
					return nil, err
				}
			case p.User.Local():
				if p, err = check(ctx, p, "password", func(u store.User) bool {
					return auth.VerifyPassword(u.PasswordHash, in.Body.Password)
				}); err != nil {
					return nil, err
				}
			case !reauthFresh(p):
				return nil, coded(http.StatusForbidden, "reauth_required", "sign in again at your identity provider first")
			}

			if err := validPin(in.Body.PIN); err != nil {
				return nil, err
			}

			hash, err := auth.HashPassword(in.Body.PIN)
			if err != nil {
				return nil, huma.Error500InternalServerError("could not store the PIN", err)
			}
			if err := deps.Store.SetPin(ctx, p.User.ID, hash); err != nil {
				return nil, statusFor(err, "could not store the PIN")
			}

			audit(ctx, deps, p, action, "account", p.User.ID, p.User.Email, nil, nil)

			// The caller has just proved who they are; asking for the PIN
			// again straight away would only be friction.
			return grant(ctx, p)
		})

		huma.Register(api, huma.Operation{
			OperationID: "startSudo",
			Method:      http.MethodPost,
			Path:        "/api/me/sudo",
			Summary:     "Turn on sudo for five minutes with your PIN",
			Tags:        []string{"auth"},
		}, func(ctx context.Context, in *pinInput) (*sudoOutput, error) {
			p, err := requireAuthenticated(ctx)
			if err != nil {
				return nil, err
			}
			if !p.User.Active() {
				return nil, admissionDenied(p.User.Admission)
			}
			if p.User.PinHash == "" {
				return nil, coded(http.StatusConflict, "pin_required", "set your sudo PIN first")
			}

			if p, err = check(ctx, p, "pin", func(u store.User) bool {
				return u.PinHash != "" && auth.VerifyPassword(u.PinHash, in.Body.PIN)
			}); err != nil {
				return nil, err
			}

			audit(ctx, deps, p, "sudo.grant", "account", p.User.ID, p.User.Email, nil, nil)

			return grant(ctx, p)
		})

		huma.Register(api, huma.Operation{
			OperationID: "endSudo",
			Method:      http.MethodDelete,
			Path:        "/api/me/sudo",
			Summary:     "Turn sudo off now",
			Tags:        []string{"auth"},
		}, func(ctx context.Context, _ *struct{}) (*struct{}, error) {
			if _, err := requireAuthenticated(ctx); err != nil {
				return nil, err
			}
			if deps.Sessions != nil {
				if err := deps.Sessions.EndSudo(ctx); err != nil {
					return nil, huma.Error500InternalServerError("could not turn sudo off", err)
				}
			}

			return nil, nil
		})
	})
}

func locked(until, now time.Time) error {
	mins := int(until.Sub(now).Minutes()) + 1

	return coded(http.StatusTooManyRequests, "pin_locked",
		fmt.Sprintf("too many wrong PINs; sudo is locked for %d more minute%s", mins, map[bool]string{true: "", false: "s"}[mins == 1]))
}
