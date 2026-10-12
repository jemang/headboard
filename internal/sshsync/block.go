package sshsync

import (
	"errors"
	"strings"
)

const (
	BeginMarker = "# BEGIN headboard (managed — do not edit)"
	EndMarker   = "# END headboard"
)

// ErrBrokenBlock means the file has a begin marker with no end. Rewriting it
// would have to guess where the block stops, and guessing wrong deletes keys
// that someone added by hand.
var ErrBrokenBlock = errors.New("authorized_keys has a headboard begin marker without an end marker")

// ErrDuplicateBlock means the file has more than one managed block. Only one
// is ever written, so a second one is a copy whose keys would never be revoked.
var ErrDuplicateBlock = errors.New("authorized_keys has more than one headboard block")

// ReplaceBlock returns existing with the managed block set to lines. Markers
// count only as whole lines; content outside the block is kept byte for byte,
// and a missing block is appended.
func ReplaceBlock(existing string, lines []string) (string, error) {
	var b strings.Builder

	b.WriteString(BeginMarker + "\n")
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	b.WriteString(EndMarker + "\n")

	begin, end, err := findBlock(existing)
	if err != nil {
		return "", err
	}

	if begin < 0 {
		if existing != "" && !strings.HasSuffix(existing, "\n") {
			existing += "\n"
		}

		return existing + b.String(), nil
	}

	return existing[:begin] + b.String() + existing[end:], nil
}

// findBlock returns the byte range of the managed block, from the start of
// its begin line to just past its end line, or -1 when there is none.
func findBlock(s string) (begin, end int, err error) {
	begin, end = -1, -1
	blocks := 0

	for off := 0; off < len(s); {
		next := strings.IndexByte(s[off:], '\n')
		lineEnd := len(s)
		if next >= 0 {
			lineEnd = off + next
		}

		switch s[off:lineEnd] {
		case BeginMarker:
			blocks++
			if blocks > 1 {
				return -1, -1, ErrDuplicateBlock
			}
			begin = off
		case EndMarker:
			if begin >= 0 && end < 0 {
				end = min(lineEnd+1, len(s))
			}
		}

		off = lineEnd + 1
	}

	if begin >= 0 && end < 0 {
		return -1, -1, ErrBrokenBlock
	}

	return begin, end, nil
}
