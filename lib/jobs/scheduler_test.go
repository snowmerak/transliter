package jobs_test

import (
	"context"
	"testing"
	"time"

	transliter "github.com/snowmerak/transliter/lib"
	"github.com/snowmerak/transliter/lib/jobs"
	"github.com/snowmerak/transliter/lib/jobs/memory"
)

type processorFunc func(context.Context, jobs.Job) (jobs.Result, error)

func (function processorFunc) Process(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return function(ctx, job)
}

func TestSchedulerProcessesQueuedJob(t *testing.T) {
	backend := memory.New(4)
	job, err := jobs.New("alice", jobs.Request{
		Model: "test-model",
		Translation: transliter.TranslationRequest{
			Source:         "hello",
			TargetLanguage: transliter.LanguageKorean,
		},
	}, time.Now(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.Create(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if err := backend.Enqueue(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	scheduler := &jobs.Scheduler{
		Queue: backend,
		Store: backend,
		Processor: processorFunc(func(context.Context, jobs.Job) (jobs.Result, error) {
			return jobs.Result{Translation: "안녕하세요"}, nil
		}),
	}
	go func() { done <- scheduler.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for {
		stored, err := backend.Get(context.Background(), job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Status == jobs.StatusSucceeded {
			if stored.Result == nil || stored.Result.Translation != "안녕하세요" {
				t.Fatalf("unexpected result: %+v", stored.Result)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not complete: %+v", stored)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
