package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireRoles cho phép chỉ định nhiều vai trò được quyền truy cập
func RequireRoles(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Gọi AuthMiddleware trước để xác thực token
		AuthMiddleware()(c)

		// Nếu AuthMiddleware đã dừng request
		if c.IsAborted() {
			return
		}

		roleValue, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Không xác định được vai trò người dùng"})
			c.Abort()
			return
		}

		role, ok := roleValue.(string)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi xử lý vai trò người dùng"})
			c.Abort()
			return
		}

		// Kiểm tra role hợp lệ
		for _, allowed := range allowedRoles {
			if role == allowed {
				c.Next()
				return
			}
		}

		// Nếu không khớp role nào
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Bạn không có quyền truy cập tài nguyên này",
		})
		c.Abort()
	}
}
