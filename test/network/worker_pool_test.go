// ==============================================================================
// File: test/network/worker_pool_test.go
// Project: lazyssh
// Description: Unit tests for the bounded concurrency worker pool verifying
//              concurrency limits, batch probing, streaming, and cancellation.
// ==============================================================================

package network_test

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vuong-ngo/lazyssh/internal/domain"
	"github.com/vuong-ngo/lazyssh/internal/infrastructure/network"
)

type mockProber struct {
	probeFunc func(ctx context.Context, target string, port int, timeout time.Duration) network.ProbeResult
}

func (m *mockProber) Probe(ctx context.Context, target string, port int, timeout time.Duration) network.ProbeResult {
	return m.probeFunc(ctx, target, port, timeout)
}

func TestWorkerPool_ConfigurationDefaults(t *testing.T) {
	pool := network.NewWorkerPool(nil, -5, -10*time.Second)

	if pool.Concurrency() != network.DefaultConcurrency {
		t.Errorf("expected default concurrency %d, got %d", network.DefaultConcurrency, pool.Concurrency())
	}
	if pool.Timeout() != network.DefaultProbeTimeout {
		t.Errorf("expected default timeout %v, got %v", network.DefaultProbeTimeout, pool.Timeout())
	}
}

func TestWorkerPool_ProbeBatch_Success(t *testing.T) {
	host1, port1, stop1 := startMockTCPServer(t)
	defer stop1()

	host2, port2, stop2 := startMockTCPServer(t)
	defer stop2()

	// Get an unused port for host3 to simulate an unreachable server
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve port: %v", err)
	}
	unreachablePort := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	hosts := []domain.Host{
		{
			Alias:    "srv1",
			HostName: host1,
			Port:     port1,
		},
		{
			Alias:    "srv2",
			HostName: host2,
			Port:     port2,
		},
		{
			Alias:    "srv3",
			HostName: "127.0.0.1",
			Port:     unreachablePort,
		},
	}

	prober := network.NewTCPProber()
	pool := network.NewWorkerPool(prober, 2, 1*time.Second)

	results := pool.ProbeBatch(context.Background(), hosts)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	res1, ok := results["srv1"]
	if !ok || !res1.Reachable {
		t.Errorf("expected srv1 to be reachable, got %+v", res1)
	}

	res2, ok := results["srv2"]
	if !ok || !res2.Reachable {
		t.Errorf("expected srv2 to be reachable, got %+v", res2)
	}

	res3, ok := results["srv3"]
	if !ok || res3.Reachable {
		t.Errorf("expected srv3 to be unreachable, got %+v", res3)
	}
}

func TestWorkerPool_ConcurrencyBound(t *testing.T) {
	const maxAllowedConcurrency int32 = 3
	const totalJobs = 15

	var currentActive int32
	var peakActive int32

	mock := &mockProber{
		probeFunc: func(ctx context.Context, target string, port int, timeout time.Duration) network.ProbeResult {
			active := atomic.AddInt32(&currentActive, 1)

			// Track peak concurrent workers
			for {
				oldPeak := atomic.LoadInt32(&peakActive)
				if active <= oldPeak {
					break
				}
				if atomic.CompareAndSwapInt32(&peakActive, oldPeak, active) {
					break
				}
			}

			// Simulate work
			time.Sleep(20 * time.Millisecond)

			atomic.AddInt32(&currentActive, -1)
			return network.ProbeResult{
				Host:      target,
				Port:      port,
				Reachable: true,
				Latency:   5 * time.Millisecond,
			}
		},
	}

	pool := network.NewWorkerPool(mock, int(maxAllowedConcurrency), time.Second)

	hosts := make([]domain.Host, totalJobs)
	for i := 0; i < totalJobs; i++ {
		hosts[i] = domain.Host{
			Alias:    fmt.Sprintf("host-%d", i),
			HostName: fmt.Sprintf("192.168.1.%d", i),
			Port:     22,
		}
	}

	results := pool.ProbeBatch(context.Background(), hosts)

	if len(results) != totalJobs {
		t.Fatalf("expected %d results, got %d", totalJobs, len(results))
	}

	peak := atomic.LoadInt32(&peakActive)
	if peak > maxAllowedConcurrency {
		t.Fatalf("concurrency limit exceeded: peak was %d, max allowed was %d", peak, maxAllowedConcurrency)
	}
	if peak < 2 {
		t.Logf("note: peak concurrency was %d (within limit %d)", peak, maxAllowedConcurrency)
	}
}

