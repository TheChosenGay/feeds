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
