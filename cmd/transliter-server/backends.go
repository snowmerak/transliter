package main

import (
	"context"
	"fmt"
	"os"

	"github.com/snowmerak/transliter/lib/jobs"
	"github.com/snowmerak/transliter/lib/jobs/memory"
	mysqlstore "github.com/snowmerak/transliter/lib/jobs/mysql"
	"github.com/snowmerak/transliter/lib/jobs/natsjs"
	"github.com/snowmerak/transliter/lib/jobs/postgres"
	redisjobs "github.com/snowmerak/transliter/lib/jobs/redis"
	sqlitestore "github.com/snowmerak/transliter/lib/jobs/sqlite"
)

type backendSet struct {
	queue  jobs.Queue
	store  jobs.Store
	closes []func()
}

func (set *backendSet) Close() {
	for index := len(set.closes) - 1; index >= 0; index-- {
		set.closes[index]()
	}
}

func buildBackends(ctx context.Context, config serverConfig) (*backendSet, error) {
	set := &backendSet{}

	if config.QueueBackend == "memory" || config.StoreBackend == "memory" {
		backend := memory.New(1024)
		if config.QueueBackend == "memory" {
			set.queue = backend
		}
		if config.StoreBackend == "memory" {
			set.store = backend
		}
	}

	if config.QueueBackend == "redis" || config.StoreBackend == "redis" {
		if config.RedisURL == "" {
			return nil, fmt.Errorf("%s is required for Redis", envRedisURL)
		}
		hostname, _ := os.Hostname()
		consumer := fmt.Sprintf("%s-%d", hostname, os.Getpid())
		backend, err := redisjobs.New(config.RedisURL, config.RedisPrefix, consumer)
		if err != nil {
			return nil, err
		}
		if err := backend.SetClaimIdle(config.JobTimeout * 2); err != nil {
			backend.Close()
			return nil, err
		}
		set.closes = append(set.closes, backend.Close)
		if config.QueueBackend == "redis" {
			set.queue = backend
		}
		if config.StoreBackend == "redis" {
			set.store = backend
		}
	}

	if config.QueueBackend == "postgres" || config.StoreBackend == "postgres" {
		if config.PostgresURL == "" {
			return nil, fmt.Errorf("%s is required for PostgreSQL", envPostgresURL)
		}
		backend, err := postgres.New(ctx, config.PostgresURL, config.JobTimeout*2)
		if err != nil {
			return nil, err
		}
		set.closes = append(set.closes, backend.Close)
		if config.QueueBackend == "postgres" {
			set.queue = backend
		}
		if config.StoreBackend == "postgres" {
			set.store = backend
		}
	}

	if config.StoreBackend == "mysql" {
		if config.MySQLDSN == "" {
			return nil, fmt.Errorf("%s is required for MySQL", envMySQLDSN)
		}
		store, err := mysqlstore.New(ctx, config.MySQLDSN)
		if err != nil {
			return nil, err
		}
		set.closes = append(set.closes, func() { _ = store.Close() })
		set.store = store
	}

	if config.StoreBackend == "sqlite" {
		if config.SQLitePath == "" {
			return nil, fmt.Errorf("%s is required for SQLite", envSQLitePath)
		}
		store, err := sqlitestore.New(ctx, config.SQLitePath)
		if err != nil {
			return nil, err
		}
		set.closes = append(set.closes, func() { _ = store.Close() })
		set.store = store
	}

	switch config.QueueBackend {
	case "nats":
		queue, err := natsjs.New(ctx, natsjs.Config{
			URL:     config.NATSURL,
			AckWait: config.JobTimeout * 2,
		})
		if err != nil {
			return nil, err
		}
		set.closes = append(set.closes, queue.Close)
		set.queue = queue
	case "nats-embedded":
		embedded, err := natsjs.NewEmbedded(ctx, natsjs.EmbeddedConfig{
			Queue: natsjs.Config{
				AckWait: config.JobTimeout * 2,
			},
			StoreDir: config.NATSStoreDir,
			InMemory: config.NATSEmbeddedMemory,
		})
		if err != nil {
			return nil, err
		}
		set.closes = append(set.closes, embedded.Close)
		set.queue = embedded.Queue
	case "memory", "redis", "postgres":
	default:
		return nil, fmt.Errorf("unknown queue backend %q", config.QueueBackend)
	}

	switch config.StoreBackend {
	case "memory", "redis", "postgres", "mysql", "sqlite":
	default:
		return nil, fmt.Errorf("unknown store backend %q", config.StoreBackend)
	}
	if set.queue == nil || set.store == nil {
		set.Close()
		return nil, fmt.Errorf("queue and store backends must both be configured")
	}
	return set, nil
}
