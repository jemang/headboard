package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/jemang/headboard/internal/sshsync"
)

// readScript prints authorized_keys, or nothing when absent. $1 is the home
// directory. Symlinks are refused rather than followed.
const readScript = `set -eu
d="$1/.ssh"
if [ -L "$d" ] || [ -L "$d/authorized_keys" ]; then echo "refusing a symlinked .ssh or authorized_keys" >&2; exit 3; fi
if [ -f "$d/authorized_keys" ]; then cat "$d/authorized_keys"; fi
`

// writeScript replaces authorized_keys with stdin through a temp file in the
// same directory and a rename, so an interruption never leaves half a file.
// $1 home. It runs as the file's owner (see asUser), so it never chowns.
const writeScript = `set -eu
d="$1/.ssh"
if [ -L "$d" ] || [ -L "$d/authorized_keys" ]; then echo "refusing a symlinked .ssh or authorized_keys" >&2; exit 3; fi
if [ ! -d "$d" ]; then mkdir -m 700 "$d"; fi
t=$(mktemp "$d/.headboard.XXXXXX")
cat > "$t"
chmod 600 "$t"
mv -f "$t" "$d/authorized_keys"
`

// switchUser runs $4 as user $1 (uid $2, gid $3) with the rest as its
// arguments. Root working inside a directory that user owns can be raced: a
// temp file or .ssh swapped for a symlink after the check sends root's write,
// chown or mv to /etc/passwd or /root/.ssh. As the user, a swapped symlink only
// reaches what the user could already reach.
//
// setpriv comes first because su opens a PAM session, and inside an LXC
// pam_systemd can wait 25 s on logind for every call. busybox has no setpriv,
// and its su has no PAM. Both take the shell's $0 and arguments after -c.
const switchUser = `u=$1 uid=$2 gid=$3 s=$4
shift 4
if command -v setpriv >/dev/null 2>&1; then
	exec setpriv --reuid="$uid" --regid="$gid" --init-groups /bin/sh -c "$s" sh "$@"
fi
exec su -s /bin/sh "$u" -c "$s" sh "$@"
`

func asUser(user string, pw sshsync.Passwd, script string, args ...string) []string {
	return append([]string{"sh", "-c", switchUser, "sh", user, strconv.Itoa(pw.UID), strconv.Itoa(pw.GID), script}, args...)
}

func unsafeLine(lines []string) error {
	for _, l := range lines {
		if err := sshsync.SafeLine(l); err != nil {
			return fmt.Errorf("refused a line from Headboard: %w", err)
		}
	}

	return nil
}

var errNoSuchUser = errors.New("unix user does not exist on the target")

// leftoverScript exits 0 when $1 exists, symlink or not. It only looks, so it
// can run as root without following anything into a write.
const leftoverScript = `test -e "$1" || test -L "$1"`

