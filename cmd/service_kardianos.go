//go:build !android

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

var serviceConfig = &service.Config{
	Name:        "surge",
	DisplayName: "Surge Download Manager",
	Description: "Blazing fast TUI download manager built in Go.",
	Arguments:   []string{"service", "__run"},
}

type program struct {
	exit   chan struct{}
	cancel context.CancelFunc
	errCh  chan error
}

func (p *program) Start(s service.Service) error {
	// We run rootCmd.Execute() directly in a goroutine rather than starting
	// a subprocess to ensure the service manager tracks the correct PID
	// and to allow for shared state/lifecycle management if needed.
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.exit = make(chan struct{})
	p.errCh = make(chan error, 1)

	go func() {
		defer close(p.exit)
		// Re-enter cobra as `server start` instead of replaying os.Args
		// (which would re-match __run and recurse into RunService). This
		// permanently overrides rootCmd's args for the rest of the process
		// lifetime, which is fine: the process is owned by the service
		// manager from here until shutdown.
		rootCmd.SetArgs([]string{"server", "start", "--is-system-service"})
		if err := rootCmd.ExecuteContext(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Service error: %v\n", err)
			p.errCh <- err
			// Notify the service manager that the service should stop.
			// Use a goroutine to avoid deadlock on Windows where s.Stop()
			// might wait for p.Stop() to return.
			go func() { _ = s.Stop() }()
		}
	}()
	return nil
}

func (p *program) Stop(s service.Service) error {
	// Gracefully stop the service by canceling the context.
	if p.cancel != nil {
		p.cancel()
	}
	if p.exit != nil {
		<-p.exit
	}

	// Return the error that caused the stop if any, so the service manager logs it.
	select {
	case err := <-p.errCh:
		return err
	default:
		return nil
	}
}

// GetService is a var so tests can swap in a mock without touching the OS
// service manager.  Tests that reassign this must NOT use t.Parallel()
// because the variable is package-scoped.
var GetService = func() (service.Service, error) {
	prg := &program{}
	return service.New(prg, serviceConfig)
}

// executablePath returns the absolute path of the running Surge binary. It is
// used in hints so users can elevate with a path sudo can resolve, since sudo's
// secure_path often excludes user directories such as ~/.local/bin.
func executablePath() string {
	if exe, err := os.Executable(); err == nil && exe != "" {
		return exe
	}
	if len(os.Args) > 0 && os.Args[0] != "" {
		return os.Args[0]
	}
	return "surge"
}

func runAction(action func(service.Service) error, successMsg string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		s, err := GetService()
		if err != nil {
			return err
		}
		if err := action(s); err != nil {
			return err
		}
		if successMsg != "" {
			fmt.Println(successMsg)
		}
		return nil
	}
}

// serviceStartTimeout bounds how long `service start` waits for the service
// manager to report the service as running. It is a variable so tests can
// shorten it.
var serviceStartTimeout = 3 * time.Second

const serviceStartPollInterval = 100 * time.Millisecond

// waitForServiceRunning polls the service manager until the service reports
// that it is running or the timeout elapses. It returns the last observed
// status so callers can explain why the start did not take effect.
func waitForServiceRunning(s service.Service, timeout time.Duration) service.Status {
	deadline := time.Now().Add(timeout)
	last := service.StatusUnknown
	for {
		if status, err := s.Status(); err == nil {
			last = status
			if status == service.StatusRunning {
				return status
			}
		}
		if !time.Now().Before(deadline) {
			return last
		}
		time.Sleep(serviceStartPollInterval)
	}
}

