package realip

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func seen(t *testing.T, trusted []netip.Prefix, remote string, headers map[string]string) string {
	t.Helper()

	var got string
	h := Middleware(trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.RemoteAddr }))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remote
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	h.ServeHTTP(httptest.NewRecorder(), r)

	return got
}

func TestForwardedHeadersAreBelievedOnlyFromTrustedProxies(t *testing.T) {
	proxy := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}

	// Anyone can send the header; from an unlisted peer it changes nothing.
	if got := seen(t, nil, "203.0.113.5:4000", map[string]string{"X-Forwarded-For": "198.51.100.1"}); got != "203.0.113.5:4000" {
		t.Errorf("untrusted peer rewrote the address to %s", got)
	}

	// Through a trusted proxy, the client is the rightmost address that is
	// not itself a trusted proxy; anything left of it the client could forge.
	if got := seen(t, proxy, "10.0.0.2:4000", map[string]string{"X-Forwarded-For": "6.6.6.6, 198.51.100.1, 10.0.0.9"}); got != "198.51.100.1:0" {
		t.Errorf("through a trusted proxy got %s, want 198.51.100.1:0", got)
	}

	if got := seen(t, proxy, "10.0.0.2:4000", map[string]string{"X-Real-IP": "198.51.100.7"}); got != "198.51.100.7:0" {
		t.Errorf("X-Real-IP from a trusted proxy: %s", got)
	}

	if got := seen(t, proxy, "10.0.0.2:4000", map[string]string{"X-Forwarded-For": "not-an-ip"}); got != "10.0.0.2:4000" {
		t.Errorf("garbage header changed the address to %s", got)
	}
}

func TestParse(t *testing.T) {
	got, err := Parse("10.0.0.0/8, 192.0.2.4")
	if err != nil || len(got) != 2 || got[1].String() != "192.0.2.4/32" {
		t.Errorf("Parse = %v, %v", got, err)
	}
	if _, err := Parse("nonsense"); err == nil {
		t.Error("accepted nonsense")
	}
	if got, err := Parse(""); err != nil || len(got) != 0 {
		t.Errorf("empty = %v, %v", got, err)
	}
}
