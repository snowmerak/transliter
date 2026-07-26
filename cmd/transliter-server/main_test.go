package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	transliter "github.com/snowmerak/transliter/lib"
	"github.com/snowmerak/transliter/lib/jobs"
)

func TestConfigFromEnv(t *testing.T) {
	t.Setenv(envQueueBackend, "nats-embedded")
	t.Setenv(envStoreBackend, "postgres")
	t.Setenv(envWorkers, "4")
	t.Setenv(envJobTimeout, "3m")
	t.Setenv(envJobRetention, "2160h")

	config, err := configFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if config.QueueBackend != "nats-embedded" ||
		config.StoreBackend != "postgres" ||
		config.Workers != 4 ||
		config.JobTimeout != 3*time.Minute ||
		config.JobRetention != 90*24*time.Hour {
		t.Fatalf("unexpected config: %+v", config)
	}
}

func TestConfigFromEnvDefaults(t *testing.T) {
	t.Setenv(envQueueBackend, "")
	t.Setenv(envStoreBackend, "")
	t.Setenv(envSQLitePath, "")
	t.Setenv(envNATSEmbeddedMemory, "")
	t.Setenv(envHTTPAddress, "")
	t.Setenv(envWorkers, "")
	t.Setenv(envJobTimeout, "")
	t.Setenv(envJobRetention, "")

	config, err := configFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if config.QueueBackend != "nats-embedded" {
		t.Fatalf("QueueBackend = %q, want nats-embedded", config.QueueBackend)
	}
	if config.StoreBackend != "sqlite" {
		t.Fatalf("StoreBackend = %q, want sqlite", config.StoreBackend)
	}
	if config.SQLitePath != "transliter-jobs.db" {
		t.Fatalf("SQLitePath = %q, want transliter-jobs.db", config.SQLitePath)
	}
	if !config.NATSEmbeddedMemory {
		t.Fatal("NATSEmbeddedMemory = false, want true")
	}
	if config.HTTPAddress != ":8080" {
		t.Fatalf("HTTPAddress = %q, want :8080", config.HTTPAddress)
	}
	if config.Workers != 1 {
		t.Fatalf("Workers = %d, want 1", config.Workers)
	}
	if config.JobTimeout != 10*time.Minute {
		t.Fatalf("JobTimeout = %s, want 10m", config.JobTimeout)
	}
	if config.JobRetention != 30*24*time.Hour {
		t.Fatalf("JobRetention = %s, want 720h", config.JobRetention)
	}
}

func TestBuildDefaultBackends(t *testing.T) {
	t.Setenv(envQueueBackend, "")
	t.Setenv(envStoreBackend, "")
	t.Setenv(envSQLitePath, "")
	t.Setenv(envNATSEmbeddedMemory, "")

	cfg, err := configFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SQLitePath = filepath.Join(t.TempDir(), "jobs.db")
	cfg.JobTimeout = time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	backends, err := buildBackends(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer backends.Close()
	if backends.queue == nil || backends.store == nil {
		t.Fatal("default backends were not configured")
	}

	now := time.Now().UTC()
	job, err := jobs.New("alice", jobs.Request{ModelCatalog: "hymt2-1.8b"}, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := backends.store.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := backends.queue.Enqueue(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	delivery, err := backends.queue.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if delivery.JobID() != job.ID {
		t.Fatalf("unexpected job ID: %q", delivery.JobID())
	}
	if err := delivery.Ack(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, err := backends.store.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != job.ID || loaded.OwnerID != "alice" {
		t.Fatalf("unexpected stored job: %+v", loaded)
	}
}

func TestBuildMemoryBackends(t *testing.T) {
	backends, err := buildBackends(context.Background(), serverConfig{
		QueueBackend: "memory",
		StoreBackend: "memory",
		JobTimeout:   time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer backends.Close()
	if backends.queue == nil || backends.store == nil {
		t.Fatal("memory backends were not configured")
	}
}

func TestBuildSQLiteStoreBackend(t *testing.T) {
	backends, err := buildBackends(context.Background(), serverConfig{
		QueueBackend:       "nats-embedded",
		StoreBackend:       "sqlite",
		SQLitePath:         filepath.Join(t.TempDir(), "jobs.db"),
		NATSEmbeddedMemory: true,
		JobTimeout:         time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer backends.Close()
	if backends.queue == nil || backends.store == nil {
		t.Fatal("sqlite store backend was not configured")
	}
}

func TestValidateJobRequest(t *testing.T) {
	err := validateJobRequest(jobs.Request{
		ModelCatalog: "hymt2-1.8b",
		Profile:      transliter.ProfileOfficial,
		Translation: transliter.TranslationRequest{
			Source:         "hello",
			SourceLanguage: transliter.LanguageEnglish,
			TargetLanguage: transliter.LanguageKorean,
			Kind:           transliter.PromptText,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}
