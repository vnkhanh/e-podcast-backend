package controllers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math/rand"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vnkhanh/e-podcast-backend/models"
	"github.com/vnkhanh/e-podcast-backend/services"
	"gorm.io/gorm"
)

// ==================== GIẢNG VIÊN TẠO ASSIGNMENT ====================

// Tạo assignment từ Gemini (dựa trên ExtractedText)
func CreateAssignmentFromGemini(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userIDStr := c.GetString("user_id")
	role := c.GetString("role")

	if role != string(models.RoleAdmin) && role != string(models.RoleLecturer) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Chỉ giảng viên/admin mới có quyền tạo bài tập"})
		return
	}

	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
		return
	}

	var req struct {
		PodcastID    string     `json:"podcast_id" binding:"required"`
		Title        string     `json:"title" binding:"required"`
		Description  string     `json:"description"`
		DueDate      *time.Time `json:"due_date"`
		MaxAttempts  int        `json:"max_attempts"`
		TimeLimit    int        `json:"time_limit"`
		PassScore    float64    `json:"pass_score"`
		NumQuestions int        `json:"num_questions"` // Số câu hỏi cần tạo
		HasPassword  bool       `json:"has_password"`
		Password     string     `json:"password"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate
	if req.MaxAttempts == 0 {
		req.MaxAttempts = 1
	}
	if req.PassScore == 0 {
		req.PassScore = 5.0
	}
	if req.NumQuestions == 0 {
		req.NumQuestions = 10
	}

	podcastUUID, _ := uuid.Parse(req.PodcastID)
	// Lấy podcast
	var podcast models.Podcast
	if err := db.Preload("Document").First(&podcast, "id = ?", podcastUUID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy podcast"})
		return
	}

	doc := podcast.Document

	if doc.ExtractedText == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tài liệu của podcast chưa có nội dung"})
		return
	}

	// Tạo assignment
	assignment := models.Assignment{
		PodcastID:   podcastUUID,
		Title:       req.Title,
		Description: req.Description,
		DueDate:     req.DueDate,
		MaxAttempts: req.MaxAttempts,
		TimeLimit:   req.TimeLimit,
		PassScore:   req.PassScore,
		IsPublished: false,
		CreatedBy:   userUUID,
		HasPassword: req.HasPassword,
		Password:    req.Password,
	}

	if err := db.Create(&assignment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo bài tập"})
		return
	}

	// Chia nhỏ văn bản
	chunks := SplitTextIntoChunksSmart(doc.ExtractedText, 2000)

	allQuestions := []models.AssignmentQuestion{}
	questionsPerChunk := (req.NumQuestions + len(chunks) - 1) / len(chunks)

	for idx, chunk := range chunks {
		if len(allQuestions) >= req.NumQuestions {
			break
		}

		prompt := fmt.Sprintf(`
Bạn là AI tạo câu hỏi trắc nghiệm đánh giá.
Tạo %d câu hỏi trắc nghiệm từ đoạn văn sau bằng tiếng Việt.

Yêu cầu:
- Mỗi câu có 4 lựa chọn (A, B, C, D)
- Chỉ 1 đáp án đúng
- Câu hỏi đánh giá sự hiểu biết sâu sắc, không chỉ thuộc lòng
- Thêm trường "explanation" giải thích tại sao đáp án đúng
- Thêm trường "points": điểm của câu (2.0 - 1.0 cho câu dễ, 0.5 cho câu khó)
- Có thể ghi theo giáo trình hoặc theo podcast chứ không được ghi theo đoạn văn
Trả về JSON:
[
  {
    "question": "Câu hỏi?",
    "explanation": "Giải thích đáp án",
    "points": 1.0,
    "options": [
      {"text": "A", "is_correct": true},
      {"text": "B", "is_correct": false},
      {"text": "C", "is_correct": false},
      {"text": "D", "is_correct": false}
    ]
  }
]

Đoạn văn số %d:
%s
`, questionsPerChunk, idx+1, chunk)

		rawResp, err := services.GeminiGenerateText(prompt)
		if err != nil {
			fmt.Printf("Gemini lỗi ở đoạn %d: %v\n", idx+1, err)
			continue
		}

		clean := strings.TrimSpace(rawResp)
		clean = strings.Trim(clean, "`")
		clean = strings.TrimPrefix(clean, "json")
		clean = strings.TrimSpace(clean)

		type OptDTO struct {
			Text      string `json:"text"`
			IsCorrect bool   `json:"is_correct"`
		}
		type QA struct {
			Question    string   `json:"question"`
			Explanation string   `json:"explanation"`
			Points      float64  `json:"points"`
			Options     []OptDTO `json:"options"`
		}

		var arr []QA
		if err := json.Unmarshal([]byte(clean), &arr); err != nil {
			fmt.Printf("Parse JSON lỗi ở đoạn %d: %v\n", idx+1, err)
			continue
		}

		for _, qa := range arr {
			if len(allQuestions) >= req.NumQuestions {
				break
			}

			q := models.AssignmentQuestion{
				AssignmentID: assignment.ID,
				Question:     qa.Question,
				Explanation:  qa.Explanation,
				Points:       qa.Points,
				SortOrder:    len(allQuestions) + 1,
			}

			if err := db.Create(&q).Error; err != nil {
				continue
			}

			for i, opt := range qa.Options {
				o := models.AssignmentOption{
					QuestionID: q.ID,
					OptionText: opt.Text,
					IsCorrect:  opt.IsCorrect,
					SortOrder:  i + 1,
				}
				db.Create(&o)
			}

			allQuestions = append(allQuestions, q)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Tạo bài tập thành công",
		"assignment": assignment,
		"total":      len(allQuestions),
		"questions":  allQuestions,
	})
}

