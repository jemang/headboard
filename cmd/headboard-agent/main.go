// Command headboard-agent writes the SSH keys Headboard assigns into
// authorized_keys on this host's Proxmox containers, or on this machine.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/jemang/headboard/internal/agent"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "/etc/headboard-agent/config", "agent config written by the installer")
	once := flag.Bool("once", false, "run one full sync and exit")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: headboard-agent [flags] [update | uninstall [--force] | version]\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	switch flag.Arg(0) {
	case "":
	case "version":
		fmt.Println("headboard-agent", version)

		return
	case "update":
		exitOn(update(*configPath))

		return
	case "uninstall":
		sub := flag.NewFlagSet("uninstall", flag.ExitOnError)
		force := sub.Bool("force", false, "remove this agent without clearing the keys it wrote or telling Headboard")
		_ = sub.Parse(flag.Args()[1:])
		exitOn(uninstall(*configPath, *force))

		return
	default:
		flag.Usage()
		os.Exit(2)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	cfg, err := agent.LoadConfig(*configPath)
	if err == nil {
		err = cfg.Validate()
	}
	if err != nil {
		log.Error("bad configuration", "path", *configPath, "err", err)
		os.Exit(1)
	}

	host := agent.DetectHost(agent.ExecRunner{})
	loop := &agent.Loop{
		Client:    agent.NewClient(cfg.URL, cfg.Token, nil),
		Host:      host,
		Apply:     agent.Apply,
		Version:   version,
		Log:       log,
		Interval:  30 * time.Second,
		FullEvery: time.Hour,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("headboard-agent starting", "version", version, "mode", host.Mode(), "headboard", cfg.URL)

	if *once {
		if _, err := loop.Once(ctx, "", true); err != nil {
			log.Error("sync failed", "err", err)
			os.Exit(1)
		}

		return
	}

	if err := loop.Run(ctx); err != nil {
		os.Exit(1)
	}
}

func exitOn(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "headboard-agent:", err)
		os.Exit(1)
	}
}

func run(argv ...string) error {
	out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", strings.Join(argv, " "), err, strings.TrimSpace(string(out)))
	}

	return nil
}

// update fetches the agent from the Headboard in the config, so no token or
// URL has to be typed again, then restarts the service onto it.
func update(configPath string) error {
	if os.Geteuid() != 0 {
		return errors.New("run as root")
	}

	cfg, err := agent.LoadConfig(configPath)
	if err == nil {
		err = cfg.Validate()
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", configPath, err)
	}

	exe, err := updateTarget()
	if err != nil {
		return err
	}

	var got string
	check := func(path string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		out, err := exec.CommandContext(ctx, path, "version").Output()
		got = strings.TrimSpace(string(out))
		if err != nil || !strings.HasPrefix(got, "headboard-agent ") {
			return fmt.Errorf("it does not run here (%q, %v)", got, err)
		}

		// Binaries are not signed yet, so at least a server cannot roll this
		// host back to an older build.
		if offered := strings.TrimPrefix(got, "headboard-agent "); agent.IsDowngrade(version, offered) {
			return fmt.Errorf("refusing to go from %s back to %s", version, offered)
		}

		return nil
	}

	client := agent.NewClient(cfg.URL, cfg.Token, &http.Client{Timeout: 5 * time.Minute})

	changed, err := agent.Update(context.Background(), client, runtime.GOARCH, exe, check)
	if err != nil {
		return err
	}
	if !changed {
		fmt.Println("headboard-agent", version, "is already the version Headboard serves.")

		return nil
	}

	if err := run("systemctl", "restart", "headboard-agent"); err != nil {
		return fmt.Errorf("installed %s but could not restart the service: %w", got, err)
	}
	fmt.Printf("Updated %s -> %s and restarted the service.\n", version, got)

	return nil
}

// updateTarget is the binary the service runs. It is never os.Executable():
// update run from a copy elsewhere would replace the copy and restart the old
// service binary while reporting success.
func updateTarget() (string, error) {
	exe := agent.Installed.Exe
	if _, err := os.Stat(exe); err != nil {
		return "", fmt.Errorf("no agent installed at %s; run the installer from Headboard", exe)
	}

	return exe, nil
}

// uninstall first removes every block this agent wrote and has Headboard
// delete it, so nothing is left behind on either side: no keys on servers
// that Headboard has forgotten, no live token. Anything that stops that keeps
// the agent installed and running. --force skips it.
func uninstall(configPath string, force bool) error {
	if os.Geteuid() != 0 {
		return errors.New("run as root")
	}

	if !force {
		if err := retire(configPath); err != nil {
			return err
		}
	}

	paths := agent.Installed
	if filepath.Dir(configPath) != paths.Config {
		paths.ConfigFile = configPath
	}

	if err := agent.Uninstall(paths, run); err != nil {
		return err
	}

	if force {
		fmt.Println("headboard-agent removed. Keys it wrote are still in place, and Headboard still lists it: revoke it there and run the cleanup commands it shows.")
	} else {
		fmt.Println("headboard-agent removed, and Headboard no longer lists it.")
	}

	return nil
}

func retire(configPath string) error {
	cfg, err := agent.LoadConfig(configPath)
	if err == nil {
		err = cfg.Validate()
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w; nothing removed (use --force to remove anyway)", configPath, err)
	}

	// Stopped so the running agent cannot write a block back mid-way.
	_ = run("systemctl", "stop", "headboard-agent")

	client := agent.NewClient(cfg.URL, cfg.Token, nil)
	out, err := agent.Retire(context.Background(), client, agent.DetectHost(agent.ExecRunner{}))

	for _, w := range out.Warnings {
		fmt.Fprintf(os.Stderr, "  %s %s: %s\n", w.Ref, w.UnixUser, w.Message)
	}
	if len(out.Cleared) > 0 {
		fmt.Printf("Removed %d key block(s).\n", len(out.Cleared))
	}

	switch {
	case errors.Is(err, agent.ErrUnauthorized):
		err = errors.New("Headboard rejected this agent's token: it was revoked there already. Use --force, then run the cleanup commands Headboard showed")
	case err == nil && !out.Retired:
		for _, p := range out.Remaining {
			fmt.Fprintf(os.Stderr, "  still written: %s %s\n", p.Ref, p.UnixUser)
		}
		err = errors.New("some keys could not be removed (see above); the agent stays installed. Fix that and run uninstall again, or use --force")
	}
	if err != nil {
		_ = run("systemctl", "start", "headboard-agent")

		return err
	}

	return nil
}
