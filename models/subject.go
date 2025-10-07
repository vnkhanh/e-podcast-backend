package models

import (
	"time"

	"github.com/google/uuid"
)

type Subject struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name      string    `gorm:"size:255;not null;unique" json:"name"`
	Status    bool      `gorm:"default:true;not null" json:"status"` // trạng thái (true: active, false: inactive)
	Slug      string    `gorm:"size:255;uniqueIndex" json:"slug"`    // slug cho URL thân thiện
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
	Chapters  []Chapter `gorm:"foreignKey:SubjectID" json:"chapters"`
}
