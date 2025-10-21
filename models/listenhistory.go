package models

import (
	"time"

	"github.com/google/uuid"
)

// Lịch sử nghe podcast
type ListeningHistory struct {
	ID uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`

	UserID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_user_podcast" json:"user_id"`
	PodcastID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_user_podcast" json:"podcast_id"`

	LastPosition    int        `json:"last_position"` // Vị trí nghe cuối (giây)
	Duration        int        `json:"duration"`      // Tổng thời lượng podcast (giây)
	Completed       bool       `gorm:"default:false" json:"completed"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	LastListenedAt  time.Time  `gorm:"autoUpdateTime" json:"last_listened_at"`
	FirstListenedAt time.Time  `gorm:"autoCreateTime" json:"first_listened_at"`

	// Khóa ngoại
	User    User    `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Podcast Podcast `gorm:"foreignKey:PodcastID;constraint:OnDelete:CASCADE" json:"podcast"`
}
