package models

import (
	"time"

	"github.com/google/uuid"
)

type Chapter struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	SubjectID uuid.UUID `gorm:"type:uuid;not null" json:"subject_id"`
	Subject   Subject   `gorm:"constraint:OnDelete:CASCADE;"`
	Title     string    `gorm:"size:255;not null" json:"title"`
	SortOrder int       `gorm:"column:sort_order;default:1" json:"sort_order"` // Thứ tự chương
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	Podcasts  []Podcast `gorm:"foreignKey:ChapterID" json:"podcasts"`
}
