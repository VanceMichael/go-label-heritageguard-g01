package domain

import (
	"encoding/json"
	"time"
)

type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobRetry     JobStatus = "retry"
	JobFailed    JobStatus = "failed"
)

type WorkerJob struct {
	ID             string          `json:"id"`
	TenantID       string          `json:"tenant_id"`
	Kind           string          `json:"kind"`
	AggregateID    string          `json:"aggregate_id"`
	Payload        json.RawMessage `json:"payload"`
	Status         JobStatus       `json:"status"`
	Attempts       int             `json:"attempts"`
	MaxAttempts    int             `json:"max_attempts"`
	AvailableAt    time.Time       `json:"available_at"`
	LeaseOwner     string          `json:"lease_owner,omitempty"`
	LeaseExpiresAt *time.Time      `json:"lease_expires_at,omitempty"`
	LastError      string          `json:"last_error,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type OutboxEvent struct {
	ID             string          `json:"id"`
	TenantID       string          `json:"tenant_id"`
	Topic          string          `json:"topic"`
	AggregateID    string          `json:"aggregate_id"`
	IdempotencyKey string          `json:"idempotency_key"`
	Payload        json.RawMessage `json:"payload"`
	Status         JobStatus       `json:"status"`
	Attempts       int             `json:"attempts"`
	MaxAttempts    int             `json:"max_attempts"`
	AvailableAt    time.Time       `json:"available_at"`
	LeaseOwner     string          `json:"lease_owner,omitempty"`
	LeaseExpiresAt *time.Time      `json:"lease_expires_at,omitempty"`
	LastError      string          `json:"last_error,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type AuditEvent struct {
	ID         string          `json:"id"`
	TenantID   string          `json:"tenant_id"`
	ActorID    string          `json:"actor_id"`
	Action     string          `json:"action"`
	ObjectType string          `json:"object_type"`
	ObjectID   string          `json:"object_id"`
	Result     string          `json:"result"`
	RequestID  string          `json:"request_id"`
	Details    json.RawMessage `json:"details"`
	CreatedAt  time.Time       `json:"created_at"`
}

type Page struct {
	Limit  int
	Offset int
}

func (p Page) Normalize() Page {
	if p.Limit <= 0 {
		p.Limit = 25
	}
	if p.Limit > 100 {
		p.Limit = 100
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	return p
}
