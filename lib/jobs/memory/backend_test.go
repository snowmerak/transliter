package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/snowmerak/transliter/lib/jobs"
)

func TestOwnerHistoryAndExpiration(t *testing.T) {
	backend := New(2)
	now := time.Now().UTC()
	alice, err := jobs.New("alice", jobs.Request{Model: "a"}, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := jobs.New("bob", jobs.Request{Model: "b"}, now.Add(time.Second), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.Create(context.Background(), alice); err != nil {
		t.Fatal(err)
	}
	if err := backend.Create(context.Background(), bob); err != nil {
		t.Fatal(err)
	}
	history, err := backend.List(context.Background(), "alice", jobs.ListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ID != alice.ID {
		t.Fatalf("owner history leaked or omitted jobs: %+v", history)
	}
	deleted, err := backend.DeleteExpired(context.Background(), now.Add(2*time.Hour))
	if err != nil || deleted != 2 {
		t.Fatalf("unexpected expiration result: deleted=%d err=%v", deleted, err)
	}
	if _, err := backend.Get(context.Background(), alice.ID); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("expected expired job to be absent, got %v", err)
	}
}
