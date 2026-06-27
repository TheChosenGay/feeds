package main

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// Block represents a single content block in a rich feed.
type Block struct {
	Type      string `json:"type"`                // text / image / video
	Content   string `json:"content,omitempty"`   // text content
	URL       string `json:"url,omitempty"`       // image/video URL
	CoverURL  string `json:"cover_url,omitempty"` // video cover thumbnail
	Width     int32  `json:"width,omitempty"`     // pixels
	Height    int32  `json:"height,omitempty"`    // pixels
	Duration  int32  `json:"duration,omitempty"`  // video duration (seconds)
	Size      int64  `json:"size,omitempty"`      // file size (bytes)
}

// Blocks is a slice of Block that implements sql.Scanner and driver.Valuer for JSONB.
type Blocks []Block

func (b *Blocks) Scan(src any) error {
	if src == nil {
		*b = Blocks{}
		return nil
	}
	var data []byte

	switch v := src.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("blocks: unsupported scan type %T", src)
	}
	return json.Unmarshal(data, b)
}

func (b Blocks) Value() (driver.Value, error) {
	if b == nil {
		return "[]", nil
	}
	return json.Marshal(b)
}

type Feed struct {
	ID        string    `json:"id"`
	AuthorID  string    `json:"author_id"`
	Blocks    Blocks    `json:"blocks"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
