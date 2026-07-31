package acl

import (
	"encoding/json"
	"strings"
	"testing"
)

// commented is written the way an operator writes one: explanations above
// rules, irregular alignment, trailing commas. Every one of those has to
// survive an edit, or the form is worse than a textarea.
const commented = `// Seed policy for the throwaway Headscale.
//
// This whole header must survive.
{
  // Who is allowed to claim which tag.
  "tagOwners": {
    "tag:prod": ["ops@"],
    "tag:ci":   ["ops@"],
  },

  "groups": {
    "group:eng": ["alice@", "bob@"],
    "group:ops": ["ops@"],
  },

  "hosts": {
    "office-lan": "10.0.0.0/24",
  },

  "acls": [
    // Engineers reach production over HTTPS only.
    {
      "action": "accept",
      "src":    ["group:eng"],
      "dst":    ["tag:prod:443"],
    },

    // Ops gets SSH to everything it owns.
    {
      "action": "accept",
      "src":    ["group:ops"],
      "dst":    ["tag:prod:22", "tag:ci:22"],
    },
  ],

  // Routes the gateway may advertise without an admin clicking approve.
  "autoApprovers": {
    "routes": {
      "10.0.0.0/24": ["tag:prod"],
    },
    "exitNode": ["tag:prod"],
  },
}
`

func mustParse(t *testing.T, text string) *Doc {
	t.Helper()

	d, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	return d
}

// The load-bearing test for the whole ACL feature.
func TestPatchPreservesEveryOtherComment(t *testing.T) {
	doc := mustParse(t, commented)

	patched, err := doc.Replace("/acls/0", Rule{
		Action: "accept",
		Src:    []string{"group:eng"},
		Dst:    []string{"tag:prod:443", "tag:prod:8443"},
	})
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}

	got := patched.Text()

	// Every comment in the original must still be present, verbatim.
	for _, comment := range []string{
		"// Seed policy for the throwaway Headscale.",
		"// This whole header must survive.",
		"// Who is allowed to claim which tag.",
		"// Engineers reach production over HTTPS only.",
		"// Ops gets SSH to everything it owns.",
		"// Routes the gateway may advertise without an admin clicking approve.",
	} {
		if !strings.Contains(got, comment) {
			t.Errorf("patching one rule destroyed a comment:\n  %s", comment)
		}
	}

	if !strings.Contains(got, "tag:prod:8443") {
		t.Error("the patch did not actually apply")
	}

	// And the untouched rule must be unchanged.
	if !strings.Contains(got, `"tag:ci:22"`) {
		t.Error("patching rule 0 disturbed rule 1")
	}
}

// A rule the patch did not name must come back byte-for-byte, not merely
// semantically equal: reformatting someone else's rule shows up as a spurious
// change in the diff and in git.
func TestUnrelatedLinesAreByteIdentical(t *testing.T) {
	doc := mustParse(t, commented)

	patched, err := doc.Replace("/hosts/office-lan", "10.1.0.0/24")
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}

	diff := Unified(commented, patched.Text())

	if diff.Identical {
		t.Fatal("the patch changed nothing")
	}

	// Exactly one line should differ.
	if diff.Added != 1 || diff.Removed != 1 {
		t.Errorf("changing one host produced +%d/-%d lines:\n%s",
			diff.Added, diff.Removed, diff)
	}
}

func TestPatchLeavesTheOriginalAlone(t *testing.T) {
	doc := mustParse(t, commented)
	before := doc.Text()

	if _, err := doc.Replace("/acls/0", Rule{Action: "accept", Src: []string{"*"}, Dst: []string{"*:*"}}); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	// hujson leaves a value partially mutated when a patch fails part-way,
	// so Patch has to work on a clone. If it did not, a failed save would
	// corrupt the in-memory document.
	if doc.Text() != before {
		t.Error("Patch mutated the receiver")
	}
}

func TestPatchRejectsBadOperations(t *testing.T) {
	doc := mustParse(t, commented)

	tests := []struct {
		name string
		ops  []Op
	}{
		{"no operations", nil},
		{"unknown op", []Op{{Op: "frobnicate", Path: "/acls/0"}}},
		{"add without a value", []Op{{Op: "add", Path: "/acls/-"}}},
		{"move without from", []Op{{Op: "move", Path: "/acls/0"}}},
		{"pointer missing a leading slash", []Op{{Op: "remove", Path: "acls/0"}}},
		{"pointer past the end", []Op{{Op: "replace", Path: "/acls/99", Value: json.RawMessage(`{}`)}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := doc.Patch(tt.ops); err == nil {
				t.Fatal("got nil error, want a rejection")
			}
		})
	}
}