func TestWorkerPool_ContextCancellation(t *testing.T) {
	mock := &mockProber{
		probeFunc: func(ctx context.Context, target string, port int, timeout time.Duration) network.ProbeResult {
			select {
			case <-ctx.Done():
				return network.ProbeResult{
					Host:      target,
					Port:      port,
					Reachable: false,
					Err:       ctx.Err(),
				}
			case <-time.After(500 * time.Millisecond):
				return network.ProbeResult{
					Host:      target,
					Port:      port,
					Reachable: true,
				}
			}
		},
	}

	pool := network.NewWorkerPool(mock, 2, time.Second)

	hosts := make([]domain.Host, 10)
	for i := 0; i < 10; i++ {
		hosts[i] = domain.Host{
			Alias:    fmt.Sprintf("host-%d", i),
			HostName: "10.0.0.1",
			Port:     22,
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after short delay
	time.AfterFunc(30*time.Millisecond, cancel)

	doneCh := make(chan struct{})
	go func() {
		_ = pool.ProbeBatch(ctx, hosts)
		close(doneCh)
	}()

	select {
	case <-doneCh:
		// Succeeded in terminating on context cancellation
	case <-time.After(2 * time.Second):
		t.Fatal("worker pool did not terminate promptly upon context cancellation")
	}
}

func TestWorkerPool_EmptyHosts(t *testing.T) {
	pool := network.NewWorkerPool(nil, 5, time.Second)

	results := pool.ProbeBatch(context.Background(), nil)
	if len(results) != 0 {
		t.Fatalf("expected empty result map for nil hosts, got %d items", len(results))
	}

	resultsEmpty := pool.ProbeBatch(context.Background(), []domain.Host{})
	if len(resultsEmpty) != 0 {
		t.Fatalf("expected empty result map for empty slice, got %d items", len(resultsEmpty))
	}

	stream := pool.ProbeStream(context.Background(), nil)
	count := 0
	for range stream {
		count++
	}
	if count != 0 {
		t.Fatalf("expected 0 items streamed for nil hosts, got %d", count)
	}
}

func TestWorkerPool_ProbeStream(t *testing.T) {
	mock := &mockProber{
		probeFunc: func(ctx context.Context, target string, port int, timeout time.Duration) network.ProbeResult {
			return network.ProbeResult{
				Host:      target,
				Port:      port,
				Reachable: true,
				Latency:   10 * time.Millisecond,
			}
		},
	}

	pool := network.NewWorkerPool(mock, 3, time.Second)

	hosts := []domain.Host{
		{Alias: "srv-a", HostName: "10.0.0.1", Port: 22},
		{Alias: "srv-b", HostName: "10.0.0.2", Port: 22},
		{Alias: "srv-c", HostName: "10.0.0.3", Port: 22},
	}

	stream := pool.ProbeStream(context.Background(), hosts)

	seen := make(map[string]bool)
	for item := range stream {
		seen[item.Alias] = true
		if !item.Result.Reachable {
			t.Errorf("expected reachable for %s", item.Alias)
		}
	}

	if len(seen) != 3 {
		t.Fatalf("expected 3 streamed items, got %d", len(seen))
	}
	for _, h := range hosts {
		if !seen[h.Alias] {
			t.Errorf("host %s was not received from stream", h.Alias)
		}
	}
}
