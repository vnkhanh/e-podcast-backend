package models

import (
	"time"

	"github.com/google/uuid"
)

type Flashcard struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	User      User      `gorm:"constraint:OnDelete:CASCADE;" json:"user"`
	PodcastID uuid.UUID `gorm:"type:uuid;not null" json:"podcast_id"`
	Podcast   Podcast   `gorm:"constraint:OnDelete:CASCADE;" json:"podcast"`
	FrontText string    `gorm:"type:text;not null" json:"front_text"`
	BackText  string    `gorm:"type:text;not null" json:"back_text"`

	SourceText    string `gorm:"type:text" json:"source_text"`
	ReferenceText string `gorm:"type:text" json:"reference_text"` // đoạn tài liệu gốc (để hiển thị trích dẫn)
	ChunkIndex    int    `json:"chunk_index"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
