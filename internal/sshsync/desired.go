package sshsync

import "sort"

type Binding struct {
	HostName      string
	AgentID       int64
	TargetRef     string
	ApprovedIP    string
	ApprovedLabel string
	Suspended     bool
}

type Assignment struct {
	HostName        string
	UnixUser        string
	HeadscaleUserID uint64
}

type StoredKey struct {
	HeadscaleUserID uint64
	Line            string
	Label           string
}

// Managed is a block an agent reported writing.
type Managed struct {
	AgentID   int64
	TargetRef string
	UnixUser  string
}

type State string

const (
	StateActive    State = "active"
	StateSuspended State = "suspended"
	StateOrphan    State = "orphan"
)

// StateOf decides whether a binding is delivered. A changed IP suspends it
// rather than following the new address: whoever can edit hosts must not be
// able to move SSH access onto another machine.
func StateOf(b Binding, ips map[string]string) State {
	ip, ok := ips[b.HostName]

	switch {
	case !ok:
		return StateOrphan
	case b.Suspended || ip != b.ApprovedIP:
		return StateSuspended
	}

	return StateActive
}

// Desired is everything one agent must hold. users maps live Headscale user
// ids to names; an id missing from it is a deleted user whose keys stop being
// emitted. A managed block with no active row is returned as a clear, which
// is how every kind of removal reaches a server without trigger code.
func Desired(agentID int64, servers []Server, users map[uint64]string, bindings []Binding, assigns []Assignment, keys []StoredKey, managed []Managed) []Target {
	ips := make(map[string]string, len(servers))
	for _, s := range servers {
		ips[s.Name] = s.IP
	}

	lines := map[uint64][]string{}
	for _, k := range keys {
		if name, ok := users[k.HeadscaleUserID]; ok {
			lines[k.HeadscaleUserID] = append(lines[k.HeadscaleUserID], Line(k.Line, name, k.Label))
		}
	}

	targets := map[string]*Target{}
	frozen := map[string]bool{}

	for _, b := range bindings {
		if b.AgentID != agentID {
			continue
		}

		switch StateOf(b, ips) {
		case StateSuspended:
			frozen[b.TargetRef] = true
		case StateActive:
			t := &Target{Ref: b.TargetRef, ExpectIP: b.ApprovedIP, ExpectLabel: b.ApprovedLabel, Users: map[string][]string{}, Clear: []string{}}

			for _, a := range assigns {
				if a.HostName != b.HostName {
					continue
				}
				if _, ok := users[a.HeadscaleUserID]; !ok {
					continue
				}
				if t.Users[a.UnixUser] == nil {
					t.Users[a.UnixUser] = []string{}
				}

				t.Users[a.UnixUser] = append(t.Users[a.UnixUser], lines[a.HeadscaleUserID]...)
			}

			targets[b.TargetRef] = t
		}
	}

	for _, m := range managed {
		if m.AgentID != agentID || frozen[m.TargetRef] {
			continue
		}

		t := targets[m.TargetRef]
		if t == nil {
			t = &Target{Ref: m.TargetRef, Users: map[string][]string{}, Clear: []string{}}
			targets[m.TargetRef] = t
		}

		if _, keep := t.Users[m.UnixUser]; !keep {
			t.Clear = append(t.Clear, m.UnixUser)
		}
	}

	out := make([]Target, 0, len(targets))
	for _, t := range targets {
		sort.Strings(t.Clear)
		out = append(out, *t)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })

	return out
}
