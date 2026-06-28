package main

import (
	"context"
	"database/sql"
	"fmt"
)

// --- Like ---

// LikeRepo handles post_likes database operations.
type LikeRepo struct {
	db *sql.DB
}

func NewLikeRepo(db *sql.DB) *LikeRepo {
	return &LikeRepo{db: db}
}

const (
	likeSQL = `
		INSERT INTO post_likes (user_id, post_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, post_id) DO NOTHING`

	unlikeSQL = `
		DELETE FROM post_likes WHERE user_id = $1 AND post_id = $2`

	countLikesSQL = `
		SELECT COUNT(*) FROM post_likes WHERE post_id = $1`

	isLikedSQL = `
		SELECT EXISTS(SELECT 1 FROM post_likes WHERE user_id = $1 AND post_id = $2)`
)

// Insert adds a like row, idempotent (ON CONFLICT DO NOTHING).
func (r *LikeRepo) Insert(ctx context.Context, userID, postID string) error {
	_, err := r.db.ExecContext(ctx, likeSQL, userID, postID)
	if err != nil {
		return fmt.Errorf("insert like: %w", err)
	}
	return nil
}

// Delete removes a like row.
func (r *LikeRepo) Delete(ctx context.Context, userID, postID string) error {
	_, err := r.db.ExecContext(ctx, unlikeSQL, userID, postID)
	if err != nil {
		return fmt.Errorf("delete like: %w", err)
	}
	return nil
}

func (r *LikeRepo) Count(ctx context.Context, postID string) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, countLikesSQL, postID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count likes: %w", err)
	}
	return count, nil
}

func (r *LikeRepo) IsLiked(ctx context.Context, userID, postID string) (bool, error) {
	var liked bool
	err := r.db.QueryRowContext(ctx, isLikedSQL, userID, postID).Scan(&liked)
	if err != nil {
		return false, fmt.Errorf("is liked: %w", err)
	}
	return liked, nil
}

// --- Comment ---

// CommentRepo handles post_comments database operations.
type CommentRepo struct {
	db *sql.DB
}

func NewCommentRepo(db *sql.DB) *CommentRepo {
	return &CommentRepo{db: db}
}

const (
	createCommentSQL = `
		INSERT INTO post_comments (user_id, post_id, content)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`

	listCommentsSQL = `
		SELECT id, user_id, post_id, content, created_at
		FROM post_comments WHERE post_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	countCommentsSQL = `
		SELECT COUNT(*) FROM post_comments WHERE post_id = $1`

	deleteCommentSQL = `
		DELETE FROM post_comments WHERE id = $1 AND user_id = $2`
)

func (r *CommentRepo) Create(ctx context.Context, userID, postID, content string) (*Comment, error) {
	c := &Comment{UserID: userID, PostID: postID, Content: content}
	err := r.db.QueryRowContext(ctx, createCommentSQL, userID, postID, content).
		Scan(&c.ID, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create comment: %w", err)
	}
	return c, nil
}

func (r *CommentRepo) ListByPost(ctx context.Context, postID string, page, pageSize int) ([]*Comment, int, error) {
	offset := (page - 1) * pageSize

	var total int
	err := r.db.QueryRowContext(ctx, countCommentsSQL, postID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count comments: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, listCommentsSQL, postID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()

	comments := make([]*Comment, 0)
	for rows.Next() {
		c := &Comment{}
		if err := rows.Scan(&c.ID, &c.UserID, &c.PostID, &c.Content, &c.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan comment: %w", err)
		}
		comments = append(comments, c)
	}
	return comments, total, rows.Err()
}

func (r *CommentRepo) Delete(ctx context.Context, id, userID string) error {
	result, err := r.db.ExecContext(ctx, deleteCommentSQL, id, userID)
	if err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("delete comment: not found or not authorized")
	}
	return nil
}

// --- Bookmark ---

// BookmarkRepo handles post_bookmarks database operations.
type BookmarkRepo struct {
	db *sql.DB
}

func NewBookmarkRepo(db *sql.DB) *BookmarkRepo {
	return &BookmarkRepo{db: db}
}

const (
	bookmarkSQL = `
		INSERT INTO post_bookmarks (user_id, post_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, post_id) DO NOTHING`

	unbookmarkSQL = `
		DELETE FROM post_bookmarks WHERE user_id = $1 AND post_id = $2`

	isBookmarkedSQL = `
		SELECT EXISTS(SELECT 1 FROM post_bookmarks WHERE user_id = $1 AND post_id = $2)`
)

func (r *BookmarkRepo) Bookmark(ctx context.Context, userID, postID string) error {
	_, err := r.db.ExecContext(ctx, bookmarkSQL, userID, postID)
	if err != nil {
		return fmt.Errorf("bookmark: %w", err)
	}
	return nil
}

func (r *BookmarkRepo) Unbookmark(ctx context.Context, userID, postID string) error {
	_, err := r.db.ExecContext(ctx, unbookmarkSQL, userID, postID)
	if err != nil {
		return fmt.Errorf("unbookmark: %w", err)
	}
	return nil
}

func (r *BookmarkRepo) IsBookmarked(ctx context.Context, userID, postID string) (bool, error) {
	var bookmarked bool
	err := r.db.QueryRowContext(ctx, isBookmarkedSQL, userID, postID).Scan(&bookmarked)
	if err != nil {
		return false, fmt.Errorf("is bookmarked: %w", err)
	}
	return bookmarked, nil
}
