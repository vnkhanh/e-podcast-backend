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

// Tạo assignment từ Gemini (dựa trên ExtractedText) với Difficulty + AllowReview
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
		PodcastID     string     `json:"podcast_id" binding:"required"`
		Title         string     `json:"title" binding:"required"`
		Description   string     `json:"description"`
		DueDate       *time.Time `json:"due_date"`
		MaxAttempts   int        `json:"max_attempts"`
		TimeLimit     int        `json:"time_limit"`
		PassScore     float64    `json:"pass_score"`
		NumQuestions  int        `json:"num_questions"`
		HasPassword   bool       `json:"has_password"`
		Password      string     `json:"password"`
		AllowReview   bool       `json:"allow_review"`
		DifficultyRat struct {
			Easy   int `json:"easy"`   // % câu dễ
			Medium int `json:"medium"` // % câu trung bình
			Hard   int `json:"hard"`   // % câu khó
		} `json:"difficulty_ratio"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set default values
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
		AllowReview: req.AllowReview,
	}

	if err := db.Create(&assignment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo bài tập"})
		return
	}

	// Chia văn bản
	chunks := SplitTextIntoChunksSmart(doc.ExtractedText, 2000)

	allQuestions := []models.AssignmentQuestion{}
	questionsPerChunk := (req.NumQuestions + len(chunks) - 1) / len(chunks)

	// Tính số câu theo độ khó
	numEasy := req.NumQuestions * req.DifficultyRat.Easy / 100
	numMedium := req.NumQuestions * req.DifficultyRat.Medium / 100
	numHard := req.NumQuestions - numEasy - numMedium

	difficultyQueue := []string{}
	for i := 0; i < numEasy; i++ {
		difficultyQueue = append(difficultyQueue, "easy")
	}
	for i := 0; i < numMedium; i++ {
		difficultyQueue = append(difficultyQueue, "medium")
	}
	for i := 0; i < numHard; i++ {
		difficultyQueue = append(difficultyQueue, "hard")
	}

	// TÍNH ĐIỂM CHO MỖI CÂU HỎI - CHIA ĐỀU 10 ĐIỂM
	pointsPerQuestion := 10.0 / float64(req.NumQuestions)

	// Sinh câu hỏi
	qCount := 0
	for idx, chunk := range chunks {
		if qCount >= req.NumQuestions {
			break
		}

		questionsForThisChunk := questionsPerChunk
		if qCount+questionsForThisChunk > req.NumQuestions {
			questionsForThisChunk = req.NumQuestions - qCount
		}

		for i := 0; i < questionsForThisChunk; i++ {
			difficulty := difficultyQueue[qCount]

			prompt := fmt.Sprintf(`
Bạn là AI tạo câu hỏi trắc nghiệm đánh giá.
Tạo 1 câu hỏi trắc nghiệm %s từ đoạn podcast sau bằng tiếng Việt.

Yêu cầu:
- 4 lựa chọn (A, B, C, D)
- 1 đáp án đúng
- Câu hỏi đánh giá sự hiểu biết, không chỉ thuộc lòng
- Thêm "explanation" giải thích đáp án
- Thêm "points": %.2f (mỗi câu có cùng điểm số)
- Có thể theo giáo trình hoặc theo podcast, không copy đoạn văn
- Ghi rõ trường "is_correct": true cho lựa chọn đúng, false cho các lựa chọn sai.

Trả về JSON:
{
    "question": "Câu hỏi?",
    "explanation": "Giải thích đáp án",
    "points": %.2f,
    "options": [
        {"text": "A", "is_correct": false/true},
        {"text": "B", "is_correct": false/true},
        {"text": "C", "is_correct": false/true},
        {"text": "D", "is_correct": false/true}
    ]
}

