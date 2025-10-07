package models

import (
	"time"

	"github.com/google/uuid"
)

type Topic struct {
	ID        uuid.UUID `json:"id" gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Name      string    `json:"name" gorm:"size:150;not null;unique"`
	Status    bool      `json:"status" gorm:"default:true"`
	Slug      string    `json:"slug" gorm:"size:150;uniqueIndex"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
	Podcasts  []Podcast `json:"-" gorm:"many2many:podcast_topics"`
}
