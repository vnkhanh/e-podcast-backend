package services

import (
	"log"
	"regexp"
	"strings"
)

// PreCleanText xử lý thô: loại mục lục, số trang, code, khoảng trắng
func PreCleanText(text string) string {
	cleaned := text

	reTOC := regexp.MustCompile(`(?i)^(.*mục lục.*|.*table of contents.*)$`)
	cleaned = reTOC.ReplaceAllString(cleaned, "")

	rePageNumber := regexp.MustCompile(`(?i)^.*(trang|page)[^\d]*\d+.*$`)
	cleaned = rePageNumber.ReplaceAllString(cleaned, "")

	reSpecialLines := regexp.MustCompile(`^[\s\W\d]*$`)
	cleaned = reSpecialLines.ReplaceAllString(cleaned, "")

	reCode := regexp.MustCompile(`(?i)^.*(const |function |class |<[^>]+>).*?$`)
	cleaned = reCode.ReplaceAllString(cleaned, "")

	reMultiNewLine := regexp.MustCompile(`\n{2,}`)
	cleaned = reMultiNewLine.ReplaceAllString(cleaned, "\n")

	return strings.TrimSpace(cleaned)
}

// CleanWithGemini sử dụng Gemini để làm sạch sâu, chuẩn hoá văn bản
func CleanWithGemini(text string) (string, error) {
	prompt := `Bạn là công cụ xử lý văn bản trích xuất từ tài liệu.
	Hãy xử lý văn bản sau với yêu cầu:
	- Xoá phần mục lục, các dòng chứa số trang, tiêu đề lặp lại
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
	1. Chỉ tóm tắt các ý chính thôi, giống như phần giới thiệu cho đoạn văn bản tôi đã gửi
	1. Không sử dụng từ ngữ chuyên môn quá khó hiểu
	2. KHÔNG sử dụng markdown, KHÔNG in đậm, KHÔNG in nghiêng, chỉ trả về văn bản thuần tuý, KHÔNG thêm ký tự đặc biệt
	3. Không bình luận, không giải thích, chỉ trả về nội dung tóm tắt.
	4. Không viết các câu như "Bài viết này nói về,..." chỉ trả về nội dung đã tóm tắt thôi.
	Đoạn văn bản cần viết lại:`

	fullPrompt := prompt + "\n\n" + text

	return GeminiGenerateText(fullPrompt)
}

// CleanTextPipeline là pipeline chính: Regex + Gemini (có chia nhỏ)
func CleanTextPipeline(rawText string) (string, error) {
	preCleaned := PreCleanText(rawText)

	totalLen := len(preCleaned)
	log.Printf("[Cleaner] Tổng độ dài trước Gemini: %d ký tự", totalLen)

	// Nếu văn bản quá dài, chia nhỏ để tránh vượt giới hạn token
	const chunkSize = 40000 // ~50k ký tự mỗi đoạn (~15k tokens)
	if totalLen > chunkSize {
		chunks := splitTextByLength(preCleaned, chunkSize)
		log.Printf("[Cleaner] Chia thành %d đoạn nhỏ để gửi Gemini...", len(chunks))

		var combined strings.Builder
		for i, chunk := range chunks {
			log.Printf("[Cleaner] → Đang xử lý đoạn %d/%d (%d ký tự)", i+1, len(chunks), len(chunk))
			cleanedChunk, err := CleanWithGemini(chunk)
			if err != nil {
				return "", err
			}
			combined.WriteString(cleanedChunk)
			combined.WriteString("\n")
		}

		result := strings.TrimSpace(combined.String())
		log.Printf("[Cleaner] Hoàn tất ghép lại (%d ký tự)", len(result))
		return result, nil
	}

	// Nếu ngắn thì xử lý 1 lần
	finalCleaned, err := CleanWithGemini(preCleaned)
	if err != nil {
		return "", err
	}
	log.Printf("[Cleaner] Hoàn tất làm sạch (%d ký tự)", len(finalCleaned))

	return finalCleaned, nil
}

func ExtractTextPipeline(rawText string) (string, error) {
	totalLen := len(rawText)
	log.Printf("[Extract] Tổng độ dài trước Gemini: %d ký tự", totalLen)

	const chunkSize = 40000
	if totalLen > chunkSize {
		chunks := splitTextByLength(rawText, chunkSize)
		log.Printf("[Extract] Chia thành %d đoạn nhỏ để tạo kịch bản...", len(chunks))

		var combined strings.Builder
		for i, chunk := range chunks {
			log.Printf("[Extract] → Đang xử lý đoạn %d/%d (%d ký tự)", i+1, len(chunks), len(chunk))
			scriptChunk, err := ExctractText(chunk)
			if err != nil {
				return "", err
			}
			combined.WriteString(scriptChunk)
			combined.WriteString("\n")
		}

		result := strings.TrimSpace(combined.String())
		log.Printf("[Extract] Hoàn tất ghép kịch bản (%d ký tự)", len(result))
		return result, nil
	}

	finalScript, err := ExctractText(rawText)
	if err != nil {
		return "", err
	}
	log.Printf("[Extract] Hoàn tất tạo kịch bản (%d ký tự)", len(finalScript))
	return finalScript, nil
}

// splitTextByLength chia văn bản dài thành nhiều đoạn nhỏ
func splitTextByLength(text string, maxLen int) []string {
	var parts []string
	runes := []rune(text)

	for i := 0; i < len(runes); {
		end := i + maxLen
		if end > len(runes) {
			end = len(runes)
		} else {
			// tìm dấu kết thúc câu gần nhất
			for end < len(runes) && runes[end] != '.' && runes[end] != '?' && runes[end] != '!' && end-i < maxLen+500 {
				end++
			}
		}
		part := strings.TrimSpace(string(runes[i:end]))
		parts = append(parts, part)
		i = end
	}

	return parts
}
