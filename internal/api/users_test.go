package api

import (
	"database/sql"
	"testing"

	"github.com/juanfont/headscale/hscontrol/types"
)

func TestDuplicateUserNameIsCaseInsensitive(t *testing.T) {
	users := []types.User{
		{Name: "jemang", ProviderIdentifier: sql.NullString{String: "https://idp/|sub-1", Valid: true}},
	}

	if !duplicateUserName(users, "Jemang") {
		t.Error("Jemang did not collide with existing jemang")
	}
	if duplicateUserName(users, "alice") {
		t.Error("alice reported as a collision against an unrelated user")
	}
}

// "user #3 deleted" tells an admin nothing a week later; the log keeps the name.
func TestUserRefNamesTheUserForTheAuditLog(t *testing.T) {
	users := []types.User{{Name: "alice", Email: "alice@example.com"}}
	users[0].ID = 3

	if got := userRef(users, 3); got == nil || got["name"] != "alice" || got["email"] != "alice@example.com" {
		t.Errorf("userRef(3) = %v, want alice's name and email", got)
	}
	if got := userRef(users, 4); got != nil {
		t.Errorf("userRef(missing) = %v, want nil", got)
	}
}
