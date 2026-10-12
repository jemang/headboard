package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/juanfont/headscale/hscontrol/types"

	"github.com/jemang/headboard/internal/acl"
	"github.com/jemang/headboard/internal/auth"
	"github.com/jemang/headboard/internal/sshsync"
	"github.com/jemang/headboard/internal/store"
	"github.com/jemang/headboard/internal/tailnet"
)

type SSHBindingView struct {
	AgentID       int64         `json:"agentId"`
	AgentName     string        `json:"agentName"`
	TargetRef     string        `json:"targetRef"`
	ApprovedIP    string        `json:"approvedIp"`
	ApprovedLabel string        `json:"approvedLabel"`
	State         sshsync.State `json:"state"`
	// Reapprovable is whether approving again would succeed now: the agent
	// reports this target at the server's current IP and nothing else does.
	Reapprovable bool `json:"reapprovable"`
}

type SSHSuggestion struct {
	AgentID   int64  `json:"agentId"`
	AgentName string `json:"agentName"`
	TargetRef string `json:"targetRef"`
	Hostname  string `json:"hostname"`
	Ambiguous bool   `json:"ambiguous"`
}

type SSHRowUser struct {
	ID      uint64 `json:"id"`
	Name    string `json:"name"`
	Keys    int    `json:"keys"`
	Missing bool   `json:"missing"`
}

type SSHRowView struct {
	UnixUser string       `json:"unixUser"`
	Users    []SSHRowUser `json:"users"`
	Warnings []string     `json:"warnings"`
}

type SSHServerView struct {
	Name          string          `json:"name"`
	IP            string          `json:"ip"`
	Binding       *SSHBindingView `json:"binding,omitempty"`
	Suggestions   []SSHSuggestion `json:"suggestions"`
	Rows          []SSHRowView    `json:"rows"`
	PendingClears []string        `json:"pendingClears"`
}

type SSHOrphanView struct {
	HostName  string   `json:"hostName"`
	UnixUsers []string `json:"unixUsers"`
	Bound     bool     `json:"bound"`
}

type SSHAccessView struct {
	Servers []SSHServerView `json:"servers"`
	Orphans []SSHOrphanView `json:"orphans"`
	// People is every Headscale user with how many SSH keys they hold, so the
	// UI can offer only people who could actually log in.
	People []SSHPerson `json:"people"`
}

// SSHPerson is a Headscale user as the SSH page sees them.
type SSHPerson struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
	Keys int    `json:"keys"`
}

// policyHosts reads the hosts map from the snapshot's policy text. An
// unparseable policy is an error, never an empty map: an empty map would
// revoke every server.
func policyHosts(snap *tailnet.Snapshot) (map[string]string, error) {
	doc, err := acl.Parse(snap.Policy.HuJSON)
	if err != nil {
		return nil, err
	}

	schema, err := doc.Schema()
	if err != nil {
		return nil, err
	}

	if schema.Hosts == nil {
		return map[string]string{}, nil
	}

	return schema.Hosts, nil
}

func liveUsers(users []types.User) map[uint64]string {
	out := make(map[uint64]string, len(users))
	for _, u := range users {
		out[uint64(u.ID)] = u.Name
	}

	return out
}

func toBindings(ts []store.SSHTarget) []sshsync.Binding {
	out := make([]sshsync.Binding, 0, len(ts))
	for _, t := range ts {
		out = append(out, sshsync.Binding{HostName: t.HostName, AgentID: t.AgentID, TargetRef: t.TargetRef, ApprovedIP: t.ApprovedIP, ApprovedLabel: t.ApprovedLabel, Suspended: t.Suspended})
	}

	return out
}

func toAssignments(as []store.SSHAssignment) []sshsync.Assignment {
	out := make([]sshsync.Assignment, 0, len(as))
	for _, a := range as {
		out = append(out, sshsync.Assignment{HostName: a.HostName, UnixUser: a.UnixUser, HeadscaleUserID: a.HeadscaleUserID})
	}

	return out
}

