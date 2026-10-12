// Package sshsync is the part of SSH key sync that Headboard and its agent
// share: what a valid key, label and unix user are, what the managed block in
// authorized_keys looks like, and what an agent should hold.
package sshsync

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/crypto/ssh"
)

// Key is a validated public key in the canonical form it is stored and emitted in.
type Key struct {
	Line        string
	Type        string
	Fingerprint string
}

// ParseKey accepts exactly one authorized_keys line. Options are rejected
// rather than stripped: a pasted command= line means the admin expected a
// restriction this feature does not enforce.
func ParseKey(text string) (Key, error) {
	pub, _, options, rest, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(text)))
	if err != nil {
		return Key{}, fmt.Errorf("not a valid public key: %w", err)
	}

	if len(options) > 0 {
		return Key{}, errors.New("key options such as command= or from= are not supported")
	}

	if len(bytes.TrimSpace(rest)) > 0 {
		return Key{}, errors.New("paste one key at a time")
	}

	return Key{
		Line:        strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))),
		Type:        pub.Type(),
		Fingerprint: ssh.FingerprintSHA256(pub),
	}, nil
}

// SafeLine is the agent's last check on a line Headboard sent before it is
// written into authorized_keys: one line, starting with a key type, so it
// cannot carry options, be a comment or a marker, or start a second entry.
func SafeLine(line string) error {
	if strings.ContainsAny(line, "\r\n") {
		return errors.New("key line contains a line break")
	}

	typ, _, _ := strings.Cut(line, " ")
	for _, prefix := range []string{"ssh-", "ecdsa-sha2-", "sk-"} {
		if strings.HasPrefix(typ, prefix) {
			return nil
		}
	}

	return errors.New("key line does not start with a key type")
}

// CleanLabel is the only gate between admin input and a line written into
// authorized_keys, so anything that could start a new line is refused.
func CleanLabel(label string) (string, error) {
	label = strings.TrimSpace(label)
	if len(label) > 64 {
		return "", errors.New("label is longer than 64 characters")
	}

	for _, r := range label {
		if r > unicode.MaxASCII || !unicode.IsPrint(r) {
			return "", errors.New("label may contain printable ASCII characters only")
		}
	}

	return label, nil
}

// Line is the authorized_keys line for a stored key. The Headscale user name
// comes from Headscale, not from validated input, so it is made safe here.
func Line(key, user, label string) string {
	comment := asciiOnly(user)
	if label != "" {
		comment += " " + label
	}

	return key + " " + comment
}

func asciiOnly(s string) string {
	var b strings.Builder

	for _, r := range s {
		if r > unicode.MaxASCII || !unicode.IsPrint(r) || r == ' ' {
			b.WriteByte('?')
		} else {
			b.WriteRune(r)
		}
	}

	return b.String()
}

var unixUser = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// ValidUnixUser is checked by the API and again by the agent, because the name
// ends up as an argument to commands run as root.
func ValidUnixUser(name string) bool { return unixUser.MatchString(name) }

var targetRef = regexp.MustCompile(`^(self|[0-9]{1,9})$`)

// ValidRef reports whether a target ref is a Proxmox VMID or "self". Refs come
// from agent reports and end up in commands an admin runs as root, so nothing
// else is ever accepted.
func ValidRef(ref string) bool { return targetRef.MatchString(ref) }

// Passwd is the part of a passwd record the agent needs.
type Passwd struct {
	Home     string
	UID, GID int
}

// ParsePasswd reads `getent passwd <user>` output from inside a target. The
// target is not trusted, so anything but exactly one sane record is refused.
func ParsePasswd(out, user string) (Passwd, error) {
	out = strings.TrimRight(out, "\n")
	if out == "" || strings.Contains(out, "\n") {
		return Passwd{}, errors.New("expected exactly one passwd record")
	}

	f := strings.Split(out, ":")
	if len(f) != 7 || f[0] != user {
		return Passwd{}, errors.New("malformed passwd record")
	}

	uid, err := strconv.ParseUint(f[2], 10, 31)
	if err != nil {
		return Passwd{}, errors.New("uid is not a number from 0 to 2^31-1")
	}

	gid, err := strconv.ParseUint(f[3], 10, 31)
	if err != nil {
		return Passwd{}, errors.New("gid is not a number from 0 to 2^31-1")
	}

	// The file steps run as this uid; 0 for anyone but root would put root
	// back inside a user-owned directory.
	if uid == 0 && user != "root" {
		return Passwd{}, errors.New("uid 0 for a user other than root")
	}

	home := f[5]
	if !strings.HasPrefix(home, "/") || strings.Contains(home, "..") {
		return Passwd{}, errors.New("home is not a clean absolute path")
	}

	for _, r := range home {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return Passwd{}, errors.New("home contains whitespace or control characters")
		}
	}

	return Passwd{Home: home, UID: int(uid), GID: int(gid)}, nil
}
