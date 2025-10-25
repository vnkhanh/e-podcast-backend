package models

import (
	"time"

	"github.com/google/uuid"
)

// ============================
// QUIZ SET (NHÓM CÂU HỎI)
// ============================
type QuizSet struct {
	ID          uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	PodcastID   uuid.UUID      `gorm:"type:uuid;not null" json:"podcast_id"`
	Podcast     Podcast        `gorm:"constraint:OnDelete:CASCADE;" json:"podcast"`
	Title       string         `gorm:"type:varchar(255);not null" json:"title"`
	Description string         `gorm:"type:text" json:"description"`
	CreatedBy   uuid.UUID      `gorm:"type:uuid;not null" json:"created_by"`
	Creator     User           `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnDelete:CASCADE;" json:"creator"`
	CreatedAt   time.Time      `gorm:"autoCreateTime" json:"created_at"`
	Questions   []QuizQuestion `gorm:"foreignKey:QuizSetID;constraint:OnDelete:CASCADE;" json:"questions"`
}

// ============================
// QUIZ QUESTION (CÂU HỎI)
// ============================
type QuizQuestion struct {
	ID         uuid.UUID    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	QuizSetID  uuid.UUID    `gorm:"type:uuid;not null" json:"quiz_set_id"`
	QuizSet    QuizSet      `gorm:"foreignKey:QuizSetID;references:ID;constraint:OnDelete:CASCADE;" json:"quiz_set"`
	Question   string       `gorm:"type:text;not null" json:"question"`
	Difficulty string       `gorm:"size:20;default:'easy'" json:"difficulty"`
	SourceText string       `gorm:"type:text" json:"source_text"`
	Hint       string       `gorm:"type:text" json:"hint"`
	CreatedAt  time.Time    `gorm:"autoCreateTime" json:"created_at"`
	Options    []QuizOption `gorm:"foreignKey:QuestionID;constraint:OnDelete:CASCADE;" json:"options"`
}

// ============================
// QUIZ OPTION (ĐÁP ÁN)
// ============================
type QuizOption struct {
	ID         uuid.UUID    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	QuestionID uuid.UUID    `gorm:"type:uuid;not null" json:"question_id"`
	Question   QuizQuestion `gorm:"foreignKey:QuestionID;references:ID;constraint:OnDelete:CASCADE;" json:"-"`
	OptionText string       `gorm:"type:text;not null" json:"option_text"`
	IsCorrect  bool         `gorm:"default:false" json:"is_correct"`
}

// ============================
// QUIZ ATTEMPT (LẦN LÀM BÀI)
// ============================
type QuizAttempt struct {
	ID             uuid.UUID            `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	UserID         uuid.UUID            `gorm:"type:uuid;not null" json:"user_id"`
	User           User                 `gorm:"constraint:OnDelete:CASCADE;" json:"user"`
	PodcastID      uuid.UUID            `gorm:"type:uuid;not null" json:"podcast_id"`
	Podcast        Podcast              `gorm:"constraint:OnDelete:CASCADE;" json:"podcast"`
	QuizSetID      uuid.UUID            `gorm:"type:uuid;not null" json:"quiz_set_id"`
	QuizSet        QuizSet              `gorm:"constraint:OnDelete:CASCADE;" json:"quiz_set"`
	Score          float64              `gorm:"type:numeric(5,2)" json:"score"`
	CorrectCount   int                  `json:"correct_count"`
	IncorrectCount int                  `json:"incorrect_count"`
	DurationSec    int                  `json:"duration_sec"`
	TakenAt        time.Time            `gorm:"autoCreateTime" json:"taken_at"`
	Histories      []QuizAttemptHistory `gorm:"foreignKey:AttemptID;constraint:OnDelete:CASCADE;" json:"histories"`
}

// ============================
// QUIZ ATTEMPT HISTORY (LỊCH SỬ CHI TIẾT)
// ============================
type QuizAttemptHistory struct {
	ID             uuid.UUID    `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AttemptID      uuid.UUID    `gorm:"type:uuid;not null" json:"attempt_id"`
	Attempt        QuizAttempt  `gorm:"foreignKey:AttemptID;references:ID;constraint:OnDelete:CASCADE;" json:"-"`
	QuestionID     uuid.UUID    `gorm:"type:uuid;not null" json:"question_id"`
	Question       QuizQuestion `gorm:"foreignKey:QuestionID;references:ID;constraint:OnDelete:CASCADE;" json:"question"`
	SelectedID     uuid.UUID    `gorm:"type:uuid;not null" json:"selected_id"`
	SelectedOption QuizOption   `gorm:"foreignKey:SelectedID;references:ID;constraint:OnDelete:CASCADE;" json:"selected_option"`
	IsCorrect      bool         `gorm:"default:false" json:"is_correct"`
	AnsweredAt     time.Time    `gorm:"autoCreateTime" json:"answered_at"`
}

// ============================
// DTO KẾT QUẢ LÀM BÀI
// ============================
type AnswerResult struct {
	QuestionID uuid.UUID       `json:"question_id"`
	Question   string          `json:"question"`
	SelectedID uuid.UUID       `json:"selected_id"`
	CorrectID  uuid.UUID       `json:"correct_id"`
	IsCorrect  bool            `json:"is_correct"`
	SourceText string          `json:"source_text"`
	Options    []QuizOptionDTO `json:"options"`
}

type QuizOptionDTO struct {
	ID         uuid.UUID `json:"id"`
	OptionText string    `json:"option_text"`
	IsCorrect  bool      `json:"is_correct"`
}
