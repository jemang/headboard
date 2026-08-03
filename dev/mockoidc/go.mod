// A separate module on purpose. This is a development-only identity provider,
// and Headboard's own go.mod stays free of it — pulling headscale's mockoidc
// command in instead would drag gvisor and the whole tailscale netstack into
// the production dependency graph for the sake of a test login.
module headboard.dev/mockoidc

go 1.26.5

require github.com/oauth2-proxy/mockoidc v0.0.0-20240214162133-caebfff84d25

require (
	github.com/go-jose/go-jose/v3 v3.0.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.0 // indirect
	golang.org/x/crypto v0.0.0-20220214200702-86341886e292 // indirect
)
