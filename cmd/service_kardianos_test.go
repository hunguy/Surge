//go:build !android

package cmd

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/kardianos/service"
	"github.com/stretchr/testify/assert"
)

type mockService struct {
	service.Service
	platform        string
	startCalled     bool
	stopCalled      bool
	installCalled   bool
	uninstallCalled bool
	status          service.Status
	statusErr       error
	startFunc       func() error
	installFunc     func() error
	uninstallFunc   func() error
}

func (m *mockService) Platform() string { return m.platform }

func (m *mockService) Start() error {
	m.startCalled = true
	if m.startFunc != nil {
		return m.startFunc()
	}
	return nil
}

func (m *mockService) Stop() error {
	m.stopCalled = true
	return nil
}

func (m *mockService) Install() error {
	m.installCalled = true
	if m.installFunc != nil {
		return m.installFunc()
	}
	return nil
}

func (m *mockService) Uninstall() error {
	m.uninstallCalled = true
	if m.uninstallFunc != nil {
		return m.uninstallFunc()
	}
	return nil
}

func (m *mockService) Status() (service.Status, error) {
	return m.status, m.statusErr
}

// stubRunSystemctl replaces the systemctl runner for the duration of a test.
func stubRunSystemctl(t *testing.T, fn func(args ...string) (string, error)) {
	t.Helper()
	orig := runSystemctl
	t.Cleanup(func() { runSystemctl = orig })
	runSystemctl = fn
}

func waitStop(t *testing.T, p *program, s service.Service) {
	stopErr := make(chan error, 1)
	go func() {
		stopErr <- p.Stop(s)
	}()

	select {
	case err := <-stopErr:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		fmt.Printf("GOROUTINE DUMP:\n%s\n", buf[:n])
		t.Fatal("p.Stop timed out")
	}
}

func TestProgramLifecycle(t *testing.T) {
	setupIsolatedCmdState(t)

	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Skipf("Skipping test because tcp listener cannot bind (e.g. Windows dual-stack provider issue): %v", err)
	}
	_ = ln.Close()

	p := &program{}
	s := &mockService{}

	// Set args to something safe so rootCmd.ExecuteContext doesn't fail on test flags
	rootCmd.SetArgs([]string{"--help"})
	resetGlobalShutdownCoordinatorForTest(nil)
	defer func() {
		rootCmd.SetArgs(nil)
		resetGlobalShutdownCoordinatorForTest(nil)
	}()

	// Test Start
	err = p.Start(s)
	assert.NoError(t, err)
	assert.NotNil(t, p.cancel)
	assert.NotNil(t, p.exit)

	// Test Stop with timeout
	waitStop(t, p, s)

	// Verify p.exit is closed
	_, ok := <-p.exit
	assert.False(t, ok, "p.exit should be closed")
}

func TestToggleServiceFunc(t *testing.T) {
	s := &mockService{}

	toggleFunc := func(enable bool) error {
		if enable {
			return s.Install()
		}
		_ = s.Stop()
		return s.Uninstall()
	}

	err := toggleFunc(true)
	assert.NoError(t, err)
	assert.True(t, s.installCalled)

	err = toggleFunc(false)
	assert.NoError(t, err)
	assert.True(t, s.stopCalled)
	assert.True(t, s.uninstallCalled)
}

// withStubbedService swaps the package-level service factory and start
// timeout for the duration of a test. Tests calling this must not use
// t.Parallel(), because GetService and serviceStartTimeout are package-scoped.
func withStubbedService(t *testing.T, s service.Service, timeout time.Duration) {
	t.Helper()
	origGetService := GetService
	origTimeout := serviceStartTimeout
	origStability := systemdStartStability
	t.Cleanup(func() {
		GetService = origGetService
		serviceStartTimeout = origTimeout
		systemdStartStability = origStability
	})
	GetService = func() (service.Service, error) { return s, nil }
	serviceStartTimeout = timeout
	// Skip the real-world stability window so tests don't sleep.
	systemdStartStability = 0
}

