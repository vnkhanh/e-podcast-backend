package controllers

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vnkhanh/e-podcast-backend/config"
	"github.com/vnkhanh/e-podcast-backend/models"
	"github.com/vnkhanh/e-podcast-backend/services"
	"github.com/vnkhanh/e-podcast-backend/utils"
	"gorm.io/gorm"
)

// /Tạo podcast (tự tạo Chapter nếu chưa có + tự tạo Tag nếu cần)
func CreatePodcastWithUpload(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userIDStr := c.GetString("user_id")

	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
		return
	}

	// === 1 Nhận file upload ===
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Không có file đính kèm"})
		return
	}

	title := c.PostForm("title")
	description := c.PostForm("description")

	// Các trường liên quan đến chương
	chapterIDStr := c.PostForm("chapter_id")
	subjectIDStr := c.PostForm("subject_id")
	chapterTitle := c.PostForm("chapter_title")

	var chapter models.Chapter

	// === 2 Tự tạo Chapter nếu chưa có ===
	if chapterIDStr != "" {
		chapterUUID, err := uuid.Parse(chapterIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "chapter_id không hợp lệ"})
			return
		}
		if err := db.First(&chapter, "id = ?", chapterUUID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Không tìm thấy chương"})
			return
		}
	} else if subjectIDStr != "" && chapterTitle != "" {
		subjectUUID, err := uuid.Parse(subjectIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "subject_id không hợp lệ"})
			return
		}

		// Kiểm tra subject tồn tại trước khi gắn vào chapter
		var subject models.Subject
		if err := db.First(&subject, "id = ?", subjectUUID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Môn học không tồn tại"})
			return
		}

		// Tìm chương trong môn học này
		if err := db.Where("subject_id = ? AND LOWER(title) = LOWER(?)", subjectUUID, chapterTitle).
			First(&chapter).Error; err != nil {

			if errors.Is(err, gorm.ErrRecordNotFound) {
				var maxOrder int
				db.Model(&models.Chapter{}).
					Where("subject_id = ?", subjectUUID).
					Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder)

				chapter = models.Chapter{
					ID:        uuid.New(),
					SubjectID: subjectUUID,
					Title:     chapterTitle,
					SortOrder: maxOrder + 1,
				}

				if err := db.Create(&chapter).Error; err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo chương mới", "details": err.Error()})
					return
				}
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cần cung cấp chapter_id hoặc (subject_id + chapter_title)"})
		return
	}

	// === 3 Upload ảnh bìa (nếu có) ===
	coverImage := ""
	if coverFile, err := c.FormFile("cover_image"); err == nil {
		imageURL, err := utils.UploadImageToSupabase(coverFile, uuid.New().String())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể upload hình ảnh", "details": err.Error()})
			return
		}
		coverImage = imageURL
	}

	//Xử lý voice
	voice := c.DefaultPostForm("voice", "vi-VN-Chirp3-HD-Puck")
	speakingRateStr := c.DefaultPostForm("speaking_rate", "1.0")
	rateValue, err := strconv.ParseFloat(speakingRateStr, 64)
	if err != nil || rateValue <= 0 {
		rateValue = 1.0
	}

	// === 5 Gọi API xử lý tài liệu ===
	authHeader := c.GetHeader("Authorization")
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header không hợp lệ"})
		return
	}
	token := parts[1]

	respData, err := services.CallUploadDocumentAPI(file, userIDStr, token, voice, rateValue)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi khi gọi UploadDocument", "details": err.Error()})
		return
	}

	taiLieuRaw, ok := respData["tai_lieu"]
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy dữ liệu tài liệu từ UploadDocument", "resp": respData})
		return
	}

	taiLieuMap, ok := taiLieuRaw.(map[string]interface{})
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Dữ liệu tài liệu không đúng định dạng", "tai_lieu_raw": taiLieuRaw})
		return
	}

	audioURL, ok := respData["audio_url"].(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy audio URL từ UploadDocument"})
		return
	}

	docIDStr, ok := taiLieuMap["id"].(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy ID tài liệu"})
		return
	}
	docUUID, _ := uuid.Parse(docIDStr)

	durationFloat, err := services.GetMP3DurationFromURL(audioURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tính thời lượng", "details": err.Error()})
		return
	}
	totalSeconds := int(durationFloat)

	// === 6 Tạo podcast mới ===
	podcast := models.Podcast{
		ID:          uuid.New(),
		ChapterID:   chapter.ID,
		DocumentID:  docUUID,
		Title:       title,
		Description: description,
		AudioURL:    audioURL,
		DurationSec: totalSeconds,
		CoverImage:  coverImage,
		Status:      "draft",
		CreatedBy:   userUUID,
		ViewCount:   0,
		LikeCount:   0,
		UpdatedBy:   &userUUID,
		UpdatedAt:   time.Now(),
	}

	if err := db.Create(&podcast).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo podcast", "details": err.Error()})
		return
	}

	// === 7 Gắn Category, Topic, Tag (tự tạo tag nếu cần) ===
	var categories []models.Category
	var topics []models.Topic
	var tags []models.Tag

	categoryIDs := c.PostFormArray("category_ids[]")
	topicIDs := c.PostFormArray("topic_ids[]")
	tagIDs := c.PostFormArray("tag_ids[]")
	tagNames := c.PostFormArray("tag_names[]") // thêm hỗ trợ tạo tag mới theo tên

	if len(categoryIDs) > 0 {
		db.Where("id IN ?", categoryIDs).Find(&categories)
		db.Model(&podcast).Association("Categories").Append(&categories)
	}

	if len(topicIDs) > 0 {
		db.Where("id IN ?", topicIDs).Find(&topics)
		db.Model(&podcast).Association("Topics").Append(&topics)
	}

	// Nếu có tag ID (chọn sẵn)
	if len(tagIDs) > 0 {
		db.Where("id IN ?", tagIDs).Find(&tags)
	}

	// Nếu có tag name (tự tạo mới nếu cần)
	for _, name := range tagNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var tag models.Tag
		if err := db.Where("LOWER(name) = LOWER(?)", name).First(&tag).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				tag = models.Tag{
					ID:   uuid.New(),
					Name: name,
				}
				if err := db.Create(&tag).Error; err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo tag mới", "details": err.Error()})
					return
				}
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		tags = append(tags, tag)
	}

	if len(tags) > 0 {
		db.Model(&podcast).Association("Tags").Append(&tags)
	}

	// === 8 Nạp lại dữ liệu quan hệ ===
	db.Preload("Categories").
		Preload("Topics").
		Preload("Tags").
		First(&podcast, "id = ?", podcast.ID)

	// === 9 Trả JSON hoàn chỉnh ===
	c.JSON(http.StatusOK, gin.H{
		"message": "Tạo podcast thành công",
		"chapter": chapter,
		"podcast": gin.H{
			"id":            podcast.ID,
			"chapter_id":    podcast.ChapterID,
			"document_id":   podcast.DocumentID,
			"title":         podcast.Title,
			"description":   podcast.Description,
			"audio_url":     podcast.AudioURL,
			"duration_sec":  podcast.DurationSec,
			"summary":       podcast.Summary,
			"view_count":    podcast.ViewCount,
			"like_count":    podcast.LikeCount,
			"status":        podcast.Status,
			"cover_image":   podcast.CoverImage,
			"created_by":    podcast.CreatedBy,
			"created_at":    podcast.CreatedAt,
			"published_at":  podcast.PublishedAt,
			"categories":    podcast.Categories,
			"topics":        podcast.Topics,
			"tags":          podcast.Tags,
			"duration_text": 0,
		},
	})
}

