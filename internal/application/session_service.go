// ==============================================================================
// File: internal/application/session_service.go
// Project: lazyssh
// Description: Application service coordinating interactive SSH terminal session
//              spawning, PTY handover hooks, and last connection tracking.
// ==============================================================================

package application

import (
	"context"
	"os/exec"
	"time"

	"github.com/vuong-ngo/lazyssh/internal/domain"
	"github.com/vuong-ngo/lazyssh/internal/infrastructure/process"
)

// SessionService orchestrates interactive SSH connections.
type SessionService struct {
	runner process.Runner
}

// NewSessionService creates a new SessionService.
func NewSessionService(runner process.Runner) *SessionService {
	if runner == nil {
		runner = process.NewPTYRunner()
	}
	return &SessionService{
		runner: runner,
	}
}

// PrepareCommand builds an *exec.Cmd for the target host without executing it.
func (s *SessionService) PrepareCommand(ctx context.Context, host domain.Host) (*exec.Cmd, error) {
	return s.runner.BuildCommand(ctx, host)
}

// Connect launches the interactive SSH process, executes terminal handover hooks,
// and updates the host's LastConnectedAt timestamp upon successful exit.
func (s *SessionService) Connect(ctx context.Context, host *domain.Host, preHook process.Hook, postHook process.Hook) error {
	if host == nil {
		return domain.ErrHostNotFound
	}

	cmd, err := s.runner.BuildCommand(ctx, *host)
	if err != nil {
		return err
	}

	now := time.Now()
	if err := s.runner.Execute(ctx, cmd, preHook, postHook); err != nil {
		return err
	}

	host.LastConnectedAt = &now
	return nil
}
