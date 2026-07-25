package natsjs

import (
	"context"
	"testing"
	"time"
)

func TestEmbeddedQueueRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	embedded, err := NewEmbedded(ctx, EmbeddedConfig{InMemory: true})
	if err != nil {
		t.Fatal(err)
	}
	defer embedded.Close()

	if err := embedded.Queue.Enqueue(ctx, "job-1"); err != nil {
		t.Fatal(err)
	}
	delivery, err := embedded.Queue.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if delivery.JobID() != "job-1" {
		t.Fatalf("unexpected job ID: %q", delivery.JobID())
	}
	if err := delivery.Ack(ctx); err != nil {
		t.Fatal(err)
	}
}
