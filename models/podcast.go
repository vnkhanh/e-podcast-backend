package models

import (
	"time"

	"github.com/google/uuid"
)

type Podcast struct {
	ID          uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	ChapterID   uuid.UUID  `gorm:"type:uuid;" json:"chapter_id"`
	Chapter     Chapter    `gorm:"constraint:RESTRICT:CASCADE;preload:true"`
	DocumentID  uuid.UUID  `gorm:"type:uuid;not null" json:"document_id"`
	Document    Document   `gorm:"constraint:RESTRICT:CASCADE;"`
	Title       string     `gorm:"size:255;not null" json:"title"`
	Description string     `gorm:"type:text" json:"description"`
	AudioURL    string     `gorm:"type:text;not null" json:"audio_url"`
	DurationSec int        `json:"duration_sec"`
	Summary     string     `gorm:"type:text" json:"summary"`
	ViewCount   int        `gorm:"default:0" json:"view_count"`
	LikeCount   int        `gorm:"default:0" json:"like_count"`
	Status      string     `gorm:"type:VARCHAR(20);default:'draft'" json:"status"` // draft | published | archived
	CoverImage  string     `gorm:"type:text" json:"cover_image"`
	CreatedBy   uuid.UUID  `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt   time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
	UpdatedBy   *uuid.UUID `gorm:"type:uuid" json:"updated_by"`
	PublishedAt *time.Time `json:"published_at"`

	Categories []Category `gorm:"many2many:podcast_categories" json:"categories"`
	Topics     []Topic    `gorm:"many2many:podcast_topics" json:"topics"`
	Tags       []Tag      `gorm:"many2many:podcast_tags" json:"tags"`
}
