// ==============================================================================
// File: test/application/session_service_test.go
// Project: lazyssh
// Description: Unit tests for SessionService verifying command preparation,
//              interactive connection hooks, and connection timestamp tracking.
// ==============================================================================

package application_test

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/vuong-ngo/lazyssh/internal/application"
	"github.com/vuong-ngo/lazyssh/internal/domain"
	"github.com/vuong-ngo/lazyssh/internal/infrastructure/process"
)

type mockRunner struct {
	lookPathFunc     func() (string, error)
	buildCommandFunc func(ctx context.Context, host domain.Host) (*exec.Cmd, error)
	executeFunc      func(ctx context.Context, cmd *exec.Cmd, preHook process.Hook, postHook process.Hook) error
}

func (m *mockRunner) LookPath() (string, error) {
	if m.lookPathFunc != nil {
		return m.lookPathFunc()
	}
	return "/usr/bin/ssh", nil
}

func (m *mockRunner) BuildCommand(ctx context.Context, host domain.Host) (*exec.Cmd, error) {
	if m.buildCommandFunc != nil {
		return m.buildCommandFunc(ctx, host)
	}
	// #nosec G204 -- mock runner synthesizes test exec.Cmd
	return exec.Command("ssh", host.SSHArgs()...), nil
}

func (m *mockRunner) Execute(ctx context.Context, cmd *exec.Cmd, preHook process.Hook, postHook process.Hook) error {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, cmd, preHook, postHook)
	}
	if preHook != nil {
		if err := preHook(); err != nil {
			return err
		}
	}
	if postHook != nil {
		defer func() {
			_ = postHook()
		}()
	}
	return nil
}

func TestSessionService_PrepareCommand(t *testing.T) {
	service := application.NewSessionService(&mockRunner{})

	host := domain.Host{
		Alias:    "bastion",
		HostName: "bastion.example.com",
		Port:     22,
	}

	cmd, err := service.PrepareCommand(context.Background(), host)
	if err != nil {
		t.Fatalf("unexpected error preparing command: %v", err)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
}

func TestSessionService_Connect_Success(t *testing.T) {
	var preHookCalled bool
	var postHookCalled bool

	runner := &mockRunner{
		executeFunc: func(ctx context.Context, cmd *exec.Cmd, preHook process.Hook, postHook process.Hook) error {
			if preHook != nil {
				_ = preHook()
			}
			if postHook != nil {
				_ = postHook()
			}
			return nil
		},
	}

	service := application.NewSessionService(runner)

	host := &domain.Host{
		Alias:    "db-prod",
		HostName: "db.internal",
		Port:     22,
	}

	preHook := func() error {
		preHookCalled = true
		return nil
	}
	postHook := func() error {
		postHookCalled = true
		return nil
	}

	err := service.Connect(context.Background(), host, preHook, postHook)
	if err != nil {
		t.Fatalf("unexpected connect error: %v", err)
	}

	if !preHookCalled || !postHookCalled {
		t.Errorf("expected both hooks to be called: pre=%v post=%v", preHookCalled, postHookCalled)
	}

	if host.LastConnectedAt == nil {
		t.Fatal("expected LastConnectedAt to be updated after successful connect")
	}

	if time.Since(*host.LastConnectedAt) > 5*time.Second {
		t.Errorf("expected recent timestamp, got %v", *host.LastConnectedAt)
	}
}

func TestSessionService_Connect_NilHost(t *testing.T) {
	service := application.NewSessionService(nil)
	err := service.Connect(context.Background(), nil, nil, nil)
	if !errors.Is(err, domain.ErrHostNotFound) {
		t.Fatalf("expected ErrHostNotFound for nil host, got: %v", err)
	}
}

func TestSessionService_Connect_ExecutionFailureDoesNotUpdateTimestamp(t *testing.T) {
	execErr := errors.New("ssh connection reset by peer")
	runner := &mockRunner{
		executeFunc: func(ctx context.Context, cmd *exec.Cmd, preHook process.Hook, postHook process.Hook) error {
			return execErr
		},
	}

	service := application.NewSessionService(runner)

	host := &domain.Host{
		Alias:    "failed-host",
		HostName: "10.0.0.99",
		Port:     22,
	}

	err := service.Connect(context.Background(), host, nil, nil)
	if err == nil {
		t.Fatal("expected connect error, got nil")
	}
	if !errors.Is(err, execErr) {
		t.Fatalf("expected error wrapping %v, got: %v", execErr, err)
	}

	if host.LastConnectedAt != nil {
		t.Errorf("LastConnectedAt should not be updated when connection fails, got %v", *host.LastConnectedAt)
	}
}
