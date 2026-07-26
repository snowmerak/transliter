package jobs

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Scheduler struct {
	Queue       Queue
	Store       Store
	Processor   Processor
	Concurrency int
	JobTimeout  time.Duration
	Now         func() time.Time
	OnError     func(error)
}

func (scheduler *Scheduler) Run(ctx context.Context) error {
	if scheduler.Queue == nil || scheduler.Store == nil || scheduler.Processor == nil {
		return fmt.Errorf("scheduler queue, store, and processor are required")
	}
	concurrency := scheduler.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	if scheduler.Now == nil {
		scheduler.Now = time.Now
	}
	var workers sync.WaitGroup
	workers.Add(concurrency)
	for range concurrency {
		go func() {
			defer workers.Done()
			scheduler.runWorker(ctx)
		}()
	}
	<-ctx.Done()
	workers.Wait()
	return nil
}

func (scheduler *Scheduler) runWorker(ctx context.Context) {
	for {
		delivery, err := scheduler.Queue.Receive(ctx)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, ErrClosed) {
				return
			}
			scheduler.report(fmt.Errorf("receive job: %w", err))
			continue
		}
		if delivery == nil {
			scheduler.report(fmt.Errorf("receive job: nil delivery"))
			continue
		}
		scheduler.handle(ctx, delivery)
	}
}

func (scheduler *Scheduler) handle(parent context.Context, delivery Delivery) {
	job, err := scheduler.Store.Get(parent, delivery.JobID())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			scheduler.report(delivery.Ack(parent))
			return
		}
		scheduler.report(fmt.Errorf("load job %s: %w", delivery.JobID(), err))
		scheduler.report(delivery.Nack(parent))
		return
	}
	if job.Status != StatusQueued && job.Status != StatusRunning {
		scheduler.report(delivery.Ack(parent))
		return
	}

	now := scheduler.Now().UTC()
	if job.Status == StatusQueued {
		job.StartedAt = &now
		if err := scheduler.Store.Update(parent, job.ID, Update{
			Status:    StatusRunning,
			UpdatedAt: now,
			StartedAt: &now,
		}); err != nil {
			scheduler.report(fmt.Errorf("mark job %s running: %w", job.ID, err))
			scheduler.report(delivery.Nack(parent))
			return
		}
	}

	ctx := parent
	cancel := func() {}
	if scheduler.JobTimeout > 0 {
		ctx, cancel = context.WithTimeout(parent, scheduler.JobTimeout)
	}
	result, processErr := scheduler.Processor.Process(ctx, job)
	cancel()

	completedAt := scheduler.Now().UTC()
	update := Update{
		Status:      StatusSucceeded,
		Result:      &result,
		UpdatedAt:   completedAt,
		CompletedAt: &completedAt,
	}
	if processErr != nil {
		update.Status = StatusFailed
		update.Result = nil
		update.Error = processErr.Error()
	}
	if err := scheduler.Store.Update(parent, job.ID, update); err != nil {
		scheduler.report(fmt.Errorf("complete job %s: %w", job.ID, err))
		scheduler.report(delivery.Nack(parent))
		return
	}
	scheduler.report(delivery.Ack(parent))
}

func (scheduler *Scheduler) report(err error) {
	if err != nil && scheduler.OnError != nil {
		scheduler.OnError(err)
	}
}
