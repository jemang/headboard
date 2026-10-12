package agent

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func binaryServer(t *testing.T, body string, status int) *Client {
	t.Helper()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ssh/agent/bin/amd64" {
			http.NotFound(w, r)

			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return NewClient(srv.URL, "hbk_x", srv.Client())
}

func installed(t *testing.T, content string) string {
	t.Helper()

	exe := filepath.Join(t.TempDir(), "headboard-agent")
	if err := os.WriteFile(exe, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}

	return exe
}

func assertFile(t *testing.T, exe, want string) {
	t.Helper()

	got, err := os.ReadFile(exe)
	if err != nil || string(got) != want {
		t.Fatalf("binary = %q, %v; want %q", got, err, want)
	}
	if info, _ := os.Stat(exe); info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".headboard-agent-*")); len(left) != 0 {
		t.Fatalf("temp files left: %v", left)
	}
}

func accept(string) error { return nil }

func TestUpdateReplacesTheBinary(t *testing.T) {
	exe := installed(t, "old")

	changed, err := Update(t.Context(), binaryServer(t, "new", http.StatusOK), "amd64", exe, accept)
	if err != nil || !changed {
		t.Fatalf("changed = %v, err = %v", changed, err)
	}
	assertFile(t, exe, "new")
}

func TestUpdateLeavesAnIdenticalBinaryAlone(t *testing.T) {
	exe := installed(t, "same")

	changed, err := Update(t.Context(), binaryServer(t, "same", http.StatusOK), "amd64", exe, accept)
	if err != nil || changed {
		t.Fatalf("changed = %v, err = %v", changed, err)
	}
	assertFile(t, exe, "same")
}

// A truncated or wrong download must never replace a working agent.
func TestUpdateKeepsTheOldBinaryWhenTheNewOneFailsItsCheck(t *testing.T) {
	exe := installed(t, "old")

	reject := func(string) error { return errors.New("not an agent") }
	if _, err := Update(t.Context(), binaryServer(t, "garbage", http.StatusOK), "amd64", exe, reject); err == nil {
		t.Fatal("a binary that failed its check was accepted")
	}
	assertFile(t, exe, "old")
}

func TestUpdateKeepsTheOldBinaryWhenTheDownloadFails(t *testing.T) {
	exe := installed(t, "old")

	if _, err := Update(t.Context(), binaryServer(t, "agent binaries are not bundled", http.StatusNotFound), "amd64", exe, accept); err == nil {
		t.Fatal("a 404 body was installed")
	}
	assertFile(t, exe, "old")
}

// A compromised or misconfigured server must not be able to roll an agent
// back to an older build with known holes. Same-version rebuilds are allowed:
// dev images all carry one version.
func TestDowngradeRefusal(t *testing.T) {
	for _, c := range []struct {
		running, offered string
		refuse           bool
	}{
		{"1.4.0", "1.3.9", true},
		{"1.4.0", "1.4.0-rc.1", true},
		{"1.4.0", "1.4.0", false},
		{"1.4.0", "1.10.0", false},
		{"0.0.0-docker", "0.0.0-docker", false},
		{"1.4.0", "garbage", true},
		{"dev", "1.0.0", false},
	} {
		if got := IsDowngrade(c.running, c.offered); got != c.refuse {
			t.Errorf("IsDowngrade(%q, %q) = %v, want %v", c.running, c.offered, got, c.refuse)
		}
	}
}

// A runaway download must not fill the disk the binary lives on.
func TestUpdateRefusesAnOversizedDownload(t *testing.T) {
	saved := maxBinary
	maxBinary = 8
	t.Cleanup(func() { maxBinary = saved })

	exe := installed(t, "old")
	if _, err := Update(t.Context(), binaryServer(t, "far more than eight bytes", http.StatusOK), "amd64", exe, accept); err == nil {
		t.Fatal("an oversized download was installed")
	}
	assertFile(t, exe, "old")
}