func TestServiceStartCmdFailsWhenServiceNotRunning(t *testing.T) {
	withStubbedService(t, &mockService{status: service.StatusStopped}, 0)

	var runErr error
	out := captureStdout(t, func() {
		runErr = serviceStartCmd.RunE(serviceStartCmd, nil)
	})

	assert.Error(t, runErr, "start must not report success when the service is not running")
	assert.Contains(t, runErr.Error(), "did not start")
	assert.NotContains(t, out, "Service started successfully")
}

func TestServiceStartCmdSucceedsWhenRunning(t *testing.T) {
	withStubbedService(t, &mockService{status: service.StatusRunning}, 0)

	var runErr error
	out := captureStdout(t, func() {
		runErr = serviceStartCmd.RunE(serviceStartCmd, nil)
	})

	assert.NoError(t, runErr)
	assert.Contains(t, out, "Service started successfully")
}

func TestServiceInstallCmdRefreshesExistingUnit(t *testing.T) {
	setupIsolatedCmdState(t)

	installCalls := 0
	s := &mockService{status: service.StatusStopped}
	s.installFunc = func() error {
		installCalls++
		if installCalls == 1 {
			return fmt.Errorf("Init already exists: /etc/systemd/system/surge.service")
		}
		return nil
	}
	withStubbedService(t, s, 0)

	var runErr error
	out := captureStdout(t, func() {
		runErr = serviceInstallCmd.RunE(serviceInstallCmd, nil)
	})

	assert.NoError(t, runErr)
	assert.Equal(t, 2, installCalls, "install should be retried after uninstall")
	assert.True(t, s.uninstallCalled, "existing unit should be removed before re-installing")
	assert.True(t, s.stopCalled, "service should be stopped before refresh")
	assert.Contains(t, out, "Service installed successfully")
	assert.Contains(t, out, "Refreshed the existing unit")
}

func TestServiceInstallCmdPropagatesErrorWhenNotInstalled(t *testing.T) {
	setupIsolatedCmdState(t)

	s := &mockService{status: service.StatusUnknown, statusErr: service.ErrNotInstalled}
	s.installFunc = func() error { return fmt.Errorf("permission denied") }
	withStubbedService(t, s, 0)

	var runErr error
	_ = captureStdout(t, func() {
		runErr = serviceInstallCmd.RunE(serviceInstallCmd, nil)
	})

	assert.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "permission denied")
	assert.False(t, s.uninstallCalled, "must not refresh when nothing is installed")
}

func TestRunServiceActionSurfacesSystemctlError(t *testing.T) {
	s := &mockService{platform: linuxSystemdPlatform}
	stubRunSystemctl(t, func(args ...string) (string, error) {
		return "Failed to start surge.service: Interactive authentication required.\n", fmt.Errorf("exit status 1")
	})

	err := runServiceAction(s, "start")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Interactive authentication required")
	assert.False(t, s.startCalled, "must not fall back to kardianos once systemctl ran")
}

func TestRunServiceActionFallsBackOffSystemd(t *testing.T) {
	s := &mockService{platform: "windows"}
	stubRunSystemctl(t, func(args ...string) (string, error) {
		t.Fatalf("systemctl must not be called on platform %q", s.platform)
		return "", nil
	})

	assert.NoError(t, runServiceAction(s, "start"))
	assert.True(t, s.startCalled)
}

// stubDarwinSystemDaemon registers a system LaunchDaemon plist in a temporary
// directory and simulates an unprivileged user. Tests calling this must not use
// t.Parallel(), because darwinSystemDaemonDir and isElevatedFunc are
// package-scoped.
func stubDarwinSystemDaemon(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	origDir := darwinSystemDaemonDir
	origElevated := isElevatedFunc
	t.Cleanup(func() {
		darwinSystemDaemonDir = origDir
		isElevatedFunc = origElevated
	})
	darwinSystemDaemonDir = dir
	isElevatedFunc = func() bool { return false }
	path := filepath.Join(dir, serviceConfig.Name+".plist")
	if err := os.WriteFile(path, []byte("plist"), 0o644); err != nil {
		t.Fatalf("write plist: %v", err)
	}
	return path
}

