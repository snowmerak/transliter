// Package redis provides rueidis-backed queue and job storage implementations.
package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/rueidis"
	"github.com/snowmerak/transliter/lib/jobs"
)

type Backend struct {
	client    rueidis.Client
	prefix    string
	group     string
	consumer  string
	claimIdle time.Duration
	groupMu   sync.Mutex
	groupOK   bool
}

var (
	_ jobs.Queue = (*Backend)(nil)
	_ jobs.Store = (*Backend)(nil)
)

func New(rawURL, prefix, consumer string) (*Backend, error) {
	options, err := rueidis.ParseURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse Redis URL: %w", err)
	}
	client, err := rueidis.NewClient(options)
	if err != nil {
		return nil, fmt.Errorf("connect Redis: %w", err)
	}
	backend, err := NewWithClient(client, prefix, consumer)
	if err != nil {
		client.Close()
		return nil, err
	}
	return backend, nil
}

func NewWithClient(client rueidis.Client, prefix, consumer string) (*Backend, error) {
	if client == nil {
		return nil, fmt.Errorf("Redis client is required")
	}
	if prefix == "" {
		prefix = "transliter"
	}
	if consumer == "" {
		consumer = "worker"
	}
	return &Backend{
		client:    client,
		prefix:    prefix,
		group:     prefix + "-workers",
		consumer:  consumer,
		claimIdle: 10 * time.Minute,
	}, nil
}

func (backend *Backend) Close() {
	backend.client.Close()
}

// SetClaimIdle sets the minimum idle time before an unacknowledged stream
// message may be reclaimed after a worker failure.
func (backend *Backend) SetClaimIdle(duration time.Duration) error {
	if duration <= 0 {
		return fmt.Errorf("Redis claim idle duration must be positive")
	}
	backend.claimIdle = duration
	return nil
}

type storedJob struct {
	OwnerID string   `json:"owner_id"`
	Job     jobs.Job `json:"job"`
}

func (backend *Backend) Create(ctx context.Context, job jobs.Job) error {
	data, err := json.Marshal(storedJob{OwnerID: job.OwnerID, Job: job})
	if err != nil {
		return err
	}
	ttl := time.Until(job.ExpiresAt)
	if ttl <= 0 {
		return fmt.Errorf("job already expired")
	}
	score := strconv.FormatInt(job.CreatedAt.UnixMilli(), 10)
	seconds := strconv.FormatInt(max(int64(ttl/time.Second), 1), 10)
	cutoff := strconv.FormatInt(job.CreatedAt.Add(-ttl).UnixMilli(), 10)
	result := backend.client.Do(
		ctx,
		backend.client.B().Arbitrary("EVAL").
			Args(
				`redis.call("SET", KEYS[1], ARGV[1], "EX", ARGV[4])
redis.call("ZREMRANGEBYSCORE", KEYS[2], "-inf", ARGV[5])
redis.call("ZADD", KEYS[2], ARGV[2], ARGV[3])
redis.call("EXPIRE", KEYS[2], ARGV[4])
return 1`,
				"2",
			).
			Keys(backend.jobKey(job.ID), backend.ownerKey(job.OwnerID)).
			Args(string(data), score, job.ID, seconds, cutoff).
			Build(),
	)
	return result.Error()
}

func (backend *Backend) Get(ctx context.Context, id string) (jobs.Job, error) {
	data, err := backend.client.Do(
		ctx,
		backend.client.B().Arbitrary("GET").Keys(backend.jobKey(id)).Build(),
	).ToString()
	if errors.Is(err, rueidis.Nil) {
		return jobs.Job{}, jobs.ErrNotFound
	}
	if err != nil {
		return jobs.Job{}, err
	}
	var stored storedJob
	if err := json.Unmarshal([]byte(data), &stored); err != nil {
		return jobs.Job{}, fmt.Errorf("decode Redis job: %w", err)
	}
	stored.Job.OwnerID = stored.OwnerID
	return stored.Job, nil
}

func (backend *Backend) Update(ctx context.Context, id string, update jobs.Update) error {
	job, err := backend.Get(ctx, id)
	if err != nil {
		return err
	}
	job.Status = update.Status
	job.Result = update.Result
	job.Error = update.Error
	job.UpdatedAt = update.UpdatedAt
	if update.StartedAt != nil {
		job.StartedAt = update.StartedAt
	}
	if update.CompletedAt != nil {
		job.CompletedAt = update.CompletedAt
	}
	data, err := json.Marshal(storedJob{OwnerID: job.OwnerID, Job: job})
	if err != nil {
		return err
	}
	result := backend.client.Do(
		ctx,
		backend.client.B().Arbitrary("SET").
			Keys(backend.jobKey(id)).
			Args(string(data), "KEEPTTL").
			Build(),
	)
	return result.Error()
}

