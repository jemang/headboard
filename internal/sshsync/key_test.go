package sshsync

import (
	"strings"
	"testing"
)

// A real ed25519 key; the comment is deliberately present to prove it is dropped.
const alice = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIO8ojS5fgNpkrBFp7XX5TD5hbJ+1i9GLNv4kmbicu6uy alice@laptop"

func TestParseKeyCanonicalisesAndDropsComment(t *testing.T) {
	k, err := ParseKey("  " + alice + "\n")
	if err != nil {
		t.Fatalf("ParseKey: %v", err)
	}
	if strings.Contains(k.Line, "alice@laptop") {
		t.Errorf("Line kept the pasted comment: %q", k.Line)
	}
	if !strings.HasPrefix(k.Line, "ssh-ed25519 AAAA") || k.Type != "ssh-ed25519" {
		t.Errorf("Line = %q, Type = %q", k.Line, k.Type)
	}
	if !strings.HasPrefix(k.Fingerprint, "SHA256:") {
		t.Errorf("Fingerprint = %q", k.Fingerprint)
	}
}

func TestParseKeyRejectsOptions(t *testing.T) {
	if _, err := ParseKey(`command="/bin/false" ` + alice); err == nil {
		t.Fatal("a key with options was accepted")
	}
}

func TestParseKeyRejectsSecondKey(t *testing.T) {
	if _, err := ParseKey(alice + "\n" + alice); err == nil {
		t.Fatal("two keys in one submission were accepted")
	}
}

func TestParseKeyRejectsGarbage(t *testing.T) {
	if _, err := ParseKey("not a key"); err == nil {
		t.Fatal("garbage was accepted")
	}
}

func TestCleanLabelRejectsNewlines(t *testing.T) {
	if _, err := CleanLabel("laptop\nssh-ed25519 AAAA evil"); err == nil {
		t.Fatal("a label with a newline was accepted")
	}
	if _, err := CleanLabel(strings.Repeat("a", 65)); err == nil {
		t.Fatal("a 65-character label was accepted")
	}
	if got, err := CleanLabel("  Alice – laptop "); err == nil {
		t.Fatalf("non-ASCII dash accepted as %q", got)
	}
	got, err := CleanLabel("  Alice - laptop ")
	if err != nil || got != "Alice - laptop" {
		t.Fatalf("CleanLabel = %q, %v", got, err)
	}
}

func TestLineStripsUnsafeCommentCharacters(t *testing.T) {
	got := Line("ssh-ed25519 AAAA", "jé\nmang", "laptop")
	if got != "ssh-ed25519 AAAA j??mang laptop" {
		t.Fatalf("Line = %q", got)
	}
}

func TestValidUnixUser(t *testing.T) {
	for _, ok := range []string{"sysadmin", "deploy", "_svc", "a-b_c9"} {
		if !ValidUnixUser(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"", "Root", "../etc", "a b", "9lives", strings.Repeat("a", 33), "a;rm"} {
		if ValidUnixUser(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestParsePasswd(t *testing.T) {
	p, err := ParsePasswd("sysadmin:x:1001:1002:,,,:/home/sysadmin:/bin/bash\n", "sysadmin")
	if err != nil || p.Home != "/home/sysadmin" || p.UID != 1001 || p.GID != 1002 {
		t.Fatalf("ParsePasswd = %+v, %v", p, err)
	}

	bad := []string{
		"",
		"sysadmin:x:1001:1002::/home/sysadmin:/bin/bash\nsysadmin:x:1:1::/root:/bin/sh",
		"other:x:1001:1002::/home/other:/bin/bash",
		"sysadmin:x:abc:1002::/home/sysadmin:/bin/bash",
		"sysadmin:x:1001:1002::home/sysadmin:/bin/bash",
		"sysadmin:x:1001:1002::/home/../root:/bin/bash",
		"sysadmin:x:1001:1002::/home/sys\tadmin:/bin/bash",
	}
	for _, out := range bad {
		if _, err := ParsePasswd(out, "sysadmin"); err == nil {
			t.Errorf("accepted %q", out)
		}
	}
}

func TestValidRef(t *testing.T) {
	for _, ok := range []string{"105", "self", "999999999"} {
		if !ValidRef(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"", "1; rm -rf /", "105'", "1234567890", "SELF", "self2", "-1"} {
		if ValidRef(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

// The ids decide who the agent becomes inside the target. Only container root
// can write them, but uid 0 for an ordinary user would run the file steps as
// root, and a negative id reaches setpriv as -1.
func TestParsePasswdRefusesRootAndOutOfRangeIDs(t *testing.T) {
	for _, rec := range []string{
		"deploy:x:0:1000::/home/deploy:/bin/sh",
		"deploy:x:-1:1000::/home/deploy:/bin/sh",
		"deploy:x:1000:-5::/home/deploy:/bin/sh",
		"deploy:x:4294967296:1000::/home/deploy:/bin/sh",
	} {
		if _, err := ParsePasswd(rec, "deploy"); err == nil {
			t.Errorf("accepted %q", rec)
		}
	}

	if _, err := ParsePasswd("root:x:0:0::/root:/bin/sh", "root"); err != nil {
		t.Errorf("root itself refused: %v", err)
	}
}