// keysRemain reports whether a user getent no longer knows still has an
// authorized_keys at the conventional home. Only a definite "no such file"
// counts as nothing left: any other failure keeps the clear pending.
func keysRemain(ctx context.Context, h Host, ref, user string) (bool, error) {
	_, err := h.Exec(ctx, ref, "", "sh", "-c", leftoverScript, "sh", "/home/"+user+"/.ssh/authorized_keys")

	var ec exitCoder
	if errors.As(err, &ec) && ec.ExitCode() == 1 {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

type exitCoder interface{ ExitCode() int }

func passwd(ctx context.Context, h Host, ref, user string) (sshsync.Passwd, error) {
	if !sshsync.ValidUnixUser(user) {
		return sshsync.Passwd{}, errors.New("invalid unix user name")
	}

	out, err := h.Exec(ctx, ref, "", "getent", "passwd", user)
	var ec exitCoder
	if errors.As(err, &ec) && ec.ExitCode() == 2 {
		return sshsync.Passwd{}, errNoSuchUser
	}
	if err != nil {
		return sshsync.Passwd{}, err
	}

	return sshsync.ParsePasswd(out, user)
}

func rewrite(ctx context.Context, h Host, ref, user string, pw sshsync.Passwd, lines []string, onlyIfManaged bool) error {
	existing, err := h.Exec(ctx, ref, "", asUser(user, pw, readScript, pw.Home)...)
	if err != nil {
		return err
	}

	if onlyIfManaged && !strings.Contains(existing, sshsync.BeginMarker) {
		return nil
	}

	next, err := sshsync.ReplaceBlock(existing, lines)
	if err != nil {
		return err
	}
	if next == existing {
		return nil
	}

	_, err = h.Exec(ctx, ref, next, asUser(user, pw, writeScript, pw.Home)...)

	return err
}

// Apply makes each target match what Headboard sent. A target that is not
// running is skipped with its clears left pending; a target whose IP no longer
// matches is reported as mismatched and left untouched.
func Apply(ctx context.Context, h Host, discovered []sshsync.Discovered, targets []sshsync.Target) Result {
	r := Result{Applied: []sshsync.Pair{}, Cleared: []sshsync.Pair{}, Mismatched: []string{}, Warnings: []sshsync.Warning{}}

	ips := map[string][]string{}
	hostnames := map[string]string{}
	for _, d := range discovered {
		ips[d.Ref] = d.IPs
		hostnames[d.Ref] = d.Hostname
	}

	// Headboard's schema caps a message at 2048 bytes and rejects the whole
	// report past it, so one noisy container must not cost every report.
	warn := func(ref, user string, err error) {
		r.Warnings = append(r.Warnings, sshsync.Warning{Ref: ref, UnixUser: user, Message: truncate(err.Error(), 2000)})
	}

	for _, t := range targets {
		got, running := ips[t.Ref]
		if !running {
			warn(t.Ref, "", errors.New("target is not running"))

			continue
		}

		if t.ExpectIP != "" && len(got) == 0 {
			warn(t.Ref, "", errors.New("could not read the target's address; skipped"))

			continue
		}

		if t.ExpectLabel != "" && hostnames[t.Ref] == "" {
			warn(t.Ref, "", errors.New("could not read the target's hostname; skipped"))

			continue
		}

		if t.ExpectLabel != "" && hostnames[t.Ref] != t.ExpectLabel {
			r.Mismatched = append(r.Mismatched, t.Ref)
			warn(t.Ref, "", fmt.Errorf("target hostname is now %q, approved as %q; binding suspended until re-approved", hostnames[t.Ref], t.ExpectLabel))

			continue
		}

		if t.ExpectIP != "" && !slices.Contains(got, t.ExpectIP) {
			r.Mismatched = append(r.Mismatched, t.Ref)
			warn(t.Ref, "", fmt.Errorf("target no longer has %s; binding suspended until re-approved", t.ExpectIP))

			continue
		}

		users := make([]string, 0, len(t.Users))
		for u := range t.Users {
			users = append(users, u)
		}
		sort.Strings(users)

		for _, u := range users {
			err := unsafeLine(t.Users[u])

			var pw sshsync.Passwd
			if err == nil {
				pw, err = passwd(ctx, h, t.Ref, u)
			}
			if err == nil {
				err = rewrite(ctx, h, t.Ref, u, pw, t.Users[u], false)
			}
			if err != nil {
				warn(t.Ref, u, err)

				continue
			}

			r.Applied = append(r.Applied, sshsync.Pair{Ref: t.Ref, UnixUser: u})
		}

		for _, u := range t.Clear {
			pw, err := passwd(ctx, h, t.Ref, u)
			switch {
			case errors.Is(err, errNoSuchUser):
				var remain bool
				if remain, err = keysRemain(ctx, h, t.Ref, u); err == nil && remain {
					err = fmt.Errorf("unix user is gone but /home/%s/.ssh/authorized_keys remains; clear kept pending", u)
				}
			case err == nil:
				err = rewrite(ctx, h, t.Ref, u, pw, nil, true)
			}
			if err != nil {
				warn(t.Ref, u, err)

				continue
			}

			r.Cleared = append(r.Cleared, sshsync.Pair{Ref: t.Ref, UnixUser: u})
		}
	}

	return r
}
