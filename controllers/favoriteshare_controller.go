package controllers

import (
	"encoding/json"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vnkhanh/e-podcast-backend/models"
	"github.com/vnkhanh/e-podcast-backend/ws"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Thêm podcast vào danh sách yêu thích với notification có đầy đủ thông tin
func AddFavorite(c *gin.Context) {
	userIDValue, _ := c.Get("user_id")
	var userID uuid.UUID
	switch v := userIDValue.(type) {
	case string:
		userID, _ = uuid.Parse(v)
	case uuid.UUID:
		userID = v
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user_id type"})
		return
	}

	podcastIDStr := c.Param("podcast_id")
	podcastID, err := uuid.Parse(podcastIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid podcast_id"})
		return
	}

	db := c.MustGet("db").(*gorm.DB)

	var fav models.Favorite
	if err := db.Where("user_id = ? AND podcast_id = ?", userID, podcastID).First(&fav).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Already favorited"})
		return
	}

	newFav := models.Favorite{
		UserID:    userID,
		PodcastID: podcastID,
	}

	tx := db.Begin()
	if err := tx.Create(&newFav).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add favorite"})
		return
	}

	if err := tx.Model(&models.Podcast{}).
		Where("id = ?", podcastID).
		UpdateColumn("like_count", gorm.Expr("like_count + ?", 1)).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update like count"})
		return
	}

	tx.Commit()

	var user models.User
	var podcast models.Podcast
	if err := db.First(&user, "id = ?", userID).Error; err == nil {
		if err := db.First(&podcast, "id = ?", podcastID).Error; err == nil {
			if podcast.CreatedBy != user.ID {
				message := user.FullName + " đã yêu thích podcast \"" + podcast.Title + "\""

				// LƯU NOTIFICATION VỚI PODCAST_ID
				noti := models.Notification{
					UserID:    podcast.CreatedBy,
					Title:     "Podcast của bạn được yêu thích",
					Message:   message,
					Type:      "favorite",
					PodcastID: &podcastID, // Thêm podcast_id để navigation
				}
				db.Create(&noti)

				// Đếm chưa đọc
				var count int64
				db.Model(&models.Notification{}).
					Where("user_id = ? AND is_read = false", podcast.CreatedBy).
					Count(&count)

				// Gửi realtime với đầy đủ thông tin
				payload := map[string]interface{}{
					"type":       "favorite_notification",
					"title":      noti.Title,
					"message":    noti.Message,
					"podcast_id": podcast.ID.String(),
					"id":         noti.ID.String(), // Thêm notification ID
				}
				if data, err := json.Marshal(payload); err == nil {
					ws.H.BroadcastToUser(podcast.CreatedBy.String(), websocket.TextMessage, data)
				}

				ws.SendBadgeUpdate(podcast.CreatedBy.String(), count)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Added to favorites"})
}

// Bỏ yêu thích
func RemoveFavorite(c *gin.Context) {
	userIDValue, _ := c.Get("user_id")
	var userID uuid.UUID
	switch v := userIDValue.(type) {
	case string:
		userID, _ = uuid.Parse(v)
	case uuid.UUID:
		userID = v
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user_id type"})
		return
	}

	podcastIDStr := c.Param("podcast_id")
	podcastID, err := uuid.Parse(podcastIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid podcast_id"})
		return
	}

	db := c.MustGet("db").(*gorm.DB)

	tx := db.Begin()
	result := tx.Where("user_id = ? AND podcast_id = ?", userID, podcastID).
		Delete(&models.Favorite{})

	if result.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove favorite"})
		return
	}

	if result.RowsAffected == 0 {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"error": "Favorite not found"})
		return
	}

	if err := tx.Model(&models.Podcast{}).
		Where("id = ? AND like_count > 0", podcastID).
		UpdateColumn("like_count", gorm.Expr("like_count - ?", 1)).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update like count"})
		return
	}

	tx.Commit()
	c.JSON(http.StatusOK, gin.H{"message": "Removed from favorites"})
}

func CheckFavorite(c *gin.Context) {
	userIDStr, _ := c.Get("user_id")
	userID, _ := uuid.Parse(userIDStr.(string))
	podcastIDStr := c.Param("podcast_id")
	podcastID, err := uuid.Parse(podcastIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid podcast_id"})
		return
	}

	db := c.MustGet("db").(*gorm.DB)
	var fav models.Favorite
	if err := db.Where("user_id = ? AND podcast_id = ?", userID, podcastID).First(&fav).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"is_favorite": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{"is_favorite": true})
}

func GetFavorites(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	// Lấy user_id
	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
		return
	}

	// ======= PHÂN TRANG =======
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "5"))
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 5
	}
	offset := (page - 1) * limit

	// ======= LỌC THEO THỜI GIAN =======
	timeFilter := c.DefaultQuery("time", "all")
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

	// ======= SẮP XẾP =======
	sortOrder := c.DefaultQuery("sort", "desc")
	orderClause := "favorites.created_at DESC"
	if sortOrder == "asc" {
		orderClause = "favorites.created_at ASC"
	}

	// ======= TRUY VẤN CHÍNH =======
	query := db.Model(&models.Favorite{}).
		Preload("Podcast.Chapter.Subject").
		Preload("Podcast.Categories").
		Preload("Podcast.Tags").
		Where("favorites.user_id = ?", userID)

	// Lọc theo thời gian
	if !startDate.IsZero() {
		query = query.Where("favorites.created_at BETWEEN ? AND ?", startDate, endDate)
	}

	// ======= ĐẾM TỔNG =======
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể đếm mục yêu thích"})
		return
	}

	// ======= LẤY DỮ LIỆU =======
	var favorites []models.Favorite
	if err := query.
		Order(orderClause).
		Limit(limit).
		Offset(offset).
		Find(&favorites).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách yêu thích"})
		return
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	// ======= TRẢ KẾT QUẢ =======
	c.JSON(http.StatusOK, gin.H{
		"message": "Lấy danh sách podcast yêu thích thành công",
		"data":    favorites,
		"pagination": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": totalPages,
		},
		"filters": gin.H{
			"time": timeFilter,
			"sort": sortOrder,
			"from": startDate,
			"to":   endDate,
		},
	})
}

func SharePodcastSocialHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		podcastID := c.Param("podcast_id")

		url := os.Getenv("FE_BASE_URL")
		link := url + "/podcast/" + podcastID
		c.JSON(http.StatusOK, gin.H{
			"message": "Link chia sẻ sẵn sàng",
			"link":    link,
		})
	}
}
