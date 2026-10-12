package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/jemang/headboard/internal/sshsync"
)

type recordingHost struct {
	fakeHost
	scripts [][]string
}

func (r *recordingHost) Exec(ctx context.Context, ref, stdin string, argv ...string) (string, error) {
	if argv[0] != "getent" {
		r.scripts = append(r.scripts, argv)
	}
	return r.fakeHost.Exec(ctx, ref, stdin, argv...)
}

// Root inside a directory the user owns can be raced into writing or chowning
// any file through a swapped symlink, so every file step runs as that user,
// named by the uid and gid getent reported.
func TestApplyRunsFileStepsAsTargetUser(t *testing.T) {
	h := &recordingHost{fakeHost: fakeHost{files: map[string]string{}}}

	Apply(t.Context(), h, running, []sshsync.Target{{
		Ref: "105", ExpectIP: "192.0.2.10", Users: map[string][]string{"deploy": {"ssh-ed25519 AAAA k"}},
	}})

	if len(h.scripts) != 2 {
		t.Fatalf("script calls = %q", h.scripts)
	}
	for _, argv := range h.scripts {
		if argv[0] != "sh" || argv[2] != switchUser || strings.Join(argv[4:7], " ") != "deploy 1001 1001" {
			t.Errorf("not run as the target user: %q", argv)
		}
	}
	if strings.Contains(writeScript, "chown") {
		t.Error("writeScript still chowns, which follows symlinks as root")
	}
}
