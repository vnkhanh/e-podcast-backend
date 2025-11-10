package models

import (
	"time"

	"github.com/google/uuid"
)

type Favorite struct {
	UserID    uuid.UUID `gorm:"type:uuid;primaryKey" json:"user_id"`
	PodcastID uuid.UUID `gorm:"type:uuid;primaryKey" json:"podcast_id"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`

	User    User    `gorm:"constraint:OnDelete:CASCADE;" json:"user,omitempty"`
	Podcast Podcast `gorm:"constraint:OnDelete:CASCADE;" json:"podcast,omitempty"`
}