func GenerateAssignmentPassword() string {
	letters := "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// Tạo assignment từ file Excel/CSV upload
func CreateAssignmentFromFile(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userIDStr := c.GetString("user_id")
	role := c.GetString("role")

	if role != string(models.RoleAdmin) && role != string(models.RoleLecturer) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Chỉ giảng viên/admin mới có quyền tạo bài tập"})
		return
	}

	userUUID, _ := uuid.Parse(userIDStr)
	podcastID := c.PostForm("podcast_id")
	title := c.PostForm("title")
	description := c.PostForm("description")
	maxAttempts, _ := strconv.Atoi(c.DefaultPostForm("max_attempts", "1"))
	timeLimit, _ := strconv.Atoi(c.DefaultPostForm("time_limit", "0"))
	passScore, _ := strconv.ParseFloat(c.DefaultPostForm("pass_score", "5.0"), 64)
	hasPassword := c.DefaultPostForm("has_password", "false") == "true"
	password := c.PostForm("password")

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Không có file đính kèm"})
		return
	}

	// Parse file (Excel/CSV)
	questions, err := parseQuestionFile(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File không hợp lệ: " + err.Error()})
		return
	}

	podcastUUID, _ := uuid.Parse(podcastID)

	// Tạo assignment
	assignment := models.Assignment{
		PodcastID:   podcastUUID,
		Title:       title,
		Description: description,
		MaxAttempts: maxAttempts,
		TimeLimit:   timeLimit,
		PassScore:   passScore,
		IsPublished: false,
		CreatedBy:   userUUID,
		HasPassword: hasPassword,
		Password:    password,
	}

	if err := db.Create(&assignment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo bài tập"})
		return
	}

	// Lưu câu hỏi
	for i, q := range questions {
		question := models.AssignmentQuestion{
			AssignmentID: assignment.ID,
			Question:     q.Question,
			Explanation:  q.Explanation,
			Points:       q.Points,
			SortOrder:    i + 1,
		}

		if err := db.Create(&question).Error; err != nil {
			continue
		}

		for j, opt := range q.Options {
			option := models.AssignmentOption{
				QuestionID: question.ID,
				OptionText: opt.Text,
				IsCorrect:  opt.IsCorrect,
				SortOrder:  j + 1,
			}
			db.Create(&option)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Tạo bài tập từ file thành công",
		"assignment": assignment,
		"total":      len(questions),
	})
}

// Helper: Parse file Excel/CSV
type QuestionDTO struct {
	Question    string
	Explanation string
	Points      float64
	Options     []OptionDTO
}

type OptionDTO struct {
	Text      string
	IsCorrect bool
}

