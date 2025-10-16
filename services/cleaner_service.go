package services

import (
	"regexp"
	"strings"
)

// PreCleanText xử lý thô: loại mục lục, số trang, code, khoảng trắng
func PreCleanText(text string) string {
	cleaned := text

	// Xoá các dòng chứa "Mục lục" hoặc "Table of Contents"
	reTOC := regexp.MustCompile(`(?i)^(.*mục lục.*|.*table of contents.*)$`)
	cleaned = reTOC.ReplaceAllString(cleaned, "")

	// Xoá các dòng chứa "Trang X" hoặc "Page X"
	rePageNumber := regexp.MustCompile(`(?i)^.*(trang|page)[^\d]*\d+.*$`)
	cleaned = rePageNumber.ReplaceAllString(cleaned, "")

	// Xoá dòng chỉ có số, ký tự đặc biệt hoặc khoảng trắng
	reSpecialLines := regexp.MustCompile(`^[\s\W\d]*$`)
	cleaned = reSpecialLines.ReplaceAllString(cleaned, "")

	// Xoá dòng có chứa code hoặc từ khoá lập trình
	reCode := regexp.MustCompile(`(?i)^.*(const |function |class |<[^>]+>).*?$`)
	cleaned = reCode.ReplaceAllString(cleaned, "")

	// Xoá nhiều dòng trống liên tiếp
	reMultiNewLine := regexp.MustCompile(`\n{2,}`)
	cleaned = reMultiNewLine.ReplaceAllString(cleaned, "\n")

	return strings.TrimSpace(cleaned)
}

// CleanWithGemini sử dụng Gemini để làm sạch sâu, chuẩn hoá văn bản
func CleanWithGemini(text string) (string, error) {
	prompt := `Bạn là công cụ xử lý văn bản trích xuất từ tài liệu.
	Hãy xử lý văn bản sau với yêu cầu:
	- Xoá phần mục lục, các dòng chứa số trang, tiêu đề lặp lại
	- Xoá code, ví dụ mã lệnh, hoặc các ký hiệu kỹ thuật
	- Làm gọn văn bản: không có dòng trống thừa, không có ký tự lạ
	- Ngắt đoạn hợp lý, dễ đọc, phù hợp để chuyển thành nội dung podcast
	- Giữ nguyên nội dung, không thêm bớt, không giải thích
	- Không in đậm, in nghiêng, không sử dụng markdown, chỉ trả về văn bản thuần tuý
	Văn bản cần làm sạch:`

	fullPrompt := prompt + "\n\n" + text

	return GeminiGenerateText(fullPrompt)
}

func SummarizeText(text string) (string, error) {
	prompt := `Tôi có một đoạn văn bản, bạn hãy giúp tôi viết lại nội dung một cách rõ ràng và gọn hơn, dễ nghe khi được chuyển thành giọng nói (audio).
	Yêu cầu:
	1. Không lược bỏ nội dung chính, không tự ý thêm thông tin không có trong văn bản, đảm bảo đủ nội dung quan trọng
	2. Ngôn ngữ tự nhiên, gần gũi, không quá khô khan
	3. Có thể thêm câu chuyển đoạn ngắn để mạch lạc hơn
	4. Không sử dụng từ ngữ chuyên môn quá khó hiểu
	5. Giọng văn trung tính, nhẹ nhàng, phù hợp để đọc lên
	6. KHÔNG sử dụng markdown, KHÔNG in đậm, KHÔNG in nghiêng, chỉ trả về văn bản thuần tuý, KHÔNG thêm ký tự đặc biệt, KHÔNG GẠCH ĐẦU DÒNG GÌ HẾT
	7. Không bình luận, không giải thích, chỉ trả về nội dung tóm tắt phù hợp để chuyển thành audio podcast
	8. Có thể bắt đầu bằng câu "Ở podcast này chúng ta sẽ cùng tìm hiểu về..." để rõ ràng hơn
	Đoạn văn bản cần viết lại:`

	fullPrompt := prompt + "\n\n" + text

	return GeminiGenerateText(fullPrompt)
}

// CleanTextPipeline là pipeline chính: Regex + Gemini
func CleanTextPipeline(rawText string) (string, error) {
	preCleaned := PreCleanText(rawText)
	finalCleaned, err := CleanWithGemini(preCleaned)
	if err != nil {
		return "", err
	}
	summary, err := SummarizeText(finalCleaned)
	if err != nil {
		return "", err
	}
	return summary, nil
}
