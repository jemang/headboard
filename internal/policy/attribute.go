package policy

import (
	"encoding/json"
	"fmt"

	policyv2 "github.com/juanfont/headscale/hscontrol/policy/v2"
	"github.com/tailscale/hujson"
)

// Attribution links a decision back to the line the operator wrote.
//
// It cannot be read off the compiled rules. The engine merges grants
// (globalFilterRules ends in mergeFilterRules), so the index of a compiled
// tailcfg.FilterRule has no fixed relationship to the index of the `acls`
// entry that produced it: several entries can collapse into one rule, and one
// entry can expand into several.
//
// So attribution recompiles instead. Each `acls`/`grants` entry is compiled on
// its own — keeping groups, hosts and tagOwners so aliases still resolve — and
// the first isolated policy that permits the connection is the one responsible.
// ACL entries are purely additive in policy v2, so evaluating one in isolation
// cannot manufacture an allow that the full policy would deny.
type Attribution struct {
	// Section is "acls" or "grants".
	Section string `json:"section"`

	// Index is the position within that section, so the UI can deep-link
	// to the row and the raw editor can jump to the right JSON pointer.
	Index int `json:"index"`

	// Pointer is the RFC 6901 JSON Pointer for the entry, e.g. "/acls/2".
	Pointer string `json:"pointer"`
}

// attributionEntry is one isolated rule and the engine compiled from it.
type attributionEntry struct {
	section string
	index   int
	pm      *policyv2.PolicyManager
}

// buildAttribution compiles one policy manager per rule entry. Called under
// attributionOnce; the result is cached until the next rebuild.
func (m *Manager) buildAttribution() {
	m.mu.RLock()
	hujsonText, users, slice := m.hujson, m.users, m.views
	m.mu.RUnlock()

	doc, err := decodeForAttribution(hujsonText)
	if err != nil {
		m.mu.Lock()
		m.attributionErr = err
		m.mu.Unlock()

		return
	}

	var entries []attributionEntry

	for _, section := range []string{"acls", "grants"} {
		raw, ok := doc[section]
		if !ok {
			continue
		}

		var rules []json.RawMessage
		if err := json.Unmarshal(raw, &rules); err != nil {
			m.mu.Lock()
			m.attributionErr = fmt.Errorf("attribution: %s: %w", section, err)
			m.mu.Unlock()

			return
		}

		for i, rule := range rules {
			isolated, err := isolatedPolicy(doc, section, rule)
			if err != nil {
				m.mu.Lock()
				m.attributionErr = err
				m.mu.Unlock()

				return
			}

			pm, err := policyv2.NewPolicyManager(isolated, users, slice)
			if err != nil {
				// A rule that will not compile alone is not a fatal
				// error for the whole tailnet: skip it, and the
				// simulator reports the decision without a source.
				continue
			}

			entries = append(entries, attributionEntry{section: section, index: i, pm: pm})
		}
	}

	m.mu.Lock()
	m.attributionEntries = entries
	m.mu.Unlock()
}

// decodeForAttribution strips comments and decodes the policy into its top
// level sections. Comments do not survive here on purpose — this path analyses
// the document, it never writes it back. internal/acl owns editing.
func decodeForAttribution(text string) (map[string]json.RawMessage, error) {
	v, err := hujson.Parse([]byte(text))
	if err != nil {
		return nil, fmt.Errorf("attribution: parsing policy: %w", err)
	}

	v.Standardize()

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(v.Pack(), &doc); err != nil {
		return nil, fmt.Errorf("attribution: decoding policy: %w", err)
	}

	return doc, nil
}

// isolatedPolicy builds a document containing one rule plus everything needed
// to resolve the aliases in it.
//
// ssh and tests are dropped deliberately: they are validated eagerly and would
// fail against a policy whose acls list has been reduced to a single entry.
func isolatedPolicy(doc map[string]json.RawMessage, section string, rule json.RawMessage) ([]byte, error) {
	out := make(map[string]json.RawMessage, 4)

	for _, keep := range []string{"groups", "hosts", "tagOwners"} {
		if raw, ok := doc[keep]; ok {
			out[keep] = raw
		}
	}

	list, err := json.Marshal([]json.RawMessage{rule})
	if err != nil {
		return nil, fmt.Errorf("attribution: marshalling rule: %w", err)
	}

	out[section] = list

	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("attribution: marshalling policy: %w", err)
	}

	return b, nil
}

// attributionFor returns the compiled per-rule engines, building them on first
// use.
func (m *Manager) attributionFor() ([]attributionEntry, error) {
	m.mu.RLock()
	once := m.attributionOnce
	m.mu.RUnlock()

	once.Do(m.buildAttribution)

	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.attributionEntries, m.attributionErr
}
