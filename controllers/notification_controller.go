package controllers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vnkhanh/e-podcast-backend/models"
	"github.com/vnkhanh/e-podcast-backend/ws"
	"gorm.io/gorm"
)

// Danh sách thông báo
func GetNotifications(c *gin.Context) {
	userIDStr, _ := c.Get("user_id")
	userID, _ := uuid.Parse(userIDStr.(string))
	db := c.MustGet("db").(*gorm.DB)

	var list []models.Notification
	if err := db.Where("user_id = ?", userID).Order("created_at DESC").Find(&list).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch notifications"})
		return
	}
	c.JSON(http.StatusOK, list)
}

// Đếm số thông báo chưa đọc
func GetUnreadCount(c *gin.Context) {
	userIDStr, _ := c.Get("user_id")
	userID, _ := uuid.Parse(userIDStr.(string))
	db := c.MustGet("db").(*gorm.DB)

	var count int64
	db.Model(&models.Notification{}).Where("user_id = ? AND is_read = false", userID).Count(&count)
	c.JSON(http.StatusOK, gin.H{"unread_count": count})
}

// Đánh dấu đã đọc
func MarkNotificationAsRead(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	id := c.Param("id")
	notificationID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid notification ID"})
		return
	}

	now := time.Now()
	if err := db.Model(&models.Notification{}).
		Where("id = ?", notificationID).
		Updates(map[string]interface{}{"is_read": true, "read_at": &now}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update notification"})
		return
	}

	// Gửi cập nhật badge realtime
	var notif models.Notification
	if err := db.First(&notif, "id = ?", notificationID).Error; err == nil {
		var count int64
		db.Model(&models.Notification{}).Where("user_id = ? AND is_read = false", notif.UserID).Count(&count)
		ws.SendBadgeUpdate(notif.UserID.String(), count)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notification marked as read"})
}

func MarkAllAsRead(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userIDStr, _ := c.Get("user_id")
	userID, _ := uuid.Parse(userIDStr.(string))

	now := time.Now()
	if err := db.Model(&models.Notification{}).
		Where("user_id = ? AND is_read = false", userID).
		Updates(map[string]interface{}{"is_read": true, "read_at": &now}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark all read"})
		return
	}

	// Gửi cập nhật badge realtime
	ws.SendBadgeUpdate(userID.String(), 0)

	c.JSON(http.StatusOK, gin.H{"message": "All notifications marked as read"})
}

// Xóa tất cả thông báo của user
// Xóa một thông báo cụ thể
func DeleteNotification(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	id := c.Param("id")
	userIDStr, _ := c.Get("user_id")
	userID, _ := uuid.Parse(userIDStr.(string))

	notifID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid notification ID"})
		return
	}

	// Kiểm tra xem thông báo có thuộc user không
	var notif models.Notification
	if err := db.First(&notif, "id = ? AND user_id = ?", notifID, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
		return
	}

	// Xóa thông báo
	if err := db.Delete(&notif).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete notification"})
		return
	}

	// Cập nhật realtime badge
	var count int64
	db.Model(&models.Notification{}).Where("user_id = ? AND is_read = false", userID).Count(&count)
	ws.SendBadgeUpdate(userID.String(), count)

	c.JSON(http.StatusOK, gin.H{"message": "Notification deleted successfully"})
}

// Xóa tất cả thông báo của user
func DeleteAllNotifications(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userIDStr, _ := c.Get("user_id")
	userID, _ := uuid.Parse(userIDStr.(string))

	if err := db.Where("user_id = ?", userID).Delete(&models.Notification{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete notifications"})
		return
	}

	// Reset realtime badge
	ws.SendBadgeUpdate(userID.String(), 0)

	c.JSON(http.StatusOK, gin.H{"message": "All notifications deleted successfully"})
}

// Xóa tất cả thông báo đã đọc, giữ lại chưa đọc
func DeleteReadNotifications(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userIDStr, _ := c.Get("user_id")
	userID, _ := uuid.Parse(userIDStr.(string))

	// Xóa tất cả thông báo đã đọc
	if err := db.Where("user_id = ? AND is_read = true", userID).
		Delete(&models.Notification{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete read notifications"})
		return
	}

	// Đếm lại số chưa đọc để cập nhật badge realtime
	var count int64
	db.Model(&models.Notification{}).Where("user_id = ? AND is_read = false", userID).Count(&count)
	ws.SendBadgeUpdate(userID.String(), count)

	c.JSON(http.StatusOK, gin.H{"message": "All read notifications deleted successfully"})
}
