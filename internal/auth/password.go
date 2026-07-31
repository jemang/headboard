package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

// Password hashing parameters. These are the argon2id values RFC 9106 suggests
// for a memory-constrained server, and they cost roughly 50ms per attempt on
// commodity hardware — slow enough to make offline cracking expensive, fast
// enough that a login does not feel broken.
const (
	argonTime    = 1
	argonMemory  = 64 * 1024 // KiB
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
)

// ErrBadCredentials is returned for every failed sign-in.
//
// One error for "no such account" and "wrong password" alike: distinguishing
// them turns the login form into a way to test whether an address has an
// account here.
var ErrBadCredentials = errors.New("email or password is incorrect")

// HashPassword returns an encoded argon2id hash.
//
// The format is the standard PHC string, so the parameters travel with the
// hash: raising the cost later does not invalidate existing passwords, because
// each one is verified with the parameters it was created under.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("password is empty")
	}

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword reports whether a password matches an encoded hash.
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}

	var memory, timeCost uint32

	var threads uint8

	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &threads); err != nil {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}

	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}

	got := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(want)))

	return subtle.ConstantTimeCompare(got, want) == 1
}

// GeneratePassword returns a readable random password, for the first-run owner.
//
// Base32 without padding, in groups: unambiguous to read out of a log and to
// type once. 20 characters of base32 is 100 bits of entropy.
func GeneratePassword() (string, error) {
	raw := make([]byte, 13)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating password: %w", err)
	}

	enc := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw))

	var groups []string
	for i := 0; i+4 <= 20; i += 4 {
		groups = append(groups, enc[i:i+4])
	}

	return strings.Join(groups, "-"), nil
}

// limiter throttles failed sign-in attempts.
//
// In memory and per process, which is the right scope for a single-instance
// control plane. It is keyed on the client address *and* the email so that one
// person fat-fingering their password cannot lock out an address from
// elsewhere, and a spray across many addresses from one source still trips.
type limiter struct {
	mu       sync.Mutex
	attempts map[string]*attempt

	max    int
	window time.Duration
}

type attempt struct {
	count int
	until time.Time
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{attempts: map[string]*attempt{}, max: max, window: window}
}

// blocked reports whether a key is currently locked out.
func (l *limiter) blocked(key string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	a, ok := l.attempts[key]
	if !ok {
		return 0, false
	}

	if time.Now().After(a.until) {
		delete(l.attempts, key)

		return 0, false
	}

	if a.count < l.max {
		return 0, false
	}

	return time.Until(a.until), true
}

// fail records a failed attempt.
func (l *limiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	a, ok := l.attempts[key]
	if !ok || time.Now().After(a.until) {
		a = &attempt{}
		l.attempts[key] = a
	}

	a.count++
	a.until = time.Now().Add(l.window)

	// Unbounded growth would be a memory leak an attacker controls, so
	// expired entries are swept whenever the map gets large.
	if len(l.attempts) > 1024 {
		now := time.Now()

		for k, v := range l.attempts {
			if now.After(v.until) {
				delete(l.attempts, k)
			}
		}
	}
}

// succeed clears the record for a key.
func (l *limiter) succeed(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.attempts, key)
}
