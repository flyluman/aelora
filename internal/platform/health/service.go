package health

import (
	"context"
	"errors"
	"time"
)

type Check struct {
	Name string
	Func func(context.Context) error
}

type DependencyStatus struct {
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

type Report struct {
	Status       string                      `json:"status"`
	Dependencies map[string]DependencyStatus `json:"dependencies"`
}

type Service struct {
	timeout time.Duration
	checks  []Check
}

func NewService(timeout time.Duration, checks ...Check) *Service {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &Service{timeout: timeout, checks: checks}
}

func (s *Service) Check(ctx context.Context) Report {
	deps := make(map[string]DependencyStatus, len(s.checks))
	if len(s.checks) == 0 {
		return Report{Status: "ok", Dependencies: deps}
	}

	type result struct {
		name   string
		status DependencyStatus
	}
	results := make(chan result, len(s.checks))

	for _, dep := range s.checks {
		check := dep
		go func() {
			start := time.Now()
			checkCtx, cancel := context.WithTimeout(ctx, s.timeout)
			defer cancel()

			err := check.Func(checkCtx)
			status := DependencyStatus{
				Status:     "ok",
				DurationMS: time.Since(start).Milliseconds(),
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				status.Status = "down"
				status.Error = err.Error()
			}
			results <- result{name: check.Name, status: status}
		}()
	}

	overall := "ok"
	for range s.checks {
		r := <-results
		deps[r.name] = r.status
		if r.status.Status != "ok" {
			overall = "degraded"
		}
	}
	return Report{Status: overall, Dependencies: deps}
}
