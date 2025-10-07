package utils

import (
	"errors"

	"github.com/vnkhanh/e-podcast-backend/services"
)

// Hàm ánh xạ phần mở rộng file sang InputType
func GetInputTypeFromExt(ext string) (services.InputType, error) {
	switch ext {
	case ".pdf":
		return services.InputPDF, nil
	case ".docx":
		return services.InputDOCX, nil
	case ".txt":
		return services.InputTXT, nil
	default:
		return "", errors.New("định dạng file không hỗ trợ")
	}
}
