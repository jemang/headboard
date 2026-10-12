package agent

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type installation struct {
	paths Paths
	ran   []string
}

func newInstallation(t *testing.T) *installation {
	t.Helper()

	root := t.TempDir()
	p := Paths{
		Exe:    filepath.Join(root, "bin", "headboard-agent"),
		Config: filepath.Join(root, "etc", "headboard-agent"),
		Unit:   filepath.Join(root, "systemd", "headboard-agent.service"),
	}
	for _, f := range []string{p.Exe, filepath.Join(p.Config, "config"), p.Unit} {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return &installation{paths: p}
}

func (i *installation) run(argv ...string) error {
	i.ran = append(i.ran, strings.Join(argv, " "))

	return nil
}

func gone(t *testing.T, paths ...string) {
	t.Helper()

	for _, p := range paths {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still exists", p)
		}
	}
}

func TestUninstallStopsTheServiceAndRemovesEverything(t *testing.T) {
	i := newInstallation(t)

	if err := Uninstall(i.paths, i.run); err != nil {
		t.Fatal(err)
	}

	want := []string{"systemctl disable --now headboard-agent", "systemctl daemon-reload"}
	if !slices.Equal(i.ran, want) {
		t.Fatalf("ran %q, want %q", i.ran, want)
	}
	gone(t, i.paths.Exe, i.paths.Config, i.paths.Unit)
}

// A half-removed agent, or one whose service was never enabled, must still
// come out clean on a second try.
func TestUninstallFinishesWhenPartsAreAlreadyGone(t *testing.T) {
	i := newInstallation(t)
	os.Remove(i.paths.Unit)

	failing := func(argv ...string) error {
		i.ran = append(i.ran, strings.Join(argv, " "))
		if argv[1] == "disable" {
			return errors.New("Unit file headboard-agent.service does not exist.")
		}

		return nil
	}

	if err := Uninstall(i.paths, failing); err != nil {
		t.Fatal(err)
	}
	gone(t, i.paths.Exe, i.paths.Config, i.paths.Unit)
}

// An update killed mid-download leaves a temp file next to the binary, and an
// agent started with -config keeps its token outside the default directory.
func TestUninstallRemovesTempFilesAndACustomConfig(t *testing.T) {
	i := newInstallation(t)
	leftover := filepath.Join(filepath.Dir(i.paths.Exe), ".headboard-agent-123")
	custom := filepath.Join(t.TempDir(), "agent.conf")
	neighbour := filepath.Join(filepath.Dir(custom), "keep-me")
	for _, f := range []string{leftover, custom, neighbour} {
		os.WriteFile(f, []byte("x"), 0o600)
	}
	i.paths.ConfigFile = custom

	if err := Uninstall(i.paths, i.run); err != nil {
		t.Fatal(err)
	}
	gone(t, leftover, custom)
	if _, err := os.Stat(neighbour); err != nil {
		t.Fatal("a file next to the custom config was removed")
	}
}

// -config is free text; a directory given by mistake must never be emptied.
func TestUninstallNeverRemovesADirectoryGivenAsConfig(t *testing.T) {
	i := newInstallation(t)
	dir := t.TempDir()
	precious := filepath.Join(dir, "precious")
	os.WriteFile(precious, []byte("x"), 0o600)
	i.paths.ConfigFile = dir

	_ = Uninstall(i.paths, i.run)

	if _, err := os.Stat(precious); err != nil {
		t.Fatal("uninstall emptied a directory passed as -config")
	}
}
