// Package postgres provides PostgreSQL-backed queue and job storage.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/snowmerak/transliter/lib/jobs"
)

const Schema = `
CREATE TABLE IF NOT EXISTS transliter_jobs (
	id text PRIMARY KEY,
	owner_id text NOT NULL,
	status text NOT NULL,
	request jsonb NOT NULL,
	result jsonb,
	error text NOT NULL DEFAULT '',
	created_at timestamptz NOT NULL,
	updated_at timestamptz NOT NULL,
	started_at timestamptz,
	completed_at timestamptz,
	expires_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS transliter_jobs_owner_created_idx
	ON transliter_jobs (owner_id, created_at DESC);
CREATE INDEX IF NOT EXISTS transliter_jobs_expires_idx
	ON transliter_jobs (expires_at);
CREATE TABLE IF NOT EXISTS transliter_queue (
	job_id text PRIMARY KEY,
	available_at timestamptz NOT NULL DEFAULT now(),
	lease_until timestamptz
);
CREATE INDEX IF NOT EXISTS transliter_queue_available_idx
	ON transliter_queue (available_at, lease_until);
`

type Backend struct {
	pool         *pgxpool.Pool
	lease        time.Duration
	pollInterval time.Duration
}

var (
	_ jobs.Queue = (*Backend)(nil)
	_ jobs.Store = (*Backend)(nil)
)

func New(ctx context.Context, rawURL string, lease time.Duration) (*Backend, error) {
	pool, err := pgxpool.New(ctx, rawURL)
	if err != nil {
		return nil, fmt.Errorf("connect PostgreSQL: %w", err)
	}
	backend := NewWithPool(pool, lease)
	if err := backend.Migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return backend, nil
}

func NewWithPool(pool *pgxpool.Pool, lease time.Duration) *Backend {
	if lease <= 0 {
		lease = 10 * time.Minute
	}
	return &Backend{
		pool:         pool,
		lease:        lease,
		pollInterval: 500 * time.Millisecond,
	}
}

func (backend *Backend) Migrate(ctx context.Context) error {
	for _, statement := range strings.Split(Schema, ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := backend.pool.Exec(ctx, statement); err != nil {
			return fmt.Errorf("migrate PostgreSQL jobs schema: %w", err)
		}
	}
	return nil
}

func (backend *Backend) Close() {
	backend.pool.Close()
}

func (backend *Backend) Create(ctx context.Context, job jobs.Job) error {
	request, err := json.Marshal(job.Request)
	if err != nil {
		return err
	}
	_, err = backend.pool.Exec(ctx, `
INSERT INTO transliter_jobs (
	id, owner_id, status, request, created_at, updated_at, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		job.ID,
		job.OwnerID,
		job.Status,
		request,
		job.CreatedAt,
		job.UpdatedAt,
		job.ExpiresAt,
	)
	return err
}

func (backend *Backend) Get(ctx context.Context, id string) (jobs.Job, error) {
	job, err := scanJob(backend.pool.QueryRow(ctx, `
SELECT id, owner_id, status, request, result, error, created_at, updated_at,
       started_at, completed_at, expires_at
FROM transliter_jobs
WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return jobs.Job{}, jobs.ErrNotFound
	}
	return job, err
}

func (backend *Backend) Update(ctx context.Context, id string, update jobs.Update) error {
	var result []byte
	var err error
	if update.Result != nil {
		result, err = json.Marshal(update.Result)
		if err != nil {
			return err
		}
	}
	tag, err := backend.pool.Exec(ctx, `
UPDATE transliter_jobs
SET status = $2,
    result = $3,
    error = $4,
    updated_at = $5,
    started_at = COALESCE($6, started_at),
    completed_at = COALESCE($7, completed_at)
WHERE id = $1`,
		id,
		update.Status,
		result,
		update.Error,
		update.UpdatedAt,
		update.StartedAt,
		update.CompletedAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return jobs.ErrNotFound
	}
	return nil
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
	before := options.Before
	if before.IsZero() {
		before = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	}
	rows, err := backend.pool.Query(ctx, `
SELECT id, owner_id, status, request, result, error, created_at, updated_at,
       started_at, completed_at, expires_at
FROM transliter_jobs
WHERE owner_id = $1 AND created_at < $2
ORDER BY created_at DESC
LIMIT $3`, ownerID, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	history := make([]jobs.Job, 0, limit)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		history = append(history, job)
	}
	return history, rows.Err()
}

func (backend *Backend) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	tag, err := backend.pool.Exec(
		ctx,
		`DELETE FROM transliter_jobs WHERE expires_at <= $1`,
		before,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (backend *Backend) Enqueue(ctx context.Context, jobID string) error {
	_, err := backend.pool.Exec(ctx, `
INSERT INTO transliter_queue (job_id, available_at)
VALUES ($1, now())
ON CONFLICT (job_id) DO UPDATE
SET available_at = EXCLUDED.available_at, lease_until = NULL`, jobID)
	return err
}

func (backend *Backend) Receive(ctx context.Context) (jobs.Delivery, error) {
	for {
		var id string
		err := backend.pool.QueryRow(ctx, `
WITH candidate AS (
	SELECT job_id
	FROM transliter_queue
	WHERE available_at <= now()
	  AND (lease_until IS NULL OR lease_until <= now())
	ORDER BY available_at
	FOR UPDATE SKIP LOCKED
	LIMIT 1
)
UPDATE transliter_queue q
SET lease_until = now() + ($1 * interval '1 millisecond')
FROM candidate
WHERE q.job_id = candidate.job_id
RETURNING q.job_id`, backend.lease.Milliseconds()).Scan(&id)
		if err == nil {
			return &delivery{backend: backend, id: id}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		timer := time.NewTimer(backend.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

type delivery struct {
	backend *Backend
	id      string
}

func (value *delivery) JobID() string {
	return value.id
}

func (value *delivery) Ack(ctx context.Context) error {
	_, err := value.backend.pool.Exec(
		ctx,
		`DELETE FROM transliter_queue WHERE job_id = $1`,
		value.id,
	)
	return err
}

func (value *delivery) Nack(ctx context.Context) error {
	_, err := value.backend.pool.Exec(ctx, `
UPDATE transliter_queue
SET available_at = now(), lease_until = NULL
WHERE job_id = $1`, value.id)
	return err
}

type rowScanner interface {
	Scan(...any) error
}

func scanJob(row rowScanner) (jobs.Job, error) {
	var job jobs.Job
	var request []byte
	var result []byte
	if err := row.Scan(
		&job.ID,
		&job.OwnerID,
		&job.Status,
		&request,
		&result,
		&job.Error,
		&job.CreatedAt,
		&job.UpdatedAt,
		&job.StartedAt,
		&job.CompletedAt,
		&job.ExpiresAt,
	); err != nil {
		return jobs.Job{}, err
	}
	if err := json.Unmarshal(request, &job.Request); err != nil {
		return jobs.Job{}, fmt.Errorf("decode PostgreSQL job request: %w", err)
	}
	if len(result) > 0 {
		job.Result = &jobs.Result{}
		if err := json.Unmarshal(result, job.Result); err != nil {
			return jobs.Job{}, fmt.Errorf("decode PostgreSQL job result: %w", err)
		}
	}
	return job, nil
}
