package services

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ExtractTextFromPDF: Đọc PDF với xử lý lỗi chi tiết hơn
func ExtractTextFromPDF(file multipart.File) (string, error) {
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, file); err != nil {
		return "", fmt.Errorf("lỗi đọc file PDF: %w", err)
	}

	reader, err := pdf.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		return "", fmt.Errorf("không thể tạo reader PDF: %w", err)
	}

	var textBuilder strings.Builder
	totalPages := reader.NumPage()
	successPages := 0
	emptyPages := 0
	errorPages := 0

	fmt.Printf("\n=== BẮT ĐẦU TRÍCH XUẤT PDF ===\n")
	fmt.Printf("📄 Tổng số trang: %d\n", totalPages)
	fmt.Printf("📦 Kích thước file: %d bytes\n\n", buf.Len())

	// Theo dõi những trang có vấn đề
	var problematicPages []int

	for i := 1; i <= totalPages; i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			fmt.Printf("⚠️  Trang %d: NULL page object\n", i)
			errorPages++
			problematicPages = append(problematicPages, i)
			continue
		}

		// Phương pháp 1: GetPlainText
		content, err := page.GetPlainText(nil)
		pageText := ""
		method := "GetPlainText"

		if err != nil {
			fmt.Printf("⚠️  Trang %d: GetPlainText failed (%v), trying GetTextByRow...\n", i, err)
			// Fallback: dùng GetTextByRow
			pageText = extractTextWithDetails(page)
			method = "GetTextByRow"
		} else {
			pageText = content
		}

		// Kiểm tra nội dung thực tế
		trimmedContent := strings.TrimSpace(pageText)
		contentLength := len(trimmedContent)

		if contentLength == 0 {
			emptyPages++
			problematicPages = append(problematicPages, i)
			fmt.Printf("❌ Trang %d: RỖNG (method: %s)\n", i, method)
			textBuilder.WriteString(fmt.Sprintf("\n--- Trang %d (rỗng) ---\n", i))
		} else {
			successPages++
			// Chỉ log mỗi 10 trang để không spam
			if i%10 == 0 || i <= 5 || i >= totalPages-5 {
				fmt.Printf("✅ Trang %d: %d ký tự (method: %s)\n", i, contentLength, method)
			}
			textBuilder.WriteString(fmt.Sprintf("\n--- Trang %d ---\n", i))
			textBuilder.WriteString(pageText)
			textBuilder.WriteString("\n")
		}
	}

	// Báo cáo chi tiết
	fmt.Printf("\n=== KẾT QUẢ TRÍCH XUẤT ===\n")
	fmt.Printf("✅ Thành công: %d trang (%.1f%%)\n", successPages, float64(successPages)/float64(totalPages)*100)
	fmt.Printf("⚠️  Rỗng: %d trang (%.1f%%)\n", emptyPages, float64(emptyPages)/float64(totalPages)*100)
	fmt.Printf("❌ Lỗi: %d trang (%.1f%%)\n", errorPages, float64(errorPages)/float64(totalPages)*100)
	fmt.Printf("📝 Tổng ký tự: %d\n", textBuilder.Len())

	if len(problematicPages) > 0 && len(problematicPages) <= 20 {
		fmt.Printf("\n🔍 Trang có vấn đề: %v\n", problematicPages)
	} else if len(problematicPages) > 20 {
		fmt.Printf("\n🔍 Có %d trang có vấn đề (quá nhiều để liệt kê)\n", len(problematicPages))
	}

	result := textBuilder.String()

	// Phân tích vấn đề
	successRate := float64(successPages) / float64(totalPages)

	fmt.Printf("\n=== CHẨN ĐOÁN ===\n")
	if successRate < 0.3 {
		fmt.Println("❌ PDF có thể là:")
		fmt.Println("   - Hình ảnh quét (cần OCR)")
		fmt.Println("   - Bị mã hóa")
		fmt.Println("   - Font đặc biệt không được hỗ trợ")
		return result, fmt.Errorf("chỉ trích xuất được %d/%d trang (%.1f%%) - PDF có thể bị mã hóa hoặc là hình ảnh quét",
			successPages, totalPages, successRate*100)
	} else if successRate < 0.7 {
		fmt.Printf("⚠️  Tỷ lệ thành công thấp (%.1f%%)\n", successRate*100)
		fmt.Println("   - Một số trang có thể là hình ảnh")
		fmt.Println("   - Font encoding không đồng nhất")
	} else {
		fmt.Printf("✅ Tỷ lệ thành công cao (%.1f%%)\n", successRate*100)
	}

	if len(result) < 1000 && totalPages > 10 {
		fmt.Printf("⚠️  Nội dung quá ngắn (%d ký tự) cho %d trang\n", len(result), totalPages)
		return result, fmt.Errorf("nội dung quá ngắn (%d ký tự) cho %d trang - cần kiểm tra PDF", len(result), totalPages)
	}

	return result, nil
}

