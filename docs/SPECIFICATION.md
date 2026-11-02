# Technical Specification

This document defines the functional and non-functional requirements, data models, and system behaviors of **lazyssh**.

---

## 1. Objectives

- Provide a high-performance terminal UI for inspecting, organizing, and connecting to SSH endpoints.
- Preserve full fidelity and round-trip integrity of `~/.ssh/config` without proprietary lock-in.
- Offer an intuitive keyboard-driven workflow matching standards established by modern CLI developer tools (`lazygit`, `k9s`).
- Support background port forward / tunnel lifecycle management.

---

## 2. Functional Requirements (FR)

### FR-1: SSH Config Discovery & Parsing
- **FR-1.1**: Automatically detect and load the user's primary SSH config file from standard paths:
  - `$HOME/.ssh/config`
  - `/etc/ssh/ssh_config` (system fallback if specified)
  - Custom file path supplied via `-F <path>` or `--config <path>`.
- **FR-1.2**: Recursively resolve `Include` directives (e.g. `Include ~/.ssh/conf.d/*` or `Include config.d/*`).
- **FR-1.3**: Support all standard OpenSSH host directives, including:
  - `Host`, `Match`
  - `HostName`
  - `User`
  - `Port`
  - `IdentityFile`
  - `ProxyJump`, `ProxyCommand`
  - `ForwardAgent`, `LocalForward`, `RemoteForward`, `DynamicForward`
  - `ServerAliveInterval`, `ServerAliveCountMax`
- **FR-1.4**: Preserve non-standard or custom directives and user comments during write operations.

### FR-2: Host Navigation, Filtering & Search
- **FR-2.1**: Display hosts in a scrollable list showing:
  - Host alias / name
  - Resolved IP address or hostname
  - User and port
  - Health/latency status icon
  - Tags and favorite indicator
- **FR-2.2**: Provide real-time fuzzy filtering (via `/` key) across:
  - Host alias
  - Target `HostName` / IP
  - SSH username
  - Associated tags or group names
- **FR-2.3**: Allow sorting hosts by:
  - Name / Alias (alphabetical)
  - Most recently used (MRU)
  - Connection latency
  - Custom favorites / bookmarks

### FR-3: Interactive Session Launching (PTY Handover)
- **FR-3.1**: Pressing `Enter` on a selected host triggers immediate connection via OpenSSH client (`ssh <alias>`).
- **FR-3.2**: Release the terminal raw mode cleanly before launching `ssh` and restore the TUI state immediately upon process exit.
- **FR-3.3**: Provide a quick copy feature (`y`) that copies the equivalent `ssh user@host -p port` command to the system clipboard.
- **FR-3.4**: Record connection timestamp in local history (`~/.config/lazyssh/history.json`).

### FR-4: Host Configuration Management (CRUD)
- **FR-4.1 (Add)**: Interactive modal form to create a new SSH host block with validation for alias uniqueness and valid port numbers.
- **FR-4.2 (Edit)**: Interactive in-TUI editor or launch `$EDITOR` directly on the host block in the config file.
- **FR-4.3 (Delete)**: Prompt for explicit confirmation before removing a host block.
- **FR-4.4 (Duplicate)**: Clone an existing host configuration to create a new one with a different alias.
- **FR-4.5 (Atomic Writes)**: Config changes must write to a temporary file before atomic renaming, preventing data loss on crash.

### FR-5: Host Tagging & Grouping
- **FR-5.1**: Allow assigning custom tags to hosts (e.g. `prod`, `database`, `eu-west`).
- **FR-5.2**: Store tags non-destructively:
  - Either as inline comments in `~/.ssh/config` (e.g. `# lazyssh-tags: prod, db`), or
  - In `~/.config/lazyssh/config.yaml`.
- **FR-5.3**: Filter host list by selecting tags or typing `tag:prod` in search bar.

### FR-6: Port Forwarding & SSH Tunnels
- **FR-6.1**: View active and configured port forwarding tunnels:
  - Local port forwards (`-L`)
  - Remote port forwards (`-R`)
  - Dynamic SOCKS5 proxies (`-D`)
- **FR-6.2**: Start and stop tunnels directly from the TUI.
- **FR-6.3**: Detect local port conflicts before starting a tunnel.
- **FR-6.4**: Display live status of active tunnels (running, failed, port in use, pid).

### FR-7: SSH Key & Agent Inspector
- **FR-7.1**: Query `ssh-agent` via UNIX domain socket (`$SSH_AUTH_SOCK`) to display loaded keys.
- **FR-7.2**: Display key algorithm (Ed25519, RSA, ECDSA), bit length, and SHA256 fingerprint.
- **FR-7.3**: Inspect configured `IdentityFile` paths for existence and correct file permissions (`0600`).

### FR-8: Host Reachability & Latency Monitoring
- **FR-8.1**: Single-host ping (`p`): performs a non-blocking TCP dial to the host's port and displays round-trip time.
- **FR-8.2**: Batch ping (`P`): tests all visible hosts using a bounded worker pool.
- **FR-8.3**: Visual status indicators:
  - 🟢 `< 50ms` (Excellent)
  - 🟡 `50ms - 150ms` (Normal)
  - 🔴 `> 150ms` (High latency)
  - ⚪ `Timeout / Unreachable`

