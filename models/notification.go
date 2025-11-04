package models

import (
	"time"

	"github.com/google/uuid"
)

type Notification struct {
	ID      uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID  uuid.UUID `gorm:"type:uuid;not null" json:"user_id"` // người nhận
	Title   string    `gorm:"size:255;not null" json:"title"`
	Message string    `gorm:"type:text;not null" json:"message"`
	Type    string    `gorm:"size:50" json:"type"`
	IsRead  bool      `gorm:"default:false" json:"is_read"`

	PodcastID  *uuid.UUID `gorm:"type:uuid" json:"podcast_id,omitempty"` // ID podcast liên quan
	CommentID  *uuid.UUID `gorm:"type:uuid" json:"comment_id,omitempty"` // ID comment liên quan (nếu có)
	RelatedURL *string    `gorm:"size:500" json:"related_url,omitempty"` // URL đích (tùy chọn)

	CreatedAt time.Time  `gorm:"autoCreateTime" json:"created_at"`
	ReadAt    *time.Time `json:"read_at,omitempty"`

	User User `gorm:"constraint:OnDelete:CASCADE;" json:"user,omitempty"`
}