func FormatSecondsToHHMMSS(seconds int) string {
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func GetPodcasts(c *gin.Context) {
	var podcasts []models.Podcast
	query := config.DB.Model(&models.Podcast{})
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

	// --- Tìm kiếm theo tên ---
	if search := c.Query("search"); search != "" {
		query = query.Where("title ILIKE ?", "%"+search+"%") // Postgres
	}
	// --- Lọc theo trạng thái ---
	if status := c.Query("status"); status != "" {
		query = query.Where("LOWER(status) = LOWER(?)", status)
	}

	// --- Phân trang ---
	limit := 10
	page := 1
	if p := c.Query("page"); p != "" {
		if _, err := fmt.Sscanf(p, "%d", &page); err != nil || page < 1 {
			page = 1
		}
	}
	if l := c.Query("limit"); l != "" {
		if _, err := fmt.Sscanf(l, "%d", &limit); err != nil || limit < 1 {
			limit = 10
		}
	}

	offset := (page - 1) * limit
	var total int64
	query.Count(&total)
	// --- Lấy dữ liệu ---
	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&podcasts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách podcast"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":       podcasts,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

// GET /api/admin/podcasts/:id
func GetPodcastDetail(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	// 1. Lấy podcast ID từ param
	podcastIDStr := c.Param("id")
	podcastID, err := uuid.Parse(podcastIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Podcast ID không hợp lệ"})
		return
	}

	// 2. Query podcast với preload tất cả quan hệ
	var podcast models.Podcast
	if err := db.Preload("Chapter").
		Preload("Chapter.Subject").
		Preload("Document").
		Preload("Categories").
		Preload("Topics").
		Preload("Tags").
		First(&podcast, "id = ?", podcastID).Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Podcast không tồn tại"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi khi lấy podcast", "details": err.Error()})
		return
	}

	// 3. Chuyển duration thành HH:MM:SS
	durationText := FormatSecondsToHHMMSS(podcast.DurationSec)

	// 4. Trả JSON chi tiết
	c.JSON(http.StatusOK, gin.H{
		"id":            podcast.ID,
		"title":         podcast.Title,
		"description":   podcast.Description,
		"status":        podcast.Status,
		"audio_url":     podcast.AudioURL,
		"duration_sec":  podcast.DurationSec,
		"duration_text": durationText,
		"cover_image":   podcast.CoverImage,
		"summary":       podcast.Summary,
		"view_count":    podcast.ViewCount,
		"like_count":    podcast.LikeCount,
		"created_at":    podcast.CreatedAt,
		"published_at":  podcast.PublishedAt,
		"created_by":    podcast.CreatedBy,
		"updated_at":    podcast.UpdatedAt,
		"updated_by":    podcast.UpdatedBy,
		"chapter": gin.H{
			"id":    podcast.Chapter.ID,
			"title": podcast.Chapter.Title,
			"subject": gin.H{
				"id":   podcast.Chapter.Subject.ID,
				"name": podcast.Chapter.Subject.Name,
			},
		},

		"document": gin.H{
			"id":             podcast.Document.ID,
			"original_name":  podcast.Document.OriginalName,
			"file_path":      podcast.Document.FilePath,
			"file_type":      podcast.Document.FileType,
			"file_size":      podcast.Document.FileSize,
			"status":         podcast.Document.Status,
			"extracted_text": podcast.Document.ExtractedText,
		},
		"categories": podcast.Categories,
		"topics":     podcast.Topics,
		"tags":       podcast.Tags,
	})
}

