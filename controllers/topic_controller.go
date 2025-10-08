package controllers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/vnkhanh/e-podcast-backend/config"
	"github.com/vnkhanh/e-podcast-backend/models"
)

// Create Topic
func CreateTopic(c *gin.Context) {
	var input struct {
		Name   string `json:"name" binding:"required"`
		Status *bool  `json:"status"` // optional
	}
	// Bind JSON từ request body
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dữ liệu không hợp lệ"})
		return
	}
	// Lấy userID từ context (nếu có)
	var userUUID *uuid.UUID
	userIDStr := c.GetString("user_id")
	if userIDStr != "" {
		parsed, err := uuid.Parse(userIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
			return
		}
		userUUID = &parsed
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tên chủ đề bắt buộc"})
		return
	}

	slugValue := slug.Make(name)

	// === Kiểm tra trùng tên hoặc slug ===
	var count int64
	config.DB.Model(&models.Topic{}).
		Where("LOWER(TRIM(name)) = ? OR slug = ?", strings.ToLower(name), slugValue).
		Count(&count)

	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tên chủ đề đã tồn tại"})
		return
	}

	topic := models.Topic{
		Name:      name,
		Status:    true, // default
		Slug:      slugValue,
		CreatedBy: userUUID, // có thể null
	}
	if input.Status != nil {
		topic.Status = *input.Status
	}

	if err := config.DB.Create(&topic).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo chủ đề"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Tạo chủ đề thành công",
		"topic":   topic,
	})
}

// Get All Topics (có search, filter, pagination)
func GetTopics(c *gin.Context) {
	var topics []models.Topic
	query := config.DB.Model(&models.Topic{})
	// Lấy userID và role từ context
	userIDStr := c.GetString("user_id")
	role := c.GetString("role")

	// Lấy userID từ context (nếu có)
	var userUUID *uuid.UUID
	if userIDStr != "" {
		parsed, err := uuid.Parse(userIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
			return
		}
		userUUID = &parsed
	}

	// Phân quyền
	if role == string(models.RoleLecturer) { // giảng viên
		query = query.Where("created_by = ?", userUUID)
	} else if role == string(models.RoleAdmin) {
		// admin: không thêm filter, lấy tất cả
	}
	// --- Tìm kiếm theo tên ---
	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%") // Postgres
		// nếu MySQL thì thay bằng: query = query.Where("name LIKE ?", "%"+search+"%")
	}

	// --- Lọc theo trạng thái ---
	if status := c.Query("status"); status != "" {
		switch status {
		case "true":
			query = query.Where("status = ?", true)
		case "false":
			query = query.Where("status = ?", false)
		}
	}

	// --- Phân trang ---
	page := 1
	limit := 10
	if p := c.Query("page"); p != "" {
		fmt.Sscanf(p, "%d", &page)
	}
	if l := c.Query("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit

	// --- Đếm tổng ---
	var total int64
	query.Count(&total)

	// --- Lấy dữ liệu ---
	if err := query.Limit(limit).Offset(offset).Order("created_at desc").Find(&topics).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách topic"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  topics,
		"page":  page,
		"limit": limit,
		"total": total,
	})
}

// Update Topic
func UpdateTopic(c *gin.Context) {
	id := c.Param("id")
	topicID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	var topic models.Topic
	if err := config.DB.First(&topic, "id = ?", topicID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy chủ đề"})
		return
	}

	var input struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tên chủ đề bắt buộc"})
		return
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tên chủ đề không được trống"})
		return
	}

	slugValue := slug.Make(name)

	// === Kiểm tra trùng tên hoặc slug với các topic khác ===
	var count int64
	config.DB.Model(&models.Topic{}).
		Where("(LOWER(TRIM(name)) = ? OR slug = ?) AND id <> ?", strings.ToLower(name), slugValue, topicID).
		Count(&count)

	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tên hoặc slug chủ đề đã tồn tại"})
		return
	}

	topic.Name = name
	topic.Slug = slugValue

	if err := config.DB.Save(&topic).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể cập nhật chủ đề"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Cập nhật chủ đề thành công",
		"topic":   topic,
	})
}

// Toggle status
func ToggleTopicStatus(c *gin.Context) {
	id := c.Param("id")
	topicID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	var topic models.Topic
	if err := config.DB.First(&topic, "id = ?", topicID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy topic"})
		return
	}

	topic.Status = !topic.Status

	if err := config.DB.Save(&topic).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể cập nhật trạng thái"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Đã đổi trạng thái thành công",
		"status":  topic.Status,
	})
}

// Delete Topic
func DeleteTopic(c *gin.Context) {
	id := c.Param("id")
	topicID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	if err := config.DB.Delete(&models.Topic{}, "id = ?", topicID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xóa topic"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Xóa thành công"})
}

func GetTopicDetail(c *gin.Context) {
	id := c.Param("id")
	var topic models.Topic
	if err := config.DB.First(&topic, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy topic"})
		return
	}
	c.JSON(http.StatusOK, topic)
}

// Lấy danh sách Topics đang hoạt động
func GetTopicsGet(c *gin.Context) {
	var topics []models.Topic
	query := config.DB.Model(&models.Topic{})

	if err := query.Where("status = ?", true).Order("created_at desc").Find(&topics).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách chủ đề"})
		return
	}

	c.JSON(http.StatusOK, topics)
}