func TestRunServiceActionExplainsDarwinNotInstalled(t *testing.T) {
	s := &mockService{
		platform:  darwinLaunchdPlatform,
		status:    service.StatusUnknown,
		statusErr: service.ErrNotInstalled,
	}

	err := runServiceAction(s, "start")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not registered as a service")
	assert.Contains(t, err.Error(), "service install")
	assert.False(t, s.startCalled, "must not invoke launchctl when no unit is installed")
}

func TestRunServiceActionExplainsDarwinLaunchDaemonNeedsRoot(t *testing.T) {
	stubDarwinSystemDaemon(t)

	s := &mockService{platform: darwinLaunchdPlatform, status: service.StatusStopped}
	s.startFunc = func() error {
		return fmt.Errorf("\"launchctl\" failed with stderr: Load failed: 5: Input/output error")
	}

	err := runServiceAction(s, "start")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "system LaunchDaemon")
	assert.Contains(t, err.Error(), "sudo")
	assert.Contains(t, err.Error(), "service start")
	assert.Contains(t, err.Error(), "Input/output error", "the launchctl error should be preserved")
}

func TestExplainDarwinRootRequiredUsesActionName(t *testing.T) {
	stubDarwinSystemDaemon(t)

	hint := explainDarwinRootRequired(&mockService{platform: darwinLaunchdPlatform}, "stop")
	assert.Contains(t, hint, "service stop")
}

func TestExplainDarwinRootRequiredSkipsWhenRoot(t *testing.T) {
	stubDarwinSystemDaemon(t)
	isElevatedFunc = func() bool { return true }

	assert.Empty(t, explainDarwinRootRequired(&mockService{platform: darwinLaunchdPlatform}, "start"))
}

func TestExplainDarwinRootRequiredSkipsWithoutDaemonAndOtherPlatforms(t *testing.T) {
	// Simulate an unprivileged user with no LaunchDaemon plist on disk.
	origDir := darwinSystemDaemonDir
	origElevated := isElevatedFunc
	t.Cleanup(func() {
		darwinSystemDaemonDir = origDir
		isElevatedFunc = origElevated
	})
	darwinSystemDaemonDir = t.TempDir()
	isElevatedFunc = func() bool { return false }

	assert.Empty(t, explainDarwinRootRequired(&mockService{platform: darwinLaunchdPlatform}, "start"))
	assert.Empty(t, explainDarwinRootRequired(&mockService{platform: linuxSystemdPlatform}, "start"))
	assert.Empty(t, explainDarwinNotInstalled(&mockService{platform: linuxSystemdPlatform}))
}

func TestServiceStartCmdExplainsDarwinLaunchDaemonNeedsRoot(t *testing.T) {
	stubDarwinSystemDaemon(t)

	s := &mockService{platform: darwinLaunchdPlatform, status: service.StatusStopped}
	s.startFunc = func() error {
		return fmt.Errorf("\"launchctl\" failed with stderr: Load failed: 5: Input/output error")
	}
	withStubbedService(t, s, 0)

	var runErr error
	out := captureStdout(t, func() {
		runErr = serviceStartCmd.RunE(serviceStartCmd, nil)
	})

	assert.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "sudo")
	assert.NotContains(t, out, "Service started successfully")
}

