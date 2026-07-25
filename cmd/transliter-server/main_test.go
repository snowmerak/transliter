package main

import (
	"context"
	"testing"
	"time"

	transliter "github.com/snowmerak/translter/lib"
	"github.com/snowmerak/translter/lib/jobs"
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
