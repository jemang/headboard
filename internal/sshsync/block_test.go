package sshsync

import (
	"errors"
	"testing"
)

func TestReplaceBlockAppendsToExistingFile(t *testing.T) {
	got, err := ReplaceBlock("ssh-rsa AAAA mine", []string{"ssh-ed25519 AAAA alice@ laptop"})
	if err != nil {
		t.Fatal(err)
	}
	want := "ssh-rsa AAAA mine\n" + BeginMarker + "\nssh-ed25519 AAAA alice@ laptop\n" + EndMarker + "\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestReplaceBlockReplacesOnlyTheBlock(t *testing.T) {
	in := "before\n" + BeginMarker + "\nold key\n" + EndMarker + "\nafter\n"
	got, err := ReplaceBlock(in, []string{"new key"})
	if err != nil {
		t.Fatal(err)
	}
	want := "before\n" + BeginMarker + "\nnew key\n" + EndMarker + "\nafter\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestReplaceBlockClearsToEmptyBlock(t *testing.T) {
	in := BeginMarker + "\nold key\n" + EndMarker + "\n"
	got, err := ReplaceBlock(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != BeginMarker+"\n"+EndMarker+"\n" {
		t.Fatalf("got %q", got)
	}
}

func TestReplaceBlockOnEmptyFile(t *testing.T) {
	got, err := ReplaceBlock("", []string{"k"})
	if err != nil || got != BeginMarker+"\nk\n"+EndMarker+"\n" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestReplaceBlockRefusesUnterminatedBlock(t *testing.T) {
	_, err := ReplaceBlock("mine\n"+BeginMarker+"\nkey\nmore of mine\n", []string{"k"})
	if !errors.Is(err, ErrBrokenBlock) {
		t.Fatalf("err = %v, want ErrBrokenBlock", err)
	}
}

func TestReplaceBlockRefusesTwoBlocks(t *testing.T) {
	two := BeginMarker + "\na\n" + EndMarker + "\nmine\n" + BeginMarker + "\nb\n" + EndMarker + "\n"
	if _, err := ReplaceBlock(two, []string{"k"}); !errors.Is(err, ErrDuplicateBlock) {
		t.Fatalf("err = %v, want ErrDuplicateBlock", err)
	}
}

func TestReplaceBlockIgnoresMarkerInsideALine(t *testing.T) {
	in := "ssh-rsa AAAA mine " + BeginMarker + "\n"
	got, err := ReplaceBlock(in, []string{"k"})
	if err != nil {
		t.Fatal(err)
	}
	if got != in+BeginMarker+"\nk\n"+EndMarker+"\n" {
		t.Fatalf("got %q", got)
	}
}
