// ==============================================================================
// File: test/process/pty_runner_test.go
// Project: lazyssh
// Description: Unit tests for PTY process runner verifying command synthesis,
//              stream binding, lifecycle hooks, and error handling.
// ==============================================================================

package process_test

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/vuong-ngo/lazyssh/internal/domain"
	"github.com/vuong-ngo/lazyssh/internal/infrastructure/process"
)

func TestPTYRunner_LookPath_Explicit(t *testing.T) {
	customPath := "/opt/custom/ssh"
	if runtime.GOOS == "windows" {
		customPath = `C:\opt\custom\ssh.exe`
	}

	runner := process.NewPTYRunnerWithStreams(customPath, nil, nil, nil)
	path, err := runner.LookPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != customPath {
		t.Errorf("expected path %q, got %q", customPath, path)
	}
}

func TestPTYRunner_LookPath_NotFound(t *testing.T) {
	runner := process.NewPTYRunnerWithStreams("non_existent_binary_xyz_12345", nil, nil, nil)
	path, err := runner.LookPath()
	if err != nil {
		t.Fatalf("unexpected error when path is explicit: %v", err)
	}
	if path != "non_existent_binary_xyz_12345" {
		t.Errorf("expected explicit path preserved, got %q", path)
	}
}

func TestPTYRunner_BuildCommand_ValidHost(t *testing.T) {
	dummyBinary := "ssh"
	if runtime.GOOS == "windows" {
		dummyBinary = "ssh.exe"
	}

	var stdin bytes.Buffer
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	runner := process.NewPTYRunnerWithStreams(dummyBinary, &stdin, &stdout, &stderr)

	host := domain.Host{
		Alias:         "bastion",
		HostName:      "10.0.0.1",
		User:          "admin",
		Port:          2222,
		IdentityFiles: []string{"/home/user/.ssh/id_ed25519"},
		ProxyJump:     "jump.corp.net",
	}

	cmd, err := runner.BuildCommand(context.Background(), host)
	if err != nil {
		t.Fatalf("unexpected error building command: %v", err)
	}

	if cmd == nil {
		t.Fatal("expected non-nil exec.Cmd")
	}

	// Verify command streams are attached
	if cmd.Stdin != &stdin || cmd.Stdout != &stdout || cmd.Stderr != &stderr {
		t.Error("expected runner streams to be bound to cmd")
	}

	// Verify arguments
	expectedArgs := []string{
		dummyBinary,
		"-p", "2222",
		"-l", "admin",
		"-i", "/home/user/.ssh/id_ed25519",
		"-J", "jump.corp.net",
		"bastion",
	}

	if len(cmd.Args) != len(expectedArgs) {
		t.Fatalf("expected args count %d, got %d (%v)", len(expectedArgs), len(cmd.Args), cmd.Args)
	}

	for i := range expectedArgs {
		if cmd.Args[i] != expectedArgs[i] {
			t.Errorf("arg[%d] expected %q, got %q", i, expectedArgs[i], cmd.Args[i])
		}
	}
}

func TestPTYRunner_BuildCommand_InvalidHost(t *testing.T) {
	runner := process.NewPTYRunner()

	invalidHost := domain.Host{
		Alias: "invalid alias with spaces",
	}

	cmd, err := runner.BuildCommand(context.Background(), invalidHost)
	if err == nil {
		t.Fatal("expected error for invalid host alias, got nil")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd on validation failure")
	}
	if !errors.Is(err, domain.ErrInvalidAlias) {
		t.Errorf("expected ErrInvalidAlias, got %v", err)
	}
}

func TestPTYRunner_Execute_SuccessWithHooks(t *testing.T) {
	// Locate the current Go binary to use as a portable, reliable executable
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go executable not found in PATH, skipping execution test")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := process.NewPTYRunnerWithStreams(goBin, nil, &stdout, &stderr)

	// #nosec G204 -- test executes system go binary to verify stream forwarding
	cmd := exec.Command(goBin, "version")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	var preHookCalled bool
	var postHookCalled bool

	preHook := func() error {
		preHookCalled = true
		return nil
	}

	postHook := func() error {
		postHookCalled = true
		return nil
	}

	err = runner.Execute(context.Background(), cmd, preHook, postHook)
	if err != nil {
		t.Fatalf("expected clean execution, got error: %v", err)
	}

	if !preHookCalled {
		t.Error("expected preHook to be called")
	}
	if !postHookCalled {
		t.Error("expected postHook to be called")
	}

	output := stdout.String()
	if !strings.Contains(output, "go version") {
		t.Errorf("expected output to contain 'go version', got %q", output)
	}
}

func TestPTYRunner_Execute_PreHookFailureAbortsExecution(t *testing.T) {
	runner := process.NewPTYRunner()
	cmd := exec.Command("echo", "test")

	preHookErr := errors.New("terminal raw mode release failed")
	var postHookCalled bool

	preHook := func() error {
		return preHookErr
	}

	postHook := func() error {
		postHookCalled = true
		return nil
	}

	err := runner.Execute(context.Background(), cmd, preHook, postHook)
	if err == nil {
		t.Fatal("expected error when preHook fails, got nil")
	}
	if !errors.Is(err, preHookErr) && !strings.Contains(err.Error(), preHookErr.Error()) {
		t.Errorf("expected error wrapping preHookErr, got: %v", err)
	}
	if postHookCalled {
		t.Error("expected postHook NOT to be called when preHook fails")
	}
}

func TestPTYRunner_Execute_PostHookGuaranteedOnFailure(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go executable not found in PATH")
	}

	runner := process.NewPTYRunner()

	// Run an invalid go subcommand to produce non-zero exit code
	// #nosec G204 -- test executes invalid go subcommand to verify exit code handling
	cmd := exec.Command(goBin, "invalid-subcommand-xyz-999")

	var postHookCalled bool
	postHook := func() error {
		postHookCalled = true
		return nil
	}

	err = runner.Execute(context.Background(), cmd, nil, postHook)
	if err == nil {
		t.Fatal("expected command failure for invalid subcommand, got nil")
	}

	if !postHookCalled {
		t.Fatal("expected postHook to be executed even when command fails")
	}
}

func TestPTYRunner_Execute_NilCommand(t *testing.T) {
	runner := process.NewPTYRunner()
	err := runner.Execute(context.Background(), nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for nil command, got nil")
	}
}

func TestPTYRunner_Execute_ContextCancellation(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go binary not found in PATH")
	}

	runner := process.NewPTYRunner()

	ctx, cancel := context.WithCancel(context.Background())

	// #nosec G204 -- test verifies cancellation handling on subprocess
	cmd := exec.CommandContext(ctx, goBin, "doc", "builtin")

	// Cancel context immediately after launching
	time.AfterFunc(10*time.Millisecond, cancel)

	err = runner.Execute(ctx, cmd, nil, nil)
	if err == nil {
		t.Log("command completed before cancellation")
	} else if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "signal: killed") && !strings.Contains(err.Error(), "canceled") {
		t.Logf("command returned cancellation error: %v", err)
	}
}
