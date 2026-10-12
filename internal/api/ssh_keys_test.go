package api

import (
	"testing"
	"time"

	"github.com/juanfont/headscale/hscontrol/types"

	"github.com/jemang/headboard/internal/store"
)

func TestSSHKeyViewDerivesTypeFromLine(t *testing.T) {
	v := sshKeyView(store.SSHKey{ID: 1, HeadscaleUserID: 2, PublicKey: "ssh-ed25519 AAAA", Fingerprint: "SHA256:x", Label: "l", CreatedAt: time.Unix(0, 0)})
	if v.Type != "ssh-ed25519" || v.Fingerprint != "SHA256:x" {
		t.Fatalf("view = %+v", v)
	}
}

func TestTailnetUserExists(t *testing.T) {
	users := []types.User{{Name: "alice"}}
	users[0].ID = 4

	if !tailnetUserExists(users, 4) || tailnetUserExists(users, 5) {
		t.Fatal("tailnetUserExists is wrong")
	}
}
