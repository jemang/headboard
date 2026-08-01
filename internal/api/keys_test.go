package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jemang/headboard/internal/hs"
)

func TestActivePreAuthKeysSkipsOnlyExpiredKeys(t *testing.T) {
	now := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	future := now.Add(time.Minute)

	got := activePreAuthKeys([]hs.PreAuthKey{
		{ID: "expired", Expiry: &past},
		{ID: "without-expiry"},
		{ID: "future", Expiry: &future, Used: true},
	}, now)

	if len(got) != 2 || got[0].ID != "without-expiry" || got[1].ID != "future" {
		t.Errorf("activePreAuthKeys = %+v, want without-expiry and future", got)
	}
}

func TestExpireActivePreAuthKeysReportsPartialFailure(t *testing.T) {
	now := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	keys := []hs.PreAuthKey{{ID: "first"}, {ID: "failed", Expiry: &future}}

	expired, failed := expireActivePreAuthKeys(t.Context(), keys, now, func(_ context.Context, id string) error {
		if id == "failed" {
			return errors.New("upstream refused")
		}
		return nil
	})

	if len(expired) != 1 || expired[0] != "first" {
		t.Errorf("expired = %v, want first", expired)
	}
	if len(failed) != 1 || failed[0] != "failed" {
		t.Errorf("failed = %v, want failed", failed)
	}
}

func TestIsProtectedAPIKey(t *testing.T) {
	const prefix = "abcdefghijkl"

	if !isProtectedAPIKey("hskey-api-"+prefix+"secret", prefix) {
		t.Fatal("configured service key was not protected")
	}
	if isProtectedAPIKey("hskey-api-zyxwvutsrqpoother", prefix) {
		t.Fatal("different API key was protected")
	}
}
