package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// Hàm gọn để xử lý prompt và trả kết quả từ Gemini
func GeminiGenerateText(prompt string) (string, error) {
	ctx := context.Background()

	client, err := genai.NewClient(ctx, option.WithAPIKey(os.Getenv("GEMINI_API_KEY")))
	if err != nil {
		return "", fmt.Errorf("không thể tạo Gemini client: %v", err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.0-flash")
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("lỗi Gemini xử lý: %v", err)
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini không trả kết quả hợp lệ")
	}
	return fmt.Sprintf("%v", resp.Candidates[0].Content.Parts[0]), nil
}

// ParseFlashcardFromText tách front/back từ text dạng Markdown
func ParseFlashcardFromText(text string) (string, string, error) {
	// Tìm **Front:** và **Back:**
	re := regexp.MustCompile(`\*\*Front:\*\*(.*?)\*\*Back:\*\*(.*)`)
	matches := re.FindStringSubmatch(text)
	if len(matches) != 3 {
		// fallback: tách theo Front: ... Back: ...
		frontIdx := strings.Index(text, "**Front:**")
		backIdx := strings.Index(text, "**Back:**")
		if frontIdx == -1 || backIdx == -1 || backIdx <= frontIdx {
			return "", "", errors.New("không tìm thấy front/back trong text")
		}
		front := strings.TrimSpace(text[frontIdx+len("**Front:**") : backIdx])
		back := strings.TrimSpace(text[backIdx+len("**Back:**"):])
		return front, back, nil
	}
	return strings.TrimSpace(matches[1]), strings.TrimSpace(matches[2]), nil
}
