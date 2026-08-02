package acl

import "encoding/json"

// Schema mirrors policyv2.Policy — the sections the form edits.
//
// It is a mirror rather than the type itself: policyv2.Policy unmarshals into
// resolved internal types (aliases, prefixes, owners) that cannot be round
// tripped back to the strings an operator wrote. The form needs the strings.
// Validation still goes through Headscale, so this being loose costs nothing.
type Schema struct {
	Groups    map[string][]string `json:"groups,omitempty"`
	Hosts     map[string]string   `json:"hosts,omitempty"`
	TagOwners map[string][]string `json:"tagOwners,omitempty"`

	ACLs   []Rule  `json:"acls,omitempty"`
	Grants []Grant `json:"grants,omitempty"`

	NodeAttrs     []NodeAttr     `json:"nodeAttrs,omitempty"`
	AutoApprovers *AutoApprovers `json:"autoApprovers,omitempty"`

	SSH []SSHRule `json:"ssh,omitempty"`

	Tests    []Test    `json:"tests,omitempty"`
	SSHTests []SSHTest `json:"sshTests,omitempty"`

	RandomizeClientPort bool `json:"randomizeClientPort,omitempty"`
}

// Rule is one entry in acls.
type Rule struct {
	Action string   `json:"action" enum:"accept"`
	Src    []string `json:"src"`
	Dst    []string `json:"dst"`

	// Proto narrows the rule to one protocol. Empty means all.
	Proto string `json:"proto,omitempty"`
}

// Grant is one entry in grants, the capability-based successor to acls.
type Grant struct {
	Src []string        `json:"src"`
	Dst []string        `json:"dst"`
	IP  []string        `json:"ip,omitempty"`
	App json.RawMessage `json:"app,omitempty"`
	Via []string        `json:"via,omitempty"`
}

// NodeAttr sets attributes on nodes rather than allowing traffic.
type NodeAttr struct {
	Target []string        `json:"target"`
	Attr   []string        `json:"attr,omitempty"`
	App    json.RawMessage `json:"app,omitempty"`
}

// AutoApprovers decides which routes get approved without an admin clicking.
//
// Tailscale's SaaS policy also carries a services map here. Headscale v0.29.3
// does not: policyv2.AutoApproverPolicy has only routes and exitNode, and
// unmarshalPolicy rejects unknown members, so a document containing services
// fails to compile with `unknown field: "services"` — the whole policy, not
// just that entry. Offering it in the form made every subsequent save fail.
type AutoApprovers struct {
	Routes   map[string][]string `json:"routes,omitempty"`
	ExitNode []string            `json:"exitNode,omitempty"`
}

// SSHRule is one entry in ssh.
type SSHRule struct {
	Action      string   `json:"action" enum:"accept,check"`
	Src         []string `json:"src"`
	Dst         []string `json:"dst"`
	Users       []string `json:"users"`
	CheckPeriod string   `json:"checkPeriod,omitempty"`

	// AcceptEnv allows environment variables through, which is worth
	// showing in the form because it widens what a session can carry.
	AcceptEnv []string `json:"acceptEnv,omitempty"`
}

// Test is one entry in tests: an assertion the policy must satisfy.
//
// Headscale refuses to store a policy whose tests fail, so these are not
// documentation — they are the write boundary.
type Test struct {
	Src string `json:"src"`

	// Proto restricts the assertion to one protocol. Headscale allows only
	// tcp, udp and sctp here, even though a rule may name others.
	Proto string `json:"proto,omitempty"`

	Accept []string `json:"accept,omitempty"`
	Deny   []string `json:"deny,omitempty"`
}

// SSHTest is one entry in sshTests. Its accept/deny/check arrays hold
// usernames rather than destinations: every listed user is asserted against
// every entry in Dst.
type SSHTest struct {
	Src    string   `json:"src"`
	Dst    []string `json:"dst"`
	Accept []string `json:"accept,omitempty"`
	Deny   []string `json:"deny,omitempty"`

	// Check demands a check-action rule specifically. An accept-action rule
	// reaching the same host does not satisfy it.
	Check []string `json:"check,omitempty"`
}

// Tokens lists every alias the form's pickers can offer, derived from the
// document plus the live tailnet.
//
// Offering only what actually resolves is the difference between a form and a
// textarea: a picker that suggests a group nobody defined produces a policy
// Headscale rejects at save time, which is exactly the Headplane experience
// this replaces.
type Tokens struct {
	Users      []string `json:"users"`
	Groups     []string `json:"groups"`
	Tags       []string `json:"tags"`
	Hosts      []string `json:"hosts"`
	AutoGroups []string `json:"autogroups"`
}

// autogroups are Headscale's built-ins. They are fixed by the policy language,
// not discovered, so they are listed rather than derived.
var autogroups = []string{
	"autogroup:internet",
	"autogroup:member",
	"autogroup:nonroot",
	"autogroup:self",
	"autogroup:tagged",
}

// TokensFor builds the picker vocabulary from this document and the tailnet's
// usernames.
func (s *Schema) TokensFor(usernames []string) Tokens {
	t := Tokens{
		Users:      make([]string, 0, len(usernames)),
		Groups:     make([]string, 0, len(s.Groups)),
		Hosts:      make([]string, 0, len(s.Hosts)),
		AutoGroups: autogroups,
	}

	// Policy v2 writes a user as "name@".
	for _, u := range usernames {
		t.Users = append(t.Users, u+"@")
	}

	for g := range s.Groups {
		t.Groups = append(t.Groups, g)
	}

	for h := range s.Hosts {
		t.Hosts = append(t.Hosts, h)
	}

	// Tags come from tagOwners: a tag with no owner cannot be applied to
	// anything, so offering it would be offering a dead end.
	t.Tags = make([]string, 0, len(s.TagOwners))
	for tag := range s.TagOwners {
		t.Tags = append(t.Tags, tag)
	}

	sortStrings(t.Users)
	sortStrings(t.Groups)
	sortStrings(t.Tags)
	sortStrings(t.Hosts)

	return t
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
