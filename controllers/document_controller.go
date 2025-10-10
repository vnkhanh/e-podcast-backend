package controllers

import (
	"log"
	"math"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vnkhanh/e-podcast-backend/config"
	"github.com/vnkhanh/e-podcast-backend/models"
	"github.com/vnkhanh/e-podcast-backend/services"
	"github.com/vnkhanh/e-podcast-backend/utils"
	"github.com/vnkhanh/e-podcast-backend/ws"
	"gorm.io/gorm"
)

func UploadDocument(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userIDStr := c.GetString("user_id")

	uid, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Không có file đính kèm"})
		return
	}
	if file.Size > 20*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File vượt quá 20MB"})
		return
	}

	ext := filepath.Ext(file.Filename)
	inputType, err := utils.GetInputTypeFromExt(ext)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	docID := uuid.New()
	publicURL, err := utils.UploadFileToSupabase(file, docID.String())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi upload Supabase", "details": err.Error()})
		return
	}

	doc := models.Document{
		ID:           docID,
		OriginalName: file.Filename,
		FilePath:     publicURL,
		FileType:     strings.TrimPrefix(ext, "."),
		FileSize:     file.Size,
		Status:       "Đã tải lên",
		UserID:       uid,
	}
	if err := db.Create(&doc).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không lưu được tài liệu", "details": err.Error()})
		return
	}

	// === Hàm tiện ích cập nhật trạng thái & gửi WS ===
	lastStatus := ""
	lastProgress := 0.0

	updateStatus := func(status string, progress float64, errorMsg string) {
		// Gửi WS chỉ khi thật sự có thay đổi đáng kể
		if status != lastStatus || math.Abs(progress-lastProgress) >= 5 || errorMsg != "" {
			db.Model(&doc).Updates(map[string]interface{}{
				"status":   status,
				"progress": progress,
			})
			ws.SendStatusUpdate(doc.ID.String(), status, progress, errorMsg)
			ws.BroadcastDocumentListChanged()
			log.Printf("[Document %s] %s - %.0f%%", doc.OriginalName, status, progress)

			lastStatus = status
			lastProgress = progress
		}
	}

	// --- Bắt đầu quy trình xử lý ---
	updateStatus("Đã tải lên", 0, "")

	// --- Trích xuất nội dung ---
	updateStatus("Đang trích xuất", 10, "")
	noiDung, err := services.NormalizeInput(services.InputSource{
		Type:       inputType,
		FileHeader: file,
	})
	if err != nil {
		updateStatus("Lỗi", 0, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể trích xuất nội dung", "details": err.Error()})
		return
	}

	cleanedContent, err := services.CleanTextPipeline(noiDung)
	if err != nil {
		updateStatus("Lỗi", 0, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể làm sạch nội dung", "details": err.Error()})
		return
	}

	// Progress mượt từ 10 → 50
	for p := 15; p <= 50; p += 10 {
		updateStatus("Đang trích xuất", float64(p), "")
		time.Sleep(200 * time.Millisecond)
	}
	db.Model(&doc).Update("extracted_text", cleanedContent)

	// --- Tạo audio ---
	updateStatus("Đang tạo audio", 55, "")
	audioURL, err := utils.CallVITSTTS(cleanedContent)
	if err != nil {
		updateStatus("Lỗi tạo audio", 0, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Không thể tạo audio từ VITS",
			"details": err.Error(),
		})
		return
	}

	// Tiến trình mượt 55 → 95
	for p := 60; p < 95; p += 10 {
		updateStatus("Đang tạo audio", float64(p), "")
		time.Sleep(200 * time.Millisecond)
	}

	// --- Hoàn tất ---
	updateStatus("Hoàn thành", 100, "")
	now := time.Now()
	db.Model(&doc).Updates(map[string]interface{}{
		"audio_url":    audioURL,
		"status":       "Hoàn thành",
		"processed_at": &now,
	})

	db.Preload("User").First(&doc, "id = ?", doc.ID)
	c.JSON(http.StatusOK, gin.H{
		"message":   "Tải lên thành công",
		"tai_lieu":  doc,
		"audio_url": audioURL,
	})
}

func GetDocuments(c *gin.Context) {
	var documents []models.Document
	query := config.DB.Model(&models.Document{})
	// Lấy userID và role từ context
	userIDStr := c.GetString("user_id")
	role := c.GetString("role")

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
		query = query.Where("user_id = ?", userUUID)
	} else if role == string(models.RoleAdmin) {
		// admin: không thêm filter, lấy tất cả
	}

	// lọc theo trạng thái
	if status := c.Query("status"); status != "" {
		switch status {
		case "Đã tải lên", "Đang trích xuất", "Đã trích xuất", "Đang tạo podcast", "Hoàn thành", "Lỗi":
			query = query.Where("status = ?", status)
		}
	}

	// tìm kiếm theo tên
	if search := c.Query("search"); search != "" {
		query = query.Where("original_name LIKE ?", "%"+search+"%")
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể đếm tổng số tài liệu"})
		return
	}

	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&documents).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách tài liệu"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  documents,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func GetDocumentDetail(c *gin.Context) {
	id := c.Param("id")
	var document models.Document
	if err := config.DB.First(&document, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy tài liệu"})
		return
	}
	c.JSON(http.StatusOK, document)
}

// Delete Tài liệu
func DeleteDocument(c *gin.Context) {
	id := c.Param("id")
	documentID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	var document models.Document
	if err := config.DB.First(&document, "id = ?", documentID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy tài liệu"})
		return
	}

	// Xóa khỏi DB
	if err := config.DB.Delete(&document).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xóa tài liệu"})
		return
	}

	// Xóa file trên Supabase (nếu có)
	if document.FilePath != "" {
		if err := utils.DeleteFileFromSupabase(document.FilePath); err != nil {
			log.Printf("Lỗi xóa file trên Supabase: %v", err)
			// Không trả về lỗi vì document đã bị xóa khỏi DB
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Xóa thành công"})
}
