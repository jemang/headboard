package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jemang/headboard/internal/sshsync"
)

type exitErr int

func (e exitErr) Error() string { return "exit" }
func (e exitErr) ExitCode() int { return int(e) }

// fakeHost keeps one authorized_keys file per unix user in memory.
type fakeHost struct {
	files   map[string]string
	missing map[string]bool
	// leftover marks a missing user whose home still holds authorized_keys.
	leftover map[string]bool
	writes   int
}

func (f *fakeHost) Mode() string                                           { return "proxmox" }
func (f *fakeHost) Discover(context.Context) ([]sshsync.Discovered, error) { return nil, nil }

func (f *fakeHost) Exec(_ context.Context, ref, stdin string, argv ...string) (string, error) {
	switch {
	case argv[0] == "getent":
		user := argv[2]
		if f.missing[user] {
			return "", exitErr(2)
		}
		return user + ":x:1001:1001::/home/" + user + ":/bin/bash\n", nil
	case argv[0] == "sh" && argv[2] == leftoverScript:
		if f.leftover[strings.TrimSuffix(strings.TrimPrefix(argv[4], "/home/"), "/.ssh/authorized_keys")] {
			return "", nil
		}
		return "", exitErr(1)
	case argv[2] == switchUser && argv[7] == readScript:
		return f.files[strings.TrimPrefix(argv[8], "/home/")], nil
	case argv[2] == switchUser && argv[7] == writeScript:
		f.writes++
		f.files[strings.TrimPrefix(argv[8], "/home/")] = stdin
		return "", nil
	}
	return "", errors.New("unexpected " + strings.Join(argv, " "))
}

var running = []sshsync.Discovered{{Ref: "105", IPs: []string{"192.0.2.10"}}}

func TestApplyWritesBlockAndKeepsOtherKeys(t *testing.T) {
	h := &fakeHost{files: map[string]string{"sysadmin": "ssh-rsa AAAA mine\n"}}

	r := Apply(t.Context(), h, running, []sshsync.Target{{
		Ref: "105", ExpectIP: "192.0.2.10",
		Users: map[string][]string{"sysadmin": {"ssh-ed25519 AAAA1 alice"}},
	}})

	if len(r.Applied) != 1 || len(r.Warnings) != 0 {
		t.Fatalf("result = %+v", r)
	}
	want := "ssh-rsa AAAA mine\n" + sshsync.BeginMarker + "\nssh-ed25519 AAAA1 alice\n" + sshsync.EndMarker + "\n"
	if h.files["sysadmin"] != want {
		t.Fatalf("file = %q", h.files["sysadmin"])
	}

	Apply(t.Context(), h, running, []sshsync.Target{{Ref: "105", ExpectIP: "192.0.2.10", Users: map[string][]string{"sysadmin": {"ssh-ed25519 AAAA1 alice"}}}})
	if h.writes != 1 {
		t.Fatalf("unchanged content was rewritten (%d writes)", h.writes)
	}
}

func TestApplyReportsMismatchAndWritesNothing(t *testing.T) {
	h := &fakeHost{files: map[string]string{}}

	r := Apply(t.Context(), h, running, []sshsync.Target{{Ref: "105", ExpectIP: "10.0.0.9", Users: map[string][]string{"deploy": {"ssh-ed25519 AAAA k"}}}})

	if len(r.Mismatched) != 1 || h.writes != 0 {
		t.Fatalf("result = %+v writes = %d", r, h.writes)
	}
}

func TestApplyClearsAndAcknowledges(t *testing.T) {
	h := &fakeHost{files: map[string]string{"staging": sshsync.BeginMarker + "\nk\n" + sshsync.EndMarker + "\n"}, missing: map[string]bool{"gone": true}}

	r := Apply(t.Context(), h, running, []sshsync.Target{{Ref: "105", Users: map[string][]string{}, Clear: []string{"staging", "gone"}}})

	if len(r.Cleared) != 2 {
		t.Fatalf("result = %+v", r)
	}
	if h.files["staging"] != sshsync.BeginMarker+"\n"+sshsync.EndMarker+"\n" {
		t.Fatalf("staging = %q", h.files["staging"])
	}
}

// A user removed without their home (userdel without -r), or hidden by a
// directory-service outage, still has the managed block on disk. Reporting it
// cleared would drop it from Headboard's books, and the keys would work again
// the day the account is back.
func TestApplyKeepsAClearPendingWhileAMissingUsersKeysRemain(t *testing.T) {
	h := &fakeHost{files: map[string]string{}, missing: map[string]bool{"gone": true}, leftover: map[string]bool{"gone": true}}

	r := Apply(t.Context(), h, running, []sshsync.Target{{Ref: "105", Users: map[string][]string{}, Clear: []string{"gone"}}})

	if len(r.Cleared) != 0 || len(r.Warnings) != 1 || r.Warnings[0].UnixUser != "gone" {
		t.Fatalf("result = %+v", r)
	}
}

