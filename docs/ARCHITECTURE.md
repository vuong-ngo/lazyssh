# Architecture Guide

This document defines the software architecture, design patterns, internal component boundaries, and complete UML specifications for **lazyssh**.

---

## Table of Contents

1. [Architectural Philosophy: Hexagonal Architecture](#1-architectural-philosophy-hexagonal-architecture)
2. [Domain Class Diagram (UML Class Diagram)](#2-domain-class-diagram-uml-class-diagram)
3. [Domain Entity Relationships (UML ER Diagram)](#3-domain-entity-relationships-uml-er-diagram)
4. [Terminal UI & The Elm Architecture](#4-terminal-ui--the-elm-architecture)
5. [Native SSH PTY Handover Lifecycle](#5-native-ssh-pty-handover-lifecycle)
6. [Terminal State Machine (UML State Diagram)](#6-terminal-state-machine-uml-state-diagram)
7. [Non-Destructive AST SSH Config Engine](#7-non-destructive-ast-ssh-config-engine)
8. [Bounded Concurrency & Health Prober Model](#8-bounded-concurrency--health-prober-model)
9. [Tunnel Supervisor Lifecycle (UML State Diagram)](#9-tunnel-supervisor-lifecycle-uml-state-diagram)
10. [Security Posture & Boundary Principles](#10-security-posture--boundary-principles)

---

## 1. Architectural Philosophy: Hexagonal Architecture

`lazyssh` implements **Hexagonal Architecture (Ports and Adapters)**. The business domain (Host Management, SSH Tunnels, Health Monitoring) is strictly isolated from presentation adapters (Bubble Tea TUI, Cobra CLI) and infrastructure adapters (File System, OS Process Execution, TCP Sockets).

```mermaid
flowchart TB
    subgraph DrivingAdapters ["Driving Adapters (Input Layer)"]
        CLI["CLI Commands Adapter\n(cmd/lazyssh: list, ping, connect)"]
        TUI["Terminal UI Adapter\n(internal/presentation/tui: Bubble Tea)"]
    end

    subgraph InboundPorts ["Inbound Ports (Application Interfaces)"]
        IHostService["<<interface>>\nHostService"]
        ISessionService["<<interface>>\nSessionService"]
        IHealthService["<<interface>>\nHealthService"]
        ITunnelService["<<interface>>\nTunnelService"]
    end

    subgraph AppCore ["Application Layer (Use Cases)"]
        HostUC["HostService Implementation\n• GetAllHosts()\n• SearchHosts()\n• SaveHost()"]
        SessionUC["SessionService Implementation\n• BuildSSHCommand()\n• LaunchInteractiveSession()"]
        HealthUC["HealthService Implementation\n• ProbeHostLatency()\n• BatchProbeAll()"]
        TunnelUC["TunnelService Implementation\n• StartTunnel()\n• StopTunnel()"]
    end

    subgraph DomainLayer ["Domain Layer (Pure Business Core)"]
        HostEntity["Entity: Host\n(Alias, HostName, User, Port, Keys)"]
        TunnelEntity["Entity: PortForward\n(Type, BindPort, TargetHost, TargetPort)"]
        DomainErrors["Domain Errors\n(ErrHostNotFound, ErrPortOccupied, ErrUnsafePerms)"]
    end

    subgraph OutboundPorts ["Outbound Ports (Infrastructure Interfaces)"]
        IConfigRepo["<<interface>>\nSSHConfigRepository"]
        IProcessRunner["<<interface>>\nProcessRunner"]
        INetProber["<<interface>>\nNetworkProber"]
        IAgentKeyring["<<interface>>\nKeyringInspector"]
    end

    subgraph DrivenAdapters ["Driven Adapters (System Infrastructure)"]
        FileRepo["AST SSH Config File Adapter\n(~/.ssh/config Atomic Read/Write)"]
        PTYRunner["OS Process Runner Adapter\n(/usr/bin/ssh PTY Handover)"]
        TCPSocketProber["TCP Socket Prober Adapter\n(net.DialTimeout 3-Way Handshake)"]
        AgentSocket["ssh-agent UNIX Socket Adapter\n($SSH_AUTH_SOCK Inspector)"]
        StructuredLogger["Structured Logger Adapter\n(log/slog into ~/.config/lazyssh/app.log)"]
    end

    %% Driving connections
    CLI --> IHostService
    CLI --> ISessionService
    TUI --> IHostService
    TUI --> ISessionService
    TUI --> IHealthService
    TUI --> ITunnelService

    IHostService -.-> HostUC
    ISessionService -.-> SessionUC
    IHealthService -.-> HealthUC
    ITunnelService -.-> TunnelUC

    %% Application to Domain
    HostUC --> DomainLayer
    SessionUC --> DomainLayer
    HealthUC --> DomainLayer
    TunnelUC --> DomainLayer

    %% Application to Outbound Ports
    HostUC --> IConfigRepo
    SessionUC --> IProcessRunner
    HealthUC --> INetProber
    HostUC --> IAgentKeyring

    %% Outbound Ports to Driven Adapters
    IConfigRepo -.-> FileRepo
    IProcessRunner -.-> PTYRunner
    INetProber -.-> TCPSocketProber
    IAgentKeyring -.-> AgentSocket

    FileRepo -.-> StructuredLogger
    PTYRunner -.-> StructuredLogger
    TCPSocketProber -.-> StructuredLogger
```

---

## 2. Domain Class Diagram (UML Class Diagram)

This class diagram depicts the object-oriented structure, interfaces, dependencies, and composition across the domain and application layers:

```mermaid
classDiagram
    direction TB

    class Host {
        +string Alias
        +string HostName
        +string User
        +int Port
        +List~string~ IdentityFiles
        +string ProxyJump
        +List~string~ Tags
        +bool IsFavorite
        +Duration Latency
        +Time LastConnectedAt
        +Validate() error
        +FormattedAddress() string
    }

    class PortForward {
        +ForwardType Type
        +string BindHost
        +int BindPort
        +string TargetHost
        +int TargetPort
        +string HostAlias
        +bool IsActive
        +int PID
        +Validate() error
        +TunnelSpec() string
    }

    class ForwardType {
        <<enumeration>>
        LOCAL
        REMOTE
        DYNAMIC
    }

    class HostRepository {
        <<interface>>
        +FindAll(ctx context.Context) List~Host~, error
        +FindByAlias(ctx context.Context, alias string) Host, error
        +Save(ctx context.Context, host Host) error
        +Delete(ctx context.Context, alias string) error
    }

    class ProcessRunner {
        <<interface>>
        +BuildNativeCmd(host Host) exec.Cmd
        +ExecuteInteractive(cmd exec.Cmd) error
    }

    class NetworkProber {
        <<interface>>
        +ProbeTCP(ctx context.Context, target string, timeout Duration) Duration, error
    }

    class TunnelSupervisor {
        <<interface>>
        +Start(ctx context.Context, forward PortForward) int, error
        +Stop(ctx context.Context, forward PortForward) error
        +Status(ctx context.Context, forward PortForward) bool
    }

    class HostService {
        -HostRepository repo
        +ListAll(ctx context.Context) List~Host~, error
        +Search(query string, hosts List~Host~) List~Host~
        +Upsert(ctx context.Context, host Host) error
        +Remove(ctx context.Context, alias string) error
    }

    class HealthService {
        -NetworkProber prober
        -int workerCount
        +ProbeOne(ctx context.Context, host Host) Duration, error
        +ProbeBatch(ctx context.Context, hosts List~Host~) Map~string, Duration~
    }

    class SessionService {
        -ProcessRunner runner
        +PrepareCommand(host Host) exec.Cmd
    }

    %% Relationships
    PortForward --> ForwardType
    Host *-- PortForward : contains forwards
    HostService ..> HostRepository : uses
    HealthService ..> NetworkProber : uses
    SessionService ..> ProcessRunner : uses
    HostService ..> Host : manages
    HealthService ..> Host : measures
```

---

## 3. Domain Entity Relationships (UML ER Diagram)

The conceptual data model representing OpenSSH hosts, tunnel definitions, local metadata, and SSH identities:

```mermaid
erDiagram
    HOST ||--o{ PORT_FORWARD : defines
    HOST ||--o{ HOST_TAG : tagged_with
    HOST ||--o| HOST_METRIC : tracks
    HOST }o--o| SSH_IDENTITY : references

    HOST {
        string alias PK "Unique SSH Host alias"
        string hostname "Resolved IP or Domain"
        string user "SSH Username"
        int port "SSH Port (default 22)"
        string proxy_jump "ProxyJump alias or address"
        string source_file "Config path (~/.ssh/config)"
        int line_number "AST origin line"
    }

    PORT_FORWARD {
        string id PK "UUID"
        string host_alias FK "References HOST.alias"
        string type "LOCAL | REMOTE | DYNAMIC"
        string bind_host "e.g. 127.0.0.1"
        int bind_port "e.g. 8080"
        string target_host "e.g. 10.0.0.5"
        int target_port "e.g. 5432"
        boolean is_active "Live tunnel status"
        int pid "OS process PID if running"
    }

    HOST_TAG {
        string host_alias FK "References HOST.alias"
        string tag_name "e.g. prod, aws, database"
    }

    HOST_METRIC {
        string host_alias FK "References HOST.alias"
        int last_latency_ms "Last TCP handshake RTT in ms"
        boolean is_reachable "Last probe status"
        timestamp last_checked_at "Timestamp of last probe"
        timestamp last_connected_at "Last user connection time"
        int total_connections "Lifetime connection count"
    }

    SSH_IDENTITY {
        string file_path PK "Path to identity file (~/.ssh/id_ed25519)"
        string key_type "ED25519 | RSA | ECDSA"
        string fingerprint "SHA256 fingerprint"
        boolean is_agent_loaded "Loaded in ssh-agent socket"
        int file_permission "POSIX permission bits (e.g. 0600)"
    }
```

---

## 4. Terminal UI & The Elm Architecture

`lazyssh` employs the **Elm Architecture** (Model-Update-View) via Bubble Tea. This sequence diagram illustrates the unidirectional event loop and message passing:

```mermaid
sequenceDiagram
    autonumber
    actor User as User (Keyboard)
    participant Term as Terminal Device
    participant EventLoop as Bubble Tea Event Loop
    participant RootModel as Root Coordinator Model
    participant ActivePanel as Focused Panel (e.g. HostListView)
    participant Subsystem as Background Worker (tea.Cmd)
    participant Lipgloss as Lipgloss View Renderer

    User->>Term: KeyPress (e.g. 'j' to navigate, 'p' to ping)
    Term->>EventLoop: Emit tea.KeyMsg
    EventLoop->>RootModel: Update(tea.KeyMsg)
    RootModel->>ActivePanel: Route to focused child Update()

    alt Navigation Command ('j' / 'k')
        ActivePanel->>ActivePanel: Mutate cursor index
        ActivePanel-->>RootModel: Return updated PanelModel, nil
    else Asynchronous I/O Command ('p' ping)
        ActivePanel->>Subsystem: Dispatch async tea.Cmd (TCP Dial)
        ActivePanel-->>RootModel: Return PanelModel, tea.Cmd
        Subsystem->>Subsystem: Execute net.DialTimeout in Goroutine
        Subsystem-->>EventLoop: Emit tea.Msg(HostPingResult)
    end

    RootModel-->>EventLoop: Return RootModel, Cmd
    EventLoop->>Lipgloss: Call RootModel.View()
    Lipgloss-->>EventLoop: Formatted ANSI string
    EventLoop->>Term: Render terminal frame buffer to screen
```

---

## 5. Native SSH PTY Handover Lifecycle

The native terminal handover mechanism suspends the Bubble Tea event loop, executes the system `/usr/bin/ssh` binary with direct PTY ownership, and restores the TUI upon session completion:

```mermaid
sequenceDiagram
    autonumber
    actor User as User (Terminal)
    participant TUI as Bubble Tea Engine
    participant SessionSVC as SessionService
    participant KernelPTY as Linux PTY / Kernel TTY
    participant SSHProc as /usr/bin/ssh Client Process
    participant RemoteServer as Remote SSH Host (:22)

    User->>TUI: Presses 'Enter' on selected host (prod-db)
    TUI->>SessionSVC: BuildCommand(host)
    SessionSVC-->>TUI: Return *exec.Cmd ("/usr/bin/ssh", "prod-db")

    rect rgb(25, 35, 55)
        note over TUI, KernelPTY: Phase 1: Terminal Suspension
        TUI->>KernelPTY: Release Terminal Raw Mode (tcsetattr ICANON/ECHO)
        TUI->>KernelPTY: Restore default Stdin / Stdout / Stderr streams
        TUI->>KernelPTY: Make hardware cursor visible (\033[?25h)
    end

    rect rgb(20, 45, 30)
        note over KernelPTY, RemoteServer: Phase 2: Native SSH PTY Execution
        TUI->>SSHProc: Spawn process via tea.ExecProcess(cmd)
        SSHProc->>KernelPTY: Allocate Master/Slave PTY pair
        SSHProc->>RemoteServer: TCP Connect & SSH Handshake
        SSHProc->>RemoteServer: Authenticate (ssh-agent, YubiKey, Keys)
        RemoteServer-->>SSHProc: Interactive Remote Shell Session Granted
        User<-->>SSHProc: Full Interactive Shell (tmux, vim, htop, SIGWINCH resize)
    end

    rect rgb(45, 25, 25)
        note over User, SSHProc: Phase 3: Session Termination
        User->>SSHProc: Disconnects or types 'exit'
        SSHProc->>KernelPTY: Flush PTY buffers and close descriptors
        SSHProc-->>TUI: Process exits with exit code (0)
    end

    rect rgb(25, 35, 55)
        note over TUI, KernelPTY: Phase 4: Terminal Restoration
        TUI->>KernelPTY: Re-acquire Terminal Raw Mode
        TUI->>KernelPTY: Switch to Alternate Screen Buffer (\033[?1049h)
        TUI->>KernelPTY: Hide hardware cursor (\033[?25l)
        TUI->>User: Redraw lazyssh Dashboard immediately
    end
```

---

## 6. Terminal State Machine (UML State Diagram)

The operational states of the terminal interface during user interactions and process handover:

```mermaid
stateDiagram-v2
    [*] --> Initializing : Application Launch

    Initializing --> TUI_Dashboard : Config Loaded & Validated
    Initializing --> FatalError : Config Parse Failure

    state TUI_Dashboard {
        [*] --> BrowsingHosts
        BrowsingHosts --> FilteringSearch : Press '/'
        FilteringSearch --> BrowsingHosts : Press 'Esc' or 'Enter'
        BrowsingHosts --> InspectingDetails : Press 'Tab'
        InspectingDetails --> BrowsingHosts : Press 'Shift+Tab'
        BrowsingHosts --> ManagingTunnels : Press 't'
        ManagingTunnels --> BrowsingHosts : Press 'Esc'
        BrowsingHosts --> HelpModalActive : Press '?'
        HelpModalActive --> BrowsingHosts : Press '?' or 'Esc'
    }

    TUI_Dashboard --> SuspendingTerminal : Press 'Enter' (Connect Host)
    
    state PTY_Session {
        SuspendingTerminal --> NativeSSHRunning : Spawn /usr/bin/ssh
        NativeSSHRunning --> SessionExited : User types 'exit' or SSH error
        SessionExited --> RestoringTerminal : Capture Exit Code
    }

    RestoringTerminal --> TUI_Dashboard : Screen Redrawn

    TUI_Dashboard --> Terminating : Press 'q' or 'Ctrl+C'
    Terminating --> [*] : Clean Exit (Code 0)
    FatalError --> [*] : Exit with Error (Code 1)
```

---

## 7. Non-Destructive AST SSH Config Engine

How `lazyssh` reads, modifies, and safely writes `~/.ssh/config` while preserving all comments, indentation, and formatting:

```mermaid
flowchart TD
    ConfigPath["Read ~/.ssh/config"] --> Scanner["bufio.Scanner (Line by Line)"]

    subgraph LexerAndAST ["Phase 1: Tokenizer & AST Construction"]
        Scanner --> LineDecision{"Analyze Line Prefix"}
        LineDecision -- Starts with '#' --> Comment["Create AST Node: CommentNode\n(Stores exact text and comment prefix)"]
        LineDecision -- Whitespace / Blank --> Blank["Create AST Node: EmptyLineNode\n(Preserves blank line counts)"]
        LineDecision -- 'Host <alias>' --> HostHeader["Create AST Node: HostHeaderNode\n(Starts Host Block context)"]
        LineDecision -- 'Key Value' --> Directive["Create AST Node: DirectiveNode\n(Preserves indentation whitespace, Key, Value)"]
        LineDecision -- 'Include <pattern>' --> Include["Create AST Node: IncludeNode\n(Recursively parsed with cycle detection)"]
    end

    Comment --> ASTDoc["In-Memory AST Document"]
    Blank --> ASTDoc
    HostHeader --> ASTDoc
    Directive --> ASTDoc
    Include --> ASTDoc

    subgraph MutationPhase ["Phase 2: In-Memory AST Mutation"]
        ASTDoc --> UserEdit{"User Updates Host via TUI"}
        UserEdit --> LocateTarget["Locate Target HostHeaderNode by Alias"]
        LocateTarget --> ModifyDirectives["Update, Insert, or Delete DirectiveNode\n(Existing comments and formatting preserved)"]
    end

    subgraph SafeAtomicWrite ["Phase 3: Crash-Proof Atomic Persistence"]
        ModifyDirectives --> MakeTemp["Create Temp File: ~/.ssh/config.tmp.XXXXXX"]
        MakeTemp --> ChmodStrict["Apply Strict Permissions: 0600 (POSIX)"]
        ChmodStrict --> Serialize["Serialize AST Nodes sequentially into Temp File"]
        Serialize --> Fsync["Call file.Sync() (Flush OS buffers to physical disk)"]
        Fsync --> CloseFD["Close Temp File Descriptor"]
        CloseFD --> AtomicRename["os.Rename(tempPath, originalPath)\n(Atomic Inode Replacement)"]
    end

    AtomicRename --> Done["Success: Configuration Updated with Zero Data Corruption Risk"]
```

---

## 8. Bounded Concurrency & Health Prober Model

The architecture of the asynchronous TCP latency prober, preventing file descriptor exhaustion through bounded worker pooling:

```mermaid
flowchart LR
    subgraph UIThread ["Bubble Tea Main Thread"]
        Trigger["User presses 'P'\n(Batch Ping All)"]
        UpdateLoop["RootModel.Update(tea.Msg)"]
        View["Render Latency Badges\n(🟢 18ms | 🟡 85ms | 🔴 Timeout)"]
    end

    subgraph WorkerPoolEngine ["Bounded Worker Pool (Concurrency = 10)"]
        Dispatcher["Prober Dispatcher\n(Context with 3s Timeout)"]
        JobChan["Buffered Jobs Channel\n(chan HostProbeJob, buf=100)"]
        
        W1["Worker #1"]
        W2["Worker #2"]
        W3["Worker #3"]
        WN["Worker #10"]
        
        ResultChan["Results Channel\n(chan ProbeResult, buf=100)"]
    end

    subgraph RemoteNetwork ["Target SSH Endpoints"]
        Target1["Host 1 (:22)"]
        Target2["Host 2 (:22)"]
        Target3["Host 3 (:22)"]
        TargetN["Host N (:22)"]
    end

    Trigger -->|"1. Return tea.Cmd"| Dispatcher
    Dispatcher -->|"2. Fan-out Host jobs"| JobChan

    JobChan --> W1
    JobChan --> W2
    JobChan --> W3
    JobChan --> WN

    W1 -->|"3. TCP SYN / net.DialTimeout"| Target1
    W2 -->|"3. TCP SYN / net.DialTimeout"| Target2
    W3 -->|"3. TCP SYN / net.DialTimeout"| Target3
    WN -->|"3. TCP SYN / net.DialTimeout"| TargetN

    Target1 -->|"4. TCP SYN-ACK (RTT)"| W1
    Target2 -->|"4. TCP SYN-ACK (RTT)"| W2
    Target3 -->|"4. TCP SYN-ACK (RTT)"| W3
    TargetN -->|"4. TCP SYN-ACK (RTT)"| WN

    W1 --> ResultChan
    W2 --> ResultChan
    W3 --> ResultChan
    WN --> ResultChan

    ResultChan -->|"5. Emit tea.Msg(ProbeResult)"| UpdateLoop
    UpdateLoop -->|"6. Pure Render"| View
```

---

## 9. Tunnel Supervisor Lifecycle (UML State Diagram)

The state machine governing background SSH port forwarding tunnels (`-L`, `-R`, `-D`):

```mermaid
stateDiagram-v2
    [*] --> Inactive : Tunnel Created

    Inactive --> PreFlightCheck : User triggers Start ('s' / 'Enter')
    
    state PreFlightCheck {
        CheckLocalPort : Test local bind port via net.Listen()
        CheckHostReachability : Test SSH host reachability
    }

    PreFlightCheck --> PortConflict : Local port already occupied
    PortConflict --> Inactive : Display error notification

    PreFlightCheck --> SpawningProcess : Port free & host reachable

    state ActiveTunnel {
        SpawningProcess --> Running : Spawn ssh -f -N -L/-R/-D
        Running --> Monitoring : Periodic health probe (every 5s)
        Monitoring --> Running : Tunnel socket responding
    }

    Monitoring --> ConnectionDropped : Socket closed or ping failed

    state Recovery {
        ConnectionDropped --> Reconnecting : Auto-reconnect enabled
        Reconnecting --> SpawningProcess : Retry attempt <= 3
        Reconnecting --> TerminatedWithFailure : Max retries exceeded
    }

    Running --> Stopping : User triggers Stop
    Stopping --> Inactive : Process killed (SIGTERM / SIGKILL)
    TerminatedWithFailure --> Inactive : Acknowledge failure
```

---

## 10. Security Posture & Boundary Principles

1. **Zero Secret Persistence**:
   - `lazyssh` never reads, buffers, or stores private key material or SSH passphrases in application memory or disk. All cryptography is delegated to OpenSSH and `ssh-agent`.
2. **POSIX Permissions Enforcement**:
   - `~/.ssh/config` modifications are written with file mode `0600` (`-rw-------`). Directories are validated for `0700` (`drwx------`).
3. **Atomic Replacement Invariant**:
   - Direct file overwrites are forbidden. All file updates follow the strict: `TempFile -> Write -> fsync() -> Close -> os.Rename()` atomic transaction chain.
4. **Command Injection Prevention**:
   - External command execution forbids raw shell strings (`sh -c`). Commands are constructed as explicit parameter slices passed directly to system exec syscalls (`syscall.Execve` / `os/exec.Command`).
5. **Cycle Detection for Include Directives**:
   - The AST resolver maintains an in-memory hash set of visited canonical paths (`filepath.EvalSymlinks`) to detect and reject recursive file inclusions or malicious symlink loops.
