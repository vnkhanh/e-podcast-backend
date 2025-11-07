package controllers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/vnkhanh/e-podcast-backend/config"
	"github.com/vnkhanh/e-podcast-backend/models"
	"gorm.io/gorm"
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

	name := strings.TrimSpace(input.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tên danh mục bắt buộc"})
		return
	}

	slugValue := GenerateSlug(name)

	// Kiểm tra trùng tên hoặc slug
	var count int64
	config.DB.Model(&models.Category{}).
		Where("LOWER(TRIM(name)) = ? OR slug = ?", strings.ToLower(name), slugValue).
		Count(&count)

	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tên hoặc slug danh mục đã tồn tại"})
		return
	}

	// Lấy userID từ context (nếu có)
	var userUUID *uuid.UUID
	userIDStr := c.GetString("user_id")
	if userIDStr != "" {
		parsed, err := uuid.Parse(userIDStr)
		if err == nil {
			userUUID = &parsed
		}
	}

	category := &models.Category{
		Name:      name,
		Slug:      slugValue,
		Status:    true, // mặc định
		CreatedBy: userUUID,
	}
	if input.Status != nil {
		category.Status = *input.Status
	}

	if err := config.DB.Create(&category).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo danh mục"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":  "Tạo danh mục thành công",
		"category": category,
	})
}

func GetCategories(c *gin.Context) {
	var categories []models.Category
	query := config.DB.Model(&models.Category{})

	// Lấy userID và role từ context
	userIDStr := c.GetString("user_id")
	role := c.GetString("role")

	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
		return
	}

	// --- Phân quyền ---
	if role == string(models.RoleLecturer) {
		query = query.Where("categories.created_by = ?", userUUID)
	} else if role == string(models.RoleAdmin) {
		query = query.Joins("LEFT JOIN users ON users.id = categories.created_by")
		if lecturer := c.Query("lecturer"); lecturer != "" {
			query = query.Where("(users.full_name ILIKE ? OR users.email ILIKE ?)", "%"+lecturer+"%", "%"+lecturer+"%")
		}
		query = query.Group("categories.id")
	}

	// --- Tìm kiếm theo tên danh mục ---
	if search := c.Query("search"); search != "" {
		query = query.Where("categories.name ILIKE ?", "%"+search+"%")
	}

	// --- Lọc theo trạng thái ---
	if status := c.Query("status"); status != "" {
		switch status {
		case "true":
			query = query.Where("categories.status = ?", true)
		case "false":
			query = query.Where("categories.status = ?", false)
		}
	}

	// --- Lọc theo ngày tạo ---
	fromDateStr := c.Query("from_date")
	toDateStr := c.Query("to_date")
	if fromDateStr != "" || toDateStr != "" {
		const layout = "2006-01-02"
		if fromDateStr != "" && toDateStr != "" {
			fromDate, err1 := time.Parse(layout, fromDateStr)
			toDate, err2 := time.Parse(layout, toDateStr)
			if err1 == nil && err2 == nil {
				toDate = toDate.Add(24 * time.Hour)
				query = query.Where("categories.created_at BETWEEN ? AND ?", fromDate, toDate)
			}
		} else if fromDateStr != "" {
			fromDate, err := time.Parse(layout, fromDateStr)
			if err == nil {
				query = query.Where("categories.created_at >= ?", fromDate)
			}
		} else if toDateStr != "" {
			toDate, err := time.Parse(layout, toDateStr)
			if err == nil {
				toDate = toDate.Add(24 * time.Hour)
				query = query.Where("categories.created_at < ?", toDate)
			}
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

	// --- Đếm tổng ---
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể đếm tổng category"})
		return
	}

	// --- Lấy dữ liệu ---
	if err := query.
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, full_name, email")
		}).
		Preload("UpdatedByUser", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, full_name, email")
		}).
		Offset(offset).
		Limit(limit).
		Order("categories.created_at DESC").
		Find(&categories).Error; err != nil {
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
	category.UpdatedBy = userUUID
	var input struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tên danh mục bắt buộc"})
		return
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tên danh mục không được trống"})
		return
	}

	slugValue := GenerateSlug(name)

	// Kiểm tra trùng tên hoặc slug với các category khác
	var count int64
	config.DB.Model(&models.Category{}).
		Where("(LOWER(TRIM(name)) = ? OR slug = ?) AND id <> ?", strings.ToLower(name), slugValue, id).
		Count(&count)

	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tên danh mục đã tồn tại"})
		return
	}

	category.Name = name
	category.Slug = slugValue

	if err := config.DB.Save(&category).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể cập nhật danh mục"})
		return
	}

	// === Tải lại category kèm thông tin người tạo & cập nhật ===
	var updatedCategory models.Category
	if err := config.DB.
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, full_name, email")
		}).
		Preload("UpdatedByUser", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, full_name, email")
		}).
		First(&updatedCategory, "id = ?", category.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tải lại dữ liệu sau khi cập nhật"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Cập nhật danh mục thành công",
		"category": updatedCategory,
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
	if err := config.DB.
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, full_name, email")
		}).
		Preload("UpdatedByUser", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, full_name, email")
		}).
		First(&category, "id = ?", id).Error; err != nil {

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

// /////USER
// Lấy danh mục nổi bật
func GetCategoriesUserPopular(c *gin.Context) {
	type CategoryWithCount struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Slug      string `json:"slug"`
		Status    bool   `json:"status"`
		Count     int64  `json:"count"`
		CreatedAt string `json:"created_at"`
	}

	var results []CategoryWithCount

	err := config.DB.
		Table("categories").
		Select(`
			categories.id,
			categories.name,
			categories.slug,
			categories.status,
			categories.created_at,
			COUNT(podcasts.id) AS count
		`).
		Joins(`
			JOIN podcast_categories ON categories.id = podcast_categories.category_id
			JOIN podcasts ON podcasts.id = podcast_categories.podcast_id
		`).
		Where("podcasts.status = ?", "published").
		Group("categories.id").
		Order("count DESC").
		Limit(6).
		Scan(&results).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách danh mục phổ biến"})
		return
	}

	// Format ngày giờ
	for i := range results {
		results[i].CreatedAt = results[i].CreatedAt[:19] // giữ định dạng yyyy-mm-dd hh:mm:ss
	}

	c.JSON(http.StatusOK, gin.H{
		"categories": results,
	})
}

// GetCategoriesUser - Lấy danh sách Category đang hoạt động (status = true)
// + Tìm kiếm + Phân trang + Sắp xếp (tối ưu cho PostgreSQL)
func GetCategoriesUser(c *gin.Context) {
	var categories []models.Category
	query := config.DB.Model(&models.Category{}).Where("status = ?", true)

	// --- Tìm kiếm theo tên (PostgreSQL hỗ trợ ILIKE để không phân biệt hoa thường) ---
	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	// --- Sắp xếp ---
	sortOrder := strings.ToLower(c.DefaultQuery("sort", "asc"))
	if sortOrder == "desc" {
		query = query.Order("name DESC")
	} else {
		query = query.Order("name ASC")
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

	// --- Đếm tổng số ---
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể đếm danh mục"})
		return
	}

	// --- Lấy dữ liệu ---
	if err := query.Offset(offset).Limit(limit).Find(&categories).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách danh mục"})
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
