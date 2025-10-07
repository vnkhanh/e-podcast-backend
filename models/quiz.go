package models

import (
	"time"

	"github.com/google/uuid"
)

type QuizQuestion struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	PodcastID uuid.UUID `gorm:"type:uuid;not null" json:"podcast_id"`
	Podcast   Podcast   `gorm:"constraint:OnDelete:CASCADE;"`

	CreatedBy     uuid.UUID `gorm:"type:uuid;not null" json:"created_by"`
	CreatedByUser User      `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnDelete:CASCADE;"`

	Question   string       `gorm:"type:text;not null" json:"question"`
	Difficulty string       `gorm:"size:20;default:'easy'" json:"difficulty"`
	CreatedAt  time.Time    `gorm:"autoCreateTime" json:"created_at"`
	Options    []QuizOption `gorm:"foreignKey:QuestionID"`
}

type QuizOption struct {
	ID         uuid.UUID    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	QuestionID uuid.UUID    `gorm:"type:uuid;not null" json:"question_id"`
	Question   QuizQuestion `gorm:"foreignKey:QuestionID;references:ID;constraint:OnDelete:CASCADE;"`
	OptionText string       `gorm:"type:text;not null" json:"option_text"`
	IsCorrect  bool         `gorm:"default:false" json:"is_correct"`
}

type QuizAttempt struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	User      User      `gorm:"constraint:OnDelete:CASCADE;"`
	PodcastID uuid.UUID `gorm:"type:uuid;not null" json:"podcast_id"`
	Podcast   Podcast   `gorm:"constraint:OnDelete:CASCADE;"`
	Score     float64   `gorm:"type:numeric(5,2)" json:"score"`
	TakenAt   time.Time `gorm:"autoCreateTime" json:"taken_at"`
}
