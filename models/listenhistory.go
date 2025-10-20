package models

import (
	"time"

	"github.com/google/uuid"
)

type PodcastListenHistory struct {
	ID        uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	PodcastID uuid.UUID  `gorm:"type:uuid;not null" json:"podcast_id"`
	UserID    *uuid.UUID `gorm:"type:uuid" json:"user_id,omitempty"` // nil nếu guest
	Seconds   int        `gorm:"not null" json:"seconds"`            // đã nghe bao nhiêu giây
	CreatedAt time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	Podcast Podcast `gorm:"constraint:OnDelete:CASCADE;" json:"podcast"`
	User    *User   `gorm:"constraint:OnDelete:SET NULL;" json:"user,omitempty"`
}
