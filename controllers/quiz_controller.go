package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vnkhanh/e-podcast-backend/models"
	"github.com/vnkhanh/e-podcast-backend/services"
	"gorm.io/gorm"
)

func GenerateQuizzesFromDocument(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userIDStr := c.GetString("user_id")

	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
		return
	}

	documentID := c.Param("id")
	var doc models.Document
	if err := db.Preload("Podcasts").First(&doc, "id = ?", documentID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy document"})
		return
	}

	if doc.ExtractedText == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Document chưa có ExtractedText"})
		return
	}

	text := strings.TrimSpace(doc.ExtractedText)
	chunks := SplitTextIntoChunksSmart(text, 2000)
	if len(chunks) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Không có nội dung để xử lý"})
		return
	}

	// Lấy podcastID đầu tiên nếu có
	var podcastID uuid.UUID
	if len(doc.Podcasts) > 0 {
		podcastID = doc.Podcasts[0].ID
	} else {
		podcastID = uuid.Nil
	}

	// Tạo QuizSet mới
	quizSet := models.QuizSet{
		PodcastID:   podcastID,
		Title:       "Trắc nghiệm ôn tập",
		Description: "Bộ câu hỏi trắc nghiệm sinh tự động từ nội dung tài liệu bằng Gemini",
		CreatedBy:   userUUID,
	}
	if err := db.Create(&quizSet).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo QuizSet mới"})
		return
	}

	allQuestions := []models.QuizQuestion{}
	const maxQuestions = 50

	// Helper retry Gemini
	retryGemini := func(prompt string, retries int) (string, error) {
		var resp string
		var err error
		for i := 0; i < retries; i++ {
			resp, err = services.GeminiGenerateText(prompt)
			if err == nil {
				return resp, nil
			}
			time.Sleep(time.Duration(i+1) * time.Second)
		}
		return "", err
	}

	for idx, chunk := range chunks {
		if len(allQuestions) >= maxQuestions {
			break
		}

		prompt := fmt.Sprintf(`
Bạn là AI tạo câu hỏi trắc nghiệm giáo dục.
Hãy tạo **1 đến 3 câu hỏi trắc nghiệm** từ đoạn podcast sau bằng tiếng Việt.

Yêu cầu:
- Mỗi câu hỏi có 4 lựa chọn (A, B, C, D).
- Ngẫu nhiên vị trí đáp án đúng.
- Ghi rõ trường "is_correct": true cho lựa chọn đúng, false cho các lựa chọn sai.
- Mỗi câu có trường "hint" (1-2 câu gợi ý giúp người học suy luận, không tiết lộ đáp án).

Trả về JSON hợp lệ đúng cấu trúc:
[
  {
    "question": "Câu hỏi là gì?",
    "difficulty": "easy|medium|hard",
    "hint": "Gợi ý liên quan đến nội dung câu hỏi và đáp án.",
    "options": [
      {"text": "Phương án A", "is_correct": true/false},
      {"text": "Phương án B", "is_correct": true/false},
      {"text": "Phương án C", "is_correct": true/false},
      {"text": "Phương án D", "is_correct": true/false}
    ]
  }
]

Chỉ trả về JSON hợp lệ, không thêm bất kỳ văn bản nào khác.

Đoạn văn số %d:
%s
`, idx+1, chunk)

		rawResp, err := retryGemini(prompt, 3)
		if err != nil {
			fmt.Printf("Gemini lỗi ở đoạn %d: %v\n", idx+1, err)
			continue
		}

		// Làm sạch JSON Gemini
		clean := strings.TrimSpace(rawResp)
		clean = strings.Trim(clean, "`")
		clean = strings.TrimPrefix(clean, "json")
		clean = strings.TrimSpace(clean)

		type Option struct {
			Text      string `json:"text"`
			IsCorrect bool   `json:"is_correct"`
		}
		type QA struct {
			Question   string   `json:"question"`
			Difficulty string   `json:"difficulty"`
			Hint       string   `json:"hint"`
			Options    []Option `json:"options"`
		}

		var arr []QA
		if err := json.Unmarshal([]byte(clean), &arr); err != nil {
			fmt.Printf("Parse JSON lỗi ở đoạn %d: %v\n", idx+1, err)
			continue
		}

		for _, qa := range arr {
			if qa.Question == "" || len(qa.Options) < 4 {
				continue
			}
			if len(allQuestions) >= maxQuestions {
				break
			}

			q := models.QuizQuestion{
				QuizSetID:  quizSet.ID,
				Question:   qa.Question,
				SourceText: chunk,
				Difficulty: qa.Difficulty,
				Hint:       qa.Hint,
				CreatedAt:  time.Now(),
			}

			if err := db.Create(&q).Error; err != nil {
				fmt.Printf("Lỗi khi tạo QuizQuestion: %v\n", err)
				continue
			}

			for _, opt := range qa.Options {
				o := models.QuizOption{
					QuestionID: q.ID,
					OptionText: opt.Text,
					IsCorrect:  opt.IsCorrect,
				}
				db.Create(&o)
			}

			allQuestions = append(allQuestions, q)
		}
	}

	if len(allQuestions) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không tạo được quiz nào"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     fmt.Sprintf("Tạo quiz thành công (%d câu hỏi, quiz cũ đã được làm mới)", len(allQuestions)),
		"quiz_set_id": quizSet.ID,
		"total":       len(allQuestions),
		"chunks":      len(chunks),
		"podcast_id":  podcastID,
		"quizzes":     allQuestions,
	})
}

