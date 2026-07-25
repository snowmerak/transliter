// Package sqlite provides pure-Go SQLite job storage via modernc.org/sqlite.
// It does not implement jobs.Queue.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/snowmerak/transliter/lib/jobs"
	_ "modernc.org/sqlite"
)

// Schema is the initial DDL applied by Migrate.
// Timestamps are UTC Unix nanoseconds so ORDER BY / range compares stay numeric.
const Schema = `
CREATE TABLE IF NOT EXISTS transliter_jobs (
	id TEXT PRIMARY KEY,
	owner_id TEXT NOT NULL,
	status TEXT NOT NULL,
	request TEXT NOT NULL,
	result TEXT NULL,
	error TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	started_at INTEGER NULL,
	completed_at INTEGER NULL,
	expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS transliter_jobs_owner_created_idx
	ON transliter_jobs (owner_id, created_at DESC);
CREATE INDEX IF NOT EXISTS transliter_jobs_expires_idx
	ON transliter_jobs (expires_at);
`

// Store persists jobs in a SQLite database file or memory DSN.
type Store struct {
	database *sql.DB
}

var _ jobs.Store = (*Store)(nil)

// New opens pathOrDSN, applies connection pragmas, migrates schema, and
// returns a ready store. pathOrDSN may be a filesystem path or a full
// modernc SQLite DSN such as file:jobs.db?_pragma=busy_timeout(5000).
func New(ctx context.Context, pathOrDSN string) (*Store, error) {
	if strings.TrimSpace(pathOrDSN) == "" {
		return nil, fmt.Errorf("SQLite path is required")
	}
	database, err := sql.Open("sqlite", normalizeDSN(pathOrDSN))
	if err != nil {
		return nil, fmt.Errorf("open SQLite: %w", err)
	}
	// SQLite allows one writer; keep the pool small and rely on WAL + busy_timeout.
	database.SetMaxOpenConns(1)
	database.SetConnMaxLifetime(0)
	if err := database.PingContext(ctx); err != nil {
		database.Close()
		return nil, fmt.Errorf("connect SQLite: %w", err)
	}
	if err := applyPragmas(ctx, database); err != nil {
		database.Close()
		return nil, err
	}
	store := NewWithDB(database)
	if err := store.Migrate(ctx); err != nil {
		database.Close()
		return nil, err
	}
	return store, nil
}

// NewWithDB wraps an existing *sql.DB. The caller owns lifecycle and pragmas.
func NewWithDB(database *sql.DB) *Store {
	return &Store{database: database}
}

// Migrate creates the jobs table and indexes when missing.
func (store *Store) Migrate(ctx context.Context) error {
	if _, err := store.database.ExecContext(ctx, Schema); err != nil {
		return fmt.Errorf("migrate SQLite jobs schema: %w", err)
	}
	return nil
}

// Close closes the underlying database.
func (store *Store) Close() error {
	return store.database.Close()
}

// Create inserts a queued job.
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
		string(request),
		encodeTime(job.CreatedAt),
		encodeTime(job.UpdatedAt),
		encodeTime(job.ExpiresAt),
	)
	return err
}

// Get loads one job by ID.
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

// Update applies a status transition and optional result payload.
func (store *Store) Update(ctx context.Context, id string, update jobs.Update) error {
	var result any
	if update.Result != nil {
		encoded, err := json.Marshal(update.Result)
		if err != nil {
			return err
		}
		result = string(encoded)
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
		encodeTime(update.UpdatedAt),
		encodeTimePtr(update.StartedAt),
		encodeTimePtr(update.CompletedAt),
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

// List returns owner-scoped history newest first.
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
		// Far-future bound keeps the created_at < ? predicate simple.
		before = time.Unix(0, 1<<62).UTC()
	}
	rows, err := store.database.QueryContext(ctx, `
SELECT id, owner_id, status, request, result, error, created_at, updated_at,
       started_at, completed_at, expires_at
FROM transliter_jobs
WHERE owner_id = ? AND created_at < ?
ORDER BY created_at DESC
LIMIT ?`, ownerID, encodeTime(before), limit)
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

// DeleteExpired removes jobs whose expires_at is at or before before.
func (store *Store) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	result, err := store.database.ExecContext(
		ctx,
		`DELETE FROM transliter_jobs WHERE expires_at <= ?`,
		encodeTime(before),
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
	var request string
	var result sql.NullString
	var createdAt int64
	var updatedAt int64
	var startedAt sql.NullInt64
	var completedAt sql.NullInt64
	var expiresAt int64
	if err := row.Scan(
		&job.ID,
		&job.OwnerID,
		&job.Status,
		&request,
		&result,
		&job.Error,
		&createdAt,
		&updatedAt,
		&startedAt,
		&completedAt,
		&expiresAt,
	); err != nil {
		return jobs.Job{}, err
	}
	if err := json.Unmarshal([]byte(request), &job.Request); err != nil {
		return jobs.Job{}, fmt.Errorf("decode SQLite job request: %w", err)
	}
	if result.Valid && result.String != "" {
		job.Result = &jobs.Result{}
		if err := json.Unmarshal([]byte(result.String), job.Result); err != nil {
			return jobs.Job{}, fmt.Errorf("decode SQLite job result: %w", err)
		}
	}
	job.CreatedAt = decodeTime(createdAt)
	job.UpdatedAt = decodeTime(updatedAt)
	job.ExpiresAt = decodeTime(expiresAt)
	if startedAt.Valid {
		value := decodeTime(startedAt.Int64)
		job.StartedAt = &value
	}
	if completedAt.Valid {
		value := decodeTime(completedAt.Int64)
		job.CompletedAt = &value
	}
	return job, nil
}

func normalizeDSN(pathOrDSN string) string {
	return strings.TrimSpace(pathOrDSN)
}

func applyPragmas(ctx context.Context, database *sql.DB) error {
	pragmas := []string{
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA foreign_keys = ON`,
		`PRAGMA synchronous = NORMAL`,
	}
	for _, pragma := range pragmas {
		if _, err := database.ExecContext(ctx, pragma); err != nil {
			// :memory: and some read-only DSNs reject WAL; keep going only for journal_mode.
			if strings.Contains(pragma, "journal_mode") {
				continue
			}
			return fmt.Errorf("apply SQLite pragma %q: %w", pragma, err)
		}
	}
	return nil
}

func encodeTime(value time.Time) int64 {
	return value.UTC().UnixNano()
}

func encodeTimePtr(value *time.Time) any {
	if value == nil {
		return nil
	}
	return encodeTime(*value)
}

func decodeTime(value int64) time.Time {
	return time.Unix(0, value).UTC()
}
