package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tabloy/keygate/internal/model"
	"github.com/uptrace/bun"
)

type auditOutboxRow struct {
	ID        string    `bun:"id"`
	Entity    string    `bun:"entity"`
	EntityID  string    `bun:"entity_id"`
	Action    string    `bun:"action"`
	CreatedAt time.Time `bun:"created_at"`
}

// FlushAuditOutbox moves a bounded batch into the operator-facing audit log.
// Selection, delivery and deletion share one transaction, so a crash can only
// leave a row pending; it cannot silently lose it.
func (s *Store) FlushAuditOutbox(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 1000 {
		return 0, fmt.Errorf("audit outbox limit must be between 1 and 1000")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	var rows []auditOutboxRow
	if err := tx.NewRaw(`
		SELECT id, entity, entity_id, action, created_at
		FROM audit_outbox
		ORDER BY created_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT ?`, limit).Scan(ctx, &rows); err != nil {
		return 0, err
	}
	for _, row := range rows {
		entry := &model.AuditLog{
			ID: row.ID, Entity: row.Entity, EntityID: row.EntityID,
			Action: "db_" + row.Action, ActorType: "system",
			Changes:   map[string]any{"source": "transactional_outbox"},
			CreatedAt: row.CreatedAt,
		}
		if _, err := tx.NewInsert().Model(entry).On("CONFLICT (id) DO NOTHING").Exec(ctx); err != nil {
			return 0, err
		}
	}
	if len(rows) > 0 {
		ids := make([]string, len(rows))
		for i := range rows {
			ids[i] = rows[i].ID
		}
		if _, err := tx.NewDelete().Table("audit_outbox").Where("id IN (?)", bun.In(ids)).Exec(ctx); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func (s *Store) StartAuditOutboxLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	flush := func() {
		flushCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		for {
			count, err := s.FlushAuditOutbox(flushCtx, 200)
			if err != nil {
				if flushCtx.Err() == nil {
					slog.Error("audit outbox flush failed", "error", err)
				}
				return
			}
			if count < 200 {
				return
			}
		}
	}
	flush()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case <-ticker.C:
			flush()
		}
	}
}
