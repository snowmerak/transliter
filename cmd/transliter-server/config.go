package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	envHTTPAddress        = "TRANSLITER_HTTP_ADDRESS"
	envQueueBackend       = "TRANSLITER_QUEUE_BACKEND"
	envStoreBackend       = "TRANSLITER_STORE_BACKEND"
	envWorkers            = "TRANSLITER_WORKERS"
	envJobTimeout         = "TRANSLITER_JOB_TIMEOUT"
	envJobRetention       = "TRANSLITER_JOB_RETENTION"
	envRedisURL           = "TRANSLITER_REDIS_URL"
	envRedisPrefix        = "TRANSLITER_REDIS_PREFIX"
	envPostgresURL        = "TRANSLITER_POSTGRES_URL"
	envMySQLDSN           = "TRANSLITER_MYSQL_DSN"
	envSQLitePath         = "TRANSLITER_SQLITE_PATH"
	envNATSURL            = "TRANSLITER_NATS_URL"
	envNATSStoreDir       = "TRANSLITER_NATS_STORE_DIR"
	envNATSEmbeddedMemory = "TRANSLITER_NATS_EMBEDDED_MEMORY"
)

type serverConfig struct {
	HTTPAddress        string
	QueueBackend       string
	StoreBackend       string
	Workers            int
	JobTimeout         time.Duration
	JobRetention       time.Duration
	RedisURL           string
	RedisPrefix        string
	PostgresURL        string
	MySQLDSN           string
	SQLitePath         string
	NATSURL            string
	NATSStoreDir       string
	NATSEmbeddedMemory bool
}

func configFromEnv() (serverConfig, error) {
	config := serverConfig{
		HTTPAddress:  valueOrDefault(envHTTPAddress, ":8080"),
		QueueBackend: valueOrDefault(envQueueBackend, "memory"),
		StoreBackend: valueOrDefault(envStoreBackend, "memory"),
		RedisURL:     os.Getenv(envRedisURL),
		RedisPrefix:  valueOrDefault(envRedisPrefix, "transliter"),
		PostgresURL:  os.Getenv(envPostgresURL),
		MySQLDSN:     os.Getenv(envMySQLDSN),
		SQLitePath:   os.Getenv(envSQLitePath),
		NATSURL:      os.Getenv(envNATSURL),
		NATSStoreDir: os.Getenv(envNATSStoreDir),
	}
	var err error
	config.Workers, err = intFromEnv(envWorkers, 1)
	if err != nil {
		return serverConfig{}, err
	}
	config.JobTimeout, err = durationFromEnv(envJobTimeout, 10*time.Minute)
	if err != nil {
		return serverConfig{}, err
	}
	config.JobRetention, err = durationFromEnv(envJobRetention, 30*24*time.Hour)
	if err != nil {
		return serverConfig{}, err
	}
	config.NATSEmbeddedMemory, err = boolFromEnv(envNATSEmbeddedMemory, false)
	if err != nil {
		return serverConfig{}, err
	}
	return config, nil
}

func valueOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func durationFromEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration", name)
	}
	return parsed, nil
}

func intFromEnv(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func boolFromEnv(name string, fallback bool) (bool, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return parsed, nil
}