// DELETE /api/admin/podcasts/:id
func DeletePodcast(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	idStr := c.Param("id")
	podcastUUID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID podcast không hợp lệ"})
		return
	}

	// Load podcast + document
	var podcast models.Podcast
	if err := db.Preload("Document").First(&podcast, "id = ?", podcastUUID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy podcast"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi truy vấn DB", "details": err.Error()})
		}
		return
	}

	// Kiểm tra có podcast khác sử dụng cùng document không
	var otherCount int64
	if podcast.DocumentID != uuid.Nil {
		db.Model(&models.Podcast{}).
			Where("document_id = ? AND id != ?", podcast.DocumentID, podcast.ID).
			Count(&otherCount)
	}

	// Xóa file trên Supabase:
	//  - cover image luôn xóa nếu có
	//  - nếu document không được dùng bởi podcast khác thì xóa document.FilePath và podcast.AudioURL
	if podcast.CoverImage != "" {
		if err := utils.DeleteFileFromSupabase(podcast.CoverImage); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xóa cover image", "details": err.Error()})
			return
		}
	}

	if otherCount == 0 {
		// xóa audio file nếu có
		// if podcast.AudioURL != "" {
		// 	if err := utils.DeleteFileFromSupabase(podcast.AudioURL); err != nil {
		// 		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xóa audio file", "details": err.Error()})
		// 		return
		// 	}
		// }
		// xóa file tài liệu nếu có
		if podcast.Document.FilePath != "" {
			if err := utils.DeleteFileFromSupabase(podcast.Document.FilePath); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xóa file tài liệu", "details": err.Error()})
				return
			}
		}
	}

	// Bắt đầu transaction để xóa DB
	tx := db.Begin()

	// Clear associations (many2many)
	if err := tx.Model(&podcast).Association("Categories").Clear(); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xóa relation categories", "details": err.Error()})
		return
	}
	if err := tx.Model(&podcast).Association("Topics").Clear(); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xóa relation topics", "details": err.Error()})
		return
	}
	if err := tx.Model(&podcast).Association("Tags").Clear(); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xóa relation tags", "details": err.Error()})
		return
	}

	// Xóa podcast
	if err := tx.Delete(&models.Podcast{}, "id = ?", podcast.ID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xóa podcast", "details": err.Error()})
		return
	}

	// Nếu document không còn podcast nào khác -> xóa document DB
	documentDeleted := false
	if otherCount == 0 && podcast.DocumentID != uuid.Nil {
		if err := tx.Delete(&models.Document{}, "id = ?", podcast.DocumentID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xóa tài liệu liên quan", "details": err.Error()})
			return
		}
		documentDeleted = true
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể commit thay đổi", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":          "Đã xóa podcast thành công",
		"document_deleted": documentDeleted,
	})
}

