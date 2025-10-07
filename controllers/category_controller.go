package controllers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gosimple/slug"
	"github.com/vnkhanh/e-podcast-backend/config"
	"github.com/vnkhanh/e-podcast-backend/models"
)

func GenerateSlug(name string) string {
	return slug.Make(name)
}

func CreateCategory(c *gin.Context) {
	var input struct {
		Name   string `json:"name" binding:"required"`
		Status *bool  `json:"status"` // optional
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	category := &models.Category{
		Name:   input.Name,
		Slug:   GenerateSlug(input.Name),
		Status: true,
	}
	if input.Status != nil {
		category.Status = *input.Status
	}
	if err := config.DB.Create(&category).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo category"})
		return
	}

	c.JSON(http.StatusCreated,
		gin.H{
			"message":  "Tạo danh mục thành công",
			"category": category,
		},
	)
}

func GetCategories(c *gin.Context) {
	var categories []models.Category
	query := config.DB.Model(&models.Category{})
	// --- Tìm kiếm theo tên ---
	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%") // Postgres
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
	limit := 10
	page := 1
	if p := c.Query("page"); p != "" {
		fmt.Sscanf(p, "%d", &page)
		if page < 1 {
			page = 1
		}
	}
	if l := c.Query("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
		if limit < 1 {
			limit = 10
		}
	}
	offset := (page - 1) * limit
	var total int64
	query.Count(&total)
	// --- Lấy dữ liệu ---
	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách category"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":       categories,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

func UpdateCategory(c *gin.Context) {
	id := c.Param("id")
	var category models.Category
	if err := config.DB.First(&category, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy category"})
		return
	}
	var input struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	category.Name = input.Name
	category.Slug = GenerateSlug(input.Name)

	if err := config.DB.Save(&category).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể cập nhật category"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message":  "Cập nhật danh mục thành công",
		"category": category,
	})
}

func DeleteCategory(c *gin.Context) {
	id := c.Param("id")
	if err := config.DB.Delete(&models.Category{}, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xóa category"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Xóa danh mục thành công"})
}

func ToggleCategoryStatus(c *gin.Context) {
	id := c.Param("id")
	var category models.Category
	if err := config.DB.First(&category, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy category"})
		return
	}
	category.Status = !category.Status
	if err := config.DB.Save(&category).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể cập nhật trạng thái category"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Đã đổi trạng thái thành công",
		"status":  category.Status,
	})
}

func GetCategoryDetail(c *gin.Context) {
	id := c.Param("id")
	var category models.Category
	if err := config.DB.First(&category, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy category"})
		return
	}
	c.JSON(http.StatusOK, category)
}

// Lấy danh sách Category đang hoạt động
func GetCategoriesGet(c *gin.Context) {
	var categories []models.Category
	query := config.DB.Model(&models.Category{})

	if err := query.Where("status = ?", true).Order("created_at desc").Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách danh mục"})
		return
	}

	c.JSON(http.StatusOK, categories)
}