func serviceStatusLabel(status service.Status) string {
	switch status {
	case service.StatusRunning:
		return "running"
	case service.StatusStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// linuxSystemdPlatform is the platform name kardianos reports when systemd is
// managing the service on Linux.
const linuxSystemdPlatform = "linux-systemd"

// darwinLaunchdPlatform is the platform name kardianos reports when launchd is
// managing the service on macOS.
const darwinLaunchdPlatform = "darwin-launchd"

// darwinSystemDaemonDir is the directory launchd reads system LaunchDaemons
// from. Surge registers there (through kardianos) because serviceConfig does not
// opt into a per-user LaunchAgent. It is a variable so tests can point at a
// temporary directory.
var darwinSystemDaemonDir = "/Library/LaunchDaemons"

// isElevatedFunc reports whether the process has root privileges. It is a
// variable so tests can simulate an unprivileged user without changing the
// process uid.
var isElevatedFunc = isElevated

// darwinLaunchDaemonPlistPath is the launchd unit Surge installs on macOS.
func darwinLaunchDaemonPlistPath() string {
	return filepath.Join(darwinSystemDaemonDir, serviceConfig.Name+".plist")
}

// explainDarwinNotInstalled returns a message when no launchd unit is
// registered yet on macOS. launchctl reports a missing unit only as a cryptic
// "Load failed: 5: Input/output error", so name the fix instead. It returns ""
// when the service is installed or the platform is not macOS.
func explainDarwinNotInstalled(s service.Service) string {
	if s.Platform() != darwinLaunchdPlatform || serviceIsInstalled(s) {
		return ""
	}
	return fmt.Sprintf("Surge is not registered as a service yet; install it first with 'sudo %s service install'",
		executablePath())
}

// explainDarwinRootRequired explains why an unprivileged process cannot control
// a system LaunchDaemon. launchctl reports this as the same cryptic I/O error it
// uses for a missing unit and suggests the wrong fix, so name the real one. It
// returns "" when the hint does not apply.
func explainDarwinRootRequired(s service.Service, action string) string {
	if s.Platform() != darwinLaunchdPlatform || isElevatedFunc() {
		return ""
	}
	path := darwinLaunchDaemonPlistPath()
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return fmt.Sprintf(
		"Surge is registered as a system LaunchDaemon (%s), which launchctl only lets root control; "+
			"retry with elevated privileges, e.g. 'sudo %s service %s'",
		path, executablePath(), action)
}

// withDarwinRootHint attaches the root-privileges explanation to a failed
// launchd action, preserving the original error.
func withDarwinRootHint(s service.Service, action string, err error) error {
	if err == nil {
		return nil
	}
	if hint := explainDarwinRootRequired(s, action); hint != "" {
		return fmt.Errorf("%s: %w", hint, err)
	}
	return err
}

// runLaunchctl invokes launchctl and returns its combined output. It is a
// variable so tests can substitute a stub.
var runLaunchctl = func(args ...string) (string, error) {
	out, err := exec.Command("launchctl", args...).CombinedOutput()
	return string(out), err
}

// darwinServiceRunning reports whether launchd currently has the Surge daemon
// loaded and running. It asks `launchctl print` rather than going through
// kardianos because launchd only reports the system daemon's PID to an
// unprivileged caller through the modern print view; the legacy
// `launchctl list` view kardianos parses reports the same daemon as stopped.
//
// This matters for `service start`: macOS loads every plist in
// /Library/LaunchDaemons at boot and a KeepAlive daemon stays up, so a repeated
// start would otherwise reach `launchctl load`, which rejects an already-loaded
// unit with a cryptic "Load failed: 5: Input/output error".
func darwinServiceRunning(s service.Service) bool {
	if s.Platform() != darwinLaunchdPlatform {
		return false
	}
	out, err := runLaunchctl("print", "system/"+serviceConfig.Name)
	if err != nil {
		return false
	}
	return strings.Contains(out, "state = running")
}

// explainDarwinLoadFailed explains launchctl's generic "Load failed: 5:
// Input/output error". launchd returns that same error both for a plist it
// cannot process and for a unit it has already loaded, and its own suggestion
// ("try running launchctl bootstrap as root") misdirects users who are already
// root. It returns "" when the hint does not apply.
func explainDarwinLoadFailed(s service.Service, err error) string {
	if s.Platform() != darwinLaunchdPlatform || err == nil {
		return ""
	}
	if !strings.Contains(err.Error(), "Input/output error") {
		return ""
	}
	return fmt.Sprintf(
		"launchd rejected the unit with its generic 'Input/output error', which it also reports when the unit "+
			"is already loaded; check 'surge service status' first, then refresh the unit with 'sudo %s service "+
			"uninstall' followed by 'sudo %s service install'",
		executablePath(), executablePath())
}

// runSystemctl invokes systemctl and returns its combined output. It is a
// variable so tests can substitute a stub. Calling systemctl directly (rather
// than through kardianos) preserves stderr, so a failed action surfaces
// systemd's own message instead of a bare "exit status 1".
var runSystemctl = func(args ...string) (string, error) {
	out, err := exec.Command("systemctl", args...).CombinedOutput()
	return string(out), err
}

func systemdUnitName() string {
	return serviceConfig.Name + ".service"
}

// runServiceAction starts or stops the service. On systemd it shells out to
// systemctl so a failure keeps systemd's error message; other service managers
// fall back to the kardianos implementation.
func runServiceAction(s service.Service, action string) error {
	if note := explainDarwinNotInstalled(s); note != "" {
		return errors.New(note)
	}
	if s.Platform() != linuxSystemdPlatform {
		switch action {
		case "start":
			err := s.Start()
			if err == nil {
				return nil
			}
			// The root-privileges hint is the actionable fix when the process is
			// unprivileged, so it takes precedence over the launchd-specific one.
			if hint := explainDarwinRootRequired(s, action); hint != "" {
				return fmt.Errorf("%s: %w", hint, err)
			}
			if hint := explainDarwinLoadFailed(s, err); hint != "" {
				return fmt.Errorf("%s: %w", hint, err)
			}
			return err
		case "stop":
			return withDarwinRootHint(s, action, s.Stop())
		default:
			return fmt.Errorf("unsupported service action %q", action)
		}
	}
	unit := systemdUnitName()
	out, err := runSystemctl(action, unit)
	if err != nil {
		if msg := strings.TrimSpace(out); msg != "" {
			return fmt.Errorf("systemctl %s %s failed: %s", action, unit, msg)
		}
		return fmt.Errorf("systemctl %s %s failed: %w", action, unit, err)
	}
	return nil
}

// serviceIsInstalled reports whether the service is already registered with the
// OS service manager.
func serviceIsInstalled(s service.Service) bool {
	_, err := s.Status()
	return err == nil || !errors.Is(err, service.ErrNotInstalled)
}

// reinstallService removes and re-registers the service so an existing unit is
// refreshed, for example after the Surge binary moved on upgrade.
func reinstallService(s service.Service) error {
	// Best-effort stop: the Windows SCM refuses to remove a running service,
	// and a stale unit may still be holding a running process.
	_ = s.Stop()
	if err := s.Uninstall(); err != nil {
		return fmt.Errorf("refresh service: uninstall failed: %w", err)
	}
	if err := s.Install(); err != nil {
		return fmt.Errorf("refresh service: install failed: %w", err)
	}
	return nil
}

// explainServiceNotRunning returns a human-readable note when an installed
// systemd unit is not running because a start condition was not met (for
// example a stale ConditionFileIsExecutable after the binary moved). It returns
// "" when there is nothing useful to add or systemd is not the service manager.
func explainServiceNotRunning(s service.Service) string {
	if s.Platform() != linuxSystemdPlatform {
		return ""
	}
	unit := systemdUnitName()
	props, err := runSystemctl("show", unit, "--property=ConditionResult")
	if err != nil && strings.TrimSpace(props) == "" {
		return ""
	}
	if !strings.Contains(props, "ConditionResult=no") {
		return ""
	}
	msg := "The installed unit was skipped because a start condition was not met"
	if detail := systemdConditionDetail(unit); detail != "" {
		msg += " (" + detail + ")"
	}
	return msg + ". Re-register it with 'surge service install' to point at the current binary."
}

// systemdConditionDetail extracts the human-readable condition failure from
// `systemctl status`; newer systemd hides it from `systemctl show`.
func systemdConditionDetail(unit string) string {
	out, _ := runSystemctl("status", unit, "--no-pager", "-l")
	if out == "" {
		return ""
	}
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "Condition:") {
			continue
		}
		detail := strings.TrimSpace(strings.TrimPrefix(trimmed, "Condition:"))
		// The reason is indented on the following lines, e.g.
		//   Condition: start condition unmet at ...
		//              └─ ConditionFileIsExecutable=/old/path was not met
		for _, next := range lines[i+1:] {
			cont := strings.TrimSpace(next)
			if !strings.HasPrefix(cont, "\u2514\u2500") {
				break
			}
			detail += " " + strings.TrimSpace(strings.TrimPrefix(cont, "\u2514\u2500"))
		}
		return detail
	}
	return ""
}