func toStoredKeys(ks []store.SSHKey) []sshsync.StoredKey {
	out := make([]sshsync.StoredKey, 0, len(ks))
	for _, k := range ks {
		out = append(out, sshsync.StoredKey{HeadscaleUserID: k.HeadscaleUserID, Line: k.PublicKey, Label: k.Label})
	}

	return out
}

func toManaged(ms []store.SSHManaged) []sshsync.Managed {
	out := make([]sshsync.Managed, 0, len(ms))
	for _, m := range ms {
		out = append(out, sshsync.Managed{AgentID: m.AgentID, TargetRef: m.TargetRef, UnixUser: m.UnixUser})
	}

	return out
}

// agentReport decodes the last report. It is untrusted telemetry: a bad one
// reads as an empty report rather than an error.
func agentReport(a store.SSHAgent) sshsync.Report {
	var r sshsync.Report
	if len(a.LastReport) > 0 {
		_ = json.Unmarshal(a.LastReport, &r)
	}

	return r
}

// agentStaleAfter is how long an agent's last report still counts. Agents poll
// every 30 s and back off to 5 min when Headboard is unreachable.
const agentStaleAfter = 6 * time.Minute

type sshState struct {
	now     time.Time
	servers []sshsync.Server
	users   map[uint64]string
	targets []store.SSHTarget
	assigns []store.SSHAssignment
	keys    []store.SSHKey
	agents  []store.SSHAgent
	managed []store.SSHManaged
}

func loadSSHState(ctx context.Context, st *store.Store, snap *tailnet.Snapshot) (sshState, error) {
	hosts, err := policyHosts(snap)
	if err != nil {
		return sshState{}, err
	}

	s := sshState{now: time.Now(), servers: sshsync.Servers(hosts), users: liveUsers(snap.Users)}

	d, err := st.SSHState(ctx)
	if err != nil {
		return sshState{}, err
	}

	s.targets, s.assigns, s.keys, s.agents, s.managed = d.Targets, d.Assignments, d.Keys, d.Agents, d.Managed

	return s, nil
}

// implausible reports a world that would revoke everything at once: no hosts
// while servers are bound, or no users while people are assigned. That is
// almost always a bad policy edit or a fresh Headscale database, not intent,
// so agents are told to wait rather than handed a mass revoke.
func (s sshState) implausible() error {
	switch {
	case len(s.servers) == 0 && len(s.targets) > 0:
		return errors.New("the policy has no hosts but servers are bound; fix the policy or unbind them")
	case len(s.users) == 0 && len(s.assigns) > 0:
		return errors.New("headscale reports no users but people are assigned")
	}

	return nil
}

func (s sshState) desired(agentID int64) []sshsync.Target {
	return sshsync.Desired(agentID, s.servers, s.users, toBindings(s.targets), toAssignments(s.assigns), toStoredKeys(s.keys), toManaged(s.managed))
}

func (s sshState) ips() map[string]string {
	out := make(map[string]string, len(s.servers))
	for _, sv := range s.servers {
		out[sv.Name] = sv.IP
	}

	return out
}

type discoveredAt struct {
	agent store.SSHAgent
	d     sshsync.Discovered
}

// online reports whether an agent's last report is recent enough to trust for
// suggestions; a long-dead agent's view of its containers is history.
func (s sshState) online(a store.SSHAgent) bool {
	return a.LastSeenAt != nil && s.now.Sub(*a.LastSeenAt) < agentStaleAfter
}

// reporting lists every discovered target, across online agents, that claims ip.
func (s sshState) reporting(ip string) []discoveredAt {
	var out []discoveredAt

	for _, a := range s.agents {
		if !s.online(a) {
			continue
		}

		for _, d := range agentReport(a).Discovered {
			if slices.Contains(d.IPs, ip) {
				out = append(out, discoveredAt{agent: a, d: d})
			}
		}
	}

	return out
}

