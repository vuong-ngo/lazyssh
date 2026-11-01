# Project Structure

`lazyssh` adheres to standard Go project conventions (as documented in [golang-standards/project-layout](https://github.com/golang-standards/project-layout)).

---

## Directory Tree

```
lazyssh/
├── cmd/
│   └── lazyssh/
│       ├── main.go                # Application entrypoint
│       ├── root.go                # Root Cobra command & global flags
│       ├── connect.go             # 'lazyssh connect' subcommand
│       ├── list.go                # 'lazyssh list' subcommand
│       ├── ping.go                # 'lazyssh ping' subcommand
│       └── version.go             # 'lazyssh version' subcommand
│
├── internal/                      # Private internal packages (not exportable)
│   ├── app/
│   │   └── app.go                 # App context initialization & shutdown hooks
│   ├── config/
│   │   ├── config.go              # YAML config loader (~/.config/lazyssh/config.yaml)
│   │   ├── defaults.go            # Default configurations
│   │   └── metadata.go            # Favorites, history, and custom tags storage
│   ├── sshconfig/
│   │   ├── parser.go              # ~/.ssh/config tokenizer and parser
│   │   ├── writer.go              # Non-destructive atomic config writer
│   │   ├── resolver.go            # Include directive resolver and host matcher
│   │   └── validator.go           # Configuration validation rules
│   ├── model/
│   │   ├── host.go                # HostConfig data structures
│   │   ├── tunnel.go              # PortForward data structures
│   │   └── key.go                 # SSH identity and agent data structures
│   ├── session/
│   │   ├── handover.go            # PTY terminal suspension & OpenSSH process handover
│   │   └── executor.go            # Local command executor & clipboard integration
│   ├── tunnel/
│   │   ├── manager.go             # Active tunnel daemon & supervisor
│   │   ├── local.go               # Local port forward handler (-L)
│   │   ├── remote.go              # Remote port forward handler (-R)
│   │   └── socks5.go              # Dynamic proxy handler (-D)
│   ├── health/
│   │   ├── ping.go                # TCP 3-way handshake latency probe
│   │   └── pool.go                # Concurrency-limited worker pool
│   ├── keyring/
│   │   ├── agent.go               # ssh-agent UNIX socket client
│   │   └── inspector.go           # Public key file reader & fingerprint calculator
│   └── tui/
│       ├── root.go                # Root Bubble Tea model & event router
│       ├── styles.go              # Lipgloss styling rules
│       ├── theme/
│       │   ├── theme.go           # Theme definitions & color palettes
│       │   ├── tokyonight.go      # Tokyo Night theme
│       │   ├── catppuccin.go      # Catppuccin Mocha theme
│       │   └── nord.go            # Nord theme
│       └── components/
│           ├── hostlist/          # Host list view & pagination model
│           ├── details/           # Host inspector panel model
│           ├── tunnels/           # Active tunnels panel model
│           ├── keys/              # SSH key inspector panel model
│           ├── search/            # Fuzzy search bar component
│           └── modal/             # Shared modal dialogs (Add/Edit/Confirm/Help)
│
├── docs/                          # Architecture, specs, and design documentation
│   ├── ARCHITECTURE.md
│   ├── SPECIFICATION.md
│   ├── CONFIGURATION.md
│   ├── KEYBINDINGS.md
│   ├── ROADMAP.md
│   ├── PROJECT_STRUCTURE.md
│   └── CONTRIBUTING.md
│
├── test/
│   ├── fixtures/                  # Sample SSH config files for unit/integration testing
│   │   ├── simple.conf
│   │   ├── includes.conf
│   │   └── complex.conf
│   └── mocks/                     # Mock implementations for testing
│
├── .gitignore                     # Git ignore rules
├── LICENSE                        # MIT License
├── Makefile                       # Developer automation tasks (build, test, lint)
├── README.md                      # Project presentation and user guide
├── go.mod                         # Go module definition
└── go.sum                         # Go module checksums
```

---

## Package Responsibilities

### `cmd/lazyssh/`
Contains the `main` package. Its sole responsibility is to parse CLI flags and arguments via [Cobra](https://github.com/spf13/cobra) and initialize the runtime. It does not contain core business logic.

### `internal/sshconfig/`
Independent of the UI. Encapsulates all reading, parsing, resolving, and writing of OpenSSH configuration files. It can be thoroughly tested with unit test fixtures without requiring a terminal.

### `internal/session/`
Responsible for transitioning the terminal environment from the Bubble Tea TUI to the raw `/usr/bin/ssh` process and restoring terminal state cleanly when the session finishes.

### `internal/health/`
Handles network probes to test host availability. Probing uses pure Go `net` standard library sockets with tight timeouts to avoid blocking the UI.

### `internal/tunnel/`
Manages background SSH port forwarding processes. Tracks process IDs, active ports, error states, and automatic restarts.

### `internal/tui/`
Implements the Bubble Tea `Model`, `Update`, and `View` functions. Decomposed into modular components (`components/`) so each panel is an independent sub-model with its own local state.
