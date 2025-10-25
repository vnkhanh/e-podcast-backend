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

func ExctractText(text string) (string, error) {
	prompt := `Bạn là một Biên tập viên/Người đọc Audio Book chuyên nghiệp, có khả năng chuyển đổi văn bản phức tạp thành lời nói trôi chảy.
	Chuyển đổi toàn bộ nội dung văn bản đã trích xuất dưới đây thành một kịch bản đọc liền mạch (solo narration), sẵn sàng cho việc chuyển thành audio.
	Yêu cầu:
	1. BỎ QUA TẤT CẢ các phần phụ trợ (như Lời giới thiệu, Mục lục, các thông tin chủ biên,...). Chỉ tập trung vào nội dung của các CHƯƠNG CHÍNH.
	2. Không lược bỏ nội dung chính, không tự ý thêm thông tin không có trong văn bản, đảm bảo đủ nội dung quan trọng.
	3. Ngôn ngữ tự nhiên, gần gũi, không quá khô khan.
	4. Nếu gặp từ ngữ chuyên môn quá khó hiểu, hãy diễn giải nó một cách đơn giản mà vẫn giữ được ý nghĩa học thuật. NẾU GẶP TỪ VIẾT TẮT, HÃY VIẾT RÕ RA VÀ KHÔNG ĐƯỢC VIẾT TẮT. VIẾT ĐÚNG CHÍNH TẢ
	5. Giọng văn trung tính, rõ ràng, có nhịp điệu (paced), và mang tính giáo dục.
	6. Bắt đầu kịch bản bằng câu: "Ở podcast này chúng ta sẽ cùng tìm hiểu về..." (thay vì đọc tiêu đề chương 1).
	7. KHÔNG sử dụng markdown, KHÔNG in đậm, KHÔNG in nghiêng, chỉ trả về văn bản thuần tuý, KHÔNG thêm ký tự đặc biệt, KHÔNG GẠCH ĐẦU DÒNG.
	8. Không bình luận, không giải thích ngoài lề, chỉ trả về nội dung kịch bản audio.
	Đoạn văn bản cần viết lại:`

	fullPrompt := prompt + "\n\n" + text

	return GeminiGenerateText(fullPrompt)
}

func SummaryText(text string) (string, error) {
	prompt := `Bạn là công cụ tóm tắt văn bản, hãy giúp tôi tóm tắt nội dung thành một đoạn văn một cách rõ ràng và ngắn gọn
	Yêu cầu:
	1. Không lược bỏ nội dung chính, không tự ý thêm thông tin không có trong văn bản, đảm bảo đủ nội dung quan trọng
	2. Ngôn ngữ tự nhiên, gần gũi, không quá khô khan
	3. Có thể thêm câu chuyển đoạn ngắn để mạch lạc hơn
	4. Không sử dụng từ ngữ chuyên môn quá khó hiểu
	5. KHÔNG sử dụng markdown, KHÔNG in đậm, KHÔNG in nghiêng, chỉ trả về văn bản thuần tuý, KHÔNG thêm ký tự đặc biệt
	6. Không bình luận, không giải thích, chỉ trả về nội dung tóm tắt.
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
	// extract, err := ExctractText(finalCleaned)
	if err != nil {
		return "", err
	}

	return finalCleaned, nil
}
