package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	transliter "github.com/snowmerak/transliter/lib"
	"github.com/snowmerak/transliter/lib/inference/openai"
	"github.com/snowmerak/transliter/lib/jobs"
	"github.com/snowmerak/transliter/lib/restapi"
	"github.com/snowmerak/transliter/models/catalog"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	config, err := configFromEnv()
	if err != nil {
		return err
	}
	inferenceConfig, err := openai.ConfigFromEnv()
	if err != nil {
		return err
	}

	flags := flag.NewFlagSet("transliter-server", flag.ContinueOnError)
	flags.StringVar(&config.HTTPAddress, "http-address", config.HTTPAddress, "HTTP listen address")
	flags.StringVar(&config.QueueBackend, "queue-backend", config.QueueBackend, "memory, redis, postgres, nats, or nats-embedded")
	flags.StringVar(&config.StoreBackend, "store-backend", config.StoreBackend, "memory, redis, postgres, mysql, or sqlite")
	flags.IntVar(&config.Workers, "workers", config.Workers, "number of scheduler workers")
	flags.DurationVar(&config.JobTimeout, "job-timeout", config.JobTimeout, "timeout for one translation job")
	flags.DurationVar(&config.JobRetention, "job-retention", config.JobRetention, "completed job retention")
	openai.RegisterFlags(flags, &inferenceConfig)
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if config.Workers <= 0 || config.JobTimeout <= 0 || config.JobRetention <= 0 {
		return fmt.Errorf("workers, job timeout, and retention must be positive")
	}

	authenticator, err := jobs.StaticAuthenticatorFromEnv()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	backends, err := buildBackends(ctx, config)
	if err != nil {
		return err
	}
	defer func() {
		stop()
		backends.Close()
	}()

	inferenceClient, err := openai.New(inferenceConfig)
	if err != nil {
		return err
	}
	processor := jobs.NewTranslationProcessor(catalog.Resolver{}, inferenceClient)
	scheduler := &jobs.Scheduler{
		Queue:       backends.queue,
		Store:       backends.store,
		Processor:   processor,
		Concurrency: config.Workers,
		JobTimeout:  config.JobTimeout,
		OnError: func(err error) {
			slog.Error("scheduler", "error", err)
		},
	}
	handler := &restapi.Handler{
		Authenticator: authenticator,
		Queue:         backends.queue,
		Store:         backends.store,
		Catalog:       catalog.Resolver{},
		Models:        inferenceClient,
		Retention:     config.JobRetention,
		Validate:      validateJobRequest,
	}
	routes, err := handler.Routes()
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              config.HTTPAddress,
		Handler:           routes,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	schedulerDone := make(chan error, 1)
	go func() { schedulerDone <- scheduler.Run(ctx) }()
	go cleanupExpired(ctx, backends.store)
	serverDone := make(chan error, 1)
	go func() {
		slog.Info(
			"transliter server listening",
			"address", config.HTTPAddress,
			"queue", config.QueueBackend,
			"store", config.StoreBackend,
		)
		serverDone <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
	case err := <-schedulerDone:
		if err != nil {
			return err
		}
	case err := <-serverDone:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}

func validateJobRequest(request jobs.Request) error {
	model, ok := catalog.Find(request.ModelCatalog)
	if !ok {
		return fmt.Errorf("unknown model catalog %q", request.ModelCatalog)
	}
	translation := request.Translation
	if !model.SupportsLanguage(translation.TargetLanguage) {
		return fmt.Errorf("model catalog does not support target language %q", translation.TargetLanguage)
	}
	if translation.SourceLanguage != "" && !model.SupportsLanguage(translation.SourceLanguage) {
		return fmt.Errorf("model catalog does not support source language %q", translation.SourceLanguage)
	}
	if _, err := model.BuildInput(translation); err != nil {
		return err
	}
	profile := request.Profile
	if profile == "" {
		profile = transliter.ProfileOfficial
	}
	_, err := model.Options(profile)
	return err
}

func cleanupExpired(ctx context.Context, store jobs.Store) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			deleted, err := store.DeleteExpired(ctx, now.UTC())
			if err != nil {
				slog.Error("delete expired jobs", "error", err)
			} else if deleted > 0 {
				slog.Info("deleted expired jobs", "count", deleted)
			}
		}
	}
}