func buildAccess(s sshState) SSHAccessView {
	ips := s.ips()

	agentName := map[int64]string{}
	warnings := map[int64][]sshsync.Warning{}
	for _, a := range s.agents {
		agentName[a.ID] = a.Name
		warnings[a.ID] = agentReport(a).Warnings
	}

	keyCount := map[uint64]int{}
	for _, k := range s.keys {
		keyCount[k.HeadscaleUserID]++
	}

	bindingFor := map[string]store.SSHTarget{}
	for _, t := range s.targets {
		bindingFor[t.HostName] = t
	}

	v := SSHAccessView{Servers: []SSHServerView{}, Orphans: []SSHOrphanView{}, People: []SSHPerson{}}
	for id, name := range s.users {
		v.People = append(v.People, SSHPerson{ID: id, Name: name, Keys: keyCount[id]})
	}
	slices.SortFunc(v.People, func(a, b SSHPerson) int { return strings.Compare(a.Name, b.Name) })

	for _, sv := range s.servers {
		view := SSHServerView{Name: sv.Name, IP: sv.IP, Suggestions: []SSHSuggestion{}, Rows: []SSHRowView{}, PendingClears: []string{}}

		t, bound := bindingFor[sv.Name]
		if bound {
			view.Binding = &SSHBindingView{
				AgentID: t.AgentID, AgentName: agentName[t.AgentID], TargetRef: t.TargetRef,
				ApprovedIP: t.ApprovedIP, ApprovedLabel: t.ApprovedLabel,
				State: sshsync.StateOf(toBindings([]store.SSHTarget{t})[0], ips),
			}
			if _, err := approvalCandidate(s, sv.Name, t.AgentID, t.TargetRef); err == nil {
				view.Binding.Reapprovable = true
			}

			// A suspended binding gets no clears, by design, but a removal
			// made meanwhile still has to show: ask what it would clear if it
			// were active again.
			wouldBe := s
			if view.Binding.State == sshsync.StateSuspended {
				wouldBe.targets = slices.Clone(s.targets)
				for i := range wouldBe.targets {
					if wouldBe.targets[i].HostName == t.HostName {
						wouldBe.targets[i].Suspended, wouldBe.targets[i].ApprovedIP = false, sv.IP
					}
				}
			}

			for _, dt := range wouldBe.desired(t.AgentID) {
				if dt.Ref == t.TargetRef {
					view.PendingClears = append(view.PendingClears, dt.Clear...)
				}
			}
		} else {
			claims := s.reporting(sv.IP)
			for _, c := range claims {
				view.Suggestions = append(view.Suggestions, SSHSuggestion{
					AgentID: c.agent.ID, AgentName: c.agent.Name, TargetRef: c.d.Ref,
					Hostname: c.d.Hostname, Ambiguous: len(claims) > 1,
				})
			}
		}

		rows := map[string]*SSHRowView{}
		var order []string
		for _, a := range s.assigns {
			if a.HostName != sv.Name {
				continue
			}

			r := rows[a.UnixUser]
			if r == nil {
				r = &SSHRowView{UnixUser: a.UnixUser, Users: []SSHRowUser{}, Warnings: []string{}}
				rows[a.UnixUser] = r
				order = append(order, a.UnixUser)

				if bound {
					for _, w := range warnings[t.AgentID] {
						if w.Ref == t.TargetRef && w.UnixUser == a.UnixUser {
							r.Warnings = append(r.Warnings, w.Message)
						}
					}
				}
			}

			name, ok := s.users[a.HeadscaleUserID]
			r.Users = append(r.Users, SSHRowUser{ID: a.HeadscaleUserID, Name: name, Keys: keyCount[a.HeadscaleUserID], Missing: !ok})
		}

		for _, u := range order {
			view.Rows = append(view.Rows, *rows[u])
		}

		v.Servers = append(v.Servers, view)
	}

	orphans := map[string]*SSHOrphanView{}
	for _, a := range s.assigns {
		if _, ok := ips[a.HostName]; ok {
			continue
		}

		o := orphans[a.HostName]
		if o == nil {
			o = &SSHOrphanView{HostName: a.HostName, UnixUsers: []string{}}
			orphans[a.HostName] = o
		}
		if !slices.Contains(o.UnixUsers, a.UnixUser) {
			o.UnixUsers = append(o.UnixUsers, a.UnixUser)
		}
	}
	for _, t := range s.targets {
		if _, ok := ips[t.HostName]; ok {
			continue
		}
		if orphans[t.HostName] == nil {
			orphans[t.HostName] = &SSHOrphanView{HostName: t.HostName, UnixUsers: []string{}}
		}
		orphans[t.HostName].Bound = true
	}
	for _, o := range orphans {
		v.Orphans = append(v.Orphans, *o)
	}
	sort.Slice(v.Orphans, func(i, j int) bool { return v.Orphans[i].HostName < v.Orphans[j].HostName })

	return v
}