// parseSystemctlProperties parses `systemctl show` output (KEY=VALUE lines).
func parseSystemctlProperties(out string) map[string]string {
	props := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		props[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return props
}

// systemdUnitProperties reads the systemd state used to judge service health.
// The bool is false when the query fails and produced no output.
func systemdUnitProperties() (map[string]string, bool) {
	out, err := runSystemctl("show", systemdUnitName(),
		"--property=ActiveState", "--property=SubState",
		"--property=ExecMainStatus", "--property=ConditionResult")
	if err != nil && strings.TrimSpace(out) == "" {
		return nil, false
	}
	return parseSystemctlProperties(out), true
}

// systemdFailureFromProps reports a terminal failure described by props, or ""
// when the unit is not in a failed or auto-restart state.
func systemdFailureFromProps(props map[string]string) string {
	switch {
	case props["ActiveState"] == "failed":
		return systemdFailureSummary("systemd reports the unit as failed", props)
	case props["SubState"] == "auto-restart":
		return systemdFailureSummary("systemd is repeatedly restarting it after a failed start", props)
	default:
		return ""
	}
}

// systemdUnitFailureReason returns a human-readable reason when systemd reports
// the unit is not actually working — crash-looping (auto-restart) or failed —
// even though kardianos reports StatusRunning because it maps systemd's
// "activating" state to running. It returns "" for non-systemd platforms and
// for a healthy unit.
func systemdUnitFailureReason(s service.Service) string {
	if s.Platform() != linuxSystemdPlatform {
		return ""
	}
	props, ok := systemdUnitProperties()
	if !ok {
		return ""
	}
	return systemdFailureFromProps(props)
}

// systemdStartStability is how long a systemd unit must stay active before a
// start counts as successful. It guards against Type=simple units that report
// "active" for a moment and then exit. It is a variable so tests can skip it.
var systemdStartStability = time.Second

// waitForSystemdHealthy waits until the systemd unit is stably active. It
// returns "" on success, or a failure reason when the unit fails, crash-loops,
// or never becomes active within the timeout.
func waitForSystemdHealthy(s service.Service, timeout, stability time.Duration) string {
	if s.Platform() != linuxSystemdPlatform {
		return ""
	}
	deadline := time.Now().Add(timeout)
	var activeSince time.Time
	var last map[string]string
	for {
		if props, ok := systemdUnitProperties(); ok {
			last = props
			if reason := systemdFailureFromProps(props); reason != "" {
				return reason
			}
			if props["ActiveState"] == "active" {
				if activeSince.IsZero() {
					activeSince = time.Now()
				}
				if time.Since(activeSince) >= stability {
					return ""
				}
			} else {
				activeSince = time.Time{}
			}
		}
		if !time.Now().Before(deadline) {
			if !activeSince.IsZero() {
				return ""
			}
			if note := explainServiceNotRunning(s); note != "" {
				return note
			}
			return systemdFailureSummary("systemd did not report the unit as running", last)
		}
		time.Sleep(serviceStartPollInterval)
	}
}

func systemdFailureSummary(prefix string, props map[string]string) string {
	msg := prefix
	if code := props["ExecMainStatus"]; code != "" && code != "0" {
		msg += fmt.Sprintf(" (last exit status %s)", code)
	}
	return msg + ". Run 'systemctl status " + systemdUnitName() + "' for details."
}

var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install Surge as a system service",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := GetService()
		if err != nil {
			return err
		}
		refreshed := false
		if err := s.Install(); err != nil {
			// kardianos refuses to overwrite an existing unit. When the service
			// is already registered, re-register it so the unit is refreshed
			// (e.g. after the binary moved on upgrade) instead of failing with a
			// cryptic "already exists" error. A genuine failure (service not
			// installed) is returned as-is so we don't mask it.
			if !serviceIsInstalled(s) {
				return err
			}
			if rerr := reinstallService(s); rerr != nil {
				return rerr
			}
			refreshed = true
		}
		// Pre-generate the service token in the system state directory so
		// that `surge token` / `surge connect` can discover it without users
		// having to use --token manually.
		token, tokenErr := ensureSystemToken()
		fmt.Println("Service installed successfully")
		if refreshed {
			fmt.Println("Refreshed the existing unit to use the current Surge executable.")
		}
		if tokenErr == nil {
			fmt.Printf("Service auth token: %s\n", token)
			fmt.Println("Use 'surge service token' to print this token again.")
		} else {
			fmt.Printf("Warning: could not persist service token: %v\n", tokenErr)
			fmt.Println("Run 'sudo surge service token' after starting the service to retrieve it.")
		}
		return nil
	},
}

var serviceUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the Surge system service",
	RunE: runAction(func(s service.Service) error {
		// Best effort stop before uninstall (Windows SCM rejects uninstall of running service)
		_ = s.Stop()
		return s.Uninstall()
	}, "Service uninstalled successfully"),
}

var serviceStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Surge system service",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := GetService()
		if err != nil {
			return err
		}
		// An already-running daemon needs no work (and, on macOS, no elevation).
		// Short-circuit before launchctl so the redundant load does not fail.
		if darwinServiceRunning(s) {
			fmt.Println("Service is already running")
			return nil
		}
		if err := runServiceAction(s, "start"); err != nil {
			return err
		}
		// Service managers can report success from Start() without the process
		// ever staying up. systemd silently skips a unit whose
		// ConditionFileIsExecutable no longer matches, maps a crash-looping
		// unit's "activating" state to running, and can report "active" for a
		// Type=simple unit that exits moments later. Verify it is actually up.
		if s.Platform() == linuxSystemdPlatform {
			if reason := waitForSystemdHealthy(s, serviceStartTimeout, systemdStartStability); reason != "" {
				return fmt.Errorf("service failed to start: %s", reason)
			}
		} else if status := waitForServiceRunning(s, serviceStartTimeout); status != service.StatusRunning {
			if note := explainServiceNotRunning(s); note != "" {
				return fmt.Errorf("service did not start (status: %s); %s", serviceStatusLabel(status), note)
			}
			return fmt.Errorf(
				"service did not start (status: %s); the installed unit may be stale, "+
					"for example pointing at an old binary path. Re-register it with "+
					"'surge service uninstall' followed by 'surge service install', "+
					"then confirm with 'surge service status'",
				serviceStatusLabel(status),
			)
		}
		fmt.Println("Service started successfully")
		return nil
	},
}

var serviceStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Surge system service",
	RunE:  runAction(func(s service.Service) error { return runServiceAction(s, "stop") }, "Service stopped successfully"),
}

// isSystemServiceRunning checks if the Kardianos service is currently running.
func isSystemServiceRunning() bool {
	s, err := GetService()
	if err != nil {
		return false
	}
	if darwinServiceRunning(s) {
		return true
	}
	status, err := s.Status()
	return err == nil && status == service.StatusRunning
}

var serviceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the Surge system service",
	RunE: runAction(func(s service.Service) error {
		status, err := s.Status()
		// launchd answers `launchctl print` truthfully for both privileged and
		// unprivileged callers, while kardianos' legacy `launchctl list` view
		// reports the system daemon as stopped to a non-root user. Trust launchd
		// when it says the daemon is up.
		if darwinServiceRunning(s) {
			status, err = service.StatusRunning, nil
		}
		if err != nil {
			// systemd surfaces a failed unit as a Status error rather than a
			// state; turn it into a readable diagnosis instead of the raw error.
			if reason := systemdUnitFailureReason(s); reason != "" {
				fmt.Printf("Service is not running: %s\n", reason)
				return nil
			}
			if note := explainServiceNotRunning(s); note != "" {
				fmt.Printf("Service is not running: %s\n", note)
				return nil
			}
			return err
		}
		switch status {
		case service.StatusRunning:
			// kardianos reports StatusRunning for systemd's "activating"
			// state, which also covers a unit stuck in auto-restart after a
			// crash. Report the truth instead of a stale "running".
			if reason := systemdUnitFailureReason(s); reason != "" {
				fmt.Printf("Service is not running: %s\n", reason)
				return nil
			}
			pid := readPIDFile(config.GetSystemRuntimeDir())
			port := readPortFile(config.GetSystemRuntimeDir())
			if pid > 0 && port > 0 {
				fmt.Printf("Service is running (PID: %d, Port: %d)\n", pid, port)
			} else if pid > 0 {
				fmt.Printf("Service is running (PID: %d)\n", pid)
			} else if port > 0 {
				fmt.Printf("Service is running (Port: %d)\n", port)
			} else {
				fmt.Println("Service is running")
			}
		case service.StatusStopped:
			fmt.Println("Service is stopped")
			if note := explainServiceNotRunning(s); note != "" {
				fmt.Println(note)
			}
		default:
			fmt.Println("Service is not installed or status is unknown")
		}
		return nil
	}, ""),
}

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage Surge as a system service",
}

