package api

import "testing"

func TestIsProtectedAPIKey(t *testing.T) {
	const prefix = "abcdefghijkl"

	if !isProtectedAPIKey("hskey-api-"+prefix+"secret", prefix) {
		t.Fatal("configured service key was not protected")
	}
	if isProtectedAPIKey("hskey-api-zyxwvutsrqpoother", prefix) {
		t.Fatal("different API key was protected")
	}
}

// The enrolment command is copied to a different machine, so an address only
// Headboard can resolve fails somewhere nobody is watching. These are the
// shapes that actually turn up: a compose service name, a localhost binding,
// and a real host that must not be flagged.
func TestUnreachableLoginServer(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		flagged bool
	}{
		{"compose service name", "http://headscale-dev:8080", true},
		{"kubernetes service", "http://headscale:8080", true},
		{"loopback", "http://127.0.0.1:8080", true},
		{"localhost", "http://localhost:8080", true},
		{"ipv6 loopback", "http://[::1]:8080", true},
		{"public hostname", "https://headscale.example.com", false},
		{"lan address", "http://192.168.1.191:8080", false},
		{"internal fqdn", "http://headscale.internal:8080", false},
		{"empty", "", false},
		{"nonsense", "://", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := unreachableLoginServer(c.url)

			if (got != "") != c.flagged {
				t.Errorf("unreachableLoginServer(%q) = %q, want flagged=%v", c.url, got, c.flagged)
			}
		})
	}
}

// Deployments that reach Headscale at one address are the common case, and
// must not be made to configure a second one.
func TestLoginServerFallsBackToTheAddressHeadboardUses(t *testing.T) {
	both := loginServer(Deps{HeadscaleURL: "https://hs.example.com"})
	if both != "https://hs.example.com" {
		t.Errorf("loginServer = %q, want the HeadscaleURL", both)
	}

	split := loginServer(Deps{
		HeadscaleURL:       "http://headscale:8080",
		HeadscalePublicURL: "https://hs.example.com",
	})
	if split != "https://hs.example.com" {
		t.Errorf("loginServer = %q, want the public URL to win", split)
	}
}