func TestApplyKeepsClearsForMissingTarget(t *testing.T) {
	h := &fakeHost{files: map[string]string{}}

	r := Apply(t.Context(), h, nil, []sshsync.Target{{Ref: "105", Clear: []string{"staging"}}})

	if len(r.Cleared) != 0 || len(r.Warnings) != 1 {
		t.Fatalf("a stopped target's clear was acknowledged: %+v", r)
	}
}

func TestApplyWarnsOnMissingUserAndBrokenBlock(t *testing.T) {
	h := &fakeHost{
		files:   map[string]string{"deploy": sshsync.BeginMarker + "\nno end\n"},
		missing: map[string]bool{"nobodyhere": true},
	}

	r := Apply(t.Context(), h, running, []sshsync.Target{{
		Ref: "105", ExpectIP: "192.0.2.10",
		Users: map[string][]string{"deploy": {"ssh-ed25519 AAAA k"}, "nobodyhere": {"ssh-ed25519 AAAA k"}, "../root": {"ssh-ed25519 AAAA k"}},
	}})

	if len(r.Applied) != 0 || len(r.Warnings) != 3 || h.writes != 0 {
		t.Fatalf("result = %+v writes = %d", r, h.writes)
	}
}

// A container whose address could not be read (booting, ip timed out) is not
// evidence of a moved IP; suspending it would freeze revocations too.
func TestApplySkipsUnknownAddressWithoutMismatch(t *testing.T) {
	h := &fakeHost{files: map[string]string{}}
	noIP := []sshsync.Discovered{{Ref: "105"}}

	r := Apply(t.Context(), h, noIP, []sshsync.Target{{Ref: "105", ExpectIP: "192.0.2.10", Users: map[string][]string{"deploy": {"ssh-ed25519 AAAA k"}}}})

	if len(r.Mismatched) != 0 || len(r.Warnings) != 1 || h.writes != 0 {
		t.Fatalf("result = %+v writes = %d", r, h.writes)
	}
}

// A container recreated under the same id with the same static IP is a
// different machine; only the hostname tells them apart.
func TestApplyTreatsChangedHostnameAsMismatch(t *testing.T) {
	h := &fakeHost{files: map[string]string{}}
	reused := []sshsync.Discovered{{Ref: "105", Hostname: "something-else", IPs: []string{"192.0.2.10"}}}

	r := Apply(t.Context(), h, reused, []sshsync.Target{{Ref: "105", ExpectIP: "192.0.2.10", ExpectLabel: "app-1", Users: map[string][]string{"deploy": {"ssh-ed25519 AAAA k"}}}})

	if len(r.Mismatched) != 1 || h.writes != 0 {
		t.Fatalf("result = %+v writes = %d", r, h.writes)
	}
}

// A target that reports no hostname cannot be checked against the approved
// label, so nothing is written; like an unreadable address, it is skipped
// rather than suspended, since a blank read is not proof of a different host.
func TestApplySkipsATargetWithNoHostname(t *testing.T) {
	h := &fakeHost{files: map[string]string{}}
	blank := []sshsync.Discovered{{Ref: "105", Hostname: "", IPs: []string{"192.0.2.10"}}}

	r := Apply(t.Context(), h, blank, []sshsync.Target{{Ref: "105", ExpectIP: "192.0.2.10", ExpectLabel: "app-1", Users: map[string][]string{"deploy": {"ssh-ed25519 AAAA k"}}}})

	if h.writes != 0 || len(r.Mismatched) != 0 || len(r.Warnings) != 1 {
		t.Fatalf("result = %+v writes = %d", r, h.writes)
	}
}

// Headboard validates keys when they are stored, but the agent is the last
// step before root's write lands in a user's file: a line that could start a
// new entry, carry options or be a comment is refused here too.
func TestApplyRefusesUnsafeLinesFromTheServer(t *testing.T) {
	for _, line := range []string{
		"ssh-ed25519 AAAA ok\nssh-rsa BBBB planted",
		"ssh-ed25519 AAAA ok\rplanted",
		`command="sh" ssh-ed25519 AAAA x`,
		"# END headboard",
		"",
	} {
		h := &fakeHost{files: map[string]string{}}
		r := Apply(t.Context(), h, running, []sshsync.Target{{Ref: "105", Users: map[string][]string{"deploy": {line}}}})

		if h.writes != 0 || len(r.Applied) != 0 || len(r.Warnings) != 1 {
			t.Errorf("%q: result = %+v writes = %d", line, r, h.writes)
		}
	}
}