Đoạn văn số %d:
%s
`, difficulty, pointsPerQuestion, pointsPerQuestion, idx+1, chunk)

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

			var qa QA
			if err := json.Unmarshal([]byte(clean), &qa); err != nil {
				fmt.Printf("Parse JSON lỗi ở đoạn %d: %v\n", idx+1, err)
				continue
			}

			// GHI ĐÈ POINTS ĐỂ ĐẢM BẢO MỖI CÂU CÓ CÙNG ĐIỂM
			qa.Points = pointsPerQuestion

			q := models.AssignmentQuestion{
				AssignmentID: assignment.ID,
				Question:     qa.Question,
				Explanation:  qa.Explanation,
				Points:       qa.Points, // Sử dụng điểm đã tính (chia đều)
				Difficulty:   difficulty,
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
			qCount++
			if qCount >= req.NumQuestions {
				break
			}
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

	// NHẬN TẤT CẢ QUERY PARAMS
	search := c.Query("search")
	subjectID := c.Query("subject_id")
	chapterID := c.Query("chapter_id")
	podcastSearch := c.Query("podcast_search")
	status := c.Query("status") // "published" | "draft"

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

	// JOIN TABLES ĐỂ FILTER
	query = query.
		Joins("LEFT JOIN podcasts ON podcasts.id = assignments.podcast_id").
		Joins("LEFT JOIN chapters ON chapters.id = podcasts.chapter_id").
		Joins("LEFT JOIN subjects ON subjects.id = chapters.subject_id")

	// -------------------------
	// FILTER THEO MÔN HỌC
	// -------------------------
	if subjectID != "" {
		query = query.Where("subjects.id = ?", subjectID)
	}

	// -------------------------
	// FILTER THEO CHƯƠNG
	// -------------------------
	if chapterID != "" {
		query = query.Where("chapters.id = ?", chapterID)
	}

	// -------------------------
	// FILTER THEO STATUS
	// -------------------------
	switch status {
	case "published":
		query = query.Where("assignments.is_published = ?", true)
	case "draft":
		query = query.Where("assignments.is_published = ?", false)
	}

	// -------------------------
	// SEARCH TỔNG HỢP (tên bài tập)
	// -------------------------
	if search != "" {
		like := "%" + search + "%"
		query = query.Where("assignments.title ILIKE ?", like)
	}

	// -------------------------
	// SEARCH THEO TÊN PODCAST
	// -------------------------
	if podcastSearch != "" {
		like := "%" + podcastSearch + "%"
		query = query.Where("podcasts.title ILIKE ?", like)
	}

	var assignments []models.Assignment

	// DISTINCT để tránh duplicate do JOIN
	if err := query.
		Distinct("assignments.*").
		Order("assignments.created_at DESC").
		Find(&assignments).Error; err != nil {
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
		AllowReview bool       `json:"allow_review"` // Cho phép sinh viên xem đáp án
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
	assignment.AllowReview = req.AllowReview

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

// Lấy danh sách câu hỏi của một assignment (dành cho giảng viên)
func GetAssignmentQuestionsForTeacher(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	role := c.GetString("role")
	userIDStr := c.GetString("user_id")

	assignmentIDStr := c.Param("id")
	assignmentUUID, err := uuid.Parse(assignmentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	// Build query kiểm tra assignment có thuộc quyền giảng viên không
	query := db.Model(&models.Assignment{})

	if role == string(models.RoleLecturer) {
		userUUID, _ := uuid.Parse(userIDStr)
		query = query.Where("assignments.created_by = ?", userUUID)
	}

	// Check assignment tồn tại & có quyền xem
	var assignment models.Assignment
	if err := query.Where("assignments.id = ?", assignmentUUID).First(&assignment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusForbidden, gin.H{"error": "Bạn không có quyền xem bài tập này"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi server"})
		}
		return
	}

	// Load danh sách câu hỏi + options
	var questions []models.AssignmentQuestion
	if err := db.
		Preload("Options", func(db *gorm.DB) *gorm.DB {
			return db.Order("assignment_options.sort_order ASC")
		}).
		Where("assignment_id = ?", assignmentUUID).
		Order("sort_order ASC").
		Find(&questions).Error; err != nil {

		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách câu hỏi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"assignment_id": assignmentUUID,
		"title":         assignment.Title,
		"questions":     questions,
	})
}

// Thêm câu hỏi
func CreateAssignmentQuestion(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	role := c.GetString("role")
	userID := c.GetString("user_id")

	assignmentIDStr := c.Param("id")
	assignmentUUID, err := uuid.Parse(assignmentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	// Check quyền
	query := db.Model(&models.Assignment{})
	if role == string(models.RoleLecturer) {
		userUUID, _ := uuid.Parse(userID)
		query = query.Where("assignments.created_by = ?", userUUID)
	}

	var assignment models.Assignment
	if err := query.Where("id = ?", assignmentUUID).First(&assignment).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Không có quyền thêm câu hỏi vào bài tập này"})
		return
	}

	// Parse body
	var body struct {
		Question    string                    `json:"question"`
		Difficulty  string                    `json:"difficulty"`
		Explanation string                    `json:"explanation"`
		Points      float64                   `json:"points"`
		SortOrder   int                       `json:"sort_order"`
		Options     []models.AssignmentOption `json:"options"`
	}

	if err := c.BindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Tạo câu hỏi
	q := models.AssignmentQuestion{
		ID:           uuid.New(),
		AssignmentID: assignmentUUID,
		Question:     body.Question,
		Difficulty:   body.Difficulty,
		Explanation:  body.Explanation,
		Points:       body.Points,
		SortOrder:    body.SortOrder,
	}

	if err := db.Create(&q).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo câu hỏi"})
		return
	}

	// Thêm đáp án (options)
	for i := range body.Options {
		body.Options[i].ID = uuid.New()
		body.Options[i].QuestionID = q.ID
	}

	if len(body.Options) > 0 {
		if err := db.Create(&body.Options).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo đáp án"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Tạo câu hỏi thành công",
		"question": q,
		"options":  body.Options,
	})
}

// Sửa câu hỏi
func UpdateAssignmentQuestion(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	role := c.GetString("role")
	userID := c.GetString("user_id")

	qID := c.Param("questionId")
	qUUID, err := uuid.Parse(qID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	// Load câu hỏi + assignment
	var question models.AssignmentQuestion
	if err := db.Preload("Assignment").Preload("Options").
		Where("id = ?", qUUID).First(&question).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy câu hỏi"})
		return
	}

	// Kiểm tra quyền
	if role == string(models.RoleLecturer) && question.Assignment.CreatedBy.String() != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Không có quyền chỉnh sửa câu hỏi này"})
		return
	}

	// Parse body
	var body struct {
		Question    string                    `json:"question"`
		Difficulty  string                    `json:"difficulty"`
		Explanation string                    `json:"explanation"`
		Points      float64                   `json:"points"`
		SortOrder   int                       `json:"sort_order"`
		Options     []models.AssignmentOption `json:"options"`
	}
	if err := c.BindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Update câu hỏi
	question.Question = body.Question
	question.Difficulty = body.Difficulty
	question.Explanation = body.Explanation
	question.Points = body.Points
	question.SortOrder = body.SortOrder
	if err := db.Save(&question).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể cập nhật câu hỏi"})
		return
	}

	// Map option hiện tại
	existingOpts := map[string]models.AssignmentOption{}
	for _, o := range question.Options {
		existingOpts[o.ID.String()] = o
	}

	// Danh sách ID option frontend giữ lại
	currentIDs := map[string]bool{}

	// Xử lý từng option trong payload
	for _, opt := range body.Options {
		if opt.ID != uuid.Nil {
			// --- CASE 1: Update Option ---
			currentIDs[opt.ID.String()] = true

			db.Model(&models.AssignmentOption{}).
				Where("id = ?", opt.ID).
				Updates(map[string]interface{}{
					"option_text": opt.OptionText,
					"is_correct":  opt.IsCorrect,
					"sort_order":  opt.SortOrder,
				})
		} else {
			// --- CASE 2: Add new option ---
			opt.ID = uuid.New()
			opt.QuestionID = question.ID
			db.Create(&opt)
			currentIDs[opt.ID.String()] = true
		}
	}

	// --- CASE 3: Delete removed options ---
	for id, oldOpt := range existingOpts {
		if !currentIDs[id] {
			db.Delete(&oldOpt)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Cập nhật thành công",
	})
}

// Xóa câu hỏi
func DeleteAssignmentQuestion(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	role := c.GetString("role")
	userID := c.GetString("user_id")

	qID := c.Param("questionId")
	qUUID, err := uuid.Parse(qID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	// Load câu hỏi + assignment để kiểm tra quyền
	var question models.AssignmentQuestion
	if err := db.Preload("Assignment").Where("id = ?", qUUID).First(&question).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy câu hỏi"})
		return
	}

	if role == string(models.RoleLecturer) {
		if question.Assignment.CreatedBy.String() != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "Không có quyền xóa câu hỏi này"})
			return
		}
	}

	// Xóa
	if err := db.Delete(&question).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xóa câu hỏi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Xóa câu hỏi thành công",
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
