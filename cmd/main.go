package main

import (
	"log"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/vnkhanh/e-podcast-backend/config"
	"github.com/vnkhanh/e-podcast-backend/routes"
)

func main() {
	// Load .env
	if err := godotenv.Load(); err != nil {
		log.Println("Không tìm thấy file .env")
	}

	config.InitDB()

	r := gin.Default()

	//Bật CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
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
