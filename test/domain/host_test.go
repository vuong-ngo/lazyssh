// ==============================================================================
// File: test/domain/host_test.go
// Project: lazyssh
// Description: Exhaustive unit tests for Host and PortForward domain entities,
//              invariants, validation boundaries, and SSH argument generation.
// Note: Uses 100% pure Go standard library (testing, errors) with zero external plugins.
// ==============================================================================

package domain_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/vuong-ngo/lazyssh/internal/domain"
)

func TestHost_Validate(t *testing.T) {
	tests := []struct {
		name    string
		host    domain.Host
		wantErr error
	}{
		{
			name: "valid minimal host",
			host: domain.Host{
				Alias:    "prod-server",
				HostName: "192.168.1.100",
			},
			wantErr: nil,
		},
		{
			name: "valid full host with all fields",
			host: domain.Host{
				Alias:         "bastion-01",
				HostName:      "bastion.example.com",
				User:          "ec2-user",
				Port:          2222,
				IdentityFiles: []string{"~/.ssh/id_ed25519", "~/.ssh/id_rsa"},
				ProxyJump:     "jumpbox",
				LocalForwards: []domain.PortForward{
					{
						Type:       domain.ForwardLocal,
						BindPort:   8080,
						TargetHost: "localhost",
						TargetPort: 80,
					},
				},
				RemoteForwards: []domain.PortForward{
					{
						Type:       domain.ForwardRemote,
						BindPort:   9090,
						TargetHost: "127.0.0.1",
						TargetPort: 9000,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "valid alias with allowed special characters",
			host: domain.Host{
				Alias: "prod_web-api.internal",
			},
			wantErr: nil,
		},
		{
			name: "empty alias string",
			host: domain.Host{
				Alias: "",
			},
			wantErr: domain.ErrEmptyAlias,
		},
		{
			name: "alias with spaces only",
			host: domain.Host{
				Alias: "    ",
			},
			wantErr: domain.ErrEmptyAlias,
		},
		{
			name: "alias with internal space",
			host: domain.Host{
				Alias: "prod web",
			},
			wantErr: domain.ErrInvalidAlias,
		},
		{
			name: "alias with internal tab",
			host: domain.Host{
				Alias: "prod\tweb",
			},
			wantErr: domain.ErrInvalidAlias,
		},
		{
			name: "alias with newline",
			host: domain.Host{
				Alias: "prod\nweb",
			},
			wantErr: domain.ErrInvalidAlias,
		},
		{
			name: "negative port number",
			host: domain.Host{
				Alias: "dev",
				Port:  -1,
			},
			wantErr: domain.ErrInvalidPort,
		},
		{
			name: "zero port (allowed, treated as unset/default 22)",
			host: domain.Host{
				Alias: "dev",
				Port:  0,
			},
			wantErr: nil,
		},
		{
			name: "port lower bound 1",
			host: domain.Host{
				Alias: "dev",
				Port:  1,
			},
			wantErr: nil,
		},
		{
			name: "port upper bound 65535",
			host: domain.Host{
				Alias: "dev",
				Port:  65535,
			},
			wantErr: nil,
		},
		{
			name: "port exceeding 65535",
			host: domain.Host{
				Alias: "dev",
				Port:  65536,
			},
			wantErr: domain.ErrInvalidPort,
		},
		{
			name: "host with invalid local forward port",
			host: domain.Host{
				Alias: "dev",
				LocalForwards: []domain.PortForward{
					{
						Type:       domain.ForwardLocal,
						BindPort:   0,
						TargetHost: "localhost",
						TargetPort: 80,
					},
				},
			},
			wantErr: domain.ErrInvalidPort,
		},
		{
			name: "host with invalid remote forward missing target",
			host: domain.Host{
				Alias: "dev",
				RemoteForwards: []domain.PortForward{
					{
						Type:       domain.ForwardRemote,
						BindPort:   8080,
						TargetHost: "",
						TargetPort: 80,
					},
				},
			},
			wantErr: domain.ErrInvalidForward,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.host.Validate()
			if tt.wantErr == nil && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestHost_EffectivePort(t *testing.T) {
	tests := []struct {
		name     string
		port     int
		expected int
	}{
		{name: "zero defaults to 22", port: 0, expected: 22},
		{name: "custom port 2222 preserved", port: 2222, expected: 2222},
		{name: "explicit standard port 22", port: 22, expected: 22},
		{name: "high port 44321", port: 44321, expected: 44321},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := domain.Host{Alias: "host", Port: tt.port}
			if got := h.EffectivePort(); got != tt.expected {
				t.Errorf("expected port %d, got %d", tt.expected, got)
			}
		})
	}
}

func TestHost_EffectiveHostName(t *testing.T) {
	tests := []struct {
		name     string
		alias    string
		hostName string
		expected string
	}{
		{
			name:     "falls back to alias when HostName is empty",
			alias:    "db-prod",
			hostName: "",
			expected: "db-prod",
		},
		{
			name:     "falls back to alias when HostName is whitespace only",
			alias:    "db-prod",
			hostName: "   ",
			expected: "db-prod",
		},
		{
			name:     "returns explicit HostName when provided",
			alias:    "db-prod",
			hostName: "10.0.1.50",
			expected: "10.0.1.50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := domain.Host{Alias: tt.alias, HostName: tt.hostName}
			if got := h.EffectiveHostName(); got != tt.expected {
				t.Errorf("expected HostName '%s', got '%s'", tt.expected, got)
			}
		})
	}
}

func TestHost_FormattedAddress(t *testing.T) {
	tests := []struct {
		name     string
		host     domain.Host
		expected string
	}{
		{
			name: "full user and custom port",
			host: domain.Host{
				Alias:    "api",
				HostName: "api.internal",
				User:     "deploy",
				Port:     8022,
			},
			expected: "deploy@api.internal:8022",
		},
		{
			name: "user with default port 22",
			host: domain.Host{
				Alias:    "api",
				HostName: "api.internal",
				User:     "ubuntu",
				Port:     0,
			},
			expected: "ubuntu@api.internal:22",
		},
		{
			name: "no user, fallback to alias",
			host: domain.Host{
				Alias: "web-direct",
				Port:  22,
			},
			expected: "web-direct:22",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.host.FormattedAddress(); got != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, got)
			}
		})
	}
}

