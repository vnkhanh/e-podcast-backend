package models

import (
	"time"

	"github.com/google/uuid"
)

type Note struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	User      User      `gorm:"constraint:OnDelete:CASCADE;" json:"user"`
	PodcastID uuid.UUID `gorm:"type:uuid;not null" json:"podcast_id"`
	Podcast   Podcast   `gorm:"constraint:OnDelete:CASCADE;" json:"podcast"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	Position  *int      `json:"position"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
