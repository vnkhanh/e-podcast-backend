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
	api.GET("/search", controllers.SearchAutocomplete(db))
	api.GET("/search/full", controllers.SearchFullHandler(db))
	api.GET("/podcasts/:podcast_id/share-social", controllers.SharePodcastSocialHandler(db))

	auth := api.Group("/auth")
	{
		auth.POST("/register", controllers.Register)
		auth.POST("/login", controllers.Login)
		auth.POST("/logingoogle", controllers.GoogleLogin)
		auth.PUT("/change-password", middleware.AuthMiddleware(), controllers.ChangePassword)
		auth.POST("/forgot-password", controllers.ForgotPassword)
		auth.POST("/reset-password", controllers.ResetPassword)
		auth.GET("/verify-reset-token", controllers.VerifyResetToken)
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

			// favorite
			account.GET("/favorites", controllers.GetFavorites)
			account.POST("/favorites/:podcast_id", controllers.AddFavorite)
			account.DELETE("/favorites/:podcast_id", controllers.RemoveFavorite)
			account.GET("/favorite/:podcast_id", controllers.CheckFavorite)

		}
		user.GET("/categories/featured", controllers.GetCategoriesUserPopular)
		user.GET("/categories", controllers.GetCategoriesUser)
		user.GET("/categories/:slug/podcasts", controllers.GetPodcastsByCategory)
		user.GET("/podcasts/featured", controllers.GetFeaturedPodcasts)
		user.GET("/podcasts/latest", controllers.GetLatestPodcasts)

		user.GET("/podcasts/:id", controllers.GetPodcastByID)
		user.POST("/documents/:id/flashcards", middleware.AuthMiddleware(), controllers.GenerateFlashcardsFromDocument)
		user.GET("/podcasts/:id/flashcards", middleware.AuthMiddleware(), controllers.GetFlashcardsByPodcast)
		user.GET("/documents/:id", controllers.GetDocumentDetail)
		user.GET("/podcasts", controllers.GetAllPublishedPodcasts)
		user.GET("/tagsget", controllers.GetTags)
		user.GET("/categoriesget", controllers.GetCategoriesGet)
		user.GET("/subjectsget", controllers.GetSubjectsGet)

		// Subject routes
		user.GET("/subjects/popular", controllers.GetPopularSubjects)
		user.GET("/subjects/:slug", middleware.OptionalAuthMiddleware(), controllers.GetSubjectDetailUser) // chi tiết môn học với tiến độ
		user.GET("/subjects", middleware.OptionalAuthMiddleware(), controllers.GetAllSubjectsUser)         // ds môn học với tiến độ

		// Quiz routes
		user.POST("/documents/:id/quizzes", middleware.AuthMiddleware(), controllers.GenerateQuizzesFromDocument) // tạo quiz
		user.GET("/podcasts/:id/quiz-sets", middleware.AuthMiddleware(), controllers.GetQuizSetsByPodcast)        // lấy ds quiz theo podcast id
		user.GET("/quiz-sets/:id/questions", middleware.AuthMiddleware(), controllers.GetQuizQuestions)           // lấy ds câu hỏi của quiz
		user.POST("/quiz-sets/:id/submit", middleware.AuthMiddleware(), controllers.SubmitQuizAttempt)            // gửi câu hỏi
		user.GET("/quiz-attempts", middleware.AuthMiddleware(), controllers.GetUserQuizAttempts)                  // lấy
		user.GET("/quiz-attempts/:attemptID", middleware.AuthMiddleware(), controllers.GetQuizAttemptDetail)      // gửi câu hỏi
		user.GET("/quiz-sets/:id/attempts", middleware.AuthMiddleware(), controllers.GetQuizAttemptsBySet)        // lấy lịch sử làm quiz
		user.DELETE("/quiz-sets/:quizset_id", middleware.AuthMiddleware(), controllers.DeleteQuizSetByCurrentUser)
		user.DELETE("/quiz-sets", middleware.AuthMiddleware(), controllers.DeleteAllQuizSetsByCurrentUser)

		// Notes
		user.POST("/notes", middleware.AuthMiddleware(), controllers.CreateNote)
		user.GET("/podcasts/:id/notes", middleware.AuthMiddleware(), controllers.GetNotesByPodcast)
		user.DELETE("/notes/:id", middleware.AuthMiddleware(), controllers.DeleteNote)

		// Listening
		user.POST("/podcasts/:id/listen", middleware.OptionalAuthMiddleware(), controllers.IncreasePodcastListenCount)
	}
	admin := api.Group("/admin")
	{
		admin.Use(
			middleware.AuthMiddleware(),
			middleware.DBMiddleware(db),
			middleware.RequireRoles("admin", "teacher"),
		)
		admin.GET("/me", controllers.GetProfileUser)

	}

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

	// ==================== Thống kê ====================
	stats := admin.Group("/stats")
	{
		stats.GET("/overview", controllers.GetDashboardOverview)
		stats.GET("/monthly-listens", controllers.GetMonthlyListens)
		stats.GET("/daily-listens", controllers.GetDailyListens)
		stats.GET("/new-users", controllers.GetNewUsers)
		// stats.GET("/top-podcasts", controllers.GetTopPodcasts)
		stats.GET("/subject-breakdown", controllers.GetSubjectBreakdown)
	}

	// ==================== Bình luận ====================
	comments := api.Group("/comments")
	{
		comments.POST("", middleware.AuthMiddleware(), controllers.CreateComment)
		comments.GET("/podcasts/:id", controllers.GetComments)
		comments.DELETE("/:id", middleware.AuthMiddleware(), controllers.DeleteComment)
	}

	// ==================== Thông báo ====================
	notifications := api.Group("/notifications")
	{
		notifications.Use(middleware.AuthMiddleware(), middleware.DBMiddleware(db))
		notifications.GET("", controllers.GetNotifications)
		notifications.GET("/unread", controllers.GetUnreadCount)
		notifications.PUT("/:id/read", controllers.MarkNotificationAsRead)
		notifications.PUT("/mark-all-read", controllers.MarkAllAsRead)
		notifications.DELETE("", controllers.DeleteAllNotifications)
		notifications.DELETE("/:id", controllers.DeleteNotification)
		notifications.DELETE("/read", controllers.DeleteReadNotifications)
	}
	// ==================== WebSocket ====================
	r.GET("/ws/document/:id", ws.HandleDocumentWebSocket)
	r.GET("/ws/podcast/:id", ws.HandlePodcastWebSocket)
	r.GET("/ws/status", ws.HandleGlobalWebSocket)
	r.GET("/ws/user", ws.HandleUserWebSocket)

	return r
}
