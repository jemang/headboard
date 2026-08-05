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
