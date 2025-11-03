package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/vnkhanh/e-podcast-backend/config"
	"github.com/vnkhanh/e-podcast-backend/models"
	"github.com/vnkhanh/e-podcast-backend/utils"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Thử Authorization header trước
		authHeader := c.GetHeader("Authorization")

		// Nếu không có, thử X-Auth-Token (cho iOS)
		if authHeader == "" {
			authHeader = c.GetHeader("X-Auth-Token")
		}

		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Thiếu Authorization header"})
			c.Abort()
			return
		}

		// Tách token khỏi chuỗi "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header không hợp lệ"})
			c.Abort()
			return
		}

		tokenString := parts[1]
		claims, err := utils.VerifyToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token không hợp lệ hoặc hết hạn"})
			c.Abort()
			return
		}

		// Kiểm tra trạng thái user trong DB
		var user models.User
		if err := config.DB.Select("status").First(&user, "id = ?", claims.UserID).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Không tìm thấy người dùng"})
			c.Abort()
			return
		}

		if user.Status != nil && !*user.Status {
			c.JSON(http.StatusForbidden, gin.H{"error": "Tài khoản đã bị tạm khóa"})
			c.Abort()
			return
		}
		// Lưu thông tin vào context để controller dùng
		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)
		c.Next()
	}
}

func OptionalAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")

		// Cho iOS: thử X-Auth-Token nếu không có Authorization
		if authHeader == "" {
			authHeader = c.GetHeader("X-Auth-Token")
		}

		// Nếu không có token -> Cho qua (anonymous)
		if authHeader == "" {
			c.Next()
			return
		}

		// Phải là "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			// Token sai định dạng -> coi như anonymous
			c.Next()
			return
		}

		tokenString := parts[1]
		claims, err := utils.VerifyToken(tokenString)
		if err != nil {
			// Token sai / hết hạn -> coi như anonymous
			c.Next()
			return
		}

		// Token hợp lệ -> lưu thông tin user
		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)
		c.Next()
	}
}