type AnswerInput struct {
	QuestionID       uuid.UUID  `json:"question_id"`
	SelectedOptionID *uuid.UUID `json:"option_id"`
}

// Nộp bài quiz
func SubmitQuizAttempt(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userIDStr := c.GetString("user_id")

	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
		return
	}

	quizSetIDStr := c.Param("id")
	quizSetUUID, err := uuid.Parse(quizSetIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quiz_set_id không hợp lệ"})
		return
	}

	var body struct {
		Answers     []AnswerInput `json:"answers"`
		DurationSec int           `json:"duration_sec"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dữ liệu gửi lên không hợp lệ"})
		return
	}

	if len(body.Answers) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Không có câu trả lời nào"})
		return
	}

	// Lấy quiz set để có podcast_id
	var quizSet models.QuizSet
	if err := db.First(&quizSet, "id = ?", quizSetUUID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy quiz set"})
		return
	}

	// Lấy danh sách questionIDs
	var questionIDs []uuid.UUID
	for _, ans := range body.Answers {
		questionIDs = append(questionIDs, ans.QuestionID)
	}

	// Đảm bảo các question thuộc quiz set này
	var count int64
	if err := db.Model(&models.QuizQuestion{}).
		Where("id IN ?", questionIDs).
		Where("quiz_set_id = ?", quizSetUUID).
		Count(&count).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể kiểm tra câu hỏi"})
		return
	}
	if int(count) != len(questionIDs) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Có câu hỏi không thuộc quiz set này"})
		return
	}

	// Lấy đáp án đúng
	var correctOptions []models.QuizOption
	if err := db.Where("question_id IN ?", questionIDs).
		Where("is_correct = ?", true).
		Find(&correctOptions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy đáp án đúng"})
		return
	}

	correctMap := make(map[uuid.UUID]uuid.UUID)
	for _, o := range correctOptions {
		correctMap[o.QuestionID] = o.ID
	}

	// Tạo attempt
	attempt := models.QuizAttempt{
		UserID:      userUUID,
		PodcastID:   quizSet.PodcastID,
		QuizSetID:   quizSetUUID,
		TakenAt:     time.Now(),
		DurationSec: body.DurationSec,
	}
	if err := db.Create(&attempt).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lưu kết quả quiz"})
		return
	}

	correctCount := 0
	total := len(body.Answers)
	results := make([]models.AnswerResult, 0, total)

	for _, ans := range body.Answers {
		// lấy question và options
		var q models.QuizQuestion
		db.Preload("Options").First(&q, "id = ?", ans.QuestionID)

		correctID := correctMap[q.ID]
		selectedID := uuid.Nil
		isCorrect := false

		if ans.SelectedOptionID != nil {
			selectedID = *ans.SelectedOptionID
			isCorrect = (selectedID == correctID)
		}

		if isCorrect {
			correctCount++
		}

		// lưu history
		history := models.QuizAttemptHistory{
			AttemptID:  attempt.ID,
			QuestionID: q.ID,
			SelectedID: selectedID, // nếu bỏ trống = uuid.Nil
			IsCorrect:  isCorrect,
			AnsweredAt: time.Now(),
		}
		db.Create(&history)

		// build DTO
		opts := make([]models.QuizOptionDTO, 0, len(q.Options))
		for _, o := range q.Options {
			opts = append(opts, models.QuizOptionDTO{
				ID:         o.ID,
				OptionText: o.OptionText,
				IsCorrect:  o.IsCorrect,
			})
		}
		results = append(results, models.AnswerResult{
			QuestionID: q.ID,
			Question:   q.Question,
			SelectedID: selectedID,
			CorrectID:  correctID,
			IsCorrect:  isCorrect,
			SourceText: q.SourceText,
			Options:    opts,
		})
	}

	score := 0.0
	if total > 0 {
		score = (float64(correctCount) / float64(total)) * 10.0
	}

	// cập nhật điểm
	db.Model(&attempt).Updates(models.QuizAttempt{
		Score:          score,
		CorrectCount:   correctCount,
		IncorrectCount: total - correctCount,
	})

	c.JSON(http.StatusOK, gin.H{
		"message":       "Nộp quiz thành công",
		"total":         total,
		"correct_count": correctCount,
		"score":         score,
		"attempt_id":    attempt.ID,
		"quiz_set_id":   quizSetUUID,
		"podcast_id":    quizSet.PodcastID,
		"results":       results,
	})
}

// Lấy lịch sử làm quiz của user
func GetUserQuizAttempts(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userIDStr := c.GetString("user_id")

	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
		return
	}

	var attempts []models.QuizAttempt
	err = db.
		Preload("Podcast", func(tx *gorm.DB) *gorm.DB {
			return tx.Select("id", "title", "description", "thumbnail_url")
		}).
		Preload("QuizSet", func(tx *gorm.DB) *gorm.DB {
			return tx.Select("id", "title", "description", "podcast_id", "created_by")
		}).
		Where("user_id = ?", userUUID).
		Order("taken_at DESC").
		Find(&attempts).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy lịch sử quiz"})
		return
	}

	if len(attempts) == 0 {
		c.JSON(http.StatusOK, gin.H{"message": "Chưa có lịch sử làm quiz nào"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":    len(attempts),
		"attempts": attempts,
	})
}

// Lấy quiz set của podcast
// Lấy danh sách câu hỏi trong 1 quiz set
func GetQuizQuestions(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	quizSetIDStr := c.Param("id")

	quizSetUUID, err := uuid.Parse(quizSetIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quiz_set_id không hợp lệ"})
		return
	}

	var questions []models.QuizQuestion
	err = db.Preload("Options").
		Preload("QuizSet").
		Order("created_at ASC").
		Where("quiz_set_id = ?", quizSetUUID).
		Find(&questions).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể truy vấn câu hỏi"})
		return
	}

	if len(questions) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "Bộ trắc nghiệm này chưa có câu hỏi nào"})
		return
	}

	// Lấy thông tin quiz set
	var quizSet models.QuizSet
	if err := db.Preload("Creator").First(&quizSet, "id = ?", quizSetUUID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy quiz set"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"quiz_set":  quizSet,
		"questions": questions,
		"total":     len(questions),
	})
}

// Lấy quiz set của podcast
// Lấy tất cả quiz sets của 1 podcast
func GetQuizSetsByPodcast(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	podcastIDStr := c.Param("id")

	// Lấy user ID từ middleware (đã decode JWT)
	userIDStr := c.GetString("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Không xác định được người dùng"})
		return
	}

	userUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id không hợp lệ"})
		return
	}

	podcastUUID, err := uuid.Parse(podcastIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "podcast_id không hợp lệ"})
		return
	}

	var quizSets []models.QuizSet
	err = db.
		Preload("Questions.Options").
		Where("podcast_id = ? AND created_by = ?", podcastUUID, userUUID).
		Order("created_at DESC").
		Find(&quizSets).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách quiz sets"})
		return
	}

	if len(quizSets) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"quiz_sets": []models.QuizSet{},
			"message":   "Bạn chưa tạo bộ trắc nghiệm nào cho podcast này",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"quiz_sets": quizSets,
		"total":     len(quizSets),
	})
}

// Lấy lịch sử làm quiz
func GetQuizAttemptsBySet(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	quizSetIDStr := c.Param("id")

	// Lấy user đang đăng nhập
	userIDStr := c.GetString("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Không xác định được người dùng"})
		return
	}
	userUUID, _ := uuid.Parse(userIDStr)

	quizSetUUID, err := uuid.Parse(quizSetIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quiz_set_id không hợp lệ"})
		return
	}

	var attempts []models.QuizAttempt
	err = db.
		Select("*").
		Preload("Podcast", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, title")
		}).
		Where("user_id = ? AND quiz_set_id = ?", userUUID, quizSetUUID).
		Order("taken_at DESC").
		Find(&attempts).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy lịch sử làm quiz"})
		return
	}

	if len(attempts) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"attempts": []models.QuizAttempt{},
			"message":  "Bạn chưa làm bài trắc nghiệm này lần nào",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"quiz_set_id": quizSetUUID,
		"attempts":    attempts,
		"total":       len(attempts),
	})
}

// Lấy chỉ tiết lần làm
func GetQuizAttemptDetail(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	attemptIDStr := c.Param("attemptID")

	userIDStr := c.GetString("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Không xác định được người dùng"})
		return
	}
	userUUID, _ := uuid.Parse(userIDStr)

	attemptUUID, err := uuid.Parse(attemptIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "attempt_id không hợp lệ"})
		return
	}

	var attempt models.QuizAttempt
	err = db.
		Preload("Podcast", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, title")
		}).
		Preload("QuizSet", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, title, description")
		}).
		Preload("Histories", func(db *gorm.DB) *gorm.DB {
			return db.Order("answered_at ASC")
		}).
		Preload("Histories.Question").
		Preload("Histories.Question.Options").
		Preload("Histories.SelectedOption").
		Where("id = ? AND user_id = ?", attemptUUID, userUUID).
		First(&attempt).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy lịch sử làm quiz này"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy chi tiết lịch sử làm quiz"})
		return
	}

	// === Thêm phần xử lý bổ sung câu hỏi bị bỏ trống ===
	var allQuestions []models.QuizQuestion
	if err := db.Preload("Options").
		Where("quiz_set_id = ?", attempt.QuizSetID).
		Find(&allQuestions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách câu hỏi"})
		return
	}

	// Map các câu hỏi đã có trong history
	historyMap := make(map[uuid.UUID]models.QuizAttemptHistory)
	for _, h := range attempt.Histories {
		historyMap[h.QuestionID] = h
	}

	// Thêm những câu hỏi chưa có history
	for _, q := range allQuestions {
		if _, exists := historyMap[q.ID]; !exists {
			blankHistory := models.QuizAttemptHistory{
				ID:         uuid.New(),
				AttemptID:  attempt.ID,
				QuestionID: q.ID,
				Question:   q,
				IsCorrect:  false,
				SelectedID: uuid.Nil,
				AnsweredAt: attempt.TakenAt,
			}
			attempt.Histories = append(attempt.Histories, blankHistory)
		}
	}

	// Sắp xếp lại Histories theo thứ tự câu hỏi
	sort.SliceStable(attempt.Histories, func(i, j int) bool {
		return attempt.Histories[i].Question.CreatedAt.Before(attempt.Histories[j].Question.CreatedAt)
	})

	c.JSON(http.StatusOK, gin.H{"attempt": attempt})
}
