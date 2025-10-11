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

// ======== HÀM CHIA NHỎ VĂN BẢN ========
// chia văn bản dài thành các đoạn ~3000 ký tự (để tránh vượt token limit)
func SplitTextIntoChunks(text string, maxLen int) []string {
	var chunks []string
	runes := []rune(text)
	for i := 0; i < len(runes); i += maxLen {
		end := i + maxLen
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}

// ======== API TẠO FLASHCARDS ========

func GenerateFlashcardsFromDocument(c *gin.Context) {
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
	chunks := SplitTextIntoChunks(text, 3000)

	if len(chunks) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Không có nội dung để xử lý"})
		return
	}

	// Xác định PodcastID (nếu có)
	var podcastID uuid.UUID
	if len(doc.Podcasts) > 0 {
		podcastID = doc.Podcasts[0].ID
	} else {
		podcastID = uuid.Nil
	}

	allFlashcards := []models.Flashcard{}

	for idx, chunk := range chunks {
		prompt := fmt.Sprintf(`
Bạn là AI hỗ trợ học tập. 
Từ đoạn văn sau, hãy tạo ra 5 flashcard bằng tiếng Việt.
Mỗi flashcard gồm:
- "front": câu hỏi, định nghĩa hoặc khái niệm
- "back": câu trả lời hoặc giải thích ngắn gọn
Trả kết quả đúng **định dạng JSON** như ví dụ:
[
  {"front": "Câu hỏi 1?", "back": "Trả lời 1"},
  {"front": "Câu hỏi 2?", "back": "Trả lời 2"}
]

Đây là đoạn văn số %d:
%s
`, idx+1, chunk)

		var rawResp string
		var try int
		for try = 0; try < 3; try++ { // thử lại tối đa 3 lần
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

		// Làm sạch output (loại bỏ markdown)
		clean := strings.TrimSpace(rawResp)
		clean = strings.TrimPrefix(clean, "```json")
		clean = strings.TrimSuffix(clean, "```")
		clean = strings.TrimSpace(clean)

		// Parse JSON
		type QA struct {
			Front string `json:"front"`
			Back  string `json:"back"`
		}
		var arr []QA
		if err := json.Unmarshal([]byte(clean), &arr); err != nil {
			fmt.Printf("Parse JSON lỗi ở đoạn %d: %v\n", idx+1, err)
			continue
		}

		for _, qa := range arr {
			if qa.Front == "" || qa.Back == "" {
				continue
			}
			fc := models.Flashcard{
				UserID:    userUUID,
				PodcastID: podcastID,
				FrontText: qa.Front,
				BackText:  qa.Back,
			}
			if err := db.Create(&fc).Error; err == nil {
				allFlashcards = append(allFlashcards, fc)
			}
		}
	}

	if len(allFlashcards) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Không tạo được flashcard nào",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Tạo flashcards thành công từ Gemini (nhiều đoạn)",
		"total":      len(allFlashcards),
		"chunks":     len(chunks),
		"flashcards": allFlashcards,
	})
}

// GET /api/user/podcasts/:id/flashcards
func GetFlashcardsByPodcast(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userIDStr := c.GetString("user_id")
	podcastIDStr := c.Param("id")

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

	var flashcards []models.Flashcard
	if err := db.
		Where("user_id = ? AND podcast_id = ?", userUUID, podcastUUID).
		Order("created_at ASC").
		Find(&flashcards).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy flashcards"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  flashcards,
		"count": len(flashcards),
	})
}
