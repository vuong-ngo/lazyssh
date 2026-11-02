// ==============================================================================
// File: internal/infrastructure/network/worker_pool.go
// Project: lazyssh
// Description: Bounded concurrency worker pool for parallel batch latency probing
//              with context cancellation and streamable result channels.
// ==============================================================================

package network

import (
	"context"
	"sync"
	"time"

	"github.com/vuong-ngo/lazyssh/internal/domain"
)

// DefaultConcurrency defines the default number of parallel prober workers.
const DefaultConcurrency = 10

// HostProbeResult couples a host alias with its measured TCP probe result.
type HostProbeResult struct {
	Alias  string
	Host   domain.Host
	Result ProbeResult
}

// WorkerPool coordinates parallel host reachability probing within a bounded worker limit.
type WorkerPool struct {
	prober      Prober
	concurrency int
	timeout     time.Duration
}

// NewWorkerPool creates a new bounded worker pool.
// If concurrency <= 0, it defaults to DefaultConcurrency (10).
// If timeout <= 0, it defaults to DefaultProbeTimeout (3s).
func NewWorkerPool(prober Prober, concurrency int, timeout time.Duration) *WorkerPool {
	if prober == nil {
		prober = NewTCPProber()
	}
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}
	if timeout <= 0 {
		timeout = DefaultProbeTimeout
	}
	return &WorkerPool{
		prober:      prober,
		concurrency: concurrency,
		timeout:     timeout,
	}
}

// Concurrency returns the configured maximum number of concurrent workers.
func (wp *WorkerPool) Concurrency() int {
	return wp.concurrency
}

// Timeout returns the configured per-probe timeout duration.
func (wp *WorkerPool) Timeout() time.Duration {
	return wp.timeout
}

// ProbeBatch executes batch probes across all provided hosts concurrently,
// blocking until all probes complete or the context is cancelled.
// It returns a map of host alias to ProbeResult.
func (wp *WorkerPool) ProbeBatch(ctx context.Context, hosts []domain.Host) map[string]ProbeResult {
	results := make(map[string]ProbeResult, len(hosts))
	if len(hosts) == 0 {
		return results
	}

	stream := wp.ProbeStream(ctx, hosts)
	for res := range stream {
		results[res.Alias] = res.Result
	}
	return results
}

// ProbeStream launches the bounded worker pool and streams results over a read-only channel
// as each host probe finishes. The channel is closed once all probes finish or upon context cancellation.
func (wp *WorkerPool) ProbeStream(ctx context.Context, hosts []domain.Host) <-chan HostProbeResult {
	out := make(chan HostProbeResult, len(hosts))

	if len(hosts) == 0 {
		close(out)
		return out
	}

	jobs := make(chan domain.Host, len(hosts))
	for _, h := range hosts {
		jobs <- h
	}
	close(jobs)

	// Determine actual worker count: min(concurrency, len(hosts))
	workerCount := wp.concurrency
	if len(hosts) < workerCount {
		workerCount = len(hosts)
	}

	var wg sync.WaitGroup
	wg.Add(workerCount)

	for i := 0; i < workerCount; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case host, ok := <-jobs:
					if !ok {
						return
					}

					target := host.EffectiveHostName()
					port := host.EffectivePort()

					res := wp.prober.Probe(ctx, target, port, wp.timeout)

					select {
					case <-ctx.Done():
						return
					case out <- HostProbeResult{
						Alias:  host.Alias,
						Host:   host,
						Result: res,
					}:
					}
				}
			}
		}()
	}

	// Coordinator goroutine to close the output channel when all workers finish
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
