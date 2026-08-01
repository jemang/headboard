package hs

import "strings"

// APIKeyPrefix returns the public identifier of an API key. Headscale API
// keys have no separator between their 12-character prefix and secret, so the
// prefix must be taken by length.
func APIKeyPrefix(key string) string {
	const (
		marker       = "hskey-api-"
		prefixLength = 12
	)

	body := strings.TrimPrefix(key, marker)
	if len(body) > prefixLength {
		body = body[:prefixLength]
	}

	return body
}
