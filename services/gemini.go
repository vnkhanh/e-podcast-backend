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

// CreateFlashcard tạo flashcard từ nội dung podcast
func CreateFlashcard(content string) (string, string, error) {
	prompt := fmt.Sprintf(`
Bạn là chuyên gia giáo dục.
Từ nội dung sau, hãy tạo 1 flashcard:
- Front: câu hỏi ngắn gọn
- Back: câu trả lời chi tiết, dễ hiểu
Nội dung: %s
Trả về định dạng Markdown: **Front:** ... **Back:** ...
`, content)

	respText, err := GeminiGenerateText(prompt)
	if err != nil {
		return "", "", err
	}

	front, back, err := ParseFlashcardFromText(respText)
	if err != nil {
		return "", "", fmt.Errorf("không parse được flashcard từ Gemini: %v\nRaw text: %s", err, respText)
	}

	return front, back, nil
}

// ParseQuizFromText tách câu hỏi/options/correct_index từ text dạng Markdown
func ParseQuizFromText(text string) (string, []string, int, error) {
	// Dùng regex tìm Question: ... Options: [...] Correct: ...
	re := regexp.MustCompile(`(?i)Question:\s*(.*?)\s*Options:\s*\[(.*?)\]\s*Correct:\s*(\d+)`)
	matches := re.FindStringSubmatch(text)
	if len(matches) == 4 {
		q := strings.TrimSpace(matches[1])
		optsRaw := strings.Split(matches[2], ",")
		opts := make([]string, len(optsRaw))
		for i, o := range optsRaw {
			opts[i] = strings.TrimSpace(strings.Trim(o, `"'`))
		}
		var correct int
		fmt.Sscanf(matches[3], "%d", &correct)
		return q, opts, correct, nil
	}

	return "", nil, 0, errors.New("không parse được quiz từ text")
}

// CreateQuiz tạo quiz trắc nghiệm từ nội dung podcast
func CreateQuiz(content string) (string, []string, int, error) {
	prompt := fmt.Sprintf(`
Bạn là chuyên gia giáo dục.
Từ nội dung sau, hãy tạo 1 câu hỏi trắc nghiệm với 4 lựa chọn.
Trả về dạng Markdown: 
Question: ... 
Options: ["...", "...", "...", "..."] 
Correct: 0
Nội dung: %s
`, content)

	respText, err := GeminiGenerateText(prompt)
	if err != nil {
		return "", nil, 0, err
	}

	question, options, correctIndex, err := ParseQuizFromText(respText)
	if err != nil {
		return "", nil, 0, fmt.Errorf("không parse được quiz từ Gemini: %v\nRaw text: %s", err, respText)
	}

	return question, options, correctIndex, nil
}
