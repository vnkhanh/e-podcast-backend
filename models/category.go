package models

import (
	"time"

	"github.com/google/uuid"
)

type Category struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name      string    `gorm:"size:100;not null;unique" json:"name"`
	Slug      string    `gorm:"size:100;uniqueIndex" json:"slug"`
	Status    bool      `gorm:"default:true" json:"status"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	Podcasts  []Podcast `gorm:"many2many:podcast_categories" json:"podcasts"`
}
