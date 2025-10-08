package controllers

import (
	"fmt"
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

	// Convert user_id từ string -> uuid.UUID
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

	// Tạo Document
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

	ws.BroadcastDocumentListChanged()
	// Update document sau khi trích xuất
	db.Model(&doc).Updates(map[string]interface{}{
		"status": "Đang trích xuất",
	})
	ws.BroadcastDocumentListChanged()
	// Bắt đầu trích xuất
	noiDung, err := services.NormalizeInput(services.InputSource{
		Type:       inputType,
		FileHeader: file,
	})
	if err != nil {
		db.Model(&doc).Updates(map[string]interface{}{
			"status": "Lỗi",
		})
		ws.BroadcastDocumentListChanged()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể trích xuất nội dung", "details": err.Error()})
		return
	}

	cleanedContent, err := services.CleanTextPipeline(noiDung)
	if err != nil {
		db.Model(&doc).Updates(map[string]interface{}{
			"status": "Lỗi",
		})
		ws.BroadcastDocumentListChanged()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể làm sạch nội dung", "details": err.Error()})
		return
	}

	// Log nội dung đã làm sạch
	fmt.Println("Nội dung đã làm sạch: ", cleanedContent)

	// Update document sau khi trích xuất
	db.Model(&doc).Updates(map[string]interface{}{
		"status":         "Đã trích xuất",
		"extracted_text": cleanedContent,
	})
	ws.BroadcastDocumentListChanged()
	// Update document sau khi trích xuất
	db.Model(&doc).Updates(map[string]interface{}{
		"status": "Đang tạo audio",
	})
	ws.BroadcastDocumentListChanged()
	// === Sinh audio từ nội dung đã làm sạch (VITS) ===
	audioURL, err := utils.CallVITSTTS(cleanedContent)
	if err != nil {
		db.Model(&doc).Updates(map[string]interface{}{
			"status": "Lỗi tạo audio",
		})
		ws.BroadcastDocumentListChanged()
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Không thể tạo audio từ VITS",
			"details": err.Error(),
		})
		return
	}
	// Coi như đã xử lý xong (chưa sinh audio)
	now := time.Now()
	db.Model(&doc).Updates(map[string]interface{}{
		"status":       "Hoàn thành",
		"processed_at": &now,
	})

	ws.BroadcastDocumentListChanged()

	// Load lại để trả JSON về cho client
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

	if err := config.DB.Delete(&models.Document{}, "id = ?", documentID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xóa tào liệu"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Xóa thành công"})
}
