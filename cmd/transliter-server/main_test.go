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
		QueueBackend: "memory",
		StoreBackend: "sqlite",
		SQLitePath:   filepath.Join(t.TempDir(), "jobs.db"),
		JobTimeout:   time.Minute,
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
		Model:   "hymt2-1.8b",
		Profile: transliter.ProfileOfficial,
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
