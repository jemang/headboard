// Command mockoidc runs a development identity provider for Headboard.
//
// It binds an explicit address rather than a random port, because mockoidc
// derives its issuer from whatever it is listening on: the issuer has to be a
// URL that both the browser and Headboard can reach, or discovery fails and the
// redirect goes somewhere unreachable.
//
// In a container those are not the same address — it must bind 0.0.0.0 but
// advertise something the browser can resolve — so MOCKOIDC_ADVERTISE splits
// the two. mockoidc only reads Server.Addr to render its issuer and endpoint
// URLs; the listener it is already serving on is untouched.
package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oauth2-proxy/mockoidc"
)

func main() {
	addr := envOr("MOCKOIDC_ADDR", "127.0.0.1:9998")
	advertise := envOr("MOCKOIDC_ADVERTISE", addr)
	clientID := envOr("MOCKOIDC_CLIENT_ID", "headboard-dev")
	clientSecret := envOr("MOCKOIDC_CLIENT_SECRET", "headboard-dev-secret")

	keypair, err := mockoidc.NewKeypair(nil)
	if err != nil {
		log.Fatalf("generating keypair: %v", err)
	}

	// mockoidc pops one user per authorisation and falls back to a default
	// once the queue empties, so the order here is the order logins happen
	// in. Subjects match what a real IdP would send; Headboard links them to
	// Headscale users on issuer + subject.
	queue := &mockoidc.UserQueue{}
	for _, u := range users() {
		queue.Push(u)
	}

	m := &mockoidc.MockOIDC{
		ClientID:                      clientID,
		ClientSecret:                  clientSecret,
		AccessTTL:                     10 * time.Minute,
		RefreshTTL:                    60 * time.Minute,
		CodeChallengeMethodsSupported: []string{"plain", "S256"},
		Keypair:                       keypair,
		SessionStore:                  mockoidc.NewSessionStore(),
		UserQueue:                     queue,
		ErrorQueue:                    &mockoidc.ErrorQueue{},
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listening on %s: %v", addr, err)
	}

	if err := m.Start(ln, nil); err != nil {
		log.Fatalf("starting mock oidc: %v", err)
	}

	// Must happen after Start, which sets Addr from the listener.
	m.Server.Addr = advertise

	fmt.Printf("mock OIDC issuer:   %s\n", m.Issuer())
	fmt.Printf("mock OIDC discovery: %s\n", m.DiscoveryEndpoint())
	fmt.Printf("client_id:          %s\n", clientID)
	fmt.Println()
	fmt.Println("logins are served from a queue, in this order:")

	for i, u := range users() {
		fmt.Printf("  %d. %s (%s)\n", i+1, u.PreferredUsername, u.Email)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	_ = m.Shutdown()
}

func users() []*mockoidc.MockUser {
	return []*mockoidc.MockUser{
		{
			Subject:           "ops-sub",
			Email:             "ops@headboard.test",
			EmailVerified:     true,
			PreferredUsername: "ops",
		},
		{
			Subject:           "alice-sub",
			Email:             "alice@headboard.test",
			EmailVerified:     true,
			PreferredUsername: "alice",
		},
		{
			Subject:           "bob-sub",
			Email:             "bob@headboard.test",
			EmailVerified:     true,
			PreferredUsername: "bob",
		},
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}