// serviceTokenCmd prints the auth token used by the system service daemon.
var serviceTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Print the auth token used by the system service daemon",
	Long: `Print the auth token that the Surge system service uses.

The system service (installed with 'surge service install') stores its token
separately from the interactive-user token. Use this command to retrieve it
when connecting via 'surge connect' or setting up the browser extension.

The running daemon also mirrors the token to a world-readable path in the
system runtime directory, so elevated privileges are usually not required. If
the token still cannot be read, retry with sudo and the full path to the surge
binary.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		token, err := readSystemServiceToken()
		if err != nil {
			var hint string
			if !isElevated() {
				// sudo's secure_path often excludes user dirs like ~/.local/bin,
				// so suggest the absolute path to the running binary.
				hint = fmt.Sprintf(" (retry with elevated privileges, e.g. 'sudo %s service token')", executablePath())
			}
			return fmt.Errorf("could not read system service token%s: %w", hint, err)
		}
		fmt.Println(token)
		return nil
	},
}

// __run is the entry point the installer writes into ExecStart
var serviceRunCmd = &cobra.Command{
	Use:    "__run",
	Hidden: true,
	RunE:   func(cmd *cobra.Command, args []string) error { return RunService() },
}

func init() {
	rootCmd.AddCommand(serviceCmd)
	serviceCmd.AddCommand(serviceInstallCmd)
	serviceCmd.AddCommand(serviceUninstallCmd)
	serviceCmd.AddCommand(serviceStartCmd)
	serviceCmd.AddCommand(serviceStopCmd)
	serviceCmd.AddCommand(serviceStatusCmd)
	serviceCmd.AddCommand(serviceTokenCmd)
	serviceCmd.AddCommand(serviceRunCmd)
}