func TestHost_SSHArgs(t *testing.T) {
	tests := []struct {
		name     string
		host     domain.Host
		expected []string
	}{
		{
			name: "minimal host (default port 22 is omitted from -p)",
			host: domain.Host{
				Alias: "simple-server",
			},
			expected: []string{"simple-server"},
		},
		{
			name: "host with non-standard port",
			host: domain.Host{
				Alias: "custom-port",
				Port:  2222,
			},
			expected: []string{"-p", "2222", "custom-port"},
		},
		{
			name: "host with user",
			host: domain.Host{
				Alias: "user-box",
				User:  "admin",
			},
			expected: []string{"-l", "admin", "user-box"},
		},
		{
			name: "host with multiple identity files and proxy jump",
			host: domain.Host{
				Alias:         "bastion-jump",
				User:          "root",
				Port:          2200,
				IdentityFiles: []string{"~/.ssh/id_rsa", "~/.ssh/id_ed25519"},
				ProxyJump:     "proxy-edge",
			},
			expected: []string{
				"-p", "2200",
				"-l", "root",
				"-i", "~/.ssh/id_rsa",
				"-i", "~/.ssh/id_ed25519",
				"-J", "proxy-edge",
				"bastion-jump",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.host.SSHArgs()
			if !reflect.DeepEqual(args, tt.expected) {
				t.Errorf("expected args %v, got %v", tt.expected, args)
			}
		})
	}
}

func TestPortForward_ValidationAndSpec(t *testing.T) {
	tests := []struct {
		name         string
		pf           domain.PortForward
		wantErr      error
		expectedSpec string
	}{
		{
			name: "valid local port forward with default bind host",
			pf: domain.PortForward{
				Type:       domain.ForwardLocal,
				BindPort:   8080,
				TargetHost: "localhost",
				TargetPort: 80,
			},
			wantErr:      nil,
			expectedSpec: "127.0.0.1:8080:localhost:80",
		},
		{
			name: "valid local port forward with explicit bind host",
			pf: domain.PortForward{
				Type:       domain.ForwardLocal,
				BindHost:   "0.0.0.0",
				BindPort:   3306,
				TargetHost: "db.internal",
				TargetPort: 3306,
			},
			wantErr:      nil,
			expectedSpec: "0.0.0.0:3306:db.internal:3306",
		},
		{
			name: "valid remote port forward",
			pf: domain.PortForward{
				Type:       domain.ForwardRemote,
				BindHost:   "127.0.0.1",
				BindPort:   9000,
				TargetHost: "192.168.1.50",
				TargetPort: 9000,
			},
			wantErr:      nil,
			expectedSpec: "127.0.0.1:9000:192.168.1.50:9000",
		},
		{
			name: "valid dynamic SOCKS5 proxy",
			pf: domain.PortForward{
				Type:     domain.ForwardDynamic,
				BindPort: 1080,
			},
			wantErr:      nil,
			expectedSpec: "127.0.0.1:1080",
		},
		{
			name: "invalid bind port zero",
			pf: domain.PortForward{
				Type:     domain.ForwardDynamic,
				BindPort: 0,
			},
			wantErr: domain.ErrInvalidPort,
		},
		{
			name: "invalid bind port out of range",
			pf: domain.PortForward{
				Type:     domain.ForwardDynamic,
				BindPort: 70000,
			},
			wantErr: domain.ErrInvalidPort,
		},
		{
			name: "invalid target port zero on local forward",
			pf: domain.PortForward{
				Type:       domain.ForwardLocal,
				BindPort:   8080,
				TargetHost: "localhost",
				TargetPort: 0,
			},
			wantErr: domain.ErrInvalidPort,
		},
		{
			name: "missing target host on remote forward",
			pf: domain.PortForward{
				Type:       domain.ForwardRemote,
				BindPort:   8080,
				TargetHost: "   ",
				TargetPort: 80,
			},
			wantErr: domain.ErrInvalidForward,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pf.Validate()
			if tt.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
			if tt.wantErr == nil && tt.expectedSpec != "" {
				if got := tt.pf.Spec(); got != tt.expectedSpec {
					t.Errorf("expected spec '%s', got '%s'", tt.expectedSpec, got)
				}
			}
		})
	}
}
