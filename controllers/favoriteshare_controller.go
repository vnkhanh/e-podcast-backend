package controllers

import (
	"encoding/json"
	"net/http"
	"os"
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
	userIDStr, _ := c.Get("user_id")
	userID, _ := uuid.Parse(userIDStr.(string))

	db := c.MustGet("db").(*gorm.DB)

	var favorites []models.Favorite
	if err := db.Preload("Podcast").Where("user_id = ?", userID).Find(&favorites).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch favorites"})
		return
	}

	c.JSON(http.StatusOK, favorites)
}

func SharePodcastSocialHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		podcastID := c.Param("podcast_id")
		podcastUUID, err := uuid.Parse(podcastID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Podcast ID không hợp lệ"})
			return
		}

		share := models.PodcastShare{
			ID:        uuid.New(),
			PodcastID: podcastUUID,
			SharedAt:  time.Now(),
		}
		db.Create(&share)

		url := os.Getenv("FE_BASE_URL")
		link := url + "/podcast/" + podcastID
		c.JSON(http.StatusOK, gin.H{
			"message": "Link chia sẻ sẵn sàng",
			"link":    link,
		})
	}
}
