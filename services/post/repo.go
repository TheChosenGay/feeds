package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
)

// PostRepository handles all post database operations.
type PostRepository struct {
	db *sql.DB
}

func NewPostRepository(db *sql.DB) *PostRepository {
	return &PostRepository{db: db}
}

const (
	createPostSQL = `
		INSERT INTO posts (author_id, content, post_type, media_urls)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`

	getPostByIDSQL = `
		SELECT id, author_id, content, post_type, media_urls, created_at, updated_at
		FROM posts WHERE id = $1`

	listPostsSQL = `
		SELECT id, author_id, content, post_type, media_urls, created_at, updated_at
		FROM posts
		WHERE ($1 = '' OR author_id = $1)
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	countPostsSQL = `
		SELECT COUNT(*) FROM posts
		WHERE ($1 = '' OR author_id = $1)`

	deletePostSQL = `
		DELETE FROM posts WHERE id = $1 AND author_id = $2`
)

func (r *PostRepository) Create(ctx context.Context, authorID, content, postType string, mediaURLs []string) (*Post, error) {
	p := &Post{AuthorID: authorID, Content: content, PostType: postType, MediaURLs: mediaURLs}
	err := r.db.QueryRowContext(ctx, createPostSQL, authorID, content, postType, pq.Array(mediaURLs)).
		Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create post: %w", err)
	}
	return p, nil
}

func (r *PostRepository) FindByID(ctx context.Context, id string) (*Post, error) {
	p := &Post{}
	urls := pq.StringArray{}
	err := r.db.QueryRowContext(ctx, getPostByIDSQL, id).
		Scan(&p.ID, &p.AuthorID, &p.Content, &p.PostType, &urls, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("find post: %w", err)
	}
	p.MediaURLs = []string(urls)
	return p, nil
}

func (r *PostRepository) List(ctx context.Context, authorID string, page, pageSize int) ([]*Post, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, countPostsSQL, authorID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count posts: %w", err)
	}

	offset := (page - 1) * pageSize
	rows, err := r.db.QueryContext(ctx, listPostsSQL, authorID, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list posts: %w", err)
	}
	defer rows.Close()

	posts := make([]*Post, 0)
	for rows.Next() {
		p := &Post{}
		urls := pq.StringArray{}
		if err := rows.Scan(&p.ID, &p.AuthorID, &p.Content, &p.PostType, &urls, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan post: %w", err)
		}
		p.MediaURLs = []string(urls)
		posts = append(posts, p)
	}
	return posts, total, rows.Err()
}

func (r *PostRepository) Delete(ctx context.Context, id, authorID string) error {
	result, err := r.db.ExecContext(ctx, deletePostSQL, id, authorID)
	if err != nil {
		return fmt.Errorf("delete post: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("delete post: not found or not authorized")
	}
	return nil
}
