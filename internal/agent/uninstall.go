package agent

import (
	"errors"
	"os"
	"path/filepath"
)

// Paths are where install.sh puts the agent.
type Paths struct {
	Exe    string
	Config string
	Unit   string
	// ConfigFile is a config given with -config outside Config. Only that
	// file is removed, never the directory it sits in.
	ConfigFile string
}

var Installed = Paths{
	Exe:    "/usr/local/bin/headboard-agent",
	Config: "/etc/headboard-agent",
	Unit:   "/etc/systemd/system/headboard-agent.service",
}

// Uninstall stops the service and removes the binary, its token and the unit.
// Keys it already wrote stay: removing the agent is not revoking anyone's
// access. A service that is already stopped or missing does not stop the
// rest, so a half-removed agent can be finished off by running it again.
func Uninstall(p Paths, run func(argv ...string) error) error {
	_ = run("systemctl", "disable", "--now", "headboard-agent")

	// Temp files an interrupted update left next to the binary.
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(p.Exe), ".headboard-agent-*"))

	var errs []error
	for _, path := range []string{p.Unit, p.Config, p.Exe} {
		if err := os.RemoveAll(path); err != nil {
			errs = append(errs, err)
		}
	}

	// These come from a glob and from -config, so only plain files go: a
	// directory named by mistake is refused, never emptied.
	for _, path := range append(leftovers, p.ConfigFile) {
		if path == "" {
			continue
		}
		if info, err := os.Lstat(path); err == nil && info.IsDir() {
			errs = append(errs, errors.New(path+" is a directory; not removed"))

			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}

	if err := run("systemctl", "daemon-reload"); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}
