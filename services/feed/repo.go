package main

import (
	"context"
	"database/sql"
	"fmt"
)

// FeedRepository handles all feed database operations.
type FeedRepository struct {
	db *sql.DB
}

func NewFeedRepository(db *sql.DB) *FeedRepository {
	return &FeedRepository{db: db}
}

const (
	createFeedSQL = `
		INSERT INTO posts (author_id, blocks)
		VALUES ($1, $2)
		RETURNING id, created_at, updated_at`

	getFeedByIDSQL = `
		SELECT id, author_id, blocks, created_at, updated_at
		FROM posts WHERE id = $1`

	deleteFeedSQL = `
		DELETE FROM posts WHERE id = $1 AND author_id = $2`
)

func (r *FeedRepository) Create(ctx context.Context, authorID string, blocks Blocks) (*Feed, error) {
	f := &Feed{AuthorID: authorID, Blocks: blocks}
	err := r.db.QueryRowContext(ctx, createFeedSQL, authorID, blocks).
		Scan(&f.ID, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create feed: %w", err)
	}
	return f, nil
}

func (r *FeedRepository) FindByID(ctx context.Context, id string) (*Feed, error) {
	f := &Feed{}
	err := r.db.QueryRowContext(ctx, getFeedByIDSQL, id).
		Scan(&f.ID, &f.AuthorID, &f.Blocks, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("find feed: %w", err)
	}
	return f, nil
}

func (r *FeedRepository) List(ctx context.Context, authorID string, page, pageSize int) ([]*Feed, int, error) {
	offset := (page - 1) * pageSize

	var total int
	var rows *sql.Rows
	var err error

	if authorID == "" {
		err = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM posts`).Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("count feeds: %w", err)
		}
		rows, err = r.db.QueryContext(ctx,
			`SELECT id, author_id, blocks, created_at, updated_at FROM posts
			 ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
			pageSize, offset)
	} else {
		err = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM posts WHERE author_id = $1`, authorID).Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("count feeds: %w", err)
		}
		rows, err = r.db.QueryContext(ctx,
			`SELECT id, author_id, blocks, created_at, updated_at FROM posts
			 WHERE author_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			authorID, pageSize, offset)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("list feeds: %w", err)
	}
	defer rows.Close()

	feeds := make([]*Feed, 0)
	for rows.Next() {
		f := &Feed{}
		if err := rows.Scan(&f.ID, &f.AuthorID, &f.Blocks, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan feed: %w", err)
		}
		feeds = append(feeds, f)
	}
	return feeds, total, rows.Err()
}

func (r *FeedRepository) Delete(ctx context.Context, id, authorID string) error {
	result, err := r.db.ExecContext(ctx, deleteFeedSQL, id, authorID)
	if err != nil {
		return fmt.Errorf("delete feed: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("delete feed: not found or not authorized")
	}
	return nil
}
