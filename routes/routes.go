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
	{
		admin.Use(middleware.AuthMiddleware(), middleware.DBMiddleware(db), middleware.RequireRoles("admin", "teacher"))

		//Quản lý môn học
		admin.POST("/subjects", controllers.CreateSubject)
		admin.GET("/subjects/:id", controllers.GetSubjectDetail)
		admin.GET("/subjects", controllers.GetSubjects)
		admin.GET("/subjectsget", controllers.GetSubjectsGet)
		admin.DELETE("/subjects/:id", controllers.DeleteSubject)
		admin.PUT("/subjects/:id", controllers.UpdateSubject)
		admin.PATCH("/subjects/:id/toggle-status", controllers.ToggleSubjectStatus)

		//Quản lý chủ đề
		admin.POST("/topics", controllers.CreateTopic)
		admin.GET("/topics", controllers.GetTopics)
		admin.GET("/topicsget", controllers.GetTopicsGet)
		admin.PUT("/topics/:id", controllers.UpdateTopic)
		admin.DELETE("/topics/:id", controllers.DeleteTopic)
		admin.PATCH("/topics/:id/toggle-status", controllers.ToggleTopicStatus)
		admin.GET("/topics/:id", controllers.GetTopicDetail)

		//Quản lý danh mục
		admin.POST("/categories", controllers.CreateCategory)
		admin.GET("/categories", controllers.GetCategories)
		admin.GET("/categoriesget", controllers.GetCategoriesGet)
		admin.PUT("/categories/:id", controllers.UpdateCategory)
		admin.DELETE("/categories/:id", controllers.DeleteCategory)
		admin.PATCH("/categories/:id/toggle-status", controllers.ToggleCategoryStatus)
		admin.GET("/categories/:id", controllers.GetCategoryDetail)

		//Quản lý tài liệu
		admin.POST("/documents", controllers.UploadDocument)
		admin.GET("/documents", controllers.GetDocuments)
		admin.GET("/documents/:id", controllers.GetDocumentDetail)
		admin.DELETE("/documents/:id", controllers.DeleteDocument)

		//Quản lý podcast
		admin.POST("/podcasts", controllers.CreatePodcastWithUpload)
		admin.GET("/podcasts", controllers.GetPodcasts)
		admin.GET("/podcasts/:id", controllers.GetPodcastDetail)

		//Quản lý tags
		admin.GET("/tags", controllers.GetTags)

		// admin.GET("/documents/:id", controllers.GetDocumentDetail)
		// admin.GET("/documents", controllers.GetDocuments)
		// admin.DELETE("/documents/:id", controllers.DeleteDocument)
		// admin.PUT("/documents/:id", controllers.UpdateDocument)
		// admin.PATCH("/documents/:id/toggle-status", controllers.ToggleDocumentStatus)

		// //Quản lý chương
		// admin.POST("/subjects/:id/chapters", controllers.CreateChapter)
		// admin.GET("/chapters/:id", controllers.GetChapterDetail)
		// admin.PUT("/chapters/:id", controllers.UpdateChapter)
		// admin.DELETE("/chapters/:id", controllers.DeleteChapter)
	}

	r.GET("/ws/document/:id", ws.HandleDocumentWebSocket)
	r.GET("/ws/status", ws.HandleGlobalWebSocket)

	return r
}
