// Package jobs defines asynchronous translation job contracts.
package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	transliter "github.com/snowmerak/transliter/lib"
)

var (
	ErrNotFound = errors.New("job not found")
	ErrClosed   = errors.New("queue closed")
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

type Request struct {
	Model         transliter.ModelID            `json:"model"`
	ProviderModel string                        `json:"provider_model,omitempty"`
	Profile       transliter.OptionProfile      `json:"profile,omitempty"`
	Translation   transliter.TranslationRequest `json:"translation"`
}

type Result struct {
	Translation   string `json:"translation"`
	ProviderModel string `json:"provider_model,omitempty"`
	FinishReason  string `json:"finish_reason,omitempty"`
	PromptTokens  int    `json:"prompt_tokens,omitempty"`
	OutputTokens  int    `json:"output_tokens,omitempty"`
	TotalTokens   int    `json:"total_tokens,omitempty"`
}

type Job struct {
	ID          string     `json:"id"`
	OwnerID     string     `json:"-"`
	Status      Status     `json:"status"`
	Request     Request    `json:"request"`
	Result      *Result    `json:"result,omitempty"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	ExpiresAt   time.Time  `json:"expires_at"`
}

type Update struct {
	Status      Status
	Result      *Result
	Error       string
	UpdatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
}

type ListOptions struct {
	Limit  int
	Before time.Time
}

// Store persists jobs and owner-scoped history.
type Store interface {
	Create(context.Context, Job) error
	Get(context.Context, string) (Job, error)
	Update(context.Context, string, Update) error
	List(context.Context, string, ListOptions) ([]Job, error)
	DeleteExpired(context.Context, time.Time) (int64, error)
}

// Delivery represents one queue message. Nack makes the job available again.
type Delivery interface {
	JobID() string
	Ack(context.Context) error
	Nack(context.Context) error
}

// Queue carries job IDs and contains no source text or API credentials.
type Queue interface {
	Enqueue(context.Context, string) error
	Receive(context.Context) (Delivery, error)
}

func NewID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate job ID: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func New(ownerID string, request Request, now time.Time, retention time.Duration) (Job, error) {
	if ownerID == "" {
		return Job{}, fmt.Errorf("job owner must not be empty")
	}
	if retention <= 0 {
		return Job{}, fmt.Errorf("job retention must be positive")
	}
	id, err := NewID()
	if err != nil {
		return Job{}, err
	}
	now = now.UTC()
	return Job{
		ID:        id,
		OwnerID:   ownerID,
		Status:    StatusQueued,
		Request:   request,
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: now.Add(retention),
	}, nil
}

func Clone(job Job) Job {
	cloned := job
	cloned.Request.Translation.Glossary = cloneMap(job.Request.Translation.Glossary)
	cloned.Request.Translation.TranslatableAttributes = append(
		[]string(nil),
		job.Request.Translation.TranslatableAttributes...,
	)
	cloned.Request.Translation.Delimiters = append(
		[]string(nil),
		job.Request.Translation.Delimiters...,
	)
	if job.Result != nil {
		result := *job.Result
		cloned.Result = &result
	}
	cloned.StartedAt = cloneTime(job.StartedAt)
	cloned.CompletedAt = cloneTime(job.CompletedAt)
	return cloned
}

func cloneMap(value map[string]string) map[string]string {
	if value == nil {
		return nil
	}
	cloned := make(map[string]string, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