// UpdatePodcastMetadata cập nhật thông tin metadata của podcast (không đụng tới audio, document, status, publish,...)
func UpdatePodcast(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userIDStr := c.GetString("user_id")

	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
		return
	}

	podcastID := c.Param("id")
	var podcast models.Podcast
	if err := db.Preload("Categories").Preload("Topics").Preload("Tags").First(&podcast, "id = ?", podcastID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy podcast"})
		return
	}

	// === 1 Nhận các field text ===
	title := c.PostForm("title")
	description := c.PostForm("description")
	status := c.PostForm("status")

	// === 2 Xử lý Chapter (cho phép cập nhật hoặc tự tạo mới) ===
	chapterIDStr := c.PostForm("chapter_id")
	subjectIDStr := c.PostForm("subject_id")
	chapterTitle := c.PostForm("chapter_title")

	var chapter models.Chapter
	if chapterIDStr != "" {
		chapterUUID, err := uuid.Parse(chapterIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "chapter_id không hợp lệ"})
			return
		}
		if err := db.First(&chapter, "id = ?", chapterUUID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Không tìm thấy chương"})
			return
		}
		podcast.ChapterID = chapter.ID
	} else if subjectIDStr != "" && chapterTitle != "" {
		subjectUUID, err := uuid.Parse(subjectIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "subject_id không hợp lệ"})
			return
		}

		// Tìm hoặc tạo chương mới
		if err := db.Where("subject_id = ? AND LOWER(title) = LOWER(?)", subjectUUID, chapterTitle).
			First(&chapter).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				var maxOrder int
				db.Model(&models.Chapter{}).Where("subject_id = ?", subjectUUID).
					Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder)
				chapter = models.Chapter{
					ID:        uuid.New(),
					SubjectID: subjectUUID,
					Title:     chapterTitle,
					SortOrder: maxOrder + 1,
				}
				if err := db.Create(&chapter).Error; err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo chương mới", "details": err.Error()})
					return
				}
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		podcast.ChapterID = chapter.ID
	}

	// === 3 Upload cover_image mới nếu có ===
	if coverFile, err := c.FormFile("cover_image"); err == nil {
		imageURL, err := utils.UploadImageToSupabase(coverFile, uuid.New().String())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể upload ảnh bìa", "details": err.Error()})
			return
		}
		podcast.CoverImage = imageURL
	}

	// === 5 Cập nhật các trường cơ bản ===
	if title != "" {
		podcast.Title = title
	}
	if description != "" {
		podcast.Description = description
	}
	if status != "" {
		podcast.Status = status
	}
	podcast.UpdatedBy = &userUUID
	podcast.UpdatedAt = time.Now()

	// === 6 Cập nhật Category / Topic / Tag ===
	categoryIDs := c.PostFormArray("category_ids[]")
	topicIDs := c.PostFormArray("topic_ids[]")
	tagIDs := c.PostFormArray("tag_ids[]")
	tagNames := c.PostFormArray("tag_names[]")

	var categories []models.Category
	var topics []models.Topic
	var tags []models.Tag

	db.Model(&podcast).Association("Categories").Clear()
	db.Model(&podcast).Association("Topics").Clear()
	db.Model(&podcast).Association("Tags").Clear()

	if len(categoryIDs) > 0 {
		db.Where("id IN ?", categoryIDs).Find(&categories)
		db.Model(&podcast).Association("Categories").Append(&categories)
	}
	if len(topicIDs) > 0 {
		db.Where("id IN ?", topicIDs).Find(&topics)
		db.Model(&podcast).Association("Topics").Append(&topics)
	}

	if len(tagIDs) > 0 {
		db.Where("id IN ?", tagIDs).Find(&tags)
	}
	for _, name := range tagNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var tag models.Tag
		if err := db.Where("LOWER(name) = LOWER(?)", name).First(&tag).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				tag = models.Tag{
					ID:   uuid.New(),
					Name: name,
				}
				db.Create(&tag)
			}
		}
		tags = append(tags, tag)
	}
	if len(tags) > 0 {
		db.Model(&podcast).Association("Tags").Append(&tags)
	}

	// === 7 Lưu thay đổi ===
	if err := db.Save(&podcast).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể cập nhật podcast", "details": err.Error()})
		return
	}

	db.Preload("Categories").Preload("Topics").Preload("Tags").First(&podcast, "id = ?", podcast.ID)

	c.JSON(http.StatusOK, gin.H{
		"message": "Cập nhật podcast thành công",
		"podcast": podcast,
	})
}

