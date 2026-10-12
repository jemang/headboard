package agent

import (
	"strconv"
	"strings"
)

// IsDowngrade reports whether installing offered over running would go back
// a version. An unreadable offered version counts as one when running is a
// real version, since nothing proves it newer. A running build without a
// version ("dev") accepts anything.
func IsDowngrade(running, offered string) bool {
	r, okR := parseSemver(running)
	if !okR {
		return false
	}

	o, okO := parseSemver(offered)
	if !okO {
		return true
	}

	return compareSemver(o, r) < 0
}

type semver struct {
	core [3]int
	pre  []string
}

func parseSemver(v string) (semver, bool) {
	v = strings.TrimPrefix(v, "v")
	v, _, _ = strings.Cut(v, "+")
	core, pre, hasPre := strings.Cut(v, "-")

	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semver{}, false
	}

	var s semver
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, false
		}
		s.core[i] = n
	}

	if hasPre {
		if pre == "" {
			return semver{}, false
		}
		s.pre = strings.Split(pre, ".")
	}

	return s, true
}

func compareSemver(a, b semver) int {
	for i := range a.core {
		if a.core[i] != b.core[i] {
			return cmpInt(a.core[i], b.core[i])
		}
	}

	// A release outranks any of its pre-releases.
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1
	case len(b.pre) == 0:
		return -1
	}

	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		x, errX := strconv.Atoi(a.pre[i])
		y, errY := strconv.Atoi(b.pre[i])

		switch {
		case errX == nil && errY == nil:
			if x != y {
				return cmpInt(x, y)
			}
		case errX == nil:
			return -1
		case errY == nil:
			return 1
		case a.pre[i] != b.pre[i]:
			return strings.Compare(a.pre[i], b.pre[i])
		}
	}

	return cmpInt(len(a.pre), len(b.pre))
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}

	return 0
}
