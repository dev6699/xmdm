package adminhttp

import (
	"context"
	"sync"
	"time"
)

const dashboardHealthTimeout = 3 * time.Second

func collectServiceHealthChecks(ctx context.Context, deps DashboardDependencies) []serviceHealthCheck {
	probes := []struct {
		label string
		check func(context.Context) error
	}{
		{
			label: "PostgreSQL",
			check: func(ctx context.Context) error {
				if deps.Database == nil {
					return nil
				}
				return deps.Database.Ping(ctx)
			},
		},
		{
			label: "Object storage",
			check: func(ctx context.Context) error {
				if deps.Artifacts == nil {
					return nil
				}
				return deps.Artifacts.HealthCheck(ctx)
			},
		},
		{
			label: "MQTT",
			check: func(ctx context.Context) error {
				if deps.PushHealth == nil {
					return nil
				}
				return deps.PushHealth.HealthCheck(ctx)
			},
		},
	}

	type result struct {
		label string
		tone  string
	}

	ctx, cancel := context.WithTimeout(ctx, dashboardHealthTimeout)
	defer cancel()

	results := make([]result, len(probes))
	var wg sync.WaitGroup
	for i, probe := range probes {
		if probe.check == nil {
			continue
		}
		wg.Add(1)
		go func(i int, probe struct {
			label string
			check func(context.Context) error
		}) {
			defer wg.Done()
			results[i] = result{
				label: probe.label,
				tone:  "good",
			}
			if err := probe.check(ctx); err != nil {
				results[i].tone = "danger"
				if ctx.Err() != nil {
					results[i].tone = "warn"
				}
			}
		}(i, probe)
	}
	wg.Wait()

	checks := make([]serviceHealthCheck, 0, len(results))
	for _, result := range results {
		if result.label == "" {
			continue
		}
		checks = append(checks, serviceHealthCheck{
			Label: result.label,
			Tone:  result.tone,
		})
	}
	return checks
}
