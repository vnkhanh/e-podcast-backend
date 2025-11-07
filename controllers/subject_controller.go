package controllers

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	db := config.DB

	role := c.GetString("role")
	userIDStr := c.GetString("user_id")

	var subjects []models.Subject
	query := db.Model(&models.Subject{}).
		Preload("Chapters").
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, full_name, email")
		})

	// Nếu là giảng viên, chỉ thấy môn của mình
	if role == string(models.RoleLecturer) {
		query = query.Where("subjects.created_by = ?", userIDStr)
	}

	// Nếu là admin, có thể lọc theo giảng viên
	if role == string(models.RoleAdmin) {
		query = query.Joins("JOIN users ON users.id = subjects.created_by")

		if lecturer := c.Query("lecturer"); lecturer != "" {
			query = query.Where("(users.full_name ILIKE ? OR users.email ILIKE ?)", "%"+lecturer+"%", "%"+lecturer+"%")
		}
	}

	// Lọc theo trạng thái
	if status := c.Query("status"); status != "" {
		switch status {
		case "true":
			query = query.Where("subjects.status = ?", true)
		case "false":
			query = query.Where("subjects.status = ?", false)
		}
	}

	// Tìm kiếm theo tên môn học
	if search := c.Query("search"); search != "" {
		query = query.Where("subjects.name ILIKE ?", "%"+search+"%")
	}

	// Lọc theo ngày tạo
	fromDateStr := c.Query("from_date")
	toDateStr := c.Query("to_date")
	if fromDateStr != "" || toDateStr != "" {
		const layout = "2006-01-02"
		if fromDateStr != "" && toDateStr != "" {
			fromDate, err1 := time.Parse(layout, fromDateStr)
			toDate, err2 := time.Parse(layout, toDateStr)
			if err1 == nil && err2 == nil {
				toDate = toDate.Add(24 * time.Hour)
				query = query.Where("subjects.created_at BETWEEN ? AND ?", fromDate, toDate)
			}
		} else if fromDateStr != "" {
			fromDate, err := time.Parse(layout, fromDateStr)
			if err == nil {
				query = query.Where("subjects.created_at >= ?", fromDate)
			}
		} else if toDateStr != "" {
			toDate, err := time.Parse(layout, toDateStr)
			if err == nil {
				toDate = toDate.Add(24 * time.Hour)
				query = query.Where("subjects.created_at < ?", toDate)
			}
		}
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể đếm tổng số môn học"})
		return
	}

	// Dữ liệu
	if err := query.
		Order("subjects.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&subjects).Error; err != nil {
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
	if err := config.DB.
		Preload("Chapters", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC")
		}).
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, full_name, email")
		}).
		Preload("UpdatedByUser", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, full_name, email")
		}).
		First(&subject, "id = ?", subjectID).Error; err != nil {

		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy môn học"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"subject":  subject,
		"chapters": subject.Chapters,
	})
}

type ChapterInput struct {
	ID        *uuid.UUID `json:"id,omitempty"` // có thể null: nếu null -> tạo mới
	Title     string     `json:"title"`
	SortOrder int        `json:"sort_order"`
}

type UpdateSubjectInput struct {
	Name     string         `json:"name"`
	Status   *bool          `json:"status"`
	Chapters []ChapterInput `json:"chapters"`
}

