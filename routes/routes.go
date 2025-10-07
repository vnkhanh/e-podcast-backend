package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/vnkhanh/e-podcast-backend/controllers"
	"github.com/vnkhanh/e-podcast-backend/middleware"
	"github.com/vnkhanh/e-podcast-backend/ws"
	"gorm.io/gorm"
)

func SetupRouter(r *gin.Engine, db *gorm.DB) *gin.Engine {
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})
	r.GET("/health", controllers.HealthCheck)

	api := r.Group("/api")

	auth := api.Group("/auth")
	{
		auth.POST("/register", controllers.Register)
		auth.POST("/login", controllers.Login)
		auth.POST("/logingoogle", controllers.GoogleLogin)
		// auth.POST("/loginfacebook", controllers.FacebookLogin)

	}

	user := api.Group("/user")
	{
		user.Use(middleware.AuthMiddleware(), middleware.DBMiddleware(db))
		// TODO: thêm các route cho user
	}

	admin := api.Group("/admin")
	admin.Use(
		middleware.AuthMiddleware(),
		middleware.DBMiddleware(db),
		middleware.RequireRoles("admin", "teacher"),
	)

	// ==================== Quản lý môn học ====================
	subjects := admin.Group("/subjects")
	{
		subjects.POST("", controllers.CreateSubject)
		subjects.GET("", controllers.GetSubjects)
		subjects.GET("/get", controllers.GetSubjectsGet)
		subjects.GET("/:id", controllers.GetSubjectDetail)
		subjects.PUT("/:id", controllers.UpdateSubject)
		subjects.DELETE("/:id", controllers.DeleteSubject)
		subjects.PATCH("/:id/toggle-status", controllers.ToggleSubjectStatus)
	}

	// ==================== Quản lý chủ đề ====================
	topics := admin.Group("/topics")
	{
		topics.POST("", controllers.CreateTopic)
		topics.GET("", controllers.GetTopics)
		topics.GET("/get", controllers.GetTopicsGet)
		topics.GET("/:id", controllers.GetTopicDetail)
		topics.PUT("/:id", controllers.UpdateTopic)
		topics.DELETE("/:id", controllers.DeleteTopic)
		topics.PATCH("/:id/toggle-status", controllers.ToggleTopicStatus)
	}

	// ==================== Quản lý danh mục ====================
	categories := admin.Group("/categories")
	{
		categories.POST("", controllers.CreateCategory)
		categories.GET("", controllers.GetCategories)
		categories.GET("/get", controllers.GetCategoriesGet)
		categories.GET("/:id", controllers.GetCategoryDetail)
		categories.PUT("/:id", controllers.UpdateCategory)
		categories.DELETE("/:id", controllers.DeleteCategory)
		categories.PATCH("/:id/toggle-status", controllers.ToggleCategoryStatus)
	}

	// ==================== Quản lý tài liệu ====================
	documents := admin.Group("/documents")
	{
		documents.POST("", controllers.UploadDocument)
		documents.GET("", controllers.GetDocuments)
		documents.GET("/:id", controllers.GetDocumentDetail)
		documents.DELETE("/:id", controllers.DeleteDocument)
		// documents.PUT("/:id", controllers.UpdateDocument)
		// documents.PATCH("/:id/toggle-status", controllers.ToggleDocumentStatus)
	}

	// ==================== Quản lý podcast ====================
	podcasts := admin.Group("/podcasts")
	{
		podcasts.POST("", controllers.CreatePodcastWithUpload)
		podcasts.GET("", controllers.GetPodcasts)
		podcasts.GET("/:id", controllers.GetPodcastDetail)
	}

	// ==================== Quản lý tag ====================
	tags := admin.Group("/tags")
	{
		tags.GET("", controllers.GetTags)
	}

	// ==================== Quản lý chương (tạm ẩn nếu chưa dùng) ====================
	// chapters := admin.Group("/chapters")
	// {
	// 	chapters.POST("/subjects/:id", controllers.CreateChapter)
	// 	chapters.GET("/:id", controllers.GetChapterDetail)
	// 	chapters.PUT("/:id", controllers.UpdateChapter)
	// 	chapters.DELETE("/:id", controllers.DeleteChapter)
	// }

	r.GET("/ws/document/:id", ws.HandleDocumentWebSocket)
	r.GET("/ws/status", ws.HandleGlobalWebSocket)

	return r
}
