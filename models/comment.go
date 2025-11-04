package models

import (
	"time"

	"github.com/google/uuid"
)

type Comment struct {
	ID        uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	PodcastID uuid.UUID  `gorm:"type:uuid;not null;index" json:"podcast_id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	ParentID  *uuid.UUID `gorm:"type:uuid;index" json:"parent_id,omitempty"`
	Content   string     `gorm:"type:text;not null" json:"content"`
	CreatedAt time.Time  `gorm:"autoCreateTime" json:"created_at"`

	User    User      `gorm:"foreignKey:UserID" json:"user"`
	Replies []Comment `gorm:"foreignKey:ParentID;constraint:OnDelete:CASCADE;" json:"replies,omitempty"`
}
