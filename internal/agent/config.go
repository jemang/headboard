// Package agent is headboard-agent: it pulls the SSH keys Headboard wants on
// this host's targets and writes them into authorized_keys.
package agent

import (
	"bufio"
	"errors"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	URL   string
	Token string
}

// LoadConfig reads the key=value file the installer writes.
func LoadConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()

	var c Config
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok {
			continue
		}

		switch k {
		case "url":
			c.URL = strings.TrimRight(v, "/")
		case "token":
			c.Token = v
		}
	}

	return c, sc.Err()
}

// Validate refuses anything but HTTPS: whoever can alter a sync response can
// plant a login key on every target.
func (c Config) Validate() error {
	u, err := url.Parse(c.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return errors.New("url must be an https:// Headboard address")
	}

	if !strings.HasPrefix(c.Token, "hbk_") {
		return errors.New("token must be an hbk_ agent token")
	}

	return nil
}
