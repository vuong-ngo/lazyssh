// ==============================================================================
// File: internal/infrastructure/process/pty_runner.go
// Project: lazyssh
// Description: OpenSSH client process runner and terminal PTY handover adapter
//              with lifecycle hooks, stream binding, and signal propagation.
// ==============================================================================

// Package process provides OS subprocess management and PTY handover for OpenSSH.
package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/vuong-ngo/lazyssh/internal/domain"
)

// ErrSSHNotFound is returned when the OpenSSH client binary is not present in PATH.
var ErrSSHNotFound = errors.New("openssh client (ssh) not found in system PATH")

// Hook defines a lifecycle callback executed before or after subprocess execution.
type Hook func() error

// Runner defines the process execution and command synthesis contract.
type Runner interface {
	// LookPath resolves the absolute filesystem path of the ssh executable.
	LookPath() (string, error)

	// BuildCommand synthesizes an *exec.Cmd configured with the host's SSH parameters.
	BuildCommand(ctx context.Context, host domain.Host) (*exec.Cmd, error)

	// Execute runs the command with pre/post terminal lifecycle hooks.
	Execute(ctx context.Context, cmd *exec.Cmd, preHook Hook, postHook Hook) error
}

// PTYRunner implements Runner using standard OS subprocess execution and stream forwarding.
type PTYRunner struct {
	binaryPath string
	stdin      io.Reader
	stdout     io.Writer
	stderr     io.Writer
}

// NewPTYRunner creates a default PTYRunner wired to standard OS streams (os.Stdin/Stdout/Stderr).
func NewPTYRunner() *PTYRunner {
	return &PTYRunner{
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
}

// NewPTYRunnerWithStreams creates a PTYRunner with custom stream bindings and explicit binary path (ideal for testing).
func NewPTYRunnerWithStreams(binaryPath string, stdin io.Reader, stdout io.Writer, stderr io.Writer) *PTYRunner {
	return &PTYRunner{
		binaryPath: binaryPath,
		stdin:      stdin,
		stdout:     stdout,
		stderr:     stderr,
	}
}

// LookPath locates the ssh client binary in PATH.
func (r *PTYRunner) LookPath() (string, error) {
	if r.binaryPath != "" {
		return r.binaryPath, nil
	}

	path, err := exec.LookPath("ssh")
	if err != nil {
		return "", ErrSSHNotFound
	}
	return path, nil
}

// BuildCommand synthesizes the exec.Cmd with host arguments, standard stream bindings, and context.
func (r *PTYRunner) BuildCommand(ctx context.Context, host domain.Host) (*exec.Cmd, error) {
	if err := host.Validate(); err != nil {
		return nil, fmt.Errorf("invalid host configuration: %w", err)
	}

	bin, err := r.LookPath()
	if err != nil {
		return nil, err
	}

	args := host.SSHArgs()

	// #nosec G204 -- bin is resolved via LookPath and args are structured flags from Host entity
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = r.stdin
	cmd.Stdout = r.stdout
	cmd.Stderr = r.stderr

	return cmd, nil
}

// Execute runs the process synchronously, guaranteeing postHook execution even if the command errors.
func (r *PTYRunner) Execute(ctx context.Context, cmd *exec.Cmd, preHook Hook, postHook Hook) error {
	if cmd == nil {
		return errors.New("cannot execute nil command")
	}

	// 1. Invoke Pre-run hook (e.g. Release terminal raw mode)
	if preHook != nil {
		if err := preHook(); err != nil {
			return fmt.Errorf("pre-run hook failed: %w", err)
		}
	}

	// Invariant: Post-run hook MUST always execute to restore terminal state
	if postHook != nil {
		defer func() {
			_ = postHook()
		}()
	}

	// 2. Synchronous process execution
	err := cmd.Run()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	if err != nil {
		return fmt.Errorf("ssh session finished with error: %w", err)
	}

	return nil
}
