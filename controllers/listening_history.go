package controllers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vnkhanh/e-podcast-backend/models"
	"gorm.io/gorm"
)

// Request body cho việc cập nhật lịch sử nghe
type SavePodcastHistoryRequest struct {
	LastPosition int   `json:"last_position" binding:"required,min=0"`
	Duration     int   `json:"duration" binding:"required,min=1"` // tổng thời lượng podcast
	Completed    *bool `json:"completed,omitempty"`
}

// SavePodcastHistory lưu hoặc cập nhật lịch sử nghe podcast
// POST /api/user/account/listening-history/:podcast_id
func SavePodcastHistory(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	userIDStr := c.GetString("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
		return
	}

	podcastIDStr := c.Param("podcast_id")
	podcastID, err := uuid.Parse(podcastIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid podcast_id"})
		return
	}

	var req SavePodcastHistoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Kiểm tra podcast tồn tại
	var podcast models.Podcast
	if err := db.First(&podcast, "id = ?", podcastID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Podcast not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query podcast"})
		return
	}

	var history models.ListeningHistory
	result := db.Where("user_id = ? AND podcast_id = ?", userID, podcastID).First(&history)
	now := time.Now()

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		// Tạo mới
		history = models.ListeningHistory{
			UserID:          userID,
			PodcastID:       podcastID,
			LastPosition:    req.LastPosition,
			Duration:        req.Duration,
			FirstListenedAt: now,
			LastListenedAt:  now,
			Completed:       false,
		}
		if req.Completed != nil && *req.Completed {
			history.Completed = true
			history.CompletedAt = &now
		}

		if err := db.Create(&history).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create listening history"})
			return
		}
	} else if result.Error == nil {
		// Cập nhật
		history.LastListenedAt = now
		history.LastPosition = req.LastPosition

		// Nếu duration từ client lớn hơn -> cập nhật
		if req.Duration > history.Duration {
			history.Duration = req.Duration
		}

		if req.Completed != nil && *req.Completed && !history.Completed {
			history.Completed = true
			history.CompletedAt = &now
		}

		if err := db.Save(&history).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update listening history"})
			return
		}
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database query failed"})
		return
	}

	db.Preload("Podcast").First(&history, "id = ?", history.ID)
	c.JSON(http.StatusOK, gin.H{
		"message": "Listening history saved successfully",
		"data":    history,
	})
}

// GetListeningHistory lấy danh sách lịch sử nghe của user
// GET /api/user/account/listening-history
func GetListeningHistory(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	// Lấy user_id từ context
	userIDStr := c.GetString("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
		return
	}

	// Pagination
	limit := 20
	if l := c.Query("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 && val <= 100 {
			limit = val
		}
	}

	offset := 0
	if o := c.Query("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	// Xây dựng truy vấn chính
	query := db.Where("user_id = ?", userID).
		Preload("Podcast.Categories").
		Preload("Podcast.Tags").
		Preload("Podcast.Topics").
		Order("last_listened_at DESC")

	// Lọc theo trạng thái hoàn thành
	if completed := c.Query("completed"); completed != "" {
		switch completed {
		case "true":
			query = query.Where("completed = ?", true)
		case "false":
			query = query.Where("completed = ?", false)
		}
	}

	// Đếm tổng số bản ghi
	var total int64
	db.Model(&models.ListeningHistory{}).Where("user_id = ?", userID).Count(&total)

	// Lấy dữ liệu có phân trang
	var histories []models.ListeningHistory
	if err := query.Limit(limit).Offset(offset).Find(&histories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch listening history"})
		return
	}

	// Trả kết quả
	c.JSON(http.StatusOK, gin.H{
		"data":   histories,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetPodcastHistory lấy lịch sử nghe của một podcast cụ thể
// GET /api/user/account/listening-history/:podcast_id
func GetPodcastHistory(c *gin.Context) {
	// Lấy user_id từ context
	userIDValue, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	userIDStr, ok := userIDValue.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID must be a string"})
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID format"})
		return
	}

	// Lấy podcast_id từ URL param
	podcastIDStr := c.Param("podcast_id")
	podcastID, err := uuid.Parse(podcastIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid podcast_id"})
		return
	}

	// Lấy DB instance
	db, exists := c.Get("db")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database connection not found"})
		return
	}
	dbInstance := db.(*gorm.DB)

	// Tìm lịch sử nghe
	var history models.ListeningHistory
	if err := dbInstance.Where("user_id = ? AND podcast_id = ?", userID, podcastID).
		Preload("Podcast").
		First(&history).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "No listening history found for this podcast"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": history,
	})
}

// DeletePodcastHistory xóa lịch sử nghe của một podcast
// DELETE /api/user/account/listening-history/:podcast_id
func DeletePodcastHistory(c *gin.Context) {
	// Lấy user_id từ context
	userIDInterface, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	userID, ok := userIDInterface.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID format"})
		return
	}

	// Lấy podcast_id từ URL param
	podcastIDStr := c.Param("podcast_id")
	podcastID, err := uuid.Parse(podcastIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid podcast_id"})
		return
	}

	// Lấy DB instance
	db, exists := c.Get("db")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database connection not found"})
		return
	}
	dbInstance := db.(*gorm.DB)

	// Xóa lịch sử
	result := dbInstance.Where("user_id = ? AND podcast_id = ?", userID, podcastID).
		Delete(&models.ListeningHistory{})

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete history"})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "No history found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Listening history deleted successfully",
	})
}

// ClearAllHistory xóa toàn bộ lịch sử nghe của user
// DELETE /api/user/account/listening-history
func ClearAllHistory(c *gin.Context) {
	// Lấy user_id từ context
	userIDValue, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	userIDStr, ok := userIDValue.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID must be a string"})
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID format"})
		return
	}

	// Lấy DB instance
	db, exists := c.Get("db")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database connection not found"})
		return
	}
	dbInstance := db.(*gorm.DB)

	// Xóa tất cả lịch sử của user
	if err := dbInstance.Where("user_id = ?", userID).Delete(&models.ListeningHistory{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clear history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "All listening history cleared successfully",
	})
}
