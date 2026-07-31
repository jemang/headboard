// Package acl edits the Headscale ACL document without destroying it.
//
// The policy is HuJSON: JSON with comments and trailing commas. Operators put
// real explanations in those comments, and a form that regenerated the file
// from a parsed model would silently delete them the first time anyone touched
// a rule. So Headboard keeps the text as the source of truth and applies RFC
// 6902 patches to its syntax tree, which leaves everything the patch did not
// name byte-identical.
package acl

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tailscale/hujson"
)

// Doc is a parsed policy document.
type Doc struct {
	value hujson.Value
	text  string
}

// ErrEmpty is returned when there is no policy to work with.
var ErrEmpty = errors.New("the policy document is empty")

// Parse reads a policy document, preserving comments and layout.
func Parse(text string) (*Doc, error) {
	if text == "" {
		return nil, ErrEmpty
	}

	v, err := hujson.Parse([]byte(text))
	if err != nil {
		return nil, fmt.Errorf("parsing policy: %w", err)
	}

	return &Doc{value: v, text: text}, nil
}

// Text returns the document as it currently stands.
func (d *Doc) Text() string { return d.text }

// Standard returns the document with comments and trailing commas removed, for
// anything that needs plain JSON. The Doc itself is untouched.
func (d *Doc) Standard() ([]byte, error) {
	clone := d.value.Clone()
	clone.Standardize()

	return clone.Pack(), nil
}

// Decode unmarshals the document into v, going through Standardize first
// because encoding/json cannot read comments.
func (d *Doc) Decode(v any) error {
	std, err := d.Standard()
	if err != nil {
		return err
	}

	if err := json.Unmarshal(std, v); err != nil {
		return fmt.Errorf("decoding policy: %w", err)
	}

	return nil
}

// Schema decodes the document into the editable mirror the form uses.
func (d *Doc) Schema() (*Schema, error) {
	var s Schema
	if err := d.Decode(&s); err != nil {
		return nil, err
	}

	return &s, nil
}

// Find returns the value at an RFC 6901 JSON Pointer, or nil.
func (d *Doc) Find(pointer string) *hujson.Value {
	return d.value.Find(pointer)
}

// Format normalises whitespace without touching comments.
func (d *Doc) Format() {
	d.value.Format()
	d.text = d.value.String()
}
