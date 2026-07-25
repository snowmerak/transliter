// Package memory provides an in-process queue and job store.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/snowmerak/transliter/lib/jobs"
)

type Backend struct {
	mu     sync.RWMutex
	values map[string]jobs.Job
	queue  chan string
}

var (
	_ jobs.Queue = (*Backend)(nil)
	_ jobs.Store = (*Backend)(nil)
)

func New(queueCapacity int) *Backend {
	if queueCapacity <= 0 {
		queueCapacity = 1024
	}
	return &Backend{
		values: make(map[string]jobs.Job),
		queue:  make(chan string, queueCapacity),
	}
}

func (backend *Backend) Create(_ context.Context, job jobs.Job) error {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.values[job.ID] = jobs.Clone(job)
	return nil
}

func (backend *Backend) Get(_ context.Context, id string) (jobs.Job, error) {
	backend.mu.RLock()
	defer backend.mu.RUnlock()
	job, ok := backend.values[id]
	if !ok {
		return jobs.Job{}, jobs.ErrNotFound
	}
	return jobs.Clone(job), nil
}

func (backend *Backend) Update(_ context.Context, id string, update jobs.Update) error {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	job, ok := backend.values[id]
	if !ok {
		return jobs.ErrNotFound
	}
	job.Status = update.Status
	job.Result = update.Result
	job.Error = update.Error
	job.UpdatedAt = update.UpdatedAt
	if update.StartedAt != nil {
		job.StartedAt = update.StartedAt
	}
	if update.CompletedAt != nil {
		job.CompletedAt = update.CompletedAt
	}
	backend.values[id] = jobs.Clone(job)
	return nil
}

func (backend *Backend) List(
	_ context.Context,
	ownerID string,
	options jobs.ListOptions,
) ([]jobs.Job, error) {
	backend.mu.RLock()
	defer backend.mu.RUnlock()
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	values := make([]jobs.Job, 0)
	for _, job := range backend.values {
		if job.OwnerID != ownerID {
			continue
		}
		if !options.Before.IsZero() && !job.CreatedAt.Before(options.Before) {
			continue
		}
		values = append(values, jobs.Clone(job))
	}
	sort.Slice(values, func(left, right int) bool {
		return values[left].CreatedAt.After(values[right].CreatedAt)
	})
	if len(values) > limit {
		values = values[:limit]
	}
	return values, nil
}

func (backend *Backend) DeleteExpired(_ context.Context, before time.Time) (int64, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	var deleted int64
	for id, job := range backend.values {
		if !job.ExpiresAt.After(before) {
			delete(backend.values, id)
			deleted++
		}
	}
	return deleted, nil
}

func (backend *Backend) Enqueue(ctx context.Context, jobID string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case backend.queue <- jobID:
		return nil
	}
}

func (backend *Backend) Receive(ctx context.Context) (jobs.Delivery, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case id := <-backend.queue:
		return &delivery{backend: backend, id: id}, nil
	}
}

type delivery struct {
	backend *Backend
	id      string
}

func (value *delivery) JobID() string {
	return value.id
}

func (*delivery) Ack(context.Context) error {
	return nil
}

func (value *delivery) Nack(ctx context.Context) error {
	return value.backend.Enqueue(ctx, value.id)
}
