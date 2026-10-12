package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The scripts run as root inside containers; this runs them for real under sh
// against a temp home, since the fake host in apply_test.go never executes them.
func TestScriptsWriteAndRefuseSymlinks(t *testing.T) {
	home := t.TempDir()
	r := ExecRunner{}

	out, err := r.Run(t.Context(), "", "sh", "-c", readScript, "sh", home)
	if err != nil || out != "" {
		t.Fatalf("read of missing file = %q, %v", out, err)
	}

	if _, err := r.Run(t.Context(), "k1\n", "sh", "-c", writeScript, "sh", home); err != nil {
		t.Fatalf("write: %v", err)
	}

	info, err := os.Stat(filepath.Join(home, ".ssh", "authorized_keys"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("authorized_keys = %v, %v", info, err)
	}
	if dir, _ := os.Stat(filepath.Join(home, ".ssh")); dir.Mode().Perm() != 0o700 {
		t.Fatalf(".ssh mode = %v", dir.Mode().Perm())
	}

	out, err = r.Run(t.Context(), "", "sh", "-c", readScript, "sh", home)
	if err != nil || out != "k1\n" {
		t.Fatalf("read back = %q, %v", out, err)
	}

	leftovers, _ := filepath.Glob(filepath.Join(home, ".ssh", ".headboard.*"))
	if len(leftovers) != 0 {
		t.Fatalf("temp files left: %v", leftovers)
	}

	evil := t.TempDir()
	os.Remove(filepath.Join(home, ".ssh", "authorized_keys"))
	if err := os.Symlink(filepath.Join(evil, "target"), filepath.Join(home, ".ssh", "authorized_keys")); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Run(t.Context(), "", "sh", "-c", readScript, "sh", home); err == nil {
		t.Fatal("read followed a symlinked authorized_keys")
	}
	if _, err := r.Run(t.Context(), "k2\n", "sh", "-c", writeScript, "sh", home); err == nil {
		t.Fatal("write followed a symlinked authorized_keys")
	}
	if _, err := os.Stat(filepath.Join(evil, "target")); err == nil {
		t.Fatal("write created the symlink target")
	}
}

// su opens a PAM session, which inside an LXC can wait 25 s on logind for
// each call, so setpriv is used whenever the target has it. The fakes stand
// in for both and print what they were asked to do before running the script.
func TestSwitchUserPrefersSetprivAndFallsBackToSu(t *testing.T) {
	r := ExecRunner{}
	bin := t.TempDir()

	fake := func(name, shift string) {
		body := "#!/bin/sh\necho \"" + name + " $*\"\nshift " + shift + "\nexec \"$@\"\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	run := func() string {
		out, err := r.Run(t.Context(), "", "env", "PATH="+bin, "/bin/sh", "-c", switchUser, "sh", "deploy", "1001", "1002", "echo ran $1", "/home/deploy")
		if err != nil {
			t.Fatalf("switchUser: %v", err)
		}
		return out
	}

	if err := os.WriteFile(filepath.Join(bin, "su"), []byte("#!/bin/sh\necho \"su $*\"\nshift 4\nexec /bin/sh -c \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, want := run(), "su -s /bin/sh deploy -c echo ran $1 sh /home/deploy\nran /home/deploy\n"; got != want {
		t.Fatalf("without setpriv:\n got %q\nwant %q", got, want)
	}

	fake("setpriv", "3")
	if got, want := run(), "setpriv --reuid=1001 --regid=1002 --init-groups /bin/sh -c echo ran $1 sh /home/deploy\nran /home/deploy\n"; got != want {
		t.Fatalf("with setpriv:\n got %q\nwant %q", got, want)
	}
}

func TestLeftoverScriptSeesFilesAndDanglingLinks(t *testing.T) {
	dir := t.TempDir()
	file, link, none := filepath.Join(dir, "f"), filepath.Join(dir, "l"), filepath.Join(dir, "n")
	os.WriteFile(file, nil, 0o600)
	os.Symlink(filepath.Join(dir, "missing"), link)

	h := Local{R: ExecRunner{}}
	for path, want := range map[string]bool{file: true, link: true, none: false} {
		got, err := keysRemain(t.Context(), localAt{h, path}, "self", "x")
		if err != nil || got != want {
			t.Errorf("%s: remain = %v, %v; want %v", filepath.Base(path), got, err, want)
		}
	}
}

// localAt points the conventional /home path at a temp file.
type localAt struct {
	Local
	path string
}

func (l localAt) Exec(ctx context.Context, ref, stdin string, argv ...string) (string, error) {
	return l.Local.Exec(ctx, ref, stdin, append(argv[:len(argv)-1:len(argv)-1], l.path)...)
}
