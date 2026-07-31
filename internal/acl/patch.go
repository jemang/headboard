package acl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tailscale/hujson"
)

// Op is one RFC 6902 operation.
//
// The form sends operations rather than a whole document. That is the entire
// reason comments survive: replacing /acls/2 rewrites exactly that array
// element and leaves the comment above it, the ones around it, and the file's
// indentation untouched.
type Op struct {
	// Op is add, remove, replace, move, copy or test.
	Op string `json:"op" enum:"add,remove,replace,move,copy,test"`

	// Path is an RFC 6901 JSON Pointer, e.g. "/acls/2" or "/groups/group:eng".
	Path string `json:"path" example:"/acls/2"`

	// From is the source pointer for move and copy.
	From string `json:"from,omitempty"`

	// Value is the new value for add, replace and test.
	Value json.RawMessage `json:"value,omitempty"`
}

// ErrNoOps is returned when a patch would change nothing.
var ErrNoOps = fmt.Errorf("the patch contains no operations")

// Patch applies operations and returns a new document.
//
// The receiver is never mutated: hujson leaves a value partially patched when
// an operation fails part-way through, so this works on a clone and only
// installs the result once everything applied.
func (d *Doc) Patch(ops []Op) (*Doc, error) {
	if len(ops) == 0 {
		return nil, ErrNoOps
	}

	for i, op := range ops {
		if err := validateOp(op); err != nil {
			return nil, fmt.Errorf("operation %d: %w", i, err)
		}
	}

	// Values are laid out to match the document *before* they reach hujson.
	//
	// hujson preserves the formatting of a value carried in the patch, but it
	// cannot re-format a subtree in place afterwards: Format works from depth
	// zero, so formatting one array leaves it un-nested relative to the file
	// and can pull an adjacent comment onto the wrong line. Getting the
	// indentation right on the way in is the only approach that leaves the
	// rest of the document untouched.
	encoded, marshalErr := encodePatch(ops, d.text)
	if marshalErr != nil {
		return nil, fmt.Errorf("encoding patch: %w", marshalErr)
	}

	// Re-parse rather than Clone. hujson.Value.Clone drops trailing commas:
	// Parse(s).String() == s, but Parse(s).Clone().String() != s, and the
	// difference is every trailing comma in the document. Re-parsing gives
	// the same isolation from the receiver at the cost of one parse, and
	// costs nothing in fidelity.
	clone, err := hujson.Parse([]byte(d.text))
	if err != nil {
		return nil, fmt.Errorf("re-parsing policy: %w", err)
	}

	if err := clone.Patch(encoded); err != nil {
		return nil, fmt.Errorf("applying patch: %w", err)
	}

	// No Format() at all, despite hujson recommending one after a patch.
	// Format normalises whitespace across the entire document, so changing a
	// single host would rewrite every hand-aligned line — 33 lines of diff for
	// a one-line edit, and the operator's column alignment silently destroyed.
	// The values were indented on the way in instead.
	return &Doc{value: clone, text: clone.String()}, nil
}

// encodePatch renders the operations as a patch document whose values are
// already indented to sit correctly in the target document.
func encodePatch(ops []Op, target string) ([]byte, error) {
	unit, ok := detectIndent(target)
	if !ok {
		unit = "\t"
	}

	// The patch document is assembled as text rather than with json.Marshal,
	// because Marshal *compacts* a json.RawMessage — it would strip the
	// indentation this function exists to add.
	var b strings.Builder

	b.WriteByte('[')

	for i, op := range ops {
		if i > 0 {
			b.WriteByte(',')
		}

		b.WriteString(`{"op":`)
		writeJSONString(&b, op.Op)
		b.WriteString(`,"path":`)
		writeJSONString(&b, op.Path)

		if op.From != "" {
			b.WriteString(`,"from":`)
			writeJSONString(&b, op.From)
		}

		if len(op.Value) > 0 {
			// A pointer's depth is how many levels down the value will
			// sit, so that is how far its continuation lines indent.
			depth := strings.Count(strings.TrimPrefix(op.Path, "/"), "/") + 1

			indented, err := indentValue(op.Value, strings.Repeat(unit, depth), unit)
			if err != nil {
				return nil, err
			}

			b.WriteString(`,"value":`)
			b.Write(indented)
		}

		b.WriteByte('}')
	}

	b.WriteByte(']')

	return []byte(b.String()), nil
}

func writeJSONString(b *strings.Builder, s string) {
	encoded, err := json.Marshal(s)
	if err != nil {
		// A Go string always marshals; this cannot happen.
		b.WriteString(`""`)

		return
	}

	b.Write(encoded)
}

// indentValue re-encodes a JSON value with the document's indentation. Scalars
// are left exactly as they are: indenting one changes nothing and json.Indent
// would still round-trip it, but leaving it untouched keeps the intent obvious.
func indentValue(raw json.RawMessage, prefix, unit string) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || (trimmed[0] != '{' && trimmed[0] != '[') {
		return raw, nil
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, prefix, unit); err != nil {
		return nil, fmt.Errorf("indenting patch value: %w", err)
	}

	return buf.Bytes(), nil
}

// detectIndent returns the document's indentation unit, and false if it uses
// tabs or nothing recognisable.
func detectIndent(s string) (string, bool) {
	var unit string

	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			continue
		}

		// A tab anywhere in the leading whitespace means the file is
		// already tab-indented; leave hujson's output alone.
		if line[0] == '\t' {
			return "", false
		}

		spaces := 0
		for spaces < len(line) && line[spaces] == ' ' {
			spaces++
		}

		if spaces == 0 || spaces == len(line) {
			continue
		}

		if unit == "" || spaces < len(unit) {
			unit = strings.Repeat(" ", spaces)
		}
	}

	return unit, unit != ""
}

func validateOp(op Op) error {
	switch op.Op {
	case "add", "replace", "test":
		if len(op.Value) == 0 {
			return fmt.Errorf("%q needs a value", op.Op)
		}
	case "remove":
	case "move", "copy":
		if op.From == "" {
			return fmt.Errorf("%q needs a from pointer", op.Op)
		}

		if err := validPointer(op.From); err != nil {
			return fmt.Errorf("from: %w", err)
		}
	default:
		return fmt.Errorf("unknown operation %q", op.Op)
	}

	return validPointer(op.Path)
}

// validPointer rejects pointers that are not RFC 6901. hujson would reject them
// too, but its error names an offset in a generated patch document rather than
// the field the operator typed.
func validPointer(p string) error {
	if p == "" {
		return nil // the whole document
	}

	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("pointer %q must start with /", p)
	}

	return nil
}

// Replace is the common case: swap the value at one pointer.
func (d *Doc) Replace(pointer string, value any) (*Doc, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encoding value: %w", err)
	}

	return d.Patch([]Op{{Op: "replace", Path: pointer, Value: raw}})
}

// SetText replaces the whole document, for the raw editor.
func SetText(text string) (*Doc, error) {
	return Parse(text)
}

// Pointer builds an RFC 6901 pointer from segments, escaping as the spec
// requires. Tags and group names contain "/" often enough that hand-built
// pointers are a real source of silent misses.
func Pointer(segments ...string) string {
	var b strings.Builder

	for _, s := range segments {
		b.WriteByte('/')
		b.WriteString(escapeToken(s))
	}

	return b.String()
}

func escapeToken(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")

	return strings.ReplaceAll(s, "/", "~1")
}

// Value exposes the underlying tree for callers that need to inspect it.
func (d *Doc) Value() *hujson.Value { return &d.value }
