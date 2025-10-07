package models

import (
	"time"

	"github.com/google/uuid"
)

type Document struct {
	ID            uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID        uuid.UUID  `gorm:"type:uuid;not null" json:"user_id"` // admin
	User          User       `gorm:"constraint:OnDelete:CASCADE;" json:"user"`
	OriginalName  string     `gorm:"size:255;not null" json:"original_name"`
	FilePath      string     `gorm:"type:text;not null" json:"file_path"`
	FileType      string     `gorm:"size:50" json:"file_type"`
	FileSize      int64      `json:"file_size"` // bytes
	ExtractedText string     `gorm:"type:text" json:"extracted_text"`
	Status        string     `gorm:"size:30;default:'Đang tải lên'" json:"status"` // Đang tải lên|Đã tải lên|Đang trích xuất|Đã trích xuất|Đang tạo podcast|Hoàn thành|Lỗi
	ProcessedAt   *time.Time `json:"processed_at"`                                 // thời gian hoàn thành trích xuất và tạo podcast
	CreatedAt     time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	Podcasts []Podcast `json:"podcasts"`
}
