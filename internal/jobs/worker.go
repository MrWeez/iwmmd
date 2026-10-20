// Package jobs runs the durable check queue and scheduler.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/mrweez/iwmmd/internal/domain"
	"github.com/mrweez/iwmmd/internal/platform/ids"
	"github.com/mrweez/iwmmd/internal/store"
)

// Worker claims and executes durable jobs.
type Worker struct {
	Store       *store.Store
	Domains     *domain.Service
	Logger      *slog.Logger
	Concurrency int
	Interval    time.Duration
}

// Run starts the scheduler and worker pool.
func (w *Worker) Run(ctx context.Context) {
	concurrency := w.Concurrency
	if concurrency < 1 {
		concurrency = 4
	}
	interval := w.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	workerID := fmt.Sprintf("%s-%s", hostname(), ids.New())
	for i := 0; i < concurrency; i++ {
		go w.loop(ctx, workerID)
	}
	if _, err := w.EnqueueDue(ctx, time.Now().UTC()); err != nil && !errors.Is(err, context.Canceled) {
		w.logger().Error("enqueue due checks", "error", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := w.EnqueueDue(ctx, time.Now().UTC()); err != nil && !errors.Is(err, context.Canceled) {
				w.logger().Error("enqueue due checks", "error", err)
			}
		}
	}
}

// EnqueueDue queues checks for active domains whose next check has passed.
func (w *Worker) EnqueueDue(ctx context.Context, now time.Time) (int, error) {
	domains, err := w.Store.DueDomains(ctx, now, 200)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, d := range domains {
		body, err := json.Marshal(map[string]string{"domain_id": d.ID})
		if err != nil {
			return created, err
		}
		_, wasCreated, err := w.Store.EnqueueJob(ctx, store.EnqueueJobInput{
			Kind:        "domain.check",
			PayloadJSON: string(body),
			RunAt:       now,
			MaxAttempts: 5,
			UniqueKey:   "domain-check:" + d.ID,
		})
		if err != nil {
			return created, err
		}
		if wasCreated {
			created++
		}
	}
	return created, nil
}

func (w *Worker) loop(ctx context.Context, workerID string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		job, err := w.Store.ClaimJob(ctx, workerID, time.Now().UTC(), 5*time.Minute)
		if err != nil && !errors.Is(err, context.Canceled) {
			w.logger().Error("claim job", "error", err)
			if !sleep(ctx, 2*time.Second) {
				return
			}
			continue
		}
		if job == nil {
			if !sleep(ctx, 2*time.Second) {
				return
			}
			continue
		}
		w.runJob(ctx, *job)
	}
}

func (w *Worker) runJob(ctx context.Context, job store.Job) {
	jobCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	err := w.process(jobCtx, job)
	now := time.Now().UTC()
	if err == nil {
		if err := w.Store.CompleteJob(ctx, job.ID, now); err != nil {
			w.logger().Error("complete job", "job_id", job.ID, "error", err)
		}
		return
	}
	if errors.Is(err, context.Canceled) && ctx.Err() != nil {
		return
	}
	message := err.Error()
	retryAt := now.Add(jobRetryDelay(job.Attempts))
	if err := w.Store.FailJob(ctx, job.ID, message, retryAt, now); err != nil {
		w.logger().Error("fail job", "job_id", job.ID, "error", err)
	}
	w.logger().Warn("job failed", "job_id", job.ID, "kind", job.Kind, "attempt", job.Attempts, "error", err)
}

func (w *Worker) process(ctx context.Context, job store.Job) error {
	switch job.Kind {
	case "domain.check":
		var payload struct {
			DomainID string `json:"domain_id"`
		}
		if err := json.Unmarshal([]byte(job.PayloadJSON), &payload); err != nil {
			return fmt.Errorf("decode domain.check payload: %w", err)
		}
		if payload.DomainID == "" {
			return errors.New("domain.check payload is missing domain_id")
		}
		_, err := w.Domains.CheckAndRecord(ctx, payload.DomainID, time.Now().UTC())
		if errors.Is(err, domain.ErrInvalidState) {
			return nil
		}
		return err
	default:
		return fmt.Errorf("unknown job kind %q", job.Kind)
	}
}

func jobRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := attempt - 1
	if shift > 4 {
		shift = 4
	}
	delay := time.Minute * time.Duration(1<<shift)
	if delay > 15*time.Minute {
		delay = 15 * time.Minute
	}
	return delay
}

func sleep(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func hostname() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		return "worker"
	}
	return host
}

func (w *Worker) logger() *slog.Logger {
	if w.Logger != nil {
		return w.Logger
	}
	return slog.Default()
}