func TestAddAndRemoveRules(t *testing.T) {
	doc := mustParse(t, commented)

	added, err := doc.Patch([]Op{{
		Op:    "add",
		Path:  "/acls/-",
		Value: json.RawMessage(`{"action":"accept","src":["group:ops"],"dst":["*:*"]}`),
	}})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	schema, err := added.Schema()
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	if len(schema.ACLs) != 3 {
		t.Fatalf("acls = %d after an add, want 3", len(schema.ACLs))
	}

	removed, err := added.Patch([]Op{{Op: "remove", Path: "/acls/0"}})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}

	schema, err = removed.Schema()
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	if len(schema.ACLs) != 2 {
		t.Fatalf("acls = %d after a remove, want 2", len(schema.ACLs))
	}

	// Removing rule 0 must remove rule 0, not shuffle the rest.
	if schema.ACLs[0].Src[0] != "group:ops" || schema.ACLs[0].Dst[0] != "tag:prod:22" {
		t.Errorf("remove took the wrong rule: %+v", schema.ACLs[0])
	}
}

func TestSchemaRoundTrip(t *testing.T) {
	doc := mustParse(t, commented)

	s, err := doc.Schema()
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	if len(s.Groups) != 2 || len(s.Groups["group:eng"]) != 2 {
		t.Errorf("groups: %+v", s.Groups)
	}

	if len(s.TagOwners) != 2 {
		t.Errorf("tagOwners: %+v", s.TagOwners)
	}

	if s.Hosts["office-lan"] != "10.0.0.0/24" {
		t.Errorf("hosts: %+v", s.Hosts)
	}

	if s.AutoApprovers == nil || len(s.AutoApprovers.ExitNode) != 1 {
		t.Errorf("autoApprovers: %+v", s.AutoApprovers)
	}

	if len(s.ACLs) != 2 || s.ACLs[1].Dst[1] != "tag:ci:22" {
		t.Errorf("acls: %+v", s.ACLs)
	}
}

// A picker that offers a group nobody defined produces a policy Headscale
// rejects at save time — the exact failure this UI exists to prevent.
func TestTokensOfferOnlyWhatResolves(t *testing.T) {
	doc := mustParse(t, commented)

	s, err := doc.Schema()
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}

	tokens := s.TokensFor([]string{"alice", "bob", "ops"})

	if got := strings.Join(tokens.Users, ","); got != "alice@,bob@,ops@" {
		t.Errorf("users: %q", got)
	}

	if got := strings.Join(tokens.Groups, ","); got != "group:eng,group:ops" {
		t.Errorf("groups: %q", got)
	}

	// Only tags with an owner: an unowned tag cannot be applied to
	// anything.
	if got := strings.Join(tokens.Tags, ","); got != "tag:ci,tag:prod" {
		t.Errorf("tags: %q", got)
	}

	if got := strings.Join(tokens.Hosts, ","); got != "office-lan" {
		t.Errorf("hosts: %q", got)
	}

	if len(tokens.AutoGroups) == 0 {
		t.Error("no autogroups offered")
	}
}

func TestUnifiedDiff(t *testing.T) {
	t.Run("identical", func(t *testing.T) {
		d := Unified(commented, commented)
		if !d.Identical || len(d.Hunks) != 0 {
			t.Errorf("got %+v", d)
		}
	})

	t.Run("counts additions and removals", func(t *testing.T) {
		modified := strings.Replace(commented, `"office-lan": "10.0.0.0/24",`, `"office-lan": "10.9.0.0/24",`, 1)

		d := Unified(commented, modified)
		if d.Identical {
			t.Fatal("identical, want a difference")
		}

		if d.Added != 1 || d.Removed != 1 {
			t.Errorf("+%d/-%d, want +1/-1", d.Added, d.Removed)
		}

		rendered := d.String()

		if !strings.Contains(rendered, `+    "office-lan": "10.9.0.0/24",`) {
			t.Errorf("the new line is missing from the rendered diff:\n%s", rendered)
		}

		if !strings.Contains(rendered, `-    "office-lan": "10.0.0.0/24",`) {
			t.Errorf("the old line is missing from the rendered diff:\n%s", rendered)
		}
	})

	t.Run("pure insertion", func(t *testing.T) {
		d := Unified("a\nb\n", "a\nnew\nb\n")
		if d.Added != 1 || d.Removed != 0 {
			t.Errorf("+%d/-%d, want +1/-0", d.Added, d.Removed)
		}
	})

	t.Run("empty to non-empty", func(t *testing.T) {
		d := Unified("", "a\nb\n")
		if d.Added != 2 || d.Removed != 0 {
			t.Errorf("+%d/-%d, want +2/-0", d.Added, d.Removed)
		}
	})
}