// PUT /admin/subjects/:id
func UpdateSubject(c *gin.Context) {
	var input UpdateSubjectInput

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dữ liệu không hợp lệ"})
		return
	}

	idParam := c.Param("id")
	subjectID, err := uuid.Parse(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	var subject models.Subject
	if err := config.DB.Preload("Chapters").First(&subject, "id = ?", subjectID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Môn học không tồn tại"})
		return
	}

	// === 1. Cập nhật thông tin cơ bản ===
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
	subject.UpdatedBy = userUUID

	name := strings.TrimSpace(input.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tên môn học không được trống"})
		return
	}

	slugValue := slug.Make(name)
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
	if input.Status != nil {
		subject.Status = *input.Status
	}

	if err := config.DB.Save(&subject).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cập nhật môn học thất bại"})
		return
	}

	// === 2. Xử lý chương ===
	existingChapterIDs := make(map[uuid.UUID]bool)
	for _, ch := range subject.Chapters {
		existingChapterIDs[ch.ID] = true
	}

	newChapterIDs := make(map[uuid.UUID]bool)

	for _, chInput := range input.Chapters {
		title := strings.TrimSpace(chInput.Title)
		if title == "" {
			continue
		}

		// Cập nhật chương cũ
		if chInput.ID != nil {
			var existing models.Chapter
			if err := config.DB.First(&existing, "id = ? AND subject_id = ?", chInput.ID, subjectID).Error; err == nil {
				existing.Title = title
				existing.SortOrder = chInput.SortOrder
				if err := config.DB.Save(&existing).Error; err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi khi cập nhật chương"})
					return
				}
				newChapterIDs[existing.ID] = true
			}
		} else {
			// Thêm mới chương
			newChapter := models.Chapter{
				SubjectID: subjectID,
				Title:     title,
				SortOrder: chInput.SortOrder,
			}
			if err := config.DB.Create(&newChapter).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi khi tạo chương mới"})
				return
			}
			newChapterIDs[newChapter.ID] = true
		}
	}

	// === 3. Xóa chương không còn trong danh sách ===
	for oldID := range existingChapterIDs {
		if !newChapterIDs[oldID] {
			var podcastCount int64
			if err := config.DB.Model(&models.Podcast{}).Where("chapter_id = ?", oldID).Count(&podcastCount).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi khi kiểm tra podcast của chương"})
				return
			}

			if podcastCount > 0 {
				var chapter models.Chapter
				_ = config.DB.Select("title").First(&chapter, "id = ?", oldID)
				c.JSON(http.StatusBadRequest, gin.H{
					"error": fmt.Sprintf("Không thể xóa '%s' vì chương này có %d podcast liên quan", chapter.Title, podcastCount),
				})
				return
			}

			if err := config.DB.Delete(&models.Chapter{}, "id = ?", oldID).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi khi xóa chương"})
				return
			}
		}
	}

	// === 4. Trả kết quả cập nhật ===
	// === 4. Trả kết quả cập nhật ===
	var updatedSubject models.Subject
	if err := config.DB.
		Preload("Chapters").
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, full_name, email")
		}).
		Preload("UpdatedByUser", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, full_name, email")
		}).
		First(&updatedSubject, "id = ?", subjectID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tải lại dữ liệu sau khi cập nhật"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Cập nhật thành công",
		"subject": updatedSubject,
	})
}

// GET /admin/chapters/:id/check-deletable
func CheckChapterDeletable(c *gin.Context) {
	idStr := c.Param("id")
	chapterID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	var count int64
	config.DB.Model(&models.Podcast{}).Where("chapter_id = ?", chapterID).Count(&count)

	if count > 0 {
		c.JSON(http.StatusOK, gin.H{"can_delete": false, "message": fmt.Sprintf("Chương này có %d podcast, không thể xóa", count)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"can_delete": true})
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

/*========= USER ==========*/
//Môn học phổ biến
type SubjectStats struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	PodcastCount int    `json:"podcast_count"`
	TotalViews   int    `json:"total_views"`
	TotalLikes   int    `json:"total_likes"`
}

// API: Lấy 5 môn học phổ biến (status = true)
func GetPopularSubjects(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	var results []SubjectStats

	// Truy vấn gộp dữ liệu: subject → chapter → podcast
	query := `
	SELECT 
		s.id,
		s.name,
		s.slug,
		COUNT(p.id) AS podcast_count,
		COALESCE(SUM(p.view_count), 0) AS total_views,
		COALESCE(SUM(p.like_count), 0) AS total_likes
	FROM subjects s
	LEFT JOIN chapters c ON c.subject_id = s.id
	LEFT JOIN podcasts p ON p.chapter_id = c.id AND p.status = 'published'
	WHERE s.status = TRUE
	GROUP BY s.id, s.name, s.slug
	ORDER BY total_views DESC
	LIMIT 5;
	`

	if err := db.Raw(query).Scan(&results).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Không thể lấy danh sách môn học phổ biến",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Lấy danh sách môn học phổ biến thành công",
		"data":    results,
	})
}

