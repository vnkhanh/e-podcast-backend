package controllers

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
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
	if file.Size > 10*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File vượt quá 10MB"})
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

	// === Cập nhật trạng thái và WS ===
	lastStatus := ""
	lastProgress := 0.0
	updateStatus := func(status string, progress float64, errorMsg string) {
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

	// --- Bắt đầu xử lý ---
	updateStatus("Đã tải lên", 0, "")

	// --- 1 TRÍCH XUẤT ---
	updateStatus("Đang trích xuất", 10, "")
	noiDung, err := services.NormalizeInput(services.InputSource{
		Type:       inputType,
		FileHeader: file,
	})
	if err != nil {
		updateStatus("Lỗi trích xuất văn bản", 0, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể trích xuất nội dung", "details": err.Error()})
		return
	}

	// --- 2 LÀM SẠCH ---
	updateStatus("Đang làm sạch", 25, "")
	cleanedContent, err := services.CleanTextPipeline(noiDung)
	if err != nil {
		updateStatus("Lỗi làm sạch nội dung", 0, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể làm sạch nội dung", "details": err.Error()})
		return
	}

	db.Model(&doc).Update("extracted_text", cleanedContent)

	// --- 3 VIẾT LẠI KỊCH BẢN AUDIO ---
	updateStatus("Đang tạo kịch bản", 45, "")
	scriptText, err := services.ExtractTextPipeline(cleanedContent)
	if err != nil {
		updateStatus("Lỗi tạo kịch bản audio", 0, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể viết lại nội dung audio", "details": err.Error()})
		return
	}
	db.Model(&doc).Update("extracted_text", scriptText)

	// --- 4 TÓM TẮT ---
	updateStatus("Đang tạo tóm tắt", 55, "")
	summary, err := services.SummaryText(cleanedContent)
	if err != nil {
		updateStatus("Lỗi tóm tắt nội dung", 0, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tóm tắt nội dung", "details": err.Error()})
		return
	}

	// --- 5 TẠO AUDIO ---
	updateStatus("Đang tạo audio", 60, "")
	voice := c.PostForm("voice")
	if voice == "" {
		voice = "vi-VN-Chirp3-HD-Puck"
	}
	rate := 1.0
	if rateStr := c.PostForm("speaking_rate"); rateStr != "" {
		if parsed, err := strconv.ParseFloat(rateStr, 64); err == nil && parsed > 0 {
			rate = parsed
		}
	}

	// === SINH AUDIO ===
	audioData, err := services.SynthesizeText(scriptText, voice, rate)
	if err != nil {
		updateStatus("Lỗi tạo audio", 0, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo audio", "details": err.Error()})
		return
	}

	// Lưu tạm vào local trước khi upload (phòng mất dữ liệu)
	tmpDir := "/app/tmp"
	os.MkdirAll(tmpDir, 0o755)
	tmpPath := filepath.Join(tmpDir, fmt.Sprintf("%s.mp3", docID.String()))

	if err := os.WriteFile(tmpPath, audioData, 0o644); err != nil {
		log.Printf("Không thể lưu file tạm: %v", err)
	}

	// --- Giả lập tiến độ upload ---
	for p := 65; p < 95; p += 10 {
		updateStatus("Đang lưu audio", float64(p), "")
		time.Sleep(200 * time.Millisecond)
	}

	// === Upload lên Supabase ===
	audioURL, err := utils.UploadAudioToSupabase(audioData, docID.String()+".mp3", "audio/mp3")
	if err != nil {
		updateStatus("Lỗi lưu audio", 0, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể upload audio", "details": err.Error()})
		return
	}

	// Xóa file tạm sau khi upload thành công
	// _ = os.Remove(tmpPath)

	// --- 6 HOÀN THÀNH ---
	updateStatus("Hoàn thành", 100, "")
	now := time.Now()
	db.Model(&doc).Updates(map[string]interface{}{
		"audio_url":    audioURL,
		"summary":      summary,
		"status":       "Hoàn thành",
		"processed_at": &now,
	})

	db.Preload("User").First(&doc, "id = ?", doc.ID)
	c.JSON(http.StatusOK, gin.H{
		"message":   "Tải lên thành công",
		"tai_lieu":  doc,
		"audio_url": audioURL,
		"summary":   summary,
	})
}

func GetDocuments(c *gin.Context) {
	var documents []models.Document
	query := config.DB.Model(&models.Document{}).Preload("User")

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
	if role == string(models.RoleLecturer) {
		// Giảng viên: chỉ thấy tài liệu của mình
		query = query.Where("user_id = ?", userUUID)
	}

	// Admin có thể lọc theo tên giảng viên
	if role == string(models.RoleAdmin) {
		if lecturer := c.Query("lecturer"); lecturer != "" {
			query = query.Joins("JOIN users ON users.id = documents.user_id").
				Where("users.full_name ILIKE ?", "%"+lecturer+"%")
		}
	}

	// Lọc theo trạng thái
	if status := c.Query("status"); status != "" {
		if status == "Lỗi" {
			query = query.Where("documents.status LIKE ?", "%Lỗi%")
		} else {
			query = query.Where("documents.status = ?", status)
		}
	}

	// Tìm kiếm theo tên tài liệu
	if search := c.Query("search"); search != "" {
		query = query.Where("original_name ILIKE ?", "%"+search+"%")
	}

	// Lọc theo ngày tạo (CreatedAt)
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	if startDate != "" && endDate != "" {
		// Nếu cả 2 đều có → lọc trong khoảng
		query = query.Where("documents.created_at BETWEEN ? AND ?", startDate, endDate)
	} else if startDate != "" {
		// Nếu chỉ có start_date
		query = query.Where("documents.created_at >= ?", startDate)
	} else if endDate != "" {
		// Nếu chỉ có end_date
		query = query.Where("documents.created_at <= ?", endDate)
	}

	// Phân trang
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

	if err := query.Order("documents.created_at DESC").
		Limit(limit).Offset(offset).
		Find(&documents).Error; err != nil {
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
