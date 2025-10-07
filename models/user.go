package models

import (
	"time"

	"github.com/google/uuid"
)

type UserRole string

const (
	RoleAdmin    UserRole = "admin"   // Quản trị hệ thống
	RoleLecturer UserRole = "teacher" // Giảng viên (quản trị nội dung)
	RoleUser     UserRole = "student" // Sinh viên (người dùng)
)

type User struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	FullName  string    `gorm:"size:150;not null" json:"full_name"`
	Email     string    `gorm:"size:150;uniqueIndex;not null" json:"email"`
	Password  string    `gorm:"type:text;not null" json:"-"`
	Role      UserRole  `gorm:"type:varchar(20);not null;default:'user'" json:"role"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Quan hệ
	Documents  []Document  `json:"documents"`
	Favorites  []Favorite  `json:"favorites"`
	Notes      []Note      `json:"notes"`
	Flashcards []Flashcard `json:"flashcards"`
}
