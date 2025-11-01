package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vnkhanh/e-podcast-backend/config"
	"github.com/vnkhanh/e-podcast-backend/models"
)

type CreateNoteRequest struct {
	PodcastID uuid.UUID `json:"podcast_id" binding:"required"`
	Content   string    `json:"content" binding:"required"`
	Position  int       `json:"position" binding:"required"`
}

// Tạo ghi chú
func CreateNote(c *gin.Context) {
	var req CreateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Lấy user_id an toàn (middleware thường set kiểu string)
	userIDStr, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Không tìm thấy user_id"})
		return
	}

	var userID uuid.UUID
	switch v := userIDStr.(type) {
	case string:
		parsed, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
			return
		}
		userID = parsed
	case uuid.UUID:
		userID = v
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Kiểu user_id không hợp lệ"})
		return
	}

	note := models.Note{
		UserID:    userID,
		PodcastID: req.PodcastID,
		Content:   req.Content,
		Position:  &req.Position,
	}

	if err := config.DB.Create(&note).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo ghi chú"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"note": note})
}

// Lấy tất cả ghi chú theo podcast
func GetNotesByPodcast(c *gin.Context) {
	podcastID := c.Param("id")

	var notes []models.Note
	if err := config.DB.
		Where("podcast_id = ?", podcastID).
		Preload("User").
		Order("position ASC").
		Find(&notes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy ghi chú"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"notes": notes})
}

// Xoá ghi chú (chỉ xoá nếu đúng user)
func DeleteNote(c *gin.Context) {
	noteID := c.Param("id")

	userIDStr, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Không tìm thấy user_id"})
		return
	}

	var userID uuid.UUID
	switch v := userIDStr.(type) {
	case string:
		parsed, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
			return
		}
		userID = parsed
	case uuid.UUID:
		userID = v
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Kiểu user_id không hợp lệ"})
		return
	}

	if err := config.DB.
		Where("id = ? AND user_id = ?", noteID, userID).
		Delete(&models.Note{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xoá ghi chú"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Đã xoá ghi chú"})
}