func (backend *Backend) List(
	ctx context.Context,
	ownerID string,
	options jobs.ListOptions,
) ([]jobs.Job, error) {
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	maximum := "+inf"
	if !options.Before.IsZero() {
		maximum = "(" + strconv.FormatInt(options.Before.UnixMilli(), 10)
	}
	messages, err := backend.client.Do(
		ctx,
		backend.client.B().Arbitrary("ZREVRANGEBYSCORE").
			Keys(backend.ownerKey(ownerID)).
			Args(maximum, "-inf", "LIMIT", "0", strconv.Itoa(limit)).
			Build(),
	).ToArray()
	if err != nil {
		return nil, err
	}
	history := make([]jobs.Job, 0, len(messages))
	for _, message := range messages {
		id, err := message.ToString()
		if err != nil {
			return nil, err
		}
		job, err := backend.Get(ctx, id)
		if errors.Is(err, jobs.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if job.OwnerID == ownerID {
			history = append(history, job)
		}
	}
	return history, nil
}

func (backend *Backend) DeleteExpired(context.Context, time.Time) (int64, error) {
	// Job keys and owner indexes have Redis TTLs. Expiration is automatic.
	return 0, nil
}

func (backend *Backend) Enqueue(ctx context.Context, jobID string) error {
	return backend.client.Do(
		ctx,
		backend.client.B().Arbitrary("XADD").
			Keys(backend.streamKey()).
			Args("*", "job_id", jobID).
			Build(),
	).Error()
}

func (backend *Backend) Receive(ctx context.Context) (jobs.Delivery, error) {
	if err := backend.ensureGroup(ctx); err != nil {
		return nil, err
	}
	for {
		reclaimed, err := backend.reclaim(ctx)
		if err != nil {
			return nil, err
		}
		if reclaimed != nil {
			return reclaimed, nil
		}
		result := backend.client.Do(
			ctx,
			backend.client.B().Arbitrary("XREADGROUP").
				Args("GROUP", backend.group, backend.consumer, "COUNT", "1", "BLOCK", "1000", "STREAMS").
				Keys(backend.streamKey()).
				Args(">").
				Blocking(),
		)
		if errors.Is(result.Error(), rueidis.Nil) {
			continue
		}
		entries, err := result.AsXRead()
		if err != nil {
			return nil, err
		}
		streamEntries := entries[backend.streamKey()]
		if len(streamEntries) == 0 {
			continue
		}
		entry := streamEntries[0]
		jobID := entry.FieldValues["job_id"]
		if jobID == "" {
			_ = backend.ack(ctx, entry.ID)
			continue
		}
		return &delivery{
			backend:   backend,
			messageID: entry.ID,
			jobID:     jobID,
		}, nil
	}
}

func (backend *Backend) reclaim(ctx context.Context) (jobs.Delivery, error) {
	result := backend.client.Do(
		ctx,
		backend.client.B().Arbitrary("XAUTOCLAIM").
			Keys(backend.streamKey()).
			Args(
				backend.group,
				backend.consumer,
				strconv.FormatInt(backend.claimIdle.Milliseconds(), 10),
				"0-0",
				"COUNT",
				"1",
			).
			Build(),
	)
	parts, err := result.ToArray()
	if errors.Is(err, rueidis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(parts) < 2 {
		return nil, fmt.Errorf("unexpected XAUTOCLAIM response")
	}
	messages, err := parts[1].ToArray()
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, nil
	}
	entry, err := messages[0].AsXRangeEntry()
	if err != nil {
		return nil, err
	}
	jobID := entry.FieldValues["job_id"]
	if jobID == "" {
		_ = backend.ack(ctx, entry.ID)
		return nil, nil
	}
	return &delivery{
		backend:   backend,
		messageID: entry.ID,
		jobID:     jobID,
	}, nil
}

func (backend *Backend) ensureGroup(ctx context.Context) error {
	backend.groupMu.Lock()
	defer backend.groupMu.Unlock()
	if backend.groupOK {
		return nil
	}
	err := backend.client.Do(
		ctx,
		backend.client.B().Arbitrary("XGROUP", "CREATE").
			Keys(backend.streamKey()).
			Args(backend.group, "0", "MKSTREAM").
			Build(),
	).Error()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("create Redis consumer group: %w", err)
	}
	backend.groupOK = true
	return nil
}

type delivery struct {
	backend   *Backend
	messageID string
	jobID     string
}

func (value *delivery) JobID() string {
	return value.jobID
}

func (value *delivery) Ack(ctx context.Context) error {
	return value.backend.ack(ctx, value.messageID)
}

func (value *delivery) Nack(ctx context.Context) error {
	return value.backend.client.Do(
		ctx,
		value.backend.client.B().Arbitrary("EVAL").
			Args(
				`redis.call("XACK", KEYS[1], ARGV[1], ARGV[2])
redis.call("XDEL", KEYS[1], ARGV[2])
return redis.call("XADD", KEYS[1], "*", "job_id", ARGV[3])`,
				"1",
			).
			Keys(value.backend.streamKey()).
			Args(value.backend.group, value.messageID, value.jobID).
			Build(),
	).Error()
}

func (backend *Backend) ack(ctx context.Context, messageID string) error {
	return backend.client.Do(
		ctx,
		backend.client.B().Arbitrary("EVAL").
			Args(
				`redis.call("XACK", KEYS[1], ARGV[1], ARGV[2])
return redis.call("XDEL", KEYS[1], ARGV[2])`,
				"1",
			).
			Keys(backend.streamKey()).
			Args(backend.group, messageID).
			Build(),
	).Error()
}

func (backend *Backend) streamKey() string {
	return backend.prefix + ":queue"
}

func (backend *Backend) jobKey(id string) string {
	return backend.prefix + ":job:" + id
}

func (backend *Backend) ownerKey(ownerID string) string {
	return backend.prefix + ":owner:" + ownerID
}