// Chi tiết môn học (có tiến độ nếu người dùng đã đăng nhập)
func GetSubjectDetailUser(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	slug := c.Param("slug")

	// Lấy thông tin môn học
	var subject models.Subject
	if err := db.
		Preload("Chapters", func(db *gorm.DB) *gorm.DB {
			return db.Order("CAST(substring(title from '[0-9]+') AS INTEGER) ASC, title ASC")
		}).
		Preload("Chapters.Podcasts", func(db *gorm.DB) *gorm.DB {
			return db.
				Where("podcasts.status = ?", "published").
				Order("CAST(substring(title from '[0-9]+') AS INTEGER) ASC, title ASC")
		}).
		Where("slug = ? AND status = ?", slug, true).
		First(&subject).Error; err != nil {

		c.JSON(http.StatusNotFound, gin.H{
			"message": "Không tìm thấy môn học",
			"error":   err.Error(),
		})
		return
	}

	// Mặc định dữ liệu trả về
	response := gin.H{
		"message": "Lấy chi tiết môn học thành công",
		"data":    subject,
	}

	// Nếu có user đăng nhập → tính tiến độ
	userID, exists := c.Get("user_id")
	if exists {
		var totalPodcasts int64
		var completedCount int64

		// Tổng số podcast trong môn học
		db.Model(&models.Podcast{}).
			Joins("JOIN chapters ON chapters.id = podcasts.chapter_id").
			Where("chapters.subject_id = ?", subject.ID).
			Count(&totalPodcasts)

		// Số podcast đã hoàn thành
		db.Model(&models.ListeningHistory{}).
			Joins("JOIN podcasts ON podcasts.id = listening_histories.podcast_id").
			Joins("JOIN chapters ON chapters.id = podcasts.chapter_id").
			Where("listening_histories.user_id = ? AND chapters.subject_id = ? AND listening_histories.completed = TRUE",
				userID, subject.ID).
			Count(&completedCount)

		overallProgress := 0.0
		if totalPodcasts > 0 {
			overallProgress = (float64(completedCount) / float64(totalPodcasts)) * 100
		}

		// Tiến độ theo từng chương
		var chapterProgress []struct {
			ChapterID uuid.UUID `json:"chapter_id"`
			Title     string    `json:"title"`
			Total     int64     `json:"total"`
			Done      int64     `json:"done"`
			Progress  float64   `json:"progress"`
		}

		db.Table("chapters").
			Select(`
				chapters.id AS chapter_id,
				chapters.title AS title,
				COUNT(podcasts.id) AS total,
				COALESCE(SUM(CASE WHEN listening_histories.completed = TRUE THEN 1 ELSE 0 END), 0) AS done
			`).
			Joins("LEFT JOIN podcasts ON podcasts.chapter_id = chapters.id").
			Joins("LEFT JOIN listening_histories ON listening_histories.podcast_id = podcasts.id AND listening_histories.user_id = ?", userID).
			Where("chapters.subject_id = ?", subject.ID).
			Group("chapters.id").
			Order("MIN(chapters.created_at) ASC").
			Scan(&chapterProgress)

		for i := range chapterProgress {
			if chapterProgress[i].Total > 0 {
				chapterProgress[i].Progress = (float64(chapterProgress[i].Done) / float64(chapterProgress[i].Total)) * 100
			}
		}

		// Thêm vào response
		response["overall_progress"] = overallProgress
		response["chapter_progress"] = chapterProgress
	}

	// Trả kết quả cuối cùng
	c.JSON(http.StatusOK, response)
}

