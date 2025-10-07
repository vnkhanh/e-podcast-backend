package controllers

import (
	"fmt"
	"net/http"

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

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	topic := models.Topic{
		Name:   input.Name,
		Status: true, // default
		Slug:   slug.Make(input.Name),
	}
	if input.Status != nil {
		topic.Status = *input.Status
	}

	if err := config.DB.Create(&topic).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo topic"})
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
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy topic"})
		return
	}

	var input struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	topic.Name = input.Name
	topic.Slug = slug.Make(input.Name)

	if err := config.DB.Save(&topic).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể cập nhật topic"})
		return
	}

	c.JSON(http.StatusOK, topic)
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
