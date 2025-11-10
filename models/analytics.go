package models

import (
	"time"

	"github.com/google/uuid"
)

// Lưu thống kê tổng hợp theo ngày
type ListeningAnalytics struct {
	ID   uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Date time.Time `gorm:"type:date;not null;uniqueIndex:idx_analytics_date" json:"date"`

	TotalListens     int64 `gorm:"default:0" json:"total_listens"`
	UniqueUsers      int64 `gorm:"default:0" json:"unique_users"`
	CompletedListens int64 `gorm:"default:0" json:"completed_listens"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// Thống kê chi tiết theo podcast
type PodcastAnalytics struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Date      time.Time `gorm:"type:date;not null;uniqueIndex:idx_podcast_analytics" json:"date"`
	PodcastID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_podcast_analytics;index" json:"podcast_id"`

	TotalPlays      int64 `gorm:"default:0" json:"total_plays"`
	UniqueListeners int64 `gorm:"default:0" json:"unique_listeners"`
	CompletedPlays  int64 `gorm:"default:0" json:"completed_plays"`
	TotalDuration   int64 `gorm:"default:0" json:"total_duration"` // Tổng thời gian nghe (giây)

	Podcast   Podcast   `gorm:"foreignKey:PodcastID;constraint:OnDelete:CASCADE" json:"podcast,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// Thống kê theo môn học
type SubjectAnalytics struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Date      time.Time `gorm:"type:date;not null;uniqueIndex:idx_subject_analytics" json:"date"`
	SubjectID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_subject_analytics;index" json:"subject_id"`

	TotalPlays int64 `gorm:"default:0" json:"total_plays"`

	Subject   Subject   `gorm:"foreignKey:SubjectID;constraint:OnDelete:CASCADE" json:"subject,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

type ListeningEvent struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID     uuid.UUID `gorm:"type:uuid;not null;index"`
	PodcastID  uuid.UUID `gorm:"type:uuid;not null;index"`
	ListenedAt time.Time `gorm:"not null;index"`
	Duration   int       // Thời gian nghe trong session này
	Position   int       // Vị trí bắt đầu
	Completed  bool      // Hoàn thành trong session này không
}