// approvalCandidate checks that a binding is safe to create: the host is a
// server, the agent reported that target with the host's IP, and no other
// target anywhere claims the same IP.
func approvalCandidate(s sshState, host string, agentID int64, ref string) (sshsync.Discovered, error) {
	ip, ok := s.ips()[host]
	if !ok {
		return sshsync.Discovered{}, huma.Error404NotFound("no such server in the policy hosts")
	}

	if !sshsync.ValidRef(ref) {
		return sshsync.Discovered{}, huma.Error422UnprocessableEntity("target must be a Proxmox container id or self")
	}

	claims := s.reporting(ip)
	if len(claims) > 1 {
		return sshsync.Discovered{}, huma.Error409Conflict("more than one target reports this IP; resolve the duplicate first")
	}

	if len(claims) == 0 || claims[0].agent.ID != agentID || claims[0].d.Ref != ref {
		return sshsync.Discovered{}, huma.Error409Conflict("that agent has not reported this target with the server's IP")
	}

	return claims[0].d, nil
}

type sshAccessOutput struct {
	Body SSHAccessView
}

type setSSHRowInput struct {
	Host     string `path:"host"`
	UnixUser string `path:"unixUser"`
	Body     struct {
		UserIDs []uint64 `json:"userIds" maxItems:"500"`
	}
}

type approveSSHTargetInput struct {
	Host string `path:"host"`
	Body struct {
		AgentID   int64  `json:"agentId"`
		TargetRef string `json:"targetRef" minLength:"1" maxLength:"64"`
	}
}

type sshHostInput struct {
	Host string `path:"host"`
}

