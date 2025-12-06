package main

import (
	"log"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/vnkhanh/e-podcast-backend/config"
	"github.com/vnkhanh/e-podcast-backend/routes"
	"github.com/vnkhanh/e-podcast-backend/utils"
)

func main() {
	// Load .env (chỉ dùng khi chạy local).
	if err := godotenv.Load(); err != nil {
		log.Println("Không tìm thấy file .env (bỏ qua khi deploy trên Render)")
	}

	config.InitDB()

	r := gin.Default()
	// Khởi động Cleanup Job
	utils.StartCleanupJob()

	// Bật CORS
	origin := os.Getenv("CORS_ORIGIN")
	allowOrigins := []string{"http://localhost:5173"}
	if origin != "" {
		allowOrigins = append(allowOrigins, origin)
	}

	r.Use(cors.New(cors.Config{
		AllowOrigins:     allowOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"*"},  
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		AllowWebSockets:  true,
		MaxAge:           12 * time.Hour,
	}))
	// Gọi SetupRouter để đăng ký route
	r = routes.SetupRouter(r, config.DB)

	// Route test server
	r.GET("/", func(c *gin.Context) {
		c.String(200, "Survey server is running")
	})

	// Lấy PORT từ env
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // mặc định nếu không có PORT
	}

	log.Println("Server running at Port:" + port)
	r.Run(":" + port)
}
