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

type PodcastShare struct {
	ID          uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	SenderID    uuid.UUID  `gorm:"type:uuid;not null" json:"sender_id"`
	Sender      User       `gorm:"constraint:OnDelete:CASCADE;" json:"sender"`
	PodcastID   uuid.UUID  `gorm:"type:uuid;not null" json:"podcast_id"`
	Podcast     Podcast    `gorm:"constraint:OnDelete:CASCADE;" json:"podcast"`
	RecipientID *uuid.UUID `json:"recipient_id"`
	Recipient   *User      `gorm:"constraint:OnDelete:SET NULL;" json:"recipient,omitempty"`
	Email       *string    `gorm:"size:255" json:"email,omitempty"`
	Message     *string    `gorm:"type:text" json:"message,omitempty"`
	SharedAt    time.Time  `gorm:"autoCreateTime" json:"shared_at"`
}
