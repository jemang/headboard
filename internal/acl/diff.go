package acl

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// SHA256 identifies a policy document by content.
//
// The write guard compares hashes rather than timestamps: Headscale's
// updatedAt has second resolution and no guarantee of monotonicity, and two
// admins saving within the same second is exactly the case the guard exists
// for.
func SHA256(text string) string {
	sum := sha256.Sum256([]byte(text))

	return hex.EncodeToString(sum[:])
}

// Hunk is a run of changed lines.
type Hunk struct {
	OldStart int      `json:"oldStart"`
	OldLines int      `json:"oldLines"`
	NewStart int      `json:"newStart"`
	NewLines int      `json:"newLines"`
	Lines    []string `json:"lines"`
}

// Diff is a unified diff between two policy documents.
type Diff struct {
	Hunks []Hunk `json:"hunks"`

	Added   int `json:"added"`
	Removed int `json:"removed"`

	// Identical is true when nothing changed, which the UI uses to say
	// "no changes" rather than showing an empty diff.
	Identical bool `json:"identical"`
}

// context is how many unchanged lines surround each hunk.
const context = 3

// Unified computes a unified diff. It is shown before every save: an ACL
// mistake locks people out of machines, so the confirm step has to show what is
// actually about to change rather than asserting that something will.
func Unified(oldText, newText string) Diff {
	if oldText == newText {
		return Diff{Identical: true, Hunks: []Hunk{}}
	}

	oldLines := splitLines(oldText)
	newLines := splitLines(newText)

	ops := diffLines(oldLines, newLines)

	d := Diff{Hunks: []Hunk{}}

	var (
		hunk    *Hunk
		oldNum  = 1
		newNum  = 1
		pending []string
	)

	flush := func() {
		if hunk != nil {
			d.Hunks = append(d.Hunks, *hunk)
			hunk = nil
		}
	}

	for i := 0; i < len(ops); i++ {
		op := ops[i]

		if op.kind == opEqual {
			if hunk == nil {
				// Remember trailing context in case a change
				// follows soon.
				pending = append(pending, " "+op.text)
				if len(pending) > context {
					pending = pending[1:]
				}
			} else {
				hunk.Lines = append(hunk.Lines, " "+op.text)
				hunk.OldLines++
				hunk.NewLines++

				// Close the hunk once enough unchanged lines
				// have passed and nothing else is coming.
				if trailingEqual(ops, i) > context {
					flush()
					pending = nil
				}
			}

			oldNum++
			newNum++

			continue
		}

		if hunk == nil {
			hunk = &Hunk{
				OldStart: oldNum - len(pending),
				NewStart: newNum - len(pending),
				OldLines: len(pending),
				NewLines: len(pending),
				Lines:    append([]string{}, pending...),
			}
			pending = nil
		}

		switch op.kind {
		case opDelete:
			hunk.Lines = append(hunk.Lines, "-"+op.text)
			hunk.OldLines++
			d.Removed++
			oldNum++
		case opInsert:
			hunk.Lines = append(hunk.Lines, "+"+op.text)
			hunk.NewLines++
			d.Added++
			newNum++
		}
	}

	flush()

	return d
}

// String renders the diff the way `diff -u` would, for logs and for the
// confirmation dialog's plain-text fallback.
func (d Diff) String() string {
	if d.Identical {
		return ""
	}

	var b strings.Builder

	for _, h := range d.Hunks {
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", h.OldStart, h.OldLines, h.NewStart, h.NewLines)

		for _, l := range h.Lines {
			b.WriteString(l)
			b.WriteByte('\n')
		}
	}

	return b.String()
}

type opKind int

const (
	opEqual opKind = iota
	opDelete
	opInsert
)

type lineOp struct {
	kind opKind
	text string
}

// diffLines is a straightforward LCS diff. Policy documents are hundreds of
// lines at most, so the quadratic table is not worth avoiding.
func diffLines(a, b []string) []lineOp {
	n, m := len(a), len(b)

	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}

	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var ops []lineOp

	i, j := 0, 0

	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, lineOp{opEqual, a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, lineOp{opDelete, a[i]})
			i++
		default:
			ops = append(ops, lineOp{opInsert, b[j]})
			j++
		}
	}

	for ; i < n; i++ {
		ops = append(ops, lineOp{opDelete, a[i]})
	}

	for ; j < m; j++ {
		ops = append(ops, lineOp{opInsert, b[j]})
	}

	return ops
}

// trailingEqual counts how many consecutive equal ops follow position i.
func trailingEqual(ops []lineOp, i int) int {
	n := 0

	for ; i < len(ops) && ops[i].kind == opEqual; i++ {
		n++
	}

	return n
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}

	s = strings.TrimSuffix(s, "\n")

	return strings.Split(s, "\n")
}
