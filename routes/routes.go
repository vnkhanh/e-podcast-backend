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
		auth.PUT("/change-password", controllers.ChangePassword)
		auth.POST("/forgot-password", controllers.ForgotPassword)
		auth.POST("/reset-password", controllers.ResetPassword)
		// auth.POST("/loginfacebook", controllers.FacebookLogin)
	}

	user := api.Group("/user")
	{
		user.Use(middleware.DBMiddleware(db))
		account := user.Group("/account")
		{
			account.Use(middleware.AuthMiddleware())
			account.GET("/me", controllers.GetProfileUser)
			account.GET("/listening-history", controllers.GetListeningHistory)
			account.POST("/listening-history/:podcast_id", controllers.SavePodcastHistory)
			account.GET("/listening-history/:podcast_id", controllers.GetPodcastHistory)
			account.DELETE("/listening-history/:podcast_id", controllers.DeletePodcastHistory)
			account.DELETE("/listening-history", controllers.ClearAllHistory)

		}
		user.GET("/categories", controllers.GetCategoriesUser)
		user.GET("/categories/:slug/podcasts", controllers.GetPodcastsByCategory)
		user.GET("/podcasts/featured", controllers.GetFeaturedPodcasts)
		user.GET("/podcasts/latest", controllers.GetLatestPodcasts)

		user.GET("/podcasts/:id", controllers.GetPodcastByID)
		user.POST("/documents/:id/flashcards", middleware.AuthMiddleware(), controllers.GenerateFlashcardsFromDocument)
		user.GET("/podcasts/:id/flashcards", middleware.AuthMiddleware(), controllers.GetFlashcardsByPodcast)
		user.GET("/documents/:id", controllers.GetDocumentDetail)

		user.GET("subjects/popular", controllers.GetPopularSubjects)
		user.GET("/subjects/:slug", controllers.GetSubjectDetailUser)

		// Quiz routes
		user.POST("/documents/:id/quizzes", middleware.AuthMiddleware(), controllers.GenerateQuizzesFromDocument) // tạo quiz
		user.GET("/podcasts/:id/quiz-sets", middleware.AuthMiddleware(), controllers.GetQuizSetsByPodcast)        // lấy ds quiz theo podcast id
		user.GET("/quiz-sets/:id/questions", middleware.AuthMiddleware(), controllers.GetQuizQuestions)           // lấy ds câu hỏi của quiz
		user.POST("/quiz-sets/:id/submit", middleware.AuthMiddleware(), controllers.SubmitQuizAttempt)            // gửi câu hỏi
		user.GET("/quiz-attempts", middleware.AuthMiddleware(), controllers.GetUserQuizAttempts)                  // lấy
		user.GET("/quiz-attempts/:attemptID", middleware.AuthMiddleware(), controllers.GetQuizAttemptDetail)      // gửi câu hỏi
		user.GET("/quiz-sets/:id/attempts", middleware.AuthMiddleware(), controllers.GetQuizAttemptsBySet)        // lấy lịch sử làm quiz

		// Listening
		user.POST("/podcasts/:id/listen", middleware.OptionalAuthMiddleware(), controllers.IncreasePodcastListenCount)
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

		// Chương
		subjects.GET("/:id/chapters", controllers.ListChaptersBySubject)
		subjects.POST("/:id/chapters", controllers.CreateChapter)
		subjects.GET("/chapters/:id/check-deletable", controllers.CheckChapterDeletable)
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
		podcasts.DELETE("/:id", controllers.DeletePodcast)
		podcasts.PUT("/:id", controllers.UpdatePodcast)
	}

	// ==================== Quản lý tag ====================
	tags := admin.Group("/tags")
	{
		tags.GET("", controllers.GetTags)
	}
	users := admin.Group("/users")
	{
		users.POST("", controllers.AdminCreateLecturer)
		users.GET("", controllers.AdminGetUsers)
		users.GET("/:id", controllers.AdminGetUserDetail)
		users.DELETE("/:id", controllers.AdminDeleteUser)
		users.PATCH("/:id/toggle-status", controllers.ToggleUserStatus)
	}
	r.GET("/ws/document/:id", ws.HandleDocumentWebSocket)
	r.GET("/ws/status", ws.HandleGlobalWebSocket)

	return r
}
