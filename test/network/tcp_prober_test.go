// ==============================================================================
// File: test/network/tcp_prober_test.go
// Project: lazyssh
// Description: Comprehensive unit tests for TCP 3-way handshake latency prober
//              covering reachability, timeouts, loopback listeners, and error cases.
// ==============================================================================

package network_test

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vuong-ngo/lazyssh/internal/domain"
	"github.com/vuong-ngo/lazyssh/internal/infrastructure/network"
)

// startMockTCPServer starts a local TCP server on loopback and accepts incoming connections.
func startMockTCPServer(t *testing.T) (host string, port int, stop func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock TCP server: %v", err)
	}

	stopCh := make(chan struct{})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-stopCh:
					return
				default:
					return
				}
			}
			_ = conn.Close()
		}
	}()

	addr := listener.Addr().(*net.TCPAddr)
	stop = func() {
		close(stopCh)
		_ = listener.Close()
	}

	return addr.IP.String(), addr.Port, stop
}

func TestTCPProber_Probe_Reachable(t *testing.T) {
	host, port, stop := startMockTCPServer(t)
	defer stop()

	prober := network.NewTCPProber()
	ctx := context.Background()

	result := prober.Probe(ctx, host, port, 2*time.Second)

	if !result.Reachable {
		t.Fatalf("expected host %s:%d to be reachable, got error: %v", host, port, result.Err)
	}
	if result.Err != nil {
		t.Fatalf("expected nil error for reachable host, got: %v", result.Err)
	}
	if result.Latency <= 0 {
		t.Fatalf("expected positive latency duration, got %v", result.Latency)
	}
	expectedAddr := net.JoinHostPort(host, strconv.Itoa(port))
	if result.Address != expectedAddr {
		t.Errorf("expected address %q, got %q", expectedAddr, result.Address)
	}
	if result.Host != host || result.Port != port {
		t.Errorf("expected host=%q port=%d, got host=%q port=%d", host, port, result.Host, result.Port)
	}
}

func TestTCPProber_Probe_Unreachable_ClosedPort(t *testing.T) {
	// Find a free port by opening and immediately closing a listener
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	prober := network.NewTCPProber()
	ctx := context.Background()

	result := prober.Probe(ctx, "127.0.0.1", port, 1*time.Second)

	if result.Reachable {
		t.Fatalf("expected closed port %d to be unreachable", port)
	}
	if result.Err == nil {
		t.Fatal("expected non-nil error when connecting to closed port")
	}
	if result.Latency != 0 {
		t.Errorf("expected latency to be 0 for failed probe, got %v", result.Latency)
	}
}

func TestTCPProber_Probe_Timeout(t *testing.T) {
	// Custom dialFunc that blocks until context is cancelled, simulating network blackhole
	mockDial := func(ctx context.Context, network, address string) (net.Conn, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
			return nil, errors.New("timeout reached")
		}
	}

	prober := network.NewTCPProberWithDialer(mockDial)
	ctx := context.Background()

	// Use very short timeout (50ms)
	result := prober.Probe(ctx, "10.255.255.1", 22, 50*time.Millisecond)

	if result.Reachable {
		t.Fatal("expected timed out probe to be unreachable")
	}
	if result.Err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(result.Err, context.DeadlineExceeded) && !strings.Contains(result.Err.Error(), "context deadline exceeded") {
		t.Errorf("expected deadline exceeded error, got: %v", result.Err)
	}
}

func TestTCPProber_Probe_PreCancelledContext(t *testing.T) {
	prober := network.NewTCPProber()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result := prober.Probe(ctx, "127.0.0.1", 22, 1*time.Second)

	if result.Reachable {
		t.Fatal("expected probe with cancelled context to be unreachable")
	}
	if result.Err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

func TestTCPProber_Probe_InvalidInputs(t *testing.T) {
	prober := network.NewTCPProber()
	ctx := context.Background()

	tests := []struct {
		name        string
		target      string
		port        int
		timeout     time.Duration
		expectedErr error
	}{
		{
			name:        "empty target hostname",
			target:      "",
			port:        22,
			timeout:     time.Second,
			expectedErr: domain.ErrEmptyHostName,
		},
		{
			name:        "whitespace only target hostname",
			target:      "   \t\n  ",
			port:        22,
			timeout:     time.Second,
			expectedErr: domain.ErrEmptyHostName,
		},
		{
			name:        "port zero",
			target:      "127.0.0.1",
			port:        0,
			timeout:     time.Second,
			expectedErr: domain.ErrInvalidPort,
		},
		{
			name:        "port negative",
			target:      "127.0.0.1",
			port:        -1,
			timeout:     time.Second,
			expectedErr: domain.ErrInvalidPort,
		},
		{
			name:        "port exceeding 65535",
			target:      "127.0.0.1",
			port:        70000,
			timeout:     time.Second,
			expectedErr: domain.ErrInvalidPort,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := prober.Probe(ctx, tc.target, tc.port, tc.timeout)
			if res.Reachable {
				t.Fatalf("expected reachable=false for invalid input %q:%d", tc.target, tc.port)
			}
			if !errors.Is(res.Err, tc.expectedErr) {
				t.Fatalf("expected error wrapping %v, got %v", tc.expectedErr, res.Err)
			}
		})
	}
}

func TestTCPProber_Probe_DefaultTimeoutFallback(t *testing.T) {
	host, port, stop := startMockTCPServer(t)
	defer stop()

	prober := network.NewTCPProber()
	ctx := context.Background()

	// timeout <= 0 should fallback to DefaultProbeTimeout (3s) and still succeed on loopback
	result := prober.Probe(ctx, host, port, 0)
	if !result.Reachable {
		t.Fatalf("expected fallback to default timeout to succeed, got error: %v", result.Err)
	}

	resultNeg := prober.Probe(ctx, host, port, -10*time.Second)
	if !resultNeg.Reachable {
		t.Fatalf("expected negative timeout fallback to succeed, got error: %v", resultNeg.Err)
	}
}

func TestTCPProber_NewTCPProberWithDialer_NilFallback(t *testing.T) {
	// Passing nil dialer should safely fallback to default dialer without panicking
	prober := network.NewTCPProberWithDialer(nil)
	if prober == nil {
		t.Fatal("expected non-nil prober")
	}

	host, port, stop := startMockTCPServer(t)
	defer stop()

	res := prober.Probe(context.Background(), host, port, time.Second)
	if !res.Reachable {
		t.Fatalf("expected reachable host, got error: %v", res.Err)
	}
}