// Lấy danh sách assignment của giảng viên
func GetTeacherAssignments(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userIDStr := c.GetString("user_id")
	role := c.GetString("role")

	// --- QUERY SEARCH PARAMETERS ---
	search := c.Query("search") // nhận search chung
	userUUID, _ := uuid.Parse(userIDStr)

	query := db.Model(&models.Assignment{}).
		Preload("Podcast").
		Preload("Podcast.Chapter").
		Preload("Podcast.Chapter.Subject").
		Preload("Questions.Options")

	// Quyền giảng viên
	if role == string(models.RoleLecturer) {
		query = query.Where("assignments.created_by = ?", userUUID)
	}

	// Admin preload creator
	if role == string(models.RoleAdmin) {
		query = query.Preload("Creator")
	}

	// -------------------------
	// SEARCH
	// Tìm theo:
	// - tên bài tập
	// - tên chương
	// - tên môn học
	// - tên podcast
	// -------------------------
	if search != "" {
		like := "%" + search + "%"

		query = query.
			Joins("LEFT JOIN podcasts ON podcasts.id = assignments.podcast_id").
			Joins("LEFT JOIN chapters ON chapters.id = podcasts.chapter_id").
			Joins("LEFT JOIN subjects ON subjects.id = chapters.subject_id").
			Where(`
				assignments.title ILIKE ?
				OR podcasts.title ILIKE ?
				OR chapters.title ILIKE ?
				OR subjects.name ILIKE ?
			`, like, like, like, like)
	}

	var assignments []models.Assignment

	if err := query.Order("assignments.created_at DESC").Find(&assignments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách bài tập"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"assignments": assignments,
		"total":       len(assignments),
	})
}

// Lấy chi tiết 1 assignment theo ID
func GetAssignmentDetailTeacher(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	role := c.GetString("role")
	userIDStr := c.GetString("user_id")

	assignmentIDStr := c.Param("id")
	assignmentUUID, err := uuid.Parse(assignmentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	query := db.Model(&models.Assignment{}).
		Preload("Podcast").
		Preload("Podcast.Chapter").
		Preload("Podcast.Chapter.Subject")

	// Admin preload creator
	if role == string(models.RoleAdmin) {
		query = query.Preload("Creator")
	}

	// Giảng viên chỉ được xem bài tập của mình
	if role == string(models.RoleLecturer) {
		userUUID, _ := uuid.Parse(userIDStr)
		query = query.Where("assignments.created_by = ?", userUUID)
	}

	var assignment models.Assignment
	if err := query.Where("assignments.id = ?", assignmentUUID).First(&assignment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy bài tập"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi server"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"assignment": assignment,
	})
}

