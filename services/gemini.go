package services

import (
	"context"
	"fmt"
	"os"

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