/*============= USER =============*/
// Lấy danh sách podcast theo slug category (chỉ podcast đã publish)
func GetPodcastsByCategory(c *gin.Context) {
	slug := c.Param("slug")

	// 1. Tìm category theo slug
	var category models.Category
	if err := config.DB.First(&category, "slug = ?", slug).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy danh mục"})
		return
	}

	// 2. Đọc query params
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "10")
	sort := c.DefaultQuery("sort", "latest") // latest | popular | duration
	search := strings.TrimSpace(c.DefaultQuery("search", ""))

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 10
	}
	offset := (page - 1) * limit

	// 3. Câu query gốc
	query := config.DB.
		Model(&models.Podcast{}).
		Joins("JOIN podcast_categories pc ON pc.podcast_id = podcasts.id").
		Where("pc.category_id = ?", category.ID).
		Where("podcasts.status = ?", "published")

	// 4. Lọc theo từ khóa (search theo tiêu đề hoặc mô tả)
	if search != "" {
		likeSearch := "%" + search + "%"
		query = query.Where("podcasts.title ILIKE ? OR podcasts.description ILIKE ?", likeSearch, likeSearch)
	}

	// 5. Đếm tổng số podcast
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể đếm số lượng podcast"})
		return
	}

	// 6. Xử lý sắp xếp (sort)
	switch sort {
	case "popular":
		query = query.Order("podcasts.view_count DESC")
	case "duration":
		query = query.Order("podcasts.duration_sec DESC")
	default:
		query = query.Order("podcasts.created_at DESC")
	}

	// 7. Lấy dữ liệu với preload
	var podcasts []models.Podcast
	if err := query.
		Preload("Chapter").
		Preload("Document").
		Preload("Categories").
		Limit(limit).
		Offset(offset).
		Find(&podcasts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách podcast"})
		return
	}

	// 8. Trả JSON kết quả
	c.JSON(http.StatusOK, gin.H{
		"category": gin.H{
			"id":   category.ID,
			"name": category.Name,
			"slug": category.Slug,
		},
		"pagination": gin.H{
			"page":       page,
			"limit":      limit,
			"total":      total,
			"totalPages": int(math.Ceil(float64(total) / float64(limit))),
		},
		"filters": gin.H{
			"sort":   sort,
			"search": search,
		},
		"podcasts": podcasts,
	})
}

// GetFeaturedPodcasts trả về danh sách podcast nổi bật (7 ngày gần đây, sắp theo lượt thích & lượt nghe)
func GetFeaturedPodcasts(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)

	var podcasts []models.Podcast

	// Lấy podcast được tạo trong 7 ngày gần đây, trạng thái published
	if err := db.
		Where("status = ?", "published").
		Where("published_at >= ?", sevenDaysAgo).
		Preload("Chapter").
		Preload("Document").
		Preload("Categories").
		Order("like_count DESC, view_count DESC").
		Limit(10).
		Find(&podcasts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách podcast nổi bật"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Danh sách podcast nổi bật",
		"podcasts": podcasts,
	})
}

// GetPodcastByID - Lấy chi tiết 1 podcast
func GetPodcastByID(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	id := c.Param("id")

	var podcast models.Podcast
	if err := db.
		Preload("Chapter").
		Preload("Chapter.Subject").
		Preload("Document").
		Preload("Document.User").
		Preload("Categories").
		Preload("Topics").
		Preload("Tags").
		First(&podcast, "id = ?", id).Error; err != nil {

		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy podcast"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi khi truy vấn dữ liệu"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Lấy chi tiết podcast thành công",
		"data":    podcast,
	})
}