func TestServiceStartCmdLeavesLinuxBehaviorAlone(t *testing.T) {
	s := &mockService{
		platform:  linuxSystemdPlatform,
		status:    service.StatusUnknown,
		statusErr: service.ErrNotInstalled,
	}
	withStubbedService(t, s, 0)
	stubRunSystemctl(t, func(args ...string) (string, error) {
		return "Failed to start surge.service: Unit surge.service not found.\n", fmt.Errorf("exit status 1")
	})

	var runErr error
	_ = captureStdout(t, func() {
		runErr = serviceStartCmd.RunE(serviceStartCmd, nil)
	})

	assert.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "systemctl start", "systemd must keep reporting its own error")
	assert.NotContains(t, runErr.Error(), "not registered as a service")
}

func TestExplainServiceNotRunningReportsUnmetCondition(t *testing.T) {
	s := &mockService{platform: linuxSystemdPlatform}
	stubRunSystemctl(t, func(args ...string) (string, error) {
		switch args[0] {
		case "show":
			return "ConditionResult=no\n", nil
		case "status":
			return "  Active: inactive (dead)\n" +
				"  Condition: start condition unmet at Mon 2026-10-12 11:11:07 PDT\n" +
				"             └─ ConditionFileIsExecutable=/old/path/surge was not met\n", nil
		default:
			return "", nil
		}
	})

	note := explainServiceNotRunning(s)
	assert.Contains(t, note, "start condition was not met")
	assert.Contains(t, note, "ConditionFileIsExecutable=/old/path/surge was not met")
	assert.Contains(t, note, "surge service install")
}

func TestExplainServiceNotRunningEmptyWhenConditionMet(t *testing.T) {
	s := &mockService{platform: linuxSystemdPlatform}
	stubRunSystemctl(t, func(args ...string) (string, error) {
		return "ConditionResult=yes\n", nil
	})

	assert.Empty(t, explainServiceNotRunning(s))
}

func TestServiceStatusCmdExplainsUnmetCondition(t *testing.T) {
	s := &mockService{platform: linuxSystemdPlatform, status: service.StatusStopped}
	withStubbedService(t, s, 0)
	stubRunSystemctl(t, func(args ...string) (string, error) {
		if args[0] == "show" {
			return "ConditionResult=no\n", nil
		}
		return "  Condition: start condition unmet\n" +
			"             └─ ConditionFileIsExecutable=/old/surge was not met\n", nil
	})

	var runErr error
	out := captureStdout(t, func() {
		runErr = serviceStatusCmd.RunE(serviceStatusCmd, nil)
	})

	assert.NoError(t, runErr)
	assert.Contains(t, out, "Service is stopped")
	assert.Contains(t, out, "start condition was not met")
}

func TestSystemdUnitFailureReasonAutoRestart(t *testing.T) {
	s := &mockService{platform: linuxSystemdPlatform}
	stubRunSystemctl(t, func(args ...string) (string, error) {
		return "ActiveState=activating\nSubState=auto-restart\nExecMainStatus=203\n", nil
	})

	reason := systemdUnitFailureReason(s)
	assert.Contains(t, reason, "repeatedly restarting")
	assert.Contains(t, reason, "last exit status 203")
}

func TestSystemdUnitFailureReasonFailed(t *testing.T) {
	s := &mockService{platform: linuxSystemdPlatform}
	stubRunSystemctl(t, func(args ...string) (string, error) {
		return "ActiveState=failed\nSubState=failed\nExecMainStatus=1\n", nil
	})

	reason := systemdUnitFailureReason(s)
	assert.Contains(t, reason, "as failed")
	assert.Contains(t, reason, "last exit status 1")
}

func TestSystemdUnitFailureReasonHealthy(t *testing.T) {
	s := &mockService{platform: linuxSystemdPlatform}
	stubRunSystemctl(t, func(args ...string) (string, error) {
		return "ActiveState=active\nSubState=running\nExecMainStatus=0\n", nil
	})

	assert.Empty(t, systemdUnitFailureReason(s))
}

