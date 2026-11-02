// ==============================================================================
// File: internal/domain/errors.go
// Project: lazyssh
// Description: Strongly-typed domain error definitions for host validation,
//              parsing, configuration bounds, and permission enforcement.
// ==============================================================================

package domain

import "errors"

// Domain-level errors for lazyssh.
var (
	// ErrEmptyAlias is returned when a host alias is empty or whitespace-only.
	ErrEmptyAlias = errors.New("host alias cannot be empty")

	// ErrInvalidAlias is returned when a host alias contains invalid characters (spaces, tabs, newlines).
	ErrInvalidAlias = errors.New("host alias contains invalid whitespace characters")

	// ErrEmptyHostName is returned when HostName or target address is empty.
	ErrEmptyHostName = errors.New("hostname cannot be empty")

	// ErrInvalidPort is returned when the port is out of the valid TCP range (1-65535).
	ErrInvalidPort = errors.New("port must be between 1 and 65535")

	// ErrHostNotFound is returned when an alias cannot be located in the configuration.
	ErrHostNotFound = errors.New("host not found")

	// ErrDuplicateHost is returned when attempting to add a host with an existing alias.
	ErrDuplicateHost = errors.New("host alias already exists")

	// ErrInvalidForward is returned when port forward parameters are malformed.
	ErrInvalidForward = errors.New("invalid port forward definition")

	// ErrUnsafePermissions is returned when SSH files have insecure file modes.
	ErrUnsafePermissions = errors.New("unsafe file permissions (expected 0600 for files or 0700 for directories)")
)