### FR-9: CLI Headless / Scriptable Commands
- **FR-9.1**: `lazyssh connect [alias]`: Connect directly without opening the full TUI dashboard.
- **FR-9.2**: `lazyssh list [--json]`: Output all parsed hosts in formatted table or JSON.
- **FR-9.3**: `lazyssh ping [alias]`: Output latency stats directly to stdout.

---

## 3. Non-Functional Requirements (NFR)

| Category | Requirement | Target Metric |
| :--- | :--- | :--- |
| **Startup Time** | Time from binary invocation to rendered TUI | `< 30ms` (for configs up to 1,000 hosts) |
| **Memory Footprint** | Resident Set Size (RSS) during regular usage | `< 20 MB` |
| **Binary Size** | Standalone compiled executable size | `< 15 MB` (uncompressed) |
| **Data Integrity** | Zero data loss on unexpected terminal kills | Atomic temp-file swap (`0600` perms) |
| **Terminal Compatibility** | Minimum terminal window dimensions | Supports down to `80x24` terminal dimensions |
| **Platform Support** | OS & Architectures | Linux, macOS, Windows (amd64, arm64) |
| **Dependencies** | External runtime requirements | None (pure Go; optional standard `/usr/bin/ssh`) |

---

## 4. Core Data Models (Go)

The enterprise domain models reside in `internal/domain/host.go` with zero external dependencies outside the Go standard library.

```go
package domain

import (
    "fmt"
    "strconv"
    "strings"
    "time"
)

// ForwardType defines the port forwarding direction or protocol.
type ForwardType string

const (
    ForwardLocal   ForwardType = "LOCAL"   // -L [bind_address:]port:host:hostport
    ForwardRemote  ForwardType = "REMOTE"  // -R [bind_address:]port:host:hostport
    ForwardDynamic ForwardType = "DYNAMIC" // -D [bind_address:]port (SOCKS5 proxy)
)

// PortForward represents a validated SSH port forwarding tunnel definition.
type PortForward struct {
    Type       ForwardType `json:"type"`        // LOCAL, REMOTE, or DYNAMIC
    BindHost   string      `json:"bind_host"`   // Local bind IP or loopback
    BindPort   int         `json:"bind_port"`   // Local listening port
    TargetHost string      `json:"target_host"` // Remote destination host
    TargetPort int         `json:"target_port"` // Remote destination port
    HostAlias  string      `json:"host_alias"`  // Associated Host alias
    IsActive   bool        `json:"is_active"`   // Live runtime status
    PID        int         `json:"pid,omitempty"` // Daemon process PID if running
}

// Host represents a parsed and validated OpenSSH host configuration entry.
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
    SourceFile      string            `json:"source_file"`
    LineNumber      int               `json:"line_number"`
    Tags            []string          `json:"tags,omitempty"`
    IsFavorite      bool              `json:"is_favorite"`
    LastConnectedAt *time.Time        `json:"last_connected_at,omitempty"`
    Latency         *time.Duration    `json:"latency,omitempty"`
    IsReachable     bool              `json:"is_reachable"`
}
```

### Domain Methods & Behaviors

| Method | Receiver | Description |
| :--- | :--- | :--- |
| `Validate()` | `*Host` | Enforces alias format, valid port boundaries (`1 - 65535`), and nested forward validity. |
| `EffectivePort()` | `*Host` | Returns the explicitly defined port, or defaults to standard `22`. |
| `EffectiveHostName()` | `*Host` | Returns `HostName` or falls back to `Alias` if `HostName` is empty. |
| `FormattedAddress()` | `*Host` | Generates a display address string (e.g. `user@hostname:2222` or `hostname`). |
| `SSHArgs()` | `*Host` | Generates the exact parameter slice for spawning `/usr/bin/ssh` subprocesses. |
| `Validate()` | `*PortForward` | Validates bind/target port boundaries and requires destination host on local/remote forwards. |
| `TunnelSpec()` | `*PortForward` | Synthesizes OpenSSH tunnel arguments (e.g. `-L 8080:127.0.0.1:80`). |

---

## 5. Error Handling & Edge Cases

The domain layer enforces strongly typed sentinel errors (`internal/domain/errors.go`) for predictable programmatic handling:

| Domain Sentinel Error | Description |
| :--- | :--- |
| `ErrEmptyAlias` | Host alias is empty or contains only whitespace. |
| `ErrInvalidPort` | Port number is outside the allowed TCP range `[1, 65535]`. |
| `ErrHostNotFound` | Requested host alias does not exist in configuration. |
| `ErrHostAlreadyExists` | Host alias collides with an existing configuration entry. |
| `ErrCircularInclude` | Recursive cycle detected while resolving OpenSSH `Include` directives. |
| `ErrPortOccupied` | Local bind port is already in use by another process. |
| `ErrUnsafePermissions` | SSH config or identity file permissions are looser than required (`0600`/`0700`). |

### Runtime Edge Cases

1. **Missing or Corrupted Config**:
   - If `~/.ssh/config` is missing, `lazyssh` prompts the user with an option to initialize an empty or template config file.
   - If syntax errors occur, display the offending file and line number without crashing.
2. **Permission Warnings**:
   - Warn if `~/.ssh/` permissions are looser than `0700` or `~/.ssh/config` is looser than `0600`.
3. **Port Collisions**:
   - When launching a tunnel, pre-test local bind port using `net.Listen("tcp", addr)`. If already occupied, return `ErrPortOccupied`.
4. **Terminal Resizing**:
   - Handle `tea.WindowSizeMsg` dynamically, reflowing panel widths and truncating long strings with ellipsis (`...`).
