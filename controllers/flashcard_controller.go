package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vnkhanh/e-podcast-backend/models"
	"github.com/vnkhanh/e-podcast-backend/services"
	"gorm.io/gorm"
)

// ======== HÀM CHIA NHỎ VĂN BẢN (CHUẨN THEO NGỮ NGHĨA) ========
func SplitTextIntoChunksSmart(text string, maxChunkSize int) []string {
	text = strings.TrimSpace(text)
	text = regexp.MustCompile(`\r?\n+`).ReplaceAllString(text, "\n") // normalize newline

	// Tách theo đoạn (xuống dòng kép)
	paragraphs := regexp.MustCompile(`\n{2,}`).Split(text, -1)

	var chunks []string
	var current strings.Builder

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		// Nếu đoạn quá dài thì tách nhỏ theo câu
		if len([]rune(p)) > maxChunkSize {
			sentences := regexp.MustCompile(`(?<=[.!?。！？])\s+`).Split(p, -1)
			for _, s := range sentences {
				s = strings.TrimSpace(s)
				if s == "" {
					continue
				}

				if len([]rune(current.String()))+len([]rune(s)) < maxChunkSize {
					current.WriteString(s + " ")
				} else {
					chunks = append(chunks, strings.TrimSpace(current.String()))
					current.Reset()
					current.WriteString(s + " ")
				}
			}
		} else {
			// nếu đoạn vừa phải, gom chung với đoạn trước cho đủ 1 chunk
			if len([]rune(current.String()))+len([]rune(p)) < maxChunkSize {
				current.WriteString(p + "\n\n")
			} else {
				chunks = append(chunks, strings.TrimSpace(current.String()))
				current.Reset()
				current.WriteString(p + "\n\n")
			}
		}
	}

	if current.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(current.String()))
	}

	return chunks
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ======== API: TẠO FLASHCARDS ========
// POST /api/user/documents/:id/flashcards
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

	// 🔥 XÓA FLASHCARD CŨ TRƯỚC KHI TẠO MỚI
	if err := db.
		Where("user_id = ? AND podcast_id = ?", userUUID, podcastID).
		Delete(&models.Flashcard{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xóa flashcards cũ"})
		return
	}

	allFlashcards := []models.Flashcard{}
	const maxFlashcards = 50 // 🔥 giới hạn tối đa

	for idx, chunk := range chunks {
		if len(allFlashcards) >= maxFlashcards {
			break // dừng sớm nếu đủ 50 flashcards
		}

		prompt := fmt.Sprintf(`
Bạn là AI hỗ trợ học tập. 
Hãy tạo **một hoặc tối đa 3 flashcards** từ đoạn văn sau bằng tiếng Việt.

Mỗi flashcard gồm:
- "front": câu hỏi, khái niệm hoặc định nghĩa
- "back": câu trả lời ngắn gọn, chính xác

Chỉ trả về JSON, ví dụ:
[
  {"front": "Câu hỏi 1?", "back": "Trả lời 1"},
  {"front": "Câu hỏi 2?", "back": "Trả lời 2"}
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

		// Làm sạch JSON trả về
		clean := strings.TrimSpace(rawResp)
		clean = strings.TrimPrefix(clean, "```json")
		clean = strings.TrimSuffix(clean, "```")
		clean = strings.TrimSpace(clean)

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
			if len(allFlashcards) >= maxFlashcards {
				break
			}

			fc := models.Flashcard{
				UserID:        userUUID,
				PodcastID:     podcastID,
				FrontText:     qa.Front,
				BackText:      qa.Back,
				ChunkIndex:    idx + 1,
				SourceText:    string([]rune(chunk)[:min(len([]rune(chunk)), 100)]),
				ReferenceText: chunk,
			}

			if err := db.Create(&fc).Error; err == nil {
				allFlashcards = append(allFlashcards, fc)
			}
		}
	}

	if len(allFlashcards) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không tạo được flashcard nào"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf(
			"Tạo flashcards thành công (tối đa %d flashcards, flashcards cũ đã được làm mới)",
			maxFlashcards,
		),
		"total":      len(allFlashcards),
		"chunks":     len(chunks),
		"flashcards": allFlashcards,
	})
}

// ======== API: LẤY FLASHCARDS THEO PODCAST ========
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
