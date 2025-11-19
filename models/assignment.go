package models

import (
	"time"

	"github.com/google/uuid"
)

// ASSIGNMENT (BÀI TẬP)
type Assignment struct {
	ID          uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	PodcastID   uuid.UUID  `gorm:"type:uuid;not null" json:"podcast_id"`
	Podcast     Podcast    `gorm:"constraint:OnDelete:CASCADE;" json:"podcast"`
	Title       string     `gorm:"type:varchar(255);not null" json:"title"`
	Description string     `gorm:"type:text" json:"description"`
	DueDate     *time.Time `json:"due_date,omitempty"`                              // Hạn nộp
	MaxAttempts int        `gorm:"default:1" json:"max_attempts"`                   // Số lần làm tối đa
	TimeLimit   int        `gorm:"default:0" json:"time_limit"`                     // Giới hạn thời gian (phút), 0 = không giới hạn
	PassScore   float64    `gorm:"type:numeric(5,2);default:5.0" json:"pass_score"` // Điểm đạt
	IsPublished bool       `gorm:"default:false" json:"is_published"`               // Đã công bố chưa

	HasPassword bool   `gorm:"default:false" json:"has_password"`
	Password    string `gorm:"type:varchar(255)" json:"password,omitempty"`
	AllowReview bool   `gorm:"default:true" json:"allow_review"` // Cho phép sinh viên xem đáp án

	CreatedBy uuid.UUID            `gorm:"type:uuid;not null" json:"created_by"`
	Creator   User                 `gorm:"foreignKey:CreatedBy;references:ID;constraint:OnDelete:CASCADE;" json:"creator"`
	CreatedAt time.Time            `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time            `gorm:"autoUpdateTime" json:"updated_at"`
	Questions []AssignmentQuestion `gorm:"foreignKey:AssignmentID;constraint:OnDelete:CASCADE;" json:"questions"`
}

// ASSIGNMENT QUESTION
type AssignmentQuestion struct {
	ID           uuid.UUID          `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AssignmentID uuid.UUID          `gorm:"type:uuid;not null" json:"assignment_id"`
	Assignment   Assignment         `gorm:"foreignKey:AssignmentID;references:ID;constraint:OnDelete:CASCADE;" json:"-"`
	Question     string             `gorm:"type:text;not null" json:"question"`
	Difficulty   string             `gorm:"type:varchar(50);default:'medium'" json:"difficulty"` // "easy", "medium", "hard"
	Explanation  string             `gorm:"type:text" json:"explanation"`                        // Giải thích đáp án
	Points       float64            `gorm:"type:numeric(5,2);default:1.0" json:"points"`         // Điểm của câu hỏi
	SortOrder    int                `gorm:"default:0" json:"sort_order"`                         // Thứ tự hiển thị
	CreatedAt    time.Time          `gorm:"autoCreateTime" json:"created_at"`
	Options      []AssignmentOption `gorm:"foreignKey:QuestionID;constraint:OnDelete:CASCADE;" json:"options"`
}

// ASSIGNMENT OPTION
type AssignmentOption struct {
	ID         uuid.UUID          `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	QuestionID uuid.UUID          `gorm:"type:uuid;not null" json:"question_id"`
	Question   AssignmentQuestion `gorm:"foreignKey:QuestionID;references:ID;constraint:OnDelete:CASCADE;" json:"-"`
	OptionText string             `gorm:"type:text;not null" json:"option_text"`
	IsCorrect  bool               `gorm:"default:false" json:"is_correct"`
	SortOrder  int                `gorm:"default:0" json:"sort_order"`
}

// ASSIGNMENT SUBMISSION (BÀI LÀM)
type AssignmentSubmission struct {
	ID           uuid.UUID          `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	AssignmentID uuid.UUID          `gorm:"type:uuid;not null" json:"assignment_id"`
	Assignment   Assignment         `gorm:"constraint:OnDelete:CASCADE;" json:"assignment"`
	UserID       uuid.UUID          `gorm:"type:uuid;not null" json:"user_id"`
	User         User               `gorm:"constraint:OnDelete:CASCADE;" json:"user"`
	AttemptNum   int                `gorm:"default:1" json:"attempt_num"` // Lần làm thứ mấy
	Score        float64            `gorm:"type:numeric(5,2)" json:"score"`
	MaxScore     float64            `gorm:"type:numeric(5,2)" json:"max_score"` // Tổng điểm tối đa
	IsPassed     bool               `gorm:"default:false" json:"is_passed"`
	TimeSpent    int                `json:"time_spent"` // Thời gian làm bài (giây)
	StartedAt    time.Time          `gorm:"autoCreateTime" json:"started_at"`
	SubmittedAt  time.Time          `gorm:"autoUpdateTime" json:"submitted_at"`
	Answers      []AssignmentAnswer `gorm:"foreignKey:SubmissionID;constraint:OnDelete:CASCADE;" json:"answers"`
}

// ASSIGNMENT ANSWER (CÂU TRẢ LỜI)
type AssignmentAnswer struct {
	ID             uuid.UUID            `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	SubmissionID   uuid.UUID            `gorm:"type:uuid;not null" json:"submission_id"`
	Submission     AssignmentSubmission `gorm:"foreignKey:SubmissionID;references:ID;constraint:OnDelete:CASCADE;" json:"-"`
	QuestionID     uuid.UUID            `gorm:"type:uuid;not null" json:"question_id"`
	Question       AssignmentQuestion   `gorm:"foreignKey:QuestionID;references:ID;constraint:OnDelete:CASCADE;" json:"question"`
	SelectedID     uuid.UUID            `gorm:"type:uuid" json:"selected_id"` // uuid.Nil nếu bỏ trống
	SelectedOption AssignmentOption     `gorm:"foreignKey:SelectedID;references:ID;" json:"selected_option,omitempty"`
	IsCorrect      bool                 `gorm:"default:false" json:"is_correct"`
	PointsEarned   float64              `gorm:"type:numeric(5,2)" json:"points_earned"`
	AnsweredAt     time.Time            `gorm:"autoCreateTime" json:"answered_at"`
}
