package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	chunks := SplitTextIntoChunksSmart(text, 900)
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

	// XÓA QUIZ CŨ TRƯỚC KHI TẠO MỚI
	if err := db.
		Where("created_by = ? AND podcast_id = ?", userUUID, podcastID).
		Delete(&models.QuizQuestion{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xóa quiz cũ"})
		return
	}

	allQuestions := []models.QuizQuestion{}
	const maxQuestions = 30 // giới hạn tối đa 30 câu

	for idx, chunk := range chunks {
		if len(allQuestions) >= maxQuestions {
			break
		}

		prompt := fmt.Sprintf(`
Bạn là AI tạo câu hỏi trắc nghiệm giáo dục.
Hãy tạo **1 đến 3 câu hỏi trắc nghiệm** từ đoạn văn sau bằng tiếng Việt.

Mỗi câu hỏi có dạng JSON như sau:
[
  {
    "question": "Câu hỏi là gì?",
    "difficulty": "easy|medium|hard",
    "options": [
      {"text": "Phương án A", "is_correct": false},
      {"text": "Phương án B", "is_correct": true},
      {"text": "Phương án C", "is_correct": false},
      {"text": "Phương án D", "is_correct": false}
    ]
  }
]

Đoạn văn số %d:
%s
`, idx+1, chunk)

		var rawResp string
		for try := 0; try < 3; try++ {
			rawResp, err = services.GeminiGenerateText(prompt)
			if err == nil {
				break
			}
			time.Sleep(1 * time.Second)
		}
		if err != nil {
			fmt.Printf("Gemini lỗi ở đoạn %d: %v\n", idx+1, err)
			continue
		}

		// Làm sạch JSON
		clean := strings.TrimSpace(rawResp)
		clean = strings.TrimPrefix(clean, "```json")
		clean = strings.TrimSuffix(clean, "```")
		clean = strings.TrimSpace(clean)

		type Option struct {
			Text      string `json:"text"`
			IsCorrect bool   `json:"is_correct"`
		}
		type QA struct {
			Question   string   `json:"question"`
			Difficulty string   `json:"difficulty"`
			Options    []Option `json:"options"`
		}

		var arr []QA
		if err := json.Unmarshal([]byte(clean), &arr); err != nil {
			fmt.Printf("Parse JSON lỗi ở đoạn %d: %v\n", idx+1, err)
			continue
		}

		for _, qa := range arr {
			if qa.Question == "" || len(qa.Options) == 0 {
				continue
			}
			if len(allQuestions) >= maxQuestions {
				break
			}

			q := models.QuizQuestion{
				PodcastID:  podcastID,
				CreatedBy:  userUUID,
				Question:   qa.Question,
				Difficulty: qa.Difficulty,
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
		"message":    fmt.Sprintf("Tạo quiz thành công (%d câu hỏi, quiz cũ đã được làm mới)", len(allQuestions)),
		"total":      len(allQuestions),
		"chunks":     len(chunks),
		"quizzes":    allQuestions,
		"podcast_id": podcastID,
	})
}

func GetQuizQuestions(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	podcastIDStr := c.Param("id")

	podcastUUID, err := uuid.Parse(podcastIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "podcastId không hợp lệ"})
		return
	}

	var questions []models.QuizQuestion
	err = db.Preload("Options").
		Preload("CreatedByUser", func(tx *gorm.DB) *gorm.DB {
			return tx.Select("id", "full_name", "email") // chỉ load thông tin cơ bản
		}).
		Where("podcast_id = ?", podcastUUID).
		Order("created_at ASC").
		Find(&questions).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể truy vấn quiz"})
		return
	}

	if len(questions) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "Chưa có câu hỏi nào cho podcast này"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total":     len(questions),
		"podcastID": podcastUUID,
		"questions": questions,
	})
}

type AnswerInput struct {
	QuestionID       uuid.UUID `json:"question_id"`
	SelectedOptionID uuid.UUID `json:"option_id"`
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

	podcastIDStr := c.Param("id")
	podcastUUID, err := uuid.Parse(podcastIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "podcast_id không hợp lệ"})
		return
	}

	var body struct {
		Answers []AnswerInput `json:"answers"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dữ liệu gửi lên không hợp lệ"})
		return
	}

	if len(body.Answers) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Không có câu trả lời nào"})
		return
	}

	// ✅ Lấy danh sách questionIDs từ body
	var questionIDs []uuid.UUID
	for _, ans := range body.Answers {
		questionIDs = append(questionIDs, ans.QuestionID)
	}

	// ✅ Lấy đáp án đúng từ DB
	var correctOptions []models.QuizOption
	if err := db.
		Where("question_id IN ?", questionIDs).
		Where("is_correct = ?", true).
		Find(&correctOptions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy đáp án đúng"})
		return
	}

	// ✅ Tạo map tra nhanh đáp án đúng
	correctMap := make(map[uuid.UUID]uuid.UUID)
	for _, opt := range correctOptions {
		correctMap[opt.QuestionID] = opt.ID
	}

	// ✅ So sánh
	total := len(body.Answers)
	correctCount := 0

	for _, ans := range body.Answers {
		selected := ans.SelectedOptionID // 👈 dùng đúng field
		correct := correctMap[ans.QuestionID]
		if selected == correct {
			correctCount++
		}

		// log debug
		fmt.Printf("Câu hỏi %v | Chọn: %v | Đúng: %v\n", ans.QuestionID, selected, correct)
	}

	// ✅ Tính điểm (trên 10)
	score := 0.0
	if total > 0 {
		score = (float64(correctCount) / float64(total)) * 10.0
	}

	// ✅ Lưu kết quả
	attempt := models.QuizAttempt{
		UserID:    userUUID,
		PodcastID: podcastUUID,
		Score:     score,
		TakenAt:   time.Now(),
	}
	if err := db.Create(&attempt).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lưu kết quả quiz"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Nộp quiz thành công",
		"total":         total,
		"correct_count": correctCount,
		"score":         score,
		"attempt_id":    attempt.ID,
	})
}

// 🔹 Lấy lịch sử làm quiz của user
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

// 🔹 Xem chi tiết 1 lần làm quiz
func GetQuizAttemptDetail(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	attemptIDStr := c.Param("attempt_id")

	attemptUUID, err := uuid.Parse(attemptIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "attempt_id không hợp lệ"})
		return
	}

	var attempt models.QuizAttempt
	err = db.
		Preload("Podcast", func(tx *gorm.DB) *gorm.DB {
			return tx.Select("id", "title", "description", "thumbnail_url")
		}).
		Preload("User", func(tx *gorm.DB) *gorm.DB {
			return tx.Select("id", "name", "email")
		}).
		First(&attempt, "id = ?", attemptUUID).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy quiz attempt"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy chi tiết quiz"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"attempt": attempt,
	})
}
