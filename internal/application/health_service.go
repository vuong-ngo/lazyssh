// ==============================================================================
// File: internal/application/health_service.go
// Project: lazyssh
// Description: Application service coordinating TCP latency checks and batch
//              probing across domain Host entities with concurrency control.
// ==============================================================================

// Package application provides orchestration use cases for host inventory, SSH sessions, and latency monitoring.
package application

import (
	"context"
	"time"

	"github.com/vuong-ngo/lazyssh/internal/domain"
	"github.com/vuong-ngo/lazyssh/internal/infrastructure/network"
)

// HealthService orchestrates network reachability and latency probing for hosts.
type HealthService struct {
	prober network.Prober
	pool   *network.WorkerPool
}

// NewHealthService creates a HealthService instance with a configured worker pool.
func NewHealthService(prober network.Prober, concurrency int, timeout time.Duration) *HealthService {
	if prober == nil {
		prober = network.NewTCPProber()
	}
	pool := network.NewWorkerPool(prober, concurrency, timeout)
	return &HealthService{
		prober: prober,
		pool:   pool,
	}
}

// ProbeHost tests a single host's reachability and updates its Latency and IsReachable attributes.
func (s *HealthService) ProbeHost(ctx context.Context, host *domain.Host) error {
	if host == nil {
		return domain.ErrHostNotFound
	}
	if err := host.Validate(); err != nil {
		return err
	}

	target := host.EffectiveHostName()
	port := host.EffectivePort()

	result := s.prober.Probe(ctx, target, port, s.pool.Timeout())

	host.Latency = result.Latency
	host.IsReachable = result.Reachable

	return result.Err
}

// ProbeBatch concurrently probes all hosts in the slice and returns updated host entities.
func (s *HealthService) ProbeBatch(ctx context.Context, hosts []domain.Host) []domain.Host {
	if len(hosts) == 0 {
		return nil
	}

	results := s.pool.ProbeBatch(ctx, hosts)

	updated := make([]domain.Host, len(hosts))
	for i, h := range hosts {
		updated[i] = h
		if res, ok := results[h.Alias]; ok {
			updated[i].Latency = res.Latency
			updated[i].IsReachable = res.Reachable
		}
	}

	return updated
}

// ProbeStream concurrently probes hosts and streams each updated host as soon as it completes.
func (s *HealthService) ProbeStream(ctx context.Context, hosts []domain.Host) <-chan domain.Host {
	out := make(chan domain.Host, len(hosts))
	if len(hosts) == 0 {
		close(out)
		return out
	}

	stream := s.pool.ProbeStream(ctx, hosts)

	go func() {
		defer close(out)
		for item := range stream {
			h := item.Host
			h.Latency = item.Result.Latency
			h.IsReachable = item.Result.Reachable

			select {
			case <-ctx.Done():
				return
			case out <- h:
			}
		}
	}()

	return out
}
