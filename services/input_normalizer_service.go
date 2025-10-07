package services

import (
	"errors"
	"mime/multipart"
)

// Định nghĩa loại input
type InputType string

const (
	InputText  InputType = "text"
	InputTXT   InputType = "txt"
	InputDOCX  InputType = "docx"
	InputPDF   InputType = "pdf"
	InputAudio InputType = "audio" // Dành cho bước sau (nếu cần tích hợp Speech-to-Text)
)

// Struct đại diện cho nguồn input
type InputSource struct {
	Type       InputType
	FileHeader *multipart.FileHeader // Nếu là file (txt, docx, pdf, audio)
	Text       string                // Nếu người dùng nhập tay
}

// Hàm xử lý input thành plain text
func NormalizeInput(input InputSource) (string, error) {
	switch input.Type {
	case InputText:
		return input.Text, nil

	case InputTXT:
		return ExtractTextFromTXT(input.FileHeader)

	case InputPDF:
		f, err := input.FileHeader.Open()
		if err != nil {
			return "", err
		}
		defer f.Close()
		return ExtractTextFromPDF(f)

	case InputDOCX:
		return ExtractTextFromDOCX(input.FileHeader)

	default:
		return "", errors.New("loại input không được hỗ trợ")
	}
}
