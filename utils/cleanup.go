package utils

import (
	"log"
	"time"

	"github.com/vnkhanh/e-podcast-backend/config"
	"github.com/vnkhanh/e-podcast-backend/models"
)

// CleanupExpiredTokens xóa các token đã hết hạn hoặc đã sử dụng
func CleanupExpiredTokens() {
	db := config.DB

	// Xóa token hết hạn HOẶC đã dùng
	result := db.Where("expires_at < ? OR used = ?", time.Now(), true).
		Delete(&models.PasswordReset{})

	if result.Error != nil {
		log.Printf("Lỗi khi xóa password reset tokens: %v", result.Error)
		return
	}

	if result.RowsAffected > 0 {
		log.Printf("Đã xóa %d password reset tokens hết hạn/đã dùng", result.RowsAffected)
	}
}

// StartCleanupJob chạy cleanup job định kỳ
func StartCleanupJob() {
	// Chạy cleanup ngay lần đầu khi khởi động
	log.Println("Đang chạy cleanup lần đầu...")
	CleanupExpiredTokens()

	// Thiết lập ticker để chạy mỗi 6 giờ
	ticker := time.NewTicker(6 * time.Hour)

	go func() {
		defer ticker.Stop()
		for range ticker.C {
			log.Println("Cleanup job được kích hoạt...")
			CleanupExpiredTokens()
		}
	}()

	log.Println("Cleanup job đã được khởi động (chạy mỗi 6 giờ)")
}