// ExtractTextFromPDFWithFallback: Thử nhiều phương pháp khác nhau
func ExtractTextFromPDFWithFallback(file multipart.File) (string, error) {
	fmt.Println("\n🔄 Bắt đầu trích xuất với fallback...")

	// Phương pháp 1: Dùng ledongthuc/pdf
	text, err := ExtractTextFromPDF(file)

	// Nếu thành công và có nội dung đủ, trả về
	if err == nil && len(strings.TrimSpace(text)) > 500 {
		fmt.Println("✅ Phương pháp 1 thành công!")
		return text, nil
	}

	fmt.Printf("\n⚠️  Phương pháp 1 không đủ tốt (error: %v)\n", err)
	fmt.Println("🔄 Thử phương pháp 2: Raw content extraction...")

	// Phương pháp 2: Reset file pointer và thử đọc raw content
	if seeker, ok := file.(io.Seeker); ok {
		seeker.Seek(0, io.SeekStart)
		rawText, rawErr := extractRawPDFContent(file)
		if rawErr == nil && len(rawText) > len(text) {
			fmt.Printf("✅ Phương pháp 2 tốt hơn! (%d vs %d ký tự)\n", len(rawText), len(text))
			return rawText, nil
		}
		fmt.Printf("⚠️  Phương pháp 2 không tốt hơn (%d vs %d ký tự)\n", len(rawText), len(text))
	}

	// Trả về kết quả tốt nhất có được
	if len(text) > 0 {
		fmt.Println("⚠️  Trả về kết quả một phần từ phương pháp 1")
		return text, fmt.Errorf("trích xuất một phần: %w", err)
	}

	return "", fmt.Errorf("không thể trích xuất text từ PDF: %w", err)
}

// extractRawPDFContent: Đọc raw content stream trong PDF
func extractRawPDFContent(file multipart.File) (string, error) {
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, file); err != nil {
		return "", err
	}

	reader, err := pdf.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		return "", err
	}

	var textBuilder strings.Builder
	totalPages := reader.NumPage()
	successCount := 0

	fmt.Printf("📄 Raw extraction: Processing %d pages...\n", totalPages)

	for i := 1; i <= totalPages; i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}

		// Phương pháp 1: GetPlainText
		text, err := page.GetPlainText(nil)
		if err == nil && len(strings.TrimSpace(text)) > 0 {
			textBuilder.WriteString(text)
			textBuilder.WriteString("\n")
			successCount++
			continue
		}

		// Phương pháp 2: GetTextByRow
		rows, rowErr := page.GetTextByRow()
		if rowErr == nil {
			hasContent := false
			for _, row := range rows {
				for _, word := range row.Content {
					textBuilder.WriteString(word.S)
					textBuilder.WriteString(" ")
					hasContent = true
				}
				textBuilder.WriteString("\n")
			}
			if hasContent {
				successCount++
			}
		}
	}

	fmt.Printf("✅ Raw extraction: %d/%d pages extracted\n", successCount, totalPages)
	return textBuilder.String(), nil
}

// extractTextWithDetails: Trích xuất text sử dụng GetTextByRow
func extractTextWithDetails(page pdf.Page) string {
	var result strings.Builder

	rows, err := page.GetTextByRow()
	if err != nil {
		return ""
	}

	for _, row := range rows {
		for _, word := range row.Content {
			result.WriteString(word.S)
			result.WriteString(" ")
		}
		result.WriteString("\n")
	}

	return result.String()
}

// DiagnosePDF: Chẩn đoán PDF để biết vấn đề
func DiagnosePDF(file multipart.File) {
	var buf bytes.Buffer
	io.Copy(&buf, file)

	reader, err := pdf.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		fmt.Printf("❌ Không thể đọc PDF: %v\n", err)
		return
	}

	totalPages := reader.NumPage()
	fmt.Printf("\n=== CHẨN ĐOÁN PDF ===\n")
	fmt.Printf("📄 Tổng số trang: %d\n", totalPages)
	fmt.Printf("📦 Kích thước: %d bytes\n", buf.Len())

	// Kiểm tra 5 trang đầu
	fmt.Println("\n🔍 Kiểm tra 5 trang đầu:")
	for i := 1; i <= 5 && i <= totalPages; i++ {
		page := reader.Page(i)

		// Test GetPlainText
		text1, err1 := page.GetPlainText(nil)
		len1 := len(strings.TrimSpace(text1))

		// Test GetTextByRow
		rows, err2 := page.GetTextByRow()
		len2 := 0
		if err2 == nil {
			for _, row := range rows {
				for _, word := range row.Content {
					len2 += len(word.S)
				}
			}
		}

		fmt.Printf("  Trang %d:\n", i)
		fmt.Printf("    GetPlainText: %d chars (err: %v)\n", len1, err1)
		fmt.Printf("    GetTextByRow: %d chars (err: %v)\n", len2, err2)
	}
}

// ✅ ExtractTextFromDOCX
func ExtractTextFromDOCX(fileHeader *multipart.FileHeader) (string, error) {
	tmpFile, err := os.CreateTemp("", "upload-*.docx")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	src, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	if _, err := io.Copy(tmpFile, src); err != nil {
		return "", err
	}

	r, err := zip.OpenReader(tmpFile.Name())
	if err != nil {
		return "", err
	}
	defer r.Close()

	var docFile *zip.File
	for _, f := range r.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return "", fmt.Errorf("không tìm thấy nội dung document.xml trong file DOCX")
	}

	rc, err := docFile.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	var buf bytes.Buffer
	decoder := xml.NewDecoder(rc)
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch se := tok.(type) {
		case xml.StartElement:
			if se.Name.Local == "t" {
				var text string
				if err := decoder.DecodeElement(&text, &se); err == nil {
					buf.WriteString(text + " ")
				}
			}
		}
	}

	return strings.TrimSpace(buf.String()), nil
}

// ✅ ExtractTextFromTXT
func ExtractTextFromTXT(fileHeader *multipart.FileHeader) (string, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(file)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