func TestSystemdUnitFailureReasonSkipsNonSystemd(t *testing.T) {
	s := &mockService{platform: "darwin-launchd"}
	stubRunSystemctl(t, func(args ...string) (string, error) {
		t.Fatalf("systemctl must not be called on platform %q", s.platform)
		return "", nil
	})

	assert.Empty(t, systemdUnitFailureReason(s))
}

func TestServiceStatusCmdReportsCrashLoop(t *testing.T) {
	s := &mockService{platform: linuxSystemdPlatform, status: service.StatusRunning}
	withStubbedService(t, s, 0)
	stubRunSystemctl(t, func(args ...string) (string, error) {
		if args[0] == "show" {
			return "ActiveState=activating\nSubState=auto-restart\nExecMainStatus=203\n", nil
		}
		return "", nil
	})

	var runErr error
	out := captureStdout(t, func() {
		runErr = serviceStatusCmd.RunE(serviceStatusCmd, nil)
	})

	assert.NoError(t, runErr)
	assert.Contains(t, out, "Service is not running")
	assert.NotContains(t, out, "Service is running")
}

func TestServiceStartCmdSucceedsOnHealthySystemd(t *testing.T) {
	s := &mockService{platform: linuxSystemdPlatform, status: service.StatusRunning}
	withStubbedService(t, s, 0)
	stubRunSystemctl(t, func(args ...string) (string, error) {
		if args[0] == "show" {
			return "ActiveState=active\nSubState=running\nExecMainStatus=0\nConditionResult=yes\n", nil
		}
		return "", nil
	})

	var runErr error
	out := captureStdout(t, func() {
		runErr = serviceStartCmd.RunE(serviceStartCmd, nil)
	})

	assert.NoError(t, runErr)
	assert.Contains(t, out, "Service started successfully")
}

func TestServiceStatusCmdHandlesFailedStateError(t *testing.T) {
	s := &mockService{
		platform:  linuxSystemdPlatform,
		status:    service.StatusUnknown,
		statusErr: fmt.Errorf("service in failed state"),
	}
	withStubbedService(t, s, 0)
	stubRunSystemctl(t, func(args ...string) (string, error) {
		if args[0] == "show" {
			return "ActiveState=failed\nSubState=failed\nExecMainStatus=1\n", nil
		}
		return "", nil
	})

	var runErr error
	out := captureStdout(t, func() {
		runErr = serviceStatusCmd.RunE(serviceStatusCmd, nil)
	})

	assert.NoError(t, runErr, "a failed unit should be reported, not surfaced as a raw error")
	assert.Contains(t, out, "Service is not running")
	assert.Contains(t, out, "as failed")
}

func TestServiceStartCmdFailsOnCrashLoop(t *testing.T) {
	s := &mockService{platform: linuxSystemdPlatform, status: service.StatusRunning}
	withStubbedService(t, s, 0)
	stubRunSystemctl(t, func(args ...string) (string, error) {
		if args[0] == "show" {
			return "ActiveState=activating\nSubState=auto-restart\nExecMainStatus=203\n", nil
		}
		return "", nil // systemctl start succeeds
	})

	var runErr error
	out := captureStdout(t, func() {
		runErr = serviceStartCmd.RunE(serviceStartCmd, nil)
	})

	assert.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "service failed to start")
	assert.NotContains(t, out, "Service started successfully")
}

func TestProgramContextCancellation(t *testing.T) {
	setupIsolatedCmdState(t)

	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Skipf("Skipping test because tcp listener cannot bind (e.g. Windows dual-stack provider issue): %v", err)
	}
	_ = ln.Close()

	p := &program{}
	s := &mockService{}

	rootCmd.SetArgs([]string{"--help"})
	resetGlobalShutdownCoordinatorForTest(nil)
	defer func() {
		rootCmd.SetArgs(nil)
		resetGlobalShutdownCoordinatorForTest(nil)
	}()

	_ = p.Start(s)

	cancel := p.cancel
	assert.NotNil(t, cancel)

	waitStop(t, p, s)
}
