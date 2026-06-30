package main

import (
	"context"
	"database/sql"
	"fmt"
)

// UserRepository handles all user database operations.
type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

const (
	createUserSQL = `
		INSERT INTO users (username, password, avatar_url)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at`

	getUserByIDSQL = `
		SELECT id, username, password, avatar_url, created_at, updated_at
		FROM users WHERE id = $1`

	getUserByUsernameSQL = `
		SELECT id, username, password, avatar_url, created_at, updated_at
		FROM users WHERE username = $1`

	deleteUserSQL = `DELETE FROM users WHERE id = $1`
)

func (r *UserRepository) Create(ctx context.Context, username, passwordHash string) (*User, error) {
	u := &User{Username: username, Password: passwordHash}
	err := r.db.QueryRowContext(ctx, createUserSQL, username, passwordHash, "").
		Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*User, error) {
	u := &User{}
	err := r.db.QueryRowContext(ctx, getUserByIDSQL, id).
		Scan(&u.ID, &u.Username, &u.Password, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return u, nil
}

func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*User, error) {
	u := &User{}
	err := r.db.QueryRowContext(ctx, getUserByUsernameSQL, username).
		Scan(&u.ID, &u.Username, &u.Password, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("find user by username: %w", err)
	}
	return u, nil
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, deleteUserSQL, id)
	return err
}

// --- Follow ---

// FollowRepo handles follow database operations.
type FollowRepo struct {
	db *sql.DB
}

func NewFollowRepo(db *sql.DB) *FollowRepo {
	return &FollowRepo{db: db}
}

const (
	followSQL = `
		INSERT INTO follows (follower_id, followed_id)
		VALUES ($1, $2)
		ON CONFLICT (follower_id, followed_id) DO NOTHING`

	unfollowSQL = `
		DELETE FROM follows WHERE follower_id = $1 AND followed_id = $2`

	getFollowersSQL = `
		SELECT follower_id FROM follows WHERE followed_id = $1
		ORDER BY created_at DESC`

	getFollowingSQL = `
		SELECT followed_id FROM follows WHERE follower_id = $1
		ORDER BY created_at DESC`

	isFollowingSQL = `
		SELECT EXISTS(SELECT 1 FROM follows WHERE follower_id = $1 AND followed_id = $2)`

	countFollowersSQL = `SELECT COUNT(*) FROM follows WHERE followed_id = $1`

	countFollowingSQL = `SELECT COUNT(*) FROM follows WHERE follower_id = $1`
)

// Follow creates a follow relationship (idempotent).
func (r *FollowRepo) Follow(ctx context.Context, followerID, followedID string) error {
	_, err := r.db.ExecContext(ctx, followSQL, followerID, followedID)
	if err != nil {
		return fmt.Errorf("follow: %w", err)
	}
	return nil
}

// Unfollow removes a follow relationship.
func (r *FollowRepo) Unfollow(ctx context.Context, followerID, followedID string) error {
	_, err := r.db.ExecContext(ctx, unfollowSQL, followerID, followedID)
	if err != nil {
		return fmt.Errorf("unfollow: %w", err)
	}
	return nil
}

// GetFollowers returns all user IDs who follow the given user.
func (r *FollowRepo) GetFollowers(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, getFollowersSQL, userID)
	if err != nil {
		return nil, fmt.Errorf("get followers: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan follower: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetFollowing returns all user IDs the given user is following.
func (r *FollowRepo) GetFollowing(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, getFollowingSQL, userID)
	if err != nil {
		return nil, fmt.Errorf("get following: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan following: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// IsFollowing checks if followerID follows followedID.
func (r *FollowRepo) IsFollowing(ctx context.Context, followerID, followedID string) (bool, error) {
	var ok bool
	err := r.db.QueryRowContext(ctx, isFollowingSQL, followerID, followedID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("is following: %w", err)
	}
	return ok, nil
}

// CountFollowers returns the number of followers for a user.
func (r *FollowRepo) CountFollowers(ctx context.Context, userID string) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, countFollowersSQL, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count followers: %w", err)
	}
	return count, nil
}

// CountFollowing returns the number of users the given user follows.
func (r *FollowRepo) CountFollowing(ctx context.Context, userID string) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, countFollowingSQL, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count following: %w", err)
	}
	return count, nil
}
