package main

import "time"

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"-"` // never serialize
	AvatarURL string    `json:"avatar_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Follow represents a follow relationship.
type Follow struct {
	FollowerID string    `json:"follower_id"`
	FollowedID string    `json:"followed_id"`
	CreatedAt  time.Time `json:"created_at"`
}
