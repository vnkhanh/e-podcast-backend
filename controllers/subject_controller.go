package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/vnkhanh/e-podcast-backend/config"
	"github.com/vnkhanh/e-podcast-backend/models"
	"gorm.io/gorm"
)

// Input cho Create / Update
type CreateSubjectInput struct {
	Name string `json:"name" binding:"required"`
}

// POST /admin/subjects
func CreateSubject(c *gin.Context) {
	var input CreateSubjectInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tên môn học bắt buộc"})
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

	// === Kiểm tra trùng tên ===
	var count int64
	config.DB.Model(&models.Subject{}).Where("LOWER(name) = LOWER(?)", input.Name).Count(&count)
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tên môn học đã tồn tại"})
		return
	}

	// Tạo subject
	subject := models.Subject{
		Name:      input.Name,
		CreatedBy: userUUID, // có thể null
		Status:    true,     // mặc định active
		Slug:      slug.Make(input.Name),
	}

	if err := config.DB.Create(&subject).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo môn học"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Tạo môn học thành công",
		"subject": subject,
	})
}

// GET /admin/subjects
func GetSubjects(c *gin.Context) {
	var subjects []models.Subject
	query := config.DB.Model(&models.Subject{})

	// Lấy userID và role từ context
	userIDStr := c.GetString("user_id")
	role := c.GetString("role")

	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
		return
	}
	// Phân quyền
	if role == string(models.RoleLecturer) { // giảng viên
		query = query.Where("created_by = ?", userUUID)
	} else if role == string(models.RoleAdmin) {
		// admin: không thêm filter, lấy tất cả
	}
	// lọc theo trạng thái
	if status := c.Query("status"); status != "" {
		switch status {
		case "true":
			query = query.Where("status = ?", true)
		case "false":
			query = query.Where("status = ?", false)
		}
	}

	// tìm kiếm theo tên
	if search := c.Query("search"); search != "" {
		query = query.Where("name ILIKE ?", "%"+search+"%")
	}

	// phân trang
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 10
	}
	offset := (page - 1) * limit

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể đếm tổng số môn học"})
		return
	}

	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&subjects).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách môn học"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  subjects,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GET /admin/subjects/:id
func GetSubjectDetail(c *gin.Context) {
	idParam := c.Param("id")
	subjectID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	var subject models.Subject
	if err := config.DB.First(&subject, "id = ?", subjectID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy môn học"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"subject": subject,
	})
}

// PUT /admin/subjects/:id
func UpdateSubject(c *gin.Context) {
	var input CreateSubjectInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tên môn học bắt buộc"})
		return
	}

	idParam := c.Param("id")
	subjectID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	var subject models.Subject
	if err := config.DB.First(&subject, "id = ?", subjectID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Môn học không tồn tại"})
		return
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tên môn học không được trống"})
		return
	}

	slugValue := slug.Make(name)

	// Kiểm tra trùng tên hoặc slug với các subject khác
	var count int64
	config.DB.Model(&models.Subject{}).
		Where("(LOWER(TRIM(name)) = ? OR slug = ?) AND id <> ?", strings.ToLower(name), slugValue, subjectID).
		Count(&count)

	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tên môn học đã tồn tại"})
		return
	}

	subject.Name = name
	subject.Slug = slugValue

	if err := config.DB.Save(&subject).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cập nhật thất bại"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Cập nhật thành công",
		"subject": subject,
	})
}

// DELETE /admin/subjects/:id
func DeleteSubject(c *gin.Context) {
	idParam := c.Param("id")
	subjectID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	var subject models.Subject
	if err := config.DB.First(&subject, "id = ?", subjectID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy môn học"})
		return
	}

	if err := config.DB.Delete(&subject).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Xóa môn học thành công"})
}

// PATCH /admin/subjects/:id/toggle-status
func ToggleSubjectStatus(c *gin.Context) {
	idParam := c.Param("id")
	subjectID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	var subject models.Subject
	if err := config.DB.First(&subject, "id = ?", subjectID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy môn học"})
		return
	}

	// đảo trạng thái
	subject.Status = !subject.Status

	if err := config.DB.Save(&subject).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Cập nhật trạng thái thành công",
		"subject": subject,
	})
}

// Lấy danh sách Subject đang hoạt động
func GetSubjectsGet(c *gin.Context) {
	var subjects []models.Subject
	query := config.DB.Model(&models.Subject{})

	if err := query.Where("status = ?", true).Order("created_at desc").Find(&subjects).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách môn học"})
		return
	}

	c.JSON(http.StatusOK, subjects)
}

// GET /api/chapters?subject_id=...
func ListChaptersBySubject(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	subjectIDStr := c.Param("id") // <-- sửa từ c.Query -> c.Param
	if subjectIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "subject_id bắt buộc"})
		return
	}

	subjectUUID, err := uuid.Parse(subjectIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "subject_id không hợp lệ"})
		return
	}

	var chapters []models.Chapter
	if err := db.Where("subject_id = ?", subjectUUID).Order("sort_order ASC").Find(&chapters).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, chapters)
}

type CreateChapterReq struct {
	SubjectID string `json:"subject_id"`
	Title     string `json:"title"`
}

func CreateChapter(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	var req CreateChapterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	subjectUUID, err := uuid.Parse(req.SubjectID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "subject_id không hợp lệ"})
		return
	}

	var subject models.Subject
	if err := db.First(&subject, "id = ?", subjectUUID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Môn học không tồn tại"})
		return
	}

	// Kiểm tra trùng tên
	var existing models.Chapter
	if err := db.Where("subject_id = ? AND LOWER(title) = LOWER(?)", subjectUUID, req.Title).First(&existing).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chương đã tồn tại"})
		return
	}

	// Lấy max sort_order
	var maxOrder int
	db.Model(&models.Chapter{}).Where("subject_id = ?", subjectUUID).Select("COALESCE(MAX(sort_order),0)").Scan(&maxOrder)

	chapter := models.Chapter{
		ID:        uuid.New(),
		SubjectID: subjectUUID,
		Title:     req.Title,
		SortOrder: maxOrder + 1,
	}

	if err := db.Create(&chapter).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "chapter": chapter})
}
