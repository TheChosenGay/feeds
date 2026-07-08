package main

import (
	"context"
	"database/sql"
	"time"
)

// Notification 数据库模型
type Notification struct {
	ID        int64
	UserID    string
	Type      string
	Title     string
	Body      string
	Payload   []byte
	ReadAt    *time.Time
	CreatedAt time.Time
}

type notifyStore struct {
	db *sql.DB
}

// Insert 写入一条通知，返回自增 ID。
func (s *notifyStore) Insert(ctx context.Context, n *Notification) (int64, error) {
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO notifications (user_id, type, title, body, payload)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		n.UserID, n.Type, n.Title, n.Body, n.Payload,
	).Scan(&n.ID)
	return n.ID, err
}

// ListByUser 按时间倒序分页查询。
func (s *notifyStore) ListByUser(ctx context.Context, userID string, cursor int64, pageSize int32) ([]Notification, int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, type, title, body, payload, read_at, created_at
		 FROM notifications
		 WHERE user_id = $1 AND ($2 = 0 OR id < $2)
		 ORDER BY created_at DESC, id DESC
		 LIMIT $3`,
		userID, cursor, pageSize+1,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &n.Payload, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, n)
	}

	var nextCursor int64
	if len(items) > int(pageSize) {
		nextCursor = items[pageSize-1].ID
		items = items[:pageSize]
	} else if len(items) > 0 {
		nextCursor = items[len(items)-1].ID
	}

	return items, nextCursor, rows.Err()
}

// MarkRead 标记已读。notificationID 为 "all" 时标记全部。
func (s *notifyStore) MarkRead(ctx context.Context, userID, notificationID string) error {
	if notificationID == "all" {
		_, err := s.db.ExecContext(ctx,
			`UPDATE notifications SET read_at = NOW()
			 WHERE user_id = $1 AND read_at IS NULL`,
			userID,
		)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = NOW()
		 WHERE id = $1 AND user_id = $2 AND read_at IS NULL`,
		notificationID, userID,
	)
	return err
}

// UnreadCount 返回未读通知数。
func (s *notifyStore) UnreadCount(ctx context.Context, userID string) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications
		 WHERE user_id = $1 AND read_at IS NULL`,
		userID,
	).Scan(&count)
	return count, err
}
