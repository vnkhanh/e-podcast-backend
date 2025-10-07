package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vnkhanh/e-podcast-backend/config"
	"github.com/vnkhanh/e-podcast-backend/models"
)

func GetTags(c *gin.Context) {
	var tags []models.Tag
	name := c.Query("name") // lấy ?name=... từ URL

	query := config.DB.Model(&models.Tag{})

	// Nếu có query name thì lọc theo LIKE
	if name != "" {
		query = query.Where("LOWER(name) LIKE LOWER(?)", "%"+name+"%")
	}

	if err := query.Order("created_at desc").Find(&tags).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách thẻ"})
		return
	}

	c.JSON(http.StatusOK, tags)
}