// Cập nhật assignment
func UpdateAssignment(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	assignmentID := c.Param("id")
	userIDStr := c.GetString("user_id")
	role := c.GetString("role")

	userUUID, _ := uuid.Parse(userIDStr)
	assUUID, _ := uuid.Parse(assignmentID)

	var assignment models.Assignment
	if err := db.First(&assignment, "id = ?", assUUID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy bài tập"})
		return
	}

	// Kiểm tra quyền
	if role != string(models.RoleAdmin) && assignment.CreatedBy != userUUID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bạn không có quyền chỉnh sửa bài tập này"})
		return
	}

	var req struct {
		Title       string     `json:"title"`
		Description string     `json:"description"`
		DueDate     *time.Time `json:"due_date"`
		MaxAttempts int        `json:"max_attempts"`
		TimeLimit   int        `json:"time_limit"`
		PassScore   float64    `json:"pass_score"`
		IsPublished bool       `json:"is_published"`
		HasPassword bool       `json:"has_password"`
		Password    string     `json:"password"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Title != "" {
		assignment.Title = req.Title
	}
	assignment.Description = req.Description
	assignment.DueDate = req.DueDate
	if req.MaxAttempts > 0 {
		assignment.MaxAttempts = req.MaxAttempts
	}
	assignment.TimeLimit = req.TimeLimit
	if req.PassScore > 0 {
		assignment.PassScore = req.PassScore
	}
	assignment.IsPublished = req.IsPublished
	assignment.HasPassword = req.HasPassword
	assignment.Password = req.Password

	if err := db.Save(&assignment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể cập nhật bài tập"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Cập nhật bài tập thành công",
		"assignment": assignment,
	})
}

// Thêm endpoint mới để verify password trước khi làm bài:
func VerifyAssignmentPassword(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	assignmentID := c.Param("id")

	var req struct {
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	assUUID, _ := uuid.Parse(assignmentID)

	var assignment models.Assignment
	if err := db.First(&assignment, "id = ?", assUUID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy bài tập"})
		return
	}

	if !assignment.HasPassword {
		c.JSON(http.StatusOK, gin.H{"valid": true})
		return
	}

	if assignment.Password != req.Password {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Mật khẩu không đúng",
			"valid": false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":   true,
		"message": "Mật khẩu chính xác",
	})
}

// Xóa assignment
func DeleteAssignment(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	assignmentID := c.Param("id")
	userIDStr := c.GetString("user_id")
	role := c.GetString("role")

	userUUID, _ := uuid.Parse(userIDStr)
	assUUID, _ := uuid.Parse(assignmentID)

	var assignment models.Assignment
	if err := db.First(&assignment, "id = ?", assUUID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy bài tập"})
		return
	}

	if role != string(models.RoleAdmin) && assignment.CreatedBy != userUUID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bạn không có quyền xóa bài tập này"})
		return
	}

	if err := db.Delete(&assignment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xóa bài tập"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Xóa bài tập thành công"})
}

// Publish/Unpublish assignment
func TogglePublishAssignment(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	assignmentID := c.Param("id")
	userIDStr := c.GetString("user_id")
	role := c.GetString("role")

	userUUID, _ := uuid.Parse(userIDStr)
	assUUID, _ := uuid.Parse(assignmentID)

	var assignment models.Assignment
	if err := db.First(&assignment, "id = ?", assUUID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy bài tập"})
		return
	}

	if role != string(models.RoleAdmin) && assignment.CreatedBy != userUUID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bạn không có quyền thay đổi trạng thái bài tập"})
		return
	}

	assignment.IsPublished = !assignment.IsPublished

	if err := db.Save(&assignment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể cập nhật trạng thái"})
		return
	}

	status := "đã ẩn"
	if assignment.IsPublished {
		status = "đã công bố"
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "Bài tập " + status,
		"is_published": assignment.IsPublished,
	})
}

// GetTeacherSubjects trả về danh sách môn học của teacher hiện tại kèm chương
func GetTeacherSubjects(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: missing user_id"})
		return
	}

	userUUID, err := uuid.Parse(userIDStr.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
		return
	}

	var subjects []models.Subject
	if err := db.Preload("Chapters").
		Where("created_by = ?", userUUID).
		Find(&subjects).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy môn học"})
		return
	}

	// Đảm bảo trả về đúng format
	c.JSON(http.StatusOK, gin.H{
		"subjects": subjects,
		"total":    len(subjects),
	})
}

// GetPodcastsByChapter trả về danh sách podcast theo chapter
func GetPodcastsByChapter(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	chapterID := c.Param("chapterID")

	if chapterID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chapterID không được để trống"})
		return
	}

	var podcasts []models.Podcast
	if err := db.Where("chapter_id = ?", chapterID).Find(&podcasts).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusOK, gin.H{"podcasts": []models.Podcast{}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy podcast"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"podcasts": podcasts})
}

// Lấy danh sách bài nộp của một assignment (giảng viên)
// GET /admin/assignments/:id/submissions
func GetAssignmentSubmissions(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	assignmentID := c.Param("id")

	// Parse UUID
	assUUID, err := uuid.Parse(assignmentID)
	if err != nil {
		c.JSON(400, gin.H{"error": "ID bài tập không hợp lệ"})
		return
	}

	// Query params
	search := c.Query("search")
	status := c.Query("status") // passed | failed
	pageStr := c.Query("page")
	limitStr := c.Query("limit")

	// Pagination default
	page := 1
	limit := 10
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}
	offset := (page - 1) * limit

	// Base query (không preload ở đây)
	query := db.Model(&models.AssignmentSubmission{}).
		Where("assignment_id = ?", assUUID)

	// Tìm kiếm theo tên
	if search != "" {
		like := "%" + search + "%"
		query = query.Joins("JOIN users ON users.id = assignment_submissions.user_id").
			Where("users.full_name ILIKE ?", like)
	}

	// Lọc pass/fail
	if status == "passed" {
		query = query.Where("is_passed = ?", true)
	}
	if status == "failed" {
		query = query.Where("is_passed = ?", false)
	}

	// Đếm tổng sau khi filter
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(500, gin.H{"error": "Không thể đếm số lượng bài nộp"})
		return
	}

	// Lấy dữ liệu theo phân trang
	var submissions []models.AssignmentSubmission
	if err := query.
		Preload("User").
		Preload("Answers").
		Order("submitted_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&submissions).Error; err != nil {

		c.JSON(500, gin.H{"error": "Không thể lấy danh sách bài nộp"})
		return
	}

	c.JSON(200, gin.H{
		"submissions": submissions,
		"total":       total,
		"page":        page,
		"limit":       limit,
	})
}

// Lấy chi tiết một bài nộp của sinh viên
// GET /admin/submissions/:id
func GetAssignmentSubmissionDetailTeacher(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	submissionID := c.Param("id")

	// Parse UUID
	subUUID, err := uuid.Parse(submissionID)
	if err != nil {
		c.JSON(400, gin.H{"error": "ID bài nộp không hợp lệ"})
		return
	}

	// Lấy chi tiết bài nộp
	var submission models.AssignmentSubmission
	if err := db.Preload("User").
		Preload("Assignment").
		Preload("Answers").
		Preload("Answers.Question").
		Preload("Answers.SelectedOption").
		Preload("Answers.Question.Options").
		First(&submission, "id = ?", subUUID).Error; err != nil {
		c.JSON(404, gin.H{"error": "Bài nộp không tìm thấy"})
		return
	}

	// Trả dữ liệu
	c.JSON(200, gin.H{
		"submission": submission,
	})
}

// ==================== SINH VIÊN LÀM BÀI ====================

// Lấy danh sách assignment theo podcast (sinh viên)
func GetAssignmentsByPodcast(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	podcastID := c.Param("id")
	userIDStr := c.GetString("user_id")

	userUUID, _ := uuid.Parse(userIDStr)
	podcastUUID, _ := uuid.Parse(podcastID)

	var assignments []models.Assignment
	if err := db.Where("podcast_id = ? AND is_published = ?", podcastUUID, true).
		Preload("Questions").
		Preload("Creator").
		Order("created_at DESC").
		Find(&assignments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách bài tập"})
		return
	}

	// Lấy số lần đã làm của user
	type AssignmentWithProgress struct {
		models.Assignment
		AttemptsUsed int     `json:"attempts_used"`
		BestScore    float64 `json:"best_score"`
	}

	var result []AssignmentWithProgress
	for _, ass := range assignments {
		var attemptsUsed int64
		var bestScore float64

		db.Model(&models.AssignmentSubmission{}).
			Where("assignment_id = ? AND user_id = ?", ass.ID, userUUID).
			Count(&attemptsUsed)

		db.Model(&models.AssignmentSubmission{}).
			Where("assignment_id = ? AND user_id = ?", ass.ID, userUUID).
			Select("COALESCE(MAX(score), 0)").
			Scan(&bestScore)

		result = append(result, AssignmentWithProgress{
			Assignment:   ass,
			AttemptsUsed: int(attemptsUsed),
			BestScore:    bestScore,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"assignments": result,
		"total":       len(result),
	})
}

// Lấy chi tiết assignment (không chặn nếu hết lượt)
func GetAssignmentDetailStudent(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	assignmentID := c.Param("id")
	userIDStr := c.GetString("user_id")

	userUUID, _ := uuid.Parse(userIDStr)
	assUUID, _ := uuid.Parse(assignmentID)

	var assignment models.Assignment
	if err := db.Preload("Questions.Options").
		First(&assignment, "id = ?", assUUID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy bài tập"})
		return
	}

	if !assignment.IsPublished {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bài tập chưa được công bố"})
		return
	}
	// Kiểm tra password nếu có
	if assignment.HasPassword {
		// Ẩn password khi trả về cho client
		assignment.Password = ""
	}
	// Lấy số lần đã làm của user
	var attemptsUsed int64
	db.Model(&models.AssignmentSubmission{}).
		Where("assignment_id = ? AND user_id = ?", assignment.ID, userUUID).
		Count(&attemptsUsed)

	c.JSON(http.StatusOK, gin.H{
		"assignment":    assignment,
		"attempts_used": attemptsUsed,
		"attempts_left": assignment.MaxAttempts - int(attemptsUsed),
	})
}

// Nộp bài assignment
func SubmitAssignment(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	assignmentID := c.Param("id")
	userIDStr := c.GetString("user_id")

	userUUID, _ := uuid.Parse(userIDStr)
	assUUID, _ := uuid.Parse(assignmentID)

	var req struct {
		Answers []struct {
			QuestionID uuid.UUID  `json:"question_id"`
			SelectedID *uuid.UUID `json:"selected_id"`
		} `json:"answers"`
		TimeSpent int `json:"time_spent"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Lấy assignment
	var assignment models.Assignment
	if err := db.Preload("Questions.Options").
		First(&assignment, "id = ?", assUUID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy bài tập"})
		return
	}

	// Kiểm tra attempts
	var attemptsUsed int64
	db.Model(&models.AssignmentSubmission{}).
		Where("assignment_id = ? AND user_id = ?", assignment.ID, userUUID).
		Count(&attemptsUsed)

	if int(attemptsUsed) >= assignment.MaxAttempts {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bạn đã hết lượt làm bài"})
		return
	}

	// Chấm điểm
	var totalScore float64
	var maxScore float64
	var answers []models.AssignmentAnswer

	for _, q := range assignment.Questions {
		maxScore += q.Points

		var userAnswer *uuid.UUID
		for _, ans := range req.Answers {
			if ans.QuestionID == q.ID {
				userAnswer = ans.SelectedID
				break
			}
		}

		var correctID uuid.UUID
		for _, opt := range q.Options {
			if opt.IsCorrect {
				correctID = opt.ID
				break
			}
		}

		isCorrect := false
		pointsEarned := 0.0
		selectedID := uuid.Nil

		if userAnswer != nil {
			selectedID = *userAnswer
			if selectedID == correctID {
				isCorrect = true
				pointsEarned = q.Points
				totalScore += q.Points
			}
		}

		answers = append(answers, models.AssignmentAnswer{
			QuestionID:   q.ID,
			SelectedID:   selectedID,
			IsCorrect:    isCorrect,
			PointsEarned: pointsEarned,
		})
	}

	// Tính điểm thang 10
	scorePercent := (totalScore / maxScore) * 10
	isPassed := scorePercent >= assignment.PassScore

	// Tạo submission
	submission := models.AssignmentSubmission{
		AssignmentID: assignment.ID,
		UserID:       userUUID,
		AttemptNum:   int(attemptsUsed) + 1,
		Score:        scorePercent,
		MaxScore:     10.0,
		IsPassed:     isPassed,
		TimeSpent:    req.TimeSpent,
		StartedAt:    time.Now().Add(-time.Duration(req.TimeSpent) * time.Second),
	}

	if err := db.Create(&submission).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lưu bài làm"})
		return
	}

	// Lưu answers
	for i := range answers {
		answers[i].SubmissionID = submission.ID
		db.Create(&answers[i])
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Nộp bài thành công",
		"submission_id": submission.ID,
		"score":         scorePercent,
		"max_score":     10.0,
		"is_passed":     isPassed,
		"total_points":  totalScore,
		"max_points":    maxScore,
	})
}