func init() {
	register(func(api huma.API, deps Deps) {
		state := func(ctx context.Context) (sshState, error) {
			snap, err := currentSnapshot(deps)
			if err != nil {
				return sshState{}, err
			}

			s, err := loadSSHState(ctx, deps.Store, snap)
			if err != nil {
				return sshState{}, huma.Error503ServiceUnavailable("could not read the policy hosts", err)
			}

			return s, nil
		}

		huma.Register(api, huma.Operation{
			OperationID: "getSSHAccess",
			Method:      http.MethodGet,
			Path:        "/api/ssh/access",
			Summary:     "Servers, bindings and who may log in as which unix user",
			Tags:        []string{"ssh"},
		}, func(ctx context.Context, _ *struct{}) (*sshAccessOutput, error) {
			if _, err := require(ctx, auth.CapManageSSH); err != nil {
				return nil, err
			}

			s, err := state(ctx)
			if err != nil {
				return nil, err
			}

			return &sshAccessOutput{Body: buildAccess(s)}, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "setSSHRow",
			Method:      http.MethodPut,
			Path:        "/api/ssh/access/{host}/{unixUser}",
			Summary:     "Replace who may log in as a unix user on a server",
			Tags:        []string{"ssh"},
		}, func(ctx context.Context, in *setSSHRowInput) (*struct{}, error) {
			p, err := requireSudo(ctx, auth.CapManageSSH)
			if err != nil {
				return nil, err
			}

			if !sshsync.ValidUnixUser(in.UnixUser) {
				return nil, huma.Error422UnprocessableEntity("unix user names must match ^[a-z_][a-z0-9_-]{0,31}$")
			}

			s, err := state(ctx)
			if err != nil {
				return nil, err
			}
			if _, ok := s.ips()[in.Host]; !ok {
				return nil, huma.Error404NotFound("no such server in the policy hosts")
			}

			ids := slices.Compact(slices.Sorted(slices.Values(in.Body.UserIDs)))
			for _, id := range ids {
				if _, ok := s.users[id]; !ok {
					return nil, huma.Error422UnprocessableEntity("one of the users no longer exists")
				}
			}

			if err := deps.Store.SetSSHRow(ctx, in.Host, in.UnixUser, ids); err != nil {
				return nil, statusFor(err, "could not save the row")
			}

			audit(ctx, deps, p, "ssh_access.set", "ssh_host", 0, in.Host, nil,
				map[string]any{"host": in.Host, "unixUser": in.UnixUser, "userIds": ids})

			return nil, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "approveSSHTarget",
			Method:      http.MethodPost,
			Path:        "/api/ssh/targets/{host}",
			Summary:     "Bind a server to the target an agent reported for it",
			Tags:        []string{"ssh"},
		}, func(ctx context.Context, in *approveSSHTargetInput) (*struct{}, error) {
			p, err := requireSudo(ctx, auth.CapManageSSH)
			if err != nil {
				return nil, err
			}

			s, err := state(ctx)
			if err != nil {
				return nil, err
			}

			d, err := approvalCandidate(s, in.Host, in.Body.AgentID, in.Body.TargetRef)
			if err != nil {
				return nil, err
			}

			t := store.SSHTarget{HostName: in.Host, AgentID: in.Body.AgentID, TargetRef: d.Ref, ApprovedIP: s.ips()[in.Host], ApprovedLabel: d.Hostname}
			err = deps.Store.ApproveSSHTarget(ctx, t, p.User.ID)
			if errors.Is(err, store.ErrTargetTaken) {
				return nil, huma.Error409Conflict(err.Error())
			}
			if err != nil {
				return nil, statusFor(err, "could not approve the binding")
			}

			audit(ctx, deps, p, "ssh_target.approve", "ssh_host", 0, in.Host, nil, t)

			return nil, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "unbindSSHTarget",
			Method:      http.MethodDelete,
			Path:        "/api/ssh/targets/{host}",
			Summary:     "Unbind a server; its keys are cleared on the next sync",
			Tags:        []string{"ssh"},
		}, func(ctx context.Context, in *sshHostInput) (*struct{}, error) {
			p, err := requireSudo(ctx, auth.CapManageSSH)
			if err != nil {
				return nil, err
			}

			if err := deps.Store.DeleteSSHTarget(ctx, in.Host); err != nil {
				return nil, statusFor(err, "could not unbind")
			}

			audit(ctx, deps, p, "ssh_target.unbind", "ssh_host", 0, in.Host, map[string]string{"host": in.Host}, nil)

			return nil, nil
		})

		huma.Register(api, huma.Operation{
			OperationID: "removeSSHOrphan",
			Method:      http.MethodDelete,
			Path:        "/api/ssh/access/{host}",
			Summary:     "Forget rows and binding of a host that left the policy",
			Tags:        []string{"ssh"},
		}, func(ctx context.Context, in *sshHostInput) (*struct{}, error) {
			p, err := requireSudo(ctx, auth.CapManageSSH)
			if err != nil {
				return nil, err
			}

			s, err := state(ctx)
			if err != nil {
				return nil, err
			}
			if _, ok := s.ips()[in.Host]; ok {
				return nil, huma.Error409Conflict("this host is still in the policy; remove its rows instead")
			}

			if err := deps.Store.DeleteSSHHost(ctx, in.Host); err != nil {
				return nil, statusFor(err, "could not remove the host")
			}

			audit(ctx, deps, p, "ssh_access.remove_orphan", "ssh_host", 0, in.Host, map[string]string{"host": in.Host}, nil)

			return nil, nil
		})
	})
}
