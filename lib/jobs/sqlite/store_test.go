package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	transliter "github.com/snowmerak/transliter/lib"
	"github.com/snowmerak/transliter/lib/jobs"
)

func TestOwnerHistoryUpdateAndExpiration(t *testing.T) {
	store, err := New(context.Background(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	alice, err := jobs.New("alice", jobs.Request{
		ModelCatalog: "hymt2-1.8b",
		Profile:      transliter.ProfileOfficial,
		Translation: transliter.TranslationRequest{
			Source:         "hello",
			SourceLanguage: transliter.LanguageEnglish,
			TargetLanguage: transliter.LanguageKorean,
			Kind:           transliter.PromptText,
		},
	}, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := jobs.New("bob", jobs.Request{ModelCatalog: "b"}, now.Add(time.Second), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), alice); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), bob); err != nil {
		t.Fatal(err)
	}

	history, err := store.List(context.Background(), "alice", jobs.ListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ID != alice.ID {
		t.Fatalf("owner history leaked or omitted jobs: %+v", history)
	}
	if history[0].Request.Translation.Source != "hello" {
		t.Fatalf("request round-trip failed: %+v", history[0].Request)
	}

	started := now.Add(time.Minute)
	completed := now.Add(2 * time.Minute)
	if err := store.Update(context.Background(), alice.ID, jobs.Update{
		Status: jobs.StatusSucceeded,
		Result: &jobs.Result{
			Translation:  "안녕하세요",
			Model:        "local",
			FinishReason: "stop",
			PromptTokens: 3,
			OutputTokens: 2,
			TotalTokens:  5,
		},
		UpdatedAt:   completed,
		StartedAt:   &started,
		CompletedAt: &completed,
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Get(context.Background(), alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != jobs.StatusSucceeded {
		t.Fatalf("status = %s", loaded.Status)
	}
	if loaded.Result == nil || loaded.Result.Translation != "안녕하세요" || loaded.Result.TotalTokens != 5 {
		t.Fatalf("result = %+v", loaded.Result)
	}
	if loaded.StartedAt == nil || !loaded.StartedAt.Equal(started) {
		t.Fatalf("started_at = %v want %v", loaded.StartedAt, started)
	}
	if loaded.CompletedAt == nil || !loaded.CompletedAt.Equal(completed) {
		t.Fatalf("completed_at = %v want %v", loaded.CompletedAt, completed)
	}

	if err := store.Update(context.Background(), "missing", jobs.Update{
		Status:    jobs.StatusFailed,
		Error:     "gone",
		UpdatedAt: now,
	}); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	deleted, err := store.DeleteExpired(context.Background(), now.Add(2*time.Hour))
	if err != nil || deleted != 2 {
		t.Fatalf("unexpected expiration result: deleted=%d err=%v", deleted, err)
	}
	if _, err := store.Get(context.Background(), alice.ID); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("expected expired job to be absent, got %v", err)
	}
}

func TestFractionalCreatedAtOrdering(t *testing.T) {
	store, err := New(context.Background(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	base := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	early := base.Add(100 * time.Millisecond) // same wall second as late if truncated poorly
	late := base.Add(900 * time.Millisecond)

	first, err := jobs.New("alice", jobs.Request{ModelCatalog: "a"}, early, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	second, err := jobs.New("alice", jobs.Request{ModelCatalog: "b"}, late, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), second); err != nil {
		t.Fatal(err)
	}

	// Newest first: late must precede early despite sharing the same second.
	history, err := store.List(context.Background(), "alice", jobs.ListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].ID != second.ID || history[1].ID != first.ID {
		t.Fatalf("fractional ordering broken: %+v", ids(history))
	}

	// before=late excludes late itself and keeps early.
	history, err = store.List(context.Background(), "alice", jobs.ListOptions{
		Limit:  10,
		Before: late,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ID != first.ID {
		t.Fatalf("before cursor ignored fractional ns: %+v", ids(history))
	}

	// expires_at equality boundary: DeleteExpired(before) uses <= .
	// first expires at early+1h; delete at that exact instant.
	deleted, err := store.DeleteExpired(context.Background(), first.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("expected exact expires_at boundary delete=1, got %d", deleted)
	}
	if _, err := store.Get(context.Background(), first.ID); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("first should be expired, got %v", err)
	}
	if _, err := store.Get(context.Background(), second.ID); err != nil {
		t.Fatalf("second should remain, got %v", err)
	}
}

func TestMemoryDSN(t *testing.T) {
	store, err := New(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	job, err := jobs.New("alice", jobs.Request{ModelCatalog: "m"}, time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != job.ID || loaded.OwnerID != "alice" {
		t.Fatalf("unexpected job: %+v", loaded)
	}
}

func ids(jobs []jobs.Job) []string {
	out := make([]string, len(jobs))
	for i, job := range jobs {
		out[i] = job.ID
	}
	return out
}
