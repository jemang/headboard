package sshsync

// Protocol is bumped when the agent and Headboard stop understanding each
// other's JSON. Agents refuse a sync with a different protocol.
const Protocol = 1

// Target is what one agent must hold on one machine.
type Target struct {
	Ref      string `json:"ref"`
	ExpectIP string `json:"expect_ip,omitempty"`
	// ExpectLabel is the hostname the container had when it was approved. A
	// container recreated under the same id and IP shows up as a new hostname.
	ExpectLabel string              `json:"expect_label,omitempty"`
	Users       map[string][]string `json:"users"`
	Clear       []string            `json:"clear"`
}

type SyncResponse struct {
	Rev      string   `json:"rev"`
	Protocol int      `json:"protocol"`
	Targets  []Target `json:"targets"`
}

// Discovered is what an agent found on its host. Guest-reported fields are
// hints for suggesting a binding, never an identity.
type Discovered struct {
	Ref      string   `json:"ref" maxLength:"16"`
	Hostname string   `json:"hostname" maxLength:"253"`
	IPs      []string `json:"ips,omitempty" maxItems:"32"`
}

type Pair struct {
	Ref      string `json:"ref"`
	UnixUser string `json:"unix_user"`
}

type Warning struct {
	Ref      string `json:"ref" maxLength:"16"`
	UnixUser string `json:"unix_user,omitempty" maxLength:"32"`
	// The agent cuts container stderr to 512 bytes; 2048 leaves room for the
	// text around it.
	Message string `json:"message" maxLength:"2048"`
}

type Report struct {
	Mode       string       `json:"mode" enum:"proxmox,local"`
	Version    string       `json:"version" maxLength:"64"`
	Protocol   int          `json:"protocol"`
	Discovered []Discovered `json:"discovered,omitempty" maxItems:"500"`
	Applied    []Pair       `json:"applied,omitempty" maxItems:"5000"`
	Cleared    []Pair       `json:"cleared,omitempty" maxItems:"5000"`
	Mismatched []string     `json:"mismatched,omitempty" maxItems:"500"`
	Warnings   []Warning    `json:"warnings,omitempty" maxItems:"500"`
}