func TestSHA256DetectsAnyChange(t *testing.T) {
	a := SHA256(commented)

	// Even whitespace counts: the guard exists to catch a concurrent edit,
	// and reformatting is an edit.
	b := SHA256(commented + "\n")

	if a == b {
		t.Error("appending a newline did not change the hash")
	}

	if SHA256(commented) != a {
		t.Error("the hash is not stable")
	}
}

func TestPointerEscaping(t *testing.T) {
	tests := []struct {
		segments []string
		want     string
	}{
		{[]string{"acls", "0"}, "/acls/0"},
		{[]string{"groups", "group:eng"}, "/groups/group:eng"},
		// "/" and "~" are the two characters RFC 6901 escapes, and both
		// turn up in real host and tag names.
		{[]string{"hosts", "a/b"}, "/hosts/a~1b"},
		{[]string{"hosts", "a~b"}, "/hosts/a~0b"},
	}

	for _, tt := range tests {
		if got := Pointer(tt.segments...); got != tt.want {
			t.Errorf("Pointer(%v) = %q, want %q", tt.segments, got, tt.want)
		}
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := Parse(""); err == nil {
		t.Error("empty text parsed")
	}

	if _, err := Parse(`{"acls": [`); err == nil {
		t.Error("truncated document parsed")
	}
}

// hujson.Format indents with tabs. A space-indented policy that gets one
// container reformatted would end up with two indentation styles in one file,
// and a diff that looks like far more changed than did.
func TestPatchKeepsTheDocumentsIndentation(t *testing.T) {
	doc := mustParse(t, commented)

	patched, err := doc.Patch([]Op{{
		Op:    "add",
		Path:  "/acls/-",
		Value: json.RawMessage(`{"action":"accept","src":["group:ops"],"dst":["tag:ci:22"]}`),
	}})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}

	for i, line := range strings.Split(patched.Text(), "\n") {
		if strings.HasPrefix(line, "\t") {
			t.Errorf("line %d is tab-indented in a space-indented document: %q", i+1, line)
		}
	}

	// The added rule must still be on its own line, not jammed onto the
	// previous one.
	if strings.Contains(patched.Text(), `},{"action"`) {
		t.Error("the added rule was not laid out on its own line")
	}

	if !strings.Contains(patched.Text(), "tag:ci:22") {
		t.Error("the add did not apply")
	}
}

// A tab-indented document must be left tab-indented.
func TestPatchLeavesTabIndentedDocumentsAlone(t *testing.T) {
	const tabbed = "{\n\t\"acls\": [\n\t\t{\"action\": \"accept\", \"src\": [\"*\"], \"dst\": [\"*:*\"]},\n\t],\n}\n"

	doc := mustParse(t, tabbed)

	patched, err := doc.Patch([]Op{{
		Op:    "add",
		Path:  "/acls/-",
		Value: json.RawMessage(`{"action":"accept","src":["*"],"dst":["*:22"]}`),
	}})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}

	if strings.Contains(patched.Text(), "\n    ") {
		t.Errorf("a tab-indented document came back with spaces:\n%s", patched.Text())
	}
}

// Replacing a whole rule must not collapse it onto one line. The form's most
// common edit is exactly this, and a five-line block turning into one is a
// visible, permanent degradation of the operator's file.
func TestReplacingARuleKeepsItReadable(t *testing.T) {
	doc := mustParse(t, commented)

	patched, err := doc.Replace("/acls/0", Rule{
		Action: "accept",
		Src:    []string{"group:eng", "bob@"},
		Dst:    []string{"tag:prod:443"},
	})
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}

	text := patched.Text()

	if strings.Contains(text, `{"action":"accept"`) {
		t.Errorf("the replaced rule was left as compact JSON:\n%s", text)
	}

	// The comment above it must still be attached to it.
	idx := strings.Index(text, "// Engineers reach production over HTTPS only.")
	if idx < 0 {
		t.Fatal("the rule's comment was lost")
	}

	if !strings.Contains(text[idx:idx+200], `"bob@"`) {
		t.Errorf("the comment is no longer next to the rule it describes:\n%s", text[idx:idx+200])
	}

	// And a scalar replace must still not be reformatted.
	scalar, err := doc.Replace("/hosts/office-lan", "10.2.0.0/24")
	if err != nil {
		t.Fatalf("scalar Replace: %v", err)
	}

	d := Unified(commented, scalar.Text())
	if d.Added != 1 || d.Removed != 1 {
		t.Errorf("a scalar replace produced +%d/-%d, want +1/-1:\n%s", d.Added, d.Removed, d)
	}
}
