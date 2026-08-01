package api

import "testing"

func TestHealthBodyForReportsUnavailableWithoutWatcher(t *testing.T) {
	body := healthBodyFor(Deps{})

	if body.HeadscaleState != "unavailable" {
		t.Errorf("HeadscaleState = %q, want unavailable", body.HeadscaleState)
	}
	if body.HeadscaleLastSynced != nil {
		t.Errorf("HeadscaleLastSynced = %v, want nil", body.HeadscaleLastSynced)
	}
}
