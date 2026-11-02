// ==============================================================================
// File: internal/domain/host.go
// Project: lazyssh
// Description: Core Host and PortForward domain entities, validation invariants,
//              and SSH client command-line argument formatting.
// ==============================================================================

package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ForwardType defines the type of SSH port forwarding tunnel.
type ForwardType string

const (
	// ForwardLocal maps a local port to a remote host:port (-L).
	ForwardLocal ForwardType = "LOCAL"

	// ForwardRemote maps a remote port to a local host:port (-R).
	ForwardRemote ForwardType = "REMOTE"

	// ForwardDynamic creates a local SOCKS5 dynamic proxy (-D).
	ForwardDynamic ForwardType = "DYNAMIC"
)

// PortForward represents a port forwarding tunnel definition.
type PortForward struct {
	Type       ForwardType `json:"type"`
	BindHost   string      `json:"bind_host,omitempty"`
	BindPort   int         `json:"bind_port"`
	TargetHost string      `json:"target_host,omitempty"`
	TargetPort int         `json:"target_port,omitempty"`
}

// Spec formats the port forward rule for OpenSSH command-line flags.
func (pf PortForward) Spec() string {
	bindHost := pf.BindHost
	if bindHost == "" {
		bindHost = "127.0.0.1"
	}

	switch pf.Type {
	case ForwardLocal:
		return fmt.Sprintf("%s:%d:%s:%d", bindHost, pf.BindPort, pf.TargetHost, pf.TargetPort)
	case ForwardRemote:
		return fmt.Sprintf("%s:%d:%s:%d", bindHost, pf.BindPort, pf.TargetHost, pf.TargetPort)
	case ForwardDynamic:
		return fmt.Sprintf("%s:%d", bindHost, pf.BindPort)
	default:
		return ""
	}
}

// Validate checks whether the port forward rule is logically sound.
func (pf PortForward) Validate() error {
	if pf.BindPort < 1 || pf.BindPort > 65535 {
		return fmt.Errorf("%w: bind port %d", ErrInvalidPort, pf.BindPort)
	}

	if pf.Type == ForwardLocal || pf.Type == ForwardRemote {
		if strings.TrimSpace(pf.TargetHost) == "" {
			return fmt.Errorf("%w: target host cannot be empty", ErrInvalidForward)
		}
		if pf.TargetPort < 1 || pf.TargetPort > 65535 {
			return fmt.Errorf("%w: target port %d", ErrInvalidPort, pf.TargetPort)
		}
	}
	return nil
}

// Host represents an SSH host configuration block.
type Host struct {
	Alias           string            `json:"alias"`
	HostName        string            `json:"hostname"`
	User            string            `json:"user,omitempty"`
	Port            int               `json:"port,omitempty"`
	IdentityFiles   []string          `json:"identity_files,omitempty"`
	ProxyJump       string            `json:"proxy_jump,omitempty"`
	ProxyCommand    string            `json:"proxy_command,omitempty"`
	ForwardAgent    bool              `json:"forward_agent,omitempty"`
	LocalForwards   []PortForward     `json:"local_forwards,omitempty"`
	RemoteForwards  []PortForward     `json:"remote_forwards,omitempty"`
	DynamicForwards []string          `json:"dynamic_forwards,omitempty"`
	ExtraDirectives map[string]string `json:"extra_directives,omitempty"`

	// Metadata maintained by lazyssh
	SourceFile      string        `json:"source_file,omitempty"`
	LineNumber      int           `json:"line_number,omitempty"`
	Tags            []string      `json:"tags,omitempty"`
	IsFavorite      bool          `json:"is_favorite"`
	LastConnectedAt *time.Time    `json:"last_connected_at,omitempty"`
	Latency         time.Duration `json:"latency,omitempty"`
	IsReachable     bool          `json:"is_reachable"`
}

// Validate ensures the Host entity invariants are respected.
func (h Host) Validate() error {
	trimmedAlias := strings.TrimSpace(h.Alias)
	if trimmedAlias == "" {
		return ErrEmptyAlias
	}

	if strings.ContainsAny(h.Alias, " \t\r\n") {
		return ErrInvalidAlias
	}

	if h.Port < 0 || h.Port > 65535 {
		return ErrInvalidPort
	}

	for _, lf := range h.LocalForwards {
		if err := lf.Validate(); err != nil {
			return err
		}
	}

	for _, rf := range h.RemoteForwards {
		if err := rf.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// EffectivePort returns the configured port, or standard port 22 if unset.
func (h Host) EffectivePort() int {
	if h.Port > 0 {
		return h.Port
	}
	return 22
}

// EffectiveHostName returns the configured HostName, or the Alias if unset.
func (h Host) EffectiveHostName() string {
	if strings.TrimSpace(h.HostName) != "" {
		return h.HostName
	}
	return h.Alias
}

// FormattedAddress returns a human-readable display string like "user@hostname:22".
func (h Host) FormattedAddress() string {
	target := h.EffectiveHostName()
	port := h.EffectivePort()

	if h.User != "" {
		return fmt.Sprintf("%s@%s:%d", h.User, target, port)
	}
	return fmt.Sprintf("%s:%d", target, port)
}

// SSHArgs builds the command-line argument slice for invoking /usr/bin/ssh.
func (h Host) SSHArgs() []string {
	var args []string

	if h.Port > 0 && h.Port != 22 {
		args = append(args, "-p", strconv.Itoa(h.Port))
	}

	if h.User != "" {
		args = append(args, "-l", h.User)
	}

	for _, key := range h.IdentityFiles {
		if strings.TrimSpace(key) != "" {
			args = append(args, "-i", key)
		}
	}

	if h.ProxyJump != "" {
		args = append(args, "-J", h.ProxyJump)
	}

	// Target host alias / address
	args = append(args, h.Alias)
	return args
}
