// ==============================================================================
// File: test/application/health_service_test.go
// Project: lazyssh
// Description: Unit tests for HealthService verifying single host probe,
//              batch probing, streaming, and error handling.
// ==============================================================================

package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vuong-ngo/lazyssh/internal/application"
	"github.com/vuong-ngo/lazyssh/internal/domain"
	"github.com/vuong-ngo/lazyssh/internal/infrastructure/network"
)

type mockProber struct {
	probeFunc func(ctx context.Context, target string, port int, timeout time.Duration) network.ProbeResult
}

func (m *mockProber) Probe(ctx context.Context, target string, port int, timeout time.Duration) network.ProbeResult {
	return m.probeFunc(ctx, target, port, timeout)
}

func TestHealthService_ProbeHost_Reachable(t *testing.T) {
	mock := &mockProber{
		probeFunc: func(ctx context.Context, target string, port int, timeout time.Duration) network.ProbeResult {
			return network.ProbeResult{
				Host:      target,
				Port:      port,
				Latency:   25 * time.Millisecond,
				Reachable: true,
			}
		},
	}

	service := application.NewHealthService(mock, 2, time.Second)

	host := &domain.Host{
		Alias:    "web-prod",
		HostName: "192.168.1.10",
		Port:     22,
	}

	err := service.ProbeHost(context.Background(), host)
	if err != nil {
		t.Fatalf("unexpected probe error: %v", err)
	}

	if !host.IsReachable {
		t.Error("expected host to be marked reachable")
	}
	if host.Latency != 25*time.Millisecond {
		t.Errorf("expected latency 25ms, got %v", host.Latency)
	}
}

func TestHealthService_ProbeHost_NilHost(t *testing.T) {
	service := application.NewHealthService(nil, 2, time.Second)
	err := service.ProbeHost(context.Background(), nil)
	if !errors.Is(err, domain.ErrHostNotFound) {
		t.Fatalf("expected ErrHostNotFound for nil host, got: %v", err)
	}
}

func TestHealthService_ProbeHost_InvalidHost(t *testing.T) {
	service := application.NewHealthService(nil, 2, time.Second)
	invalidHost := &domain.Host{
		Alias: "invalid with spaces",
	}

	err := service.ProbeHost(context.Background(), invalidHost)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !errors.Is(err, domain.ErrInvalidAlias) {
		t.Errorf("expected ErrInvalidAlias, got %v", err)
	}
}

func TestHealthService_ProbeBatch(t *testing.T) {
	mock := &mockProber{
		probeFunc: func(ctx context.Context, target string, port int, timeout time.Duration) network.ProbeResult {
			reachable := target != "unreachable.local"
			return network.ProbeResult{
				Host:      target,
				Port:      port,
				Latency:   15 * time.Millisecond,
				Reachable: reachable,
			}
		},
	}

	service := application.NewHealthService(mock, 3, time.Second)

	hosts := []domain.Host{
		{Alias: "srv1", HostName: "srv1.local", Port: 22},
		{Alias: "srv2", HostName: "unreachable.local", Port: 22},
	}

	updated := service.ProbeBatch(context.Background(), hosts)

	if len(updated) != 2 {
		t.Fatalf("expected 2 updated hosts, got %d", len(updated))
	}

	if !updated[0].IsReachable || updated[0].Latency != 15*time.Millisecond {
		t.Errorf("expected srv1 to be reachable with 15ms latency, got reachable=%v, lat=%v", updated[0].IsReachable, updated[0].Latency)
	}

	if updated[1].IsReachable {
		t.Errorf("expected srv2 to be unreachable")
	}
}

func TestHealthService_ProbeBatch_Empty(t *testing.T) {
	service := application.NewHealthService(nil, 2, time.Second)
	res := service.ProbeBatch(context.Background(), nil)
	if res != nil {
		t.Fatalf("expected nil for empty batch, got %v", res)
	}
}

func TestHealthService_ProbeStream(t *testing.T) {
	mock := &mockProber{
		probeFunc: func(ctx context.Context, target string, port int, timeout time.Duration) network.ProbeResult {
			return network.ProbeResult{
				Host:      target,
				Port:      port,
				Latency:   12 * time.Millisecond,
				Reachable: true,
			}
		},
	}

	service := application.NewHealthService(mock, 2, time.Second)

	hosts := []domain.Host{
		{Alias: "node-a", HostName: "10.0.0.1", Port: 22},
		{Alias: "node-b", HostName: "10.0.0.2", Port: 22},
	}

	stream := service.ProbeStream(context.Background(), hosts)

	count := 0
	for h := range stream {
		count++
		if !h.IsReachable || h.Latency != 12*time.Millisecond {
			t.Errorf("unexpected host data in stream: %+v", h)
		}
	}

	if count != 2 {
		t.Fatalf("expected 2 streamed hosts, got %d", count)
	}
}