// Xem kết quả bài làm
func GetSubmissionDetail(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	submissionID := c.Param("submission_id")
	userIDStr := c.GetString("user_id")

	userUUID, _ := uuid.Parse(userIDStr)
	subUUID, _ := uuid.Parse(submissionID)

	var submission models.AssignmentSubmission
	if err := db.Preload("Assignment").
		Preload("Answers.Question.Options").
		Preload("Answers.SelectedOption").
		First(&submission, "id = ?", subUUID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy bài làm"})
		return
	}

	if submission.UserID != userUUID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bạn không có quyền xem bài làm này"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"submission": submission,
	})
}

// Lấy lịch sử làm bài của user
func GetUserSubmissions(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	assignmentID := c.Param("id")
	userIDStr := c.GetString("user_id")

	userUUID, _ := uuid.Parse(userIDStr)
	assUUID, _ := uuid.Parse(assignmentID)

	var submissions []models.AssignmentSubmission
	if err := db.Where("assignment_id = ? AND user_id = ?", assUUID, userUUID).
		Preload("Assignment").
		Order("submitted_at DESC").
		Find(&submissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy lịch sử làm bài"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"submissions": submissions,
		"total":       len(submissions),
	})
}

func parseQuestionFile(file *multipart.FileHeader) ([]QuestionDTO, error) {
	f, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var questions []QuestionDTO

	// Skip header
	for i := 1; i < len(records); i++ {
		row := records[i]
		if len(row) < 8 {
			continue
		}

		points, _ := strconv.ParseFloat(row[6], 64)

		question := QuestionDTO{
			Question:    row[0],
			Explanation: row[7],
			Points:      points,
			Options: []OptionDTO{
				{Text: row[1], IsCorrect: strings.ToUpper(row[5]) == "A"},
				{Text: row[2], IsCorrect: strings.ToUpper(row[5]) == "B"},
				{Text: row[3], IsCorrect: strings.ToUpper(row[5]) == "C"},
				{Text: row[4], IsCorrect: strings.ToUpper(row[5]) == "D"},
			},
		}

		questions = append(questions, question)
	}

	return questions, nil
}
