# Project Structure

`lazyssh` is engineered using **Hexagonal Architecture (Ports and Adapters)** to enforce strict separation of concerns, testability, and zero domain bloat, adhering to standard Go project conventions ([golang-standards/project-layout](https://github.com/golang-standards/project-layout)).

---

## Directory Tree

```
lazyssh/
├── cmd/
│   └── lazyssh/
│       ├── main.go                       # Application entrypoint & version injection
│       ├── root.go                       # Root Cobra CLI command & global flags
│       ├── connect.go                    # 'lazyssh connect' headless command
│       ├── list.go                       # 'lazyssh list' JSON/table command
│       ├── ping.go                       # 'lazyssh ping' standalone probe command
│       └── version.go                    # 'lazyssh version' build information
│
├── internal/                             # Private application packages (not exportable)
│   │
│   ├── domain/                           # Enterprise Domain Layer (Pure Business Core)
│   │   ├── host.go                       # Host & PortForward entities, validation, helpers
│   │   └── errors.go                     # Strongly typed sentinel domain errors
│   │
│   ├── application/                      # Application Layer (Use Cases & Orchestration)
│   │   ├── host_service.go               # Host listing, search, mutation coordination
│   │   ├── session_service.go            # SSH session preparation and invocation
│   │   ├── health_service.go             # Latency probe scheduling & dispatching
│   │   └── tunnel_service.go             # Port forward lifecycle supervisor
│   │
│   ├── infrastructure/                   # Driven Adapters (Infrastructure Implementations)
│   │   ├── sshconfig/                    # OpenSSH config engine (AST, Parser, Writer)
│   │   │   ├── ast.go                    # In-memory AST nodes (comments, indentation, CRLF)
│   │   │   ├── parser.go                 # Streaming parser with recursive include resolution
│   │   │   └── writer.go                 # Atomic persistence (temp file + fsync + 0600)
│   │   ├── process/                      # OS process & terminal handover adapter
│   │   │   └── pty_runner.go             # PTY raw mode suspension, /usr/bin/ssh exec, restoration
│   │   ├── network/                      # TCP socket latency probe adapter
│   │   │   ├── tcp_prober.go             # Pure Go TCP 3-way handshake RTT prober (net.DialTimeout)
│   │   │   └── worker_pool.go            # Bounded concurrency worker pool with context cancellation
│   │   └── keyring/                      # System SSH agent adapter
│   │       └── agent.go                  # ssh-agent UNIX domain socket client & key inspector
│   │
│   └── interfaces/                       # Driving Adapters (Presentation & User Interface)
│       └── tui/                          # Terminal UI (Bubble Tea / Lipgloss)
│           ├── model.go                  # Root Model, Update, View coordinator
│           ├── styles.go                 # Lipgloss theme definitions & styling rules
│           └── components/               # Modular sub-models (Elm Architecture)
│               ├── hostlist/             # Host table / list view & navigation
│               ├── details/              # Host details inspector panel
│               ├── tunnels/              # Active port forwarding tunnels panel
│               ├── keys/                 # SSH identity keys inspector panel
│               └── search/               # Fuzzy filter input bar
│
├── test/                                 # Isolated Testing Suite (Separated from production code)
│   ├── domain/
│   │   └── host_test.go                  # Unit tests for Host & PortForward domain logic
│   └── sshconfig/
│       └── parser_test.go                # Comprehensive tests for AST parser, writer, CRLF, permissions
│
├── docs/                                 # Architecture, specifications, and design documentation
│   ├── ARCHITECTURE.md                   # Comprehensive architecture guide & UML diagrams
│   ├── KEYBINDINGS.md                    # Interactive keybindings quick reference
│   ├── PROJECT_STRUCTURE.md              # Project layout & package responsibility guide
│   └── SPECIFICATION.md                  # Functional and non-functional requirements
│
├── .github/                              # CI/CD Workflows & Linter Configurations
│   ├── workflows/
│   │   ├── ci.yml                        # Cross-platform CI matrix (Ubuntu, macOS, Windows)
│   │   └── release.yml                   # GoReleaser multi-architecture automated releases
│   ├── .golangci.yml                     # Strict enterprise Go linter configuration
│   └── .goreleaser.yml                   # Cross-compilation targets & packaging specs
│
├── Makefile                              # Developer automation tasks (build, test, lint, check)
├── LICENSE                               # MIT License
├── go.mod                                # Go module definition
└── go.sum                                # Go module checksums
```

---

## Package Responsibilities

### `cmd/lazyssh/`
Contains the application entrypoint. Its sole responsibility is to bootstrap the application, bind CLI flags via [Cobra](https://github.com/spf13/cobra), inject build-time version stamps, and initialize the presentation layer. It contains zero business logic.

### `internal/domain/`
The heart of the application containing pure enterprise business logic and entities.
- **`host.go`**: Defines `Host`, `PortForward`, and `ForwardType` entities with pure business validation (`Validate()`), effective address calculation (`EffectivePort()`, `EffectiveHostName()`), and argument synthesis (`SSHArgs()`). Has **zero external dependencies** outside the Go standard library.
- **`errors.go`**: Defines strongly typed domain sentinel errors (`ErrEmptyAlias`, `ErrInvalidPort`, `ErrHostNotFound`, `ErrCircularInclude`, etc.) enabling clean error matching across layers.

### `internal/application/`
Orchestrates domain entities and interfaces with driven infrastructure ports. Houses use case implementations:
- Managing host inventory and fuzzy search filtering.
- Coordinating background TCP health probe batches across hosts.
- Supervising active port forward tunnels and monitoring PID lifecycles.

### `internal/infrastructure/sshconfig/`
Provides a 100% round-trip lossless OpenSSH configuration parser and writer.
- **`ast.go`**: Models configuration documents as an Abstract Syntax Tree (AST), retaining comments, blank lines, indentation whitespace, and platform-specific CRLF/LF line endings.
- **`parser.go`**: Tokenizes and parses `~/.ssh/config` streams with recursive `Include` directive resolution and visited-path cycle detection.
- **`writer.go`**: Implements crash-proof atomic updates by writing to an adjacent temporary file with POSIX `0600` permissions, executing `file.Sync()` (`fsync`), and atomically replacing the file via `os.Rename`.

### `internal/infrastructure/process/`
Handles OS-level process management and terminal PTY handovers:
- Releases Bubble Tea raw mode (`tcsetattr`) and restores standard I/O streams.
- Hands terminal control to the native `/usr/bin/ssh` binary.
- Cleanly recovers the TUI screen buffer and keyboard raw mode upon process exit.

### `internal/infrastructure/network/`
Implements network reachability and latency probing:
- **`tcp_prober.go`**: Measures round-trip time (RTT) via TCP 3-way handshake (`net.DialTimeout`) without performing full SSH authentication.
- **`worker_pool.go`**: A bounded-concurrency worker pool (default 10 workers) preventing file descriptor starvation during batch pings.

### `internal/interfaces/tui/`
The presentation layer built on [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lipgloss](https://github.com/charmbracelet/lipgloss). Follows the Elm Architecture:
- Root model coordinates active panel focus and global hotkeys.
- Decoupled panel components maintain isolated view states and communicate via typed messages.

### `test/`
Strictly isolated test directory. Production packages in `internal/` contain no `*_test.go` files, ensuring clean build separation and preventing test fixtures or test utilities from leaking into binary builds.
