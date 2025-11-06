package controllers

import (
	"errors"
	"fmt"
	"math"
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
		updateAnalyticsAsync(db, userID, podcastID, history.Completed)

	} else if result.Error == nil {
		// Cập nhật
		history.LastListenedAt = now
		history.LastPosition = req.LastPosition

		// Nếu duration từ client lớn hơn -> cập nhật
		if req.Duration > history.Duration {
			history.Duration = req.Duration
		}

		wasCompleted := history.Completed
		if req.Completed != nil && *req.Completed && !history.Completed {
			history.Completed = true
			history.CompletedAt = &now
		}

		if err := db.Save(&history).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update listening history"})
			return
		}
		// CẬP NHẬT ANALYTICS
		if !wasCompleted && history.Completed {
			updateAnalyticsAsync(db, userID, podcastID, true)
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

// updateAnalyticsAsync cập nhật analytics khi có lượt nghe mới
func updateAnalyticsAsync(db *gorm.DB, userID, podcastID uuid.UUID, completed bool) {
	go func() {
		today := time.Now().Truncate(24 * time.Hour)
		completedInt := 0
		if completed {
			completedInt = 1
		}

		// 1. Update daily analytics
		db.Exec(`
			INSERT INTO listening_analytics (id, date, total_listens, unique_users, completed_listens, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, 1, 1, $2, NOW(), NOW())
			ON CONFLICT (date) DO UPDATE SET
				total_listens = listening_analytics.total_listens + 1,
				completed_listens = listening_analytics.completed_listens + $2,
				updated_at = NOW()
		`, today, completedInt)

		// 2. Update podcast analytics
		db.Exec(`
			INSERT INTO podcast_analytics (id, date, podcast_id, total_plays, unique_listeners, completed_plays, total_duration, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, $2, 1, 1, $3, 0, NOW(), NOW())
			ON CONFLICT (date, podcast_id) DO UPDATE SET
				total_plays = podcast_analytics.total_plays + 1,
				completed_plays = podcast_analytics.completed_plays + $3,
				updated_at = NOW()
		`, today, podcastID, completedInt)

		// 3. Update subject analytics (nếu có)
		var podcast models.Podcast
		if err := db.Preload("Chapter").First(&podcast, podcastID).Error; err == nil {
			if podcast.ChapterID != uuid.Nil {
				var chapter models.Chapter
				if err := db.First(&chapter, podcast.ChapterID).Error; err == nil && chapter.SubjectID != uuid.Nil {
					db.Exec(`
						INSERT INTO subject_analytics (id, date, subject_id, total_plays, created_at, updated_at)
						VALUES (gen_random_uuid(), $1, $2, 1, NOW(), NOW())
						ON CONFLICT (date, subject_id) DO UPDATE SET
							total_plays = subject_analytics.total_plays + 1,
							updated_at = NOW()
					`, today, chapter.SubjectID)
				}
			}
		}
	}()
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

	// ==== PHÂN TRANG ====
	page := 1
	limit := 10
	if val, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil && val > 0 {
		page = val
	}
	if val, err := strconv.Atoi(c.DefaultQuery("limit", "10")); err == nil && val > 0 && val <= 100 {
		limit = val
	}
	offset := (page - 1) * limit

	// ==== SẮP XẾP ====
	sortOrder := c.DefaultQuery("sort", "desc") // desc (mới nhất) | asc (cũ nhất)
	orderClause := "last_listened_at DESC"
	if sortOrder == "asc" {
		orderClause = "last_listened_at ASC"
	}

	// ==== LỌC THEO TRẠNG THÁI ====
	completed := c.Query("completed") // true / false / ""

	// ==== LỌC THEO THỜI GIAN ====
	timeFilter := c.DefaultQuery("time", "all")
	// "today", "week", "month", "year", "custom", "all"
	startDate := time.Time{}
	endDate := time.Now()

	switch timeFilter {
	case "today":
		startDate = time.Now().Truncate(24 * time.Hour)
	case "week":
		startDate = time.Now().AddDate(0, 0, -7)
	case "month":
		startDate = time.Now().AddDate(0, -1, 0)
	case "year":
		startDate = time.Now().AddDate(-1, 0, 0)
	case "custom":
		from := c.Query("from")
		to := c.Query("to")
		if from != "" {
			if parsed, err := time.Parse("2006-01-02", from); err == nil {
				startDate = parsed
			}
		}
		if to != "" {
			if parsed, err := time.Parse("2006-01-02", to); err == nil {
				endDate = parsed
			}
		}
	}

	// ==== XÂY DỰNG TRUY VẤN ====
	query := db.Model(&models.ListeningHistory{}).
		Where("user_id = ?", userID).
		Preload("Podcast.Chapter.Subject").
		Preload("Podcast.Categories").
		Preload("Podcast.Tags")

	// Lọc theo trạng thái
	switch completed {
	case "true":
		query = query.Where("completed = ?", true)
	case "false":
		query = query.Where("completed = ?", false)
	}

	// Lọc theo thời gian
	if !startDate.IsZero() {
		query = query.Where("last_listened_at BETWEEN ? AND ?", startDate, endDate)
	}

	// ==== ĐẾM TỔNG ====
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể đếm lịch sử nghe"})
		return
	}

	// ==== LẤY DỮ LIỆU ====
	var histories []models.ListeningHistory
	if err := query.Order(orderClause).Limit(limit).Offset(offset).Find(&histories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy lịch sử nghe"})
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	// ==== TRẢ KẾT QUẢ ====
	c.JSON(http.StatusOK, gin.H{
		"message": "Lấy lịch sử nghe thành công",
		"data":    histories,
		"pagination": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": totalPages,
		},
		"filters": gin.H{
			"time":      timeFilter,
			"completed": completed,
			"sort":      sortOrder,
			"from":      startDate,
			"to":        endDate,
		},
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
	// Bước 1: Lấy user_id từ context
	userIDValue, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	userIDStr, ok := userIDValue.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID format"})
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Bước 2: Lấy podcast_id từ URL param
	podcastIDStr := c.Param("podcast_id")
	podcastID, err := uuid.Parse(podcastIDStr)
	if err != nil {
		fmt.Println("[DeletePodcastHistory] Invalid podcast_id:", podcastIDStr, "Error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid podcast_id"})
		return
	}

	// Bước 3: Lấy DB instance
	dbValue, exists := c.Get("db")
	if !exists {
		fmt.Println("[DeletePodcastHistory] Database instance not found in context")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database connection not found"})
		return
	}
	db, ok := dbValue.(*gorm.DB)
	if !ok {
		fmt.Println("[DeletePodcastHistory] Invalid DB instance type")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid database instance"})
		return
	}

	// Log dữ liệu đầu vào
	fmt.Printf("[DeletePodcastHistory] user_id=%v | podcast_id=%v\n", userID, podcastID)

	// Bước 4: Thực hiện xóa
	result := db.Where("user_id = ? AND podcast_id = ?", userID, podcastID).
		Delete(&models.ListeningHistory{})

	if result.Error != nil {
		fmt.Println("[DeletePodcastHistory] DB error:", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}

	// Bước 5: Kiểm tra có record nào bị xóa không
	if result.RowsAffected == 0 {
		fmt.Printf("[DeletePodcastHistory] No record found for user=%v podcast=%v\n", userID, podcastID)
		c.JSON(http.StatusNotFound, gin.H{"error": "No history found"})
		return
	}

	// Thành công
	fmt.Printf("[DeletePodcastHistory] Deleted %d record(s)\n", result.RowsAffected)
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