// Danh sách môn học + tiến độ học tập (nếu có user)
func GetAllSubjectsUser(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	search := c.Query("search")
	sortOrder := c.DefaultQuery("sort", "az")
	page := c.DefaultQuery("page", "1")
	limit := c.DefaultQuery("limit", "10")

	pageNum, _ := strconv.Atoi(page)
	limitNum, _ := strconv.Atoi(limit)
	if pageNum < 1 {
		pageNum = 1
	}
	offset := (pageNum - 1) * limitNum

	// Tạo base query (bắt đầu từ subjects)
	baseQuery := db.Table("subjects").
		Select("DISTINCT subjects.id, subjects.name, subjects.slug, subjects.status, subjects.created_at, subjects.updated_at, subjects.created_by, subjects.updated_by").
		Joins("LEFT JOIN chapters ON chapters.subject_id = subjects.id").
		Where("subjects.status = ?", true)

	// Nếu có tìm kiếm thì tìm theo tên môn học hoặc tên chương
	if search != "" {
		searchPattern := "%" + search + "%"
		baseQuery = baseQuery.Where("subjects.name ILIKE ? OR chapters.title ILIKE ?", searchPattern, searchPattern)
	}

	// Đếm tổng số bản ghi
	var total int64
	if err := db.Table("(?) as subquery", baseQuery).
		Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Lỗi khi đếm tổng môn học",
			"error":   err.Error(),
		})
		return
	}

	// Áp dụng sắp xếp
	if sortOrder == "za" {
		baseQuery = baseQuery.Order("subjects.name DESC")
	} else {
		baseQuery = baseQuery.Order("subjects.name ASC")
	}

	// Áp dụng phân trang và lấy danh sách môn học
	var subjects []models.Subject
	if err := baseQuery.
		Preload("Chapters", func(db *gorm.DB) *gorm.DB {
			return db.Order("CAST(substring(title from '[0-9]+') AS INTEGER) ASC, title ASC")
		}).
		Limit(limitNum).
		Offset(offset).
		Find(&subjects).Error; err != nil {

		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "Lỗi khi lấy danh sách môn học",
			"error":   err.Error(),
		})
		return
	}

	// Kiểm tra user đăng nhập
	userID, exists := c.Get("user_id")

	// Nếu chưa đăng nhập thì trả về danh sách cơ bản
	if !exists {
		c.JSON(http.StatusOK, gin.H{
			"message":  "Lấy danh sách môn học thành công (không có tiến độ)",
			"subjects": subjects,
			"pagination": gin.H{
				"page":  pageNum,
				"limit": limitNum,
				"total": total,
				"pages": int(math.Ceil(float64(total) / float64(limitNum))),
			},
		})
		return
	}

	// Nếu có user thì tính tiến độ
	type SubjectProgress struct {
		SubjectID       uuid.UUID `json:"subject_id"`
		Name            string    `json:"name"`
		TotalPodcasts   int64     `json:"total_podcasts"`
		Completed       int64     `json:"completed"`
		ProgressPercent float64   `json:"progress_percent"`
	}

	var progressList []SubjectProgress

	db.Table("subjects").
		Select(`
			subjects.id AS subject_id,
			subjects.name AS name,
			COUNT(podcasts.id) AS total_podcasts,
			COALESCE(SUM(CASE WHEN listening_histories.completed = TRUE THEN 1 ELSE 0 END), 0) AS completed
		`).
		Joins("LEFT JOIN chapters ON chapters.subject_id = subjects.id").
		Joins("LEFT JOIN podcasts ON podcasts.chapter_id = chapters.id").
		Joins("LEFT JOIN listening_histories ON listening_histories.podcast_id = podcasts.id AND listening_histories.user_id = ?", userID).
		Where("subjects.status = ?", true).
		Group("subjects.id").
		Scan(&progressList)

	for i := range progressList {
		if progressList[i].TotalPodcasts > 0 {
			progressList[i].ProgressPercent = (float64(progressList[i].Completed) / float64(progressList[i].TotalPodcasts)) * 100
		}
	}

	// Trả kết quả
	c.JSON(http.StatusOK, gin.H{
		"message":  "Lấy danh sách môn học thành công",
		"subjects": subjects,
		"progress": progressList,
		"pagination": gin.H{
			"page":  pageNum,
			"limit": limitNum,
			"total": total,
			"pages": int(math.Ceil(float64(total) / float64(limitNum))),
		},
	})
}
