package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/VanceMichael/go-base-heritageguard-g01/internal/domain"
)

func (s *Store) Append(ctx context.Context, event domain.AuditEvent) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO audit_events(
			id, tenant_id, actor_id, action, object_type, object_id,
			result, request_id, details, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.ID, event.TenantID, event.ActorID, event.Action, event.ObjectType,
		event.ObjectID, event.Result, event.RequestID, []byte(event.Details), timeText(event.CreatedAt))
	if err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}
	return nil
}

func insertAudit(ctx context.Context, tx *sql.Tx, event domain.AuditEvent) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO audit_events(
			id, tenant_id, actor_id, action, object_type, object_id,
			result, request_id, details, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.ID, event.TenantID, event.ActorID, event.Action, event.ObjectType,
		event.ObjectID, event.Result, event.RequestID, []byte(event.Details), timeText(event.CreatedAt))
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

func (s *Store) List(ctx context.Context, tenantID, objectID string, page domain.Page) ([]domain.AuditEvent, error) {
	page = page.Normalize()
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, tenant_id, actor_id, action, object_type, object_id,
		       result, request_id, details, created_at
		FROM audit_events
		WHERE tenant_id = ? AND (? = '' OR object_id = ?)
		ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?
	`, tenantID, objectID, objectID, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer closeQuietly(rows)
	var events []domain.AuditEvent
	for rows.Next() {
		var event domain.AuditEvent
		var details []byte
		var createdAt string
		if err := rows.Scan(&event.ID, &event.TenantID, &event.ActorID, &event.Action,
			&event.ObjectType, &event.ObjectID, &event.Result, &event.RequestID,
			&details, &createdAt); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		event.Details = append([]byte(nil), details...)
		if event.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, fmt.Errorf("scan audit event created_at: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit events: %w", err)
	}
	return events, nil
}
