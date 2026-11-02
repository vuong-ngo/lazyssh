// ==============================================================================
// File: internal/infrastructure/network/tcp_prober.go
// Project: lazyssh
// Description: Pure Go TCP 3-way handshake latency prober measuring round-trip
//              time without performing full SSH cryptographic authentication.
// ==============================================================================

package network

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/vuong-ngo/lazyssh/internal/domain"
)

// DefaultProbeTimeout is the fallback timeout if a non-positive duration is passed.
const DefaultProbeTimeout = 3 * time.Second

// ProbeResult contains the diagnostic measurements and status of a TCP probe.
type ProbeResult struct {
	Host      string        `json:"host"`
	Port      int           `json:"port"`
	Address   string        `json:"address"`
	Latency   time.Duration `json:"latency"`
	Reachable bool          `json:"reachable"`
	Err       error         `json:"error,omitempty"`
}

// Prober defines the contract for network reachability and latency probing.
type Prober interface {
	// Probe executes a reachability test against the target host and port.
	Probe(ctx context.Context, target string, port int, timeout time.Duration) ProbeResult
}

// DialContextFunc matches the signature of net.Dialer.DialContext.
type DialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

// TCPProber implements Prober using pure Go standard library TCP sockets.
type TCPProber struct {
	dialFunc DialContextFunc
}

// NewTCPProber creates a standard TCPProber with default OS network dialing.
func NewTCPProber() *TCPProber {
	dialer := &net.Dialer{}
	return &TCPProber{
		dialFunc: dialer.DialContext,
	}
}

// NewTCPProberWithDialer creates a TCPProber using a custom dialing function (useful for mocks/tests).
func NewTCPProberWithDialer(dialFunc DialContextFunc) *TCPProber {
	if dialFunc == nil {
		dialer := &net.Dialer{}
		dialFunc = dialer.DialContext
	}
	return &TCPProber{
		dialFunc: dialFunc,
	}
}

// Probe performs a non-blocking TCP 3-way handshake to measure latency.
// It connects, records the elapsed round-trip time, immediately closes the socket,
// and returns the diagnostic ProbeResult.
func (p *TCPProber) Probe(ctx context.Context, target string, port int, timeout time.Duration) ProbeResult {
	cleanTarget := strings.TrimSpace(target)
	if cleanTarget == "" {
		return ProbeResult{
			Host:      target,
			Port:      port,
			Reachable: false,
			Err:       domain.ErrEmptyHostName,
		}
	}

	if port < 1 || port > 65535 {
		return ProbeResult{
			Host:      cleanTarget,
			Port:      port,
			Reachable: false,
			Err:       fmt.Errorf("%w: %d", domain.ErrInvalidPort, port),
		}
	}

	if timeout <= 0 {
		timeout = DefaultProbeTimeout
	}

	addr := net.JoinHostPort(cleanTarget, strconv.Itoa(port))

	// Enforce the probe deadline via context
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	conn, err := p.dialFunc(probeCtx, "tcp", addr)
	elapsed := time.Since(start)

	if err != nil {
		return ProbeResult{
			Host:      cleanTarget,
			Port:      port,
			Address:   addr,
			Latency:   0,
			Reachable: false,
			Err:       err,
		}
	}

	_ = conn.Close()

	return ProbeResult{
		Host:      cleanTarget,
		Port:      port,
		Address:   addr,
		Latency:   elapsed,
		Reachable: true,
		Err:       nil,
	}
}
