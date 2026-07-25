// Package mysql provides MySQL job storage. It does not implement jobs.Queue.
package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/snowmerak/transliter/lib/jobs"
)

const Schema = `
CREATE TABLE IF NOT EXISTS transliter_jobs (
	id varchar(64) PRIMARY KEY,
	owner_id varchar(255) NOT NULL,
	status varchar(32) NOT NULL,
	request json NOT NULL,
	result json NULL,
	error text NOT NULL,
	created_at datetime(6) NOT NULL,
	updated_at datetime(6) NOT NULL,
	started_at datetime(6) NULL,
	completed_at datetime(6) NULL,
	expires_at datetime(6) NOT NULL,
	INDEX transliter_jobs_owner_created_idx (owner_id, created_at DESC),
	INDEX transliter_jobs_expires_idx (expires_at)
) ENGINE=InnoDB;
`

type Store struct {
	database *sql.DB
}

var _ jobs.Store = (*Store)(nil)

func New(ctx context.Context, rawDSN string) (*Store, error) {
	config, err := driver.ParseDSN(rawDSN)
	if err != nil {
		return nil, fmt.Errorf("parse MySQL DSN: %w", err)
	}
	config.ParseTime = true
	config.Loc = time.UTC
	database, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open MySQL: %w", err)
	}
	if err := database.PingContext(ctx); err != nil {
		database.Close()
		return nil, fmt.Errorf("connect MySQL: %w", err)
	}
	store := NewWithDB(database)
	if err := store.Migrate(ctx); err != nil {
		database.Close()
		return nil, err
	}
	return store, nil
}

func NewWithDB(database *sql.DB) *Store {
	return &Store{database: database}
}

func (store *Store) Migrate(ctx context.Context) error {
	if _, err := store.database.ExecContext(ctx, Schema); err != nil {
		return fmt.Errorf("migrate MySQL jobs schema: %w", err)
	}
	return nil
}

func (store *Store) Close() error {
	return store.database.Close()
}

func (store *Store) Create(ctx context.Context, job jobs.Job) error {
	request, err := json.Marshal(job.Request)
	if err != nil {
		return err
	}
	_, err = store.database.ExecContext(ctx, `
INSERT INTO transliter_jobs (
	id, owner_id, status, request, error, created_at, updated_at, expires_at
) VALUES (?, ?, ?, ?, '', ?, ?, ?)`,
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

func (store *Store) Get(ctx context.Context, id string) (jobs.Job, error) {
	job, err := scanJob(store.database.QueryRowContext(ctx, `
SELECT id, owner_id, status, request, result, error, created_at, updated_at,
       started_at, completed_at, expires_at
FROM transliter_jobs
WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return jobs.Job{}, jobs.ErrNotFound
	}
	return job, err
}

func (store *Store) Update(ctx context.Context, id string, update jobs.Update) error {
	var result []byte
	var err error
	if update.Result != nil {
		result, err = json.Marshal(update.Result)
		if err != nil {
			return err
		}
	}
	sqlResult, err := store.database.ExecContext(ctx, `
UPDATE transliter_jobs
SET status = ?,
    result = ?,
    error = ?,
    updated_at = ?,
    started_at = COALESCE(?, started_at),
    completed_at = COALESCE(?, completed_at)
WHERE id = ?`,
		update.Status,
		result,
		update.Error,
		update.UpdatedAt,
		update.StartedAt,
		update.CompletedAt,
		id,
	)
	if err != nil {
		return err
	}
	affected, err := sqlResult.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return jobs.ErrNotFound
	}
	return nil
}

func (store *Store) List(
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
	rows, err := store.database.QueryContext(ctx, `
SELECT id, owner_id, status, request, result, error, created_at, updated_at,
       started_at, completed_at, expires_at
FROM transliter_jobs
WHERE owner_id = ? AND created_at < ?
ORDER BY created_at DESC
LIMIT ?`, ownerID, before, limit)
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

func (store *Store) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	result, err := store.database.ExecContext(
		ctx,
		`DELETE FROM transliter_jobs WHERE expires_at <= ?`,
		before,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
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
		return jobs.Job{}, fmt.Errorf("decode MySQL job request: %w", err)
	}
	if len(result) > 0 {
		job.Result = &jobs.Result{}
		if err := json.Unmarshal(result, job.Result); err != nil {
			return jobs.Job{}, fmt.Errorf("decode MySQL job result: %w", err)
		}
	}
	return job, nil
}
