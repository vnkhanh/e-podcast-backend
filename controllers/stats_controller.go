package controllers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vnkhanh/e-podcast-backend/models"
	"gorm.io/gorm"
)

type (
	Point struct {
		Date  string `json:"date"`
		Count int64  `json:"count"`
	}

	MonthlyPoint struct {
		Month string `json:"month"`
		Count int64  `json:"count"`
	}

	TopPodcast struct {
		PodcastID  string `json:"podcast_id"`
		Title      string `json:"title"`
		AudioURL   string `json:"audio_url"`
		TotalPlays int64  `json:"total_plays"`
		LikeCount  int64  `json:"like_count"`
	}

	SubjectStat struct {
		Subject string `json:"subject"`
		Plays   int64  `json:"plays"`
	}
)

func GetDailyListens(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	fromStr, toStr := c.Query("from"), c.Query("to")
	to := time.Now()
	from := to.AddDate(0, 0, -7)

	if fromStr != "" {
		if t, err := time.Parse("2006-01-02", fromStr); err == nil {
			from = t
		}
	}
	if toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil {
			to = t
		}
	}

	var res []Point
	// Dùng analytics thay vì listening_histories
	db.Raw(`
		SELECT TO_CHAR(date, 'YYYY-MM-DD') AS date,
		       total_listens AS count
		FROM listening_analytics
		WHERE date BETWEEN ? AND ?
		ORDER BY date
	`, from, to).Scan(&res)

	c.JSON(http.StatusOK, res)
}

func GetMonthlyListens(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	year := time.Now().Year()
	if yStr := c.Query("year"); yStr != "" {
		if y, err := strconv.Atoi(yStr); err == nil {
			year = y
		}
	}

	var res []MonthlyPoint
	// Dùng analytics
	db.Raw(`
		SELECT TO_CHAR(date, 'YYYY-MM') AS month,
		       SUM(total_listens) AS count
		FROM listening_analytics
		WHERE EXTRACT(YEAR FROM date) = ?
		GROUP BY month 
		ORDER BY month
	`, year).Scan(&res)

	c.JSON(http.StatusOK, res)
}

func GetDashboardOverview(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	now := time.Now()

	var totalUsers, totalPodcasts int64
	db.Model(&models.User{}).Count(&totalUsers)
	db.Model(&models.Podcast{}).Count(&totalPodcasts)

	// Tổng lượt nghe 30 ngày từ analytics
	var totalListens30d int64
	db.Model(&models.ListeningAnalytics{}).
		Where("date >= ?", now.AddDate(0, 0, -30)).
		Select("COALESCE(SUM(total_listens), 0)").
		Scan(&totalListens30d)

	// Tỷ lệ hoàn thành từ analytics
	var sumCompleted, sumTotal int64
	db.Model(&models.ListeningAnalytics{}).
		Where("date >= ?", now.AddDate(0, 0, -30)).
		Select("COALESCE(SUM(completed_listens), 0)").
		Scan(&sumCompleted)
	db.Model(&models.ListeningAnalytics{}).
		Where("date >= ?", now.AddDate(0, 0, -30)).
		Select("COALESCE(SUM(total_listens), 0)").
		Scan(&sumTotal)

	completionRate := 0.0
	if sumTotal > 0 {
		completionRate = float64(sumCompleted) / float64(sumTotal) * 100
	}

	// Top podcast từ podcast_analytics (30 ngày gần nhất)
	var tops []TopPodcast
	db.Raw(`
		SELECT pa.podcast_id, p.title, p.audio_url, p.like_count,
		       SUM(pa.total_plays) AS total_plays
		FROM podcast_analytics pa
		JOIN podcasts p ON p.id = pa.podcast_id
		WHERE pa.date >= ?
		GROUP BY pa.podcast_id, p.title, p.audio_url, p.like_count
		ORDER BY total_plays DESC
		LIMIT 5
	`, now.AddDate(0, 0, -30)).Scan(&tops)

	c.JSON(http.StatusOK, gin.H{
		"total_users":       totalUsers,
		"total_podcasts":    totalPodcasts,
		"total_listens_30d": totalListens30d,
		"completion_rate":   completionRate,
		"top_podcasts":      tops,
	})
}

func GetSubjectBreakdown(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	var out []SubjectStat

	// Dùng subject_analytics (30 ngày gần nhất)
	db.Raw(`
		SELECT s.name AS subject, SUM(sa.total_plays) AS plays
		FROM subject_analytics sa
		JOIN subjects s ON s.id = sa.subject_id
		WHERE sa.date >= ?
		GROUP BY s.id, s.name
		ORDER BY plays DESC
		LIMIT 20
	`, time.Now().AddDate(0, 0, -30)).Scan(&out)

	c.JSON(http.StatusOK, out)
}

// ===================== Người dùng mới =====================
func GetNewUsers(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	from := time.Now().AddDate(0, 0, -days)

	var res []Point
	db.Raw(`
		WITH date_series AS (
			SELECT generate_series(
				?::date,
				CURRENT_DATE,
				'1 day'::interval
			)::date AS date
		)
		SELECT 
			TO_CHAR(ds.date, 'YYYY-MM-DD') AS date,
			COALESCE(COUNT(u.id), 0) AS count
		FROM date_series ds
		LEFT JOIN users u ON u.created_at::date = ds.date
		GROUP BY ds.date
		ORDER BY ds.date
	`, from).Scan(&res)

	c.JSON(http.StatusOK, res)
}

// GetPodcastAnalytics lấy analytics của 1 podcast cụ thể
func GetPodcastAnalytics(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	podcastID, err := uuid.Parse(c.Param("podcast_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid podcast_id"})
		return
	}

	days := 30 // mặc định 30 ngày
	from := time.Now().AddDate(0, 0, -days)

	var analytics []models.PodcastAnalytics
	db.Where("podcast_id = ? AND date >= ?", podcastID, from).
		Order("date DESC").
		Find(&analytics)

	c.JSON(http.StatusOK, gin.H{
		"data": analytics,
	})
}

// GetUserListeningStats thống kê của user hiện tại
func GetUserListeningStats(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userIDStr := c.GetString("user_id")

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid user"})
		return
	}

	// Tổng số podcast đã nghe
	var totalPodcasts int64
	db.Model(&models.ListeningHistory{}).
		Where("user_id = ?", userID).
		Distinct("podcast_id").
		Count(&totalPodcasts)

	// Tổng thời gian nghe
	var totalDuration int64
	db.Model(&models.ListeningHistory{}).
		Where("user_id = ?", userID).
		Select("COALESCE(SUM(last_position), 0)").
		Scan(&totalDuration)

	// Số podcast hoàn thành
	var completedPodcasts int64
	db.Model(&models.ListeningHistory{}).
		Where("user_id = ? AND completed = ?", userID, true).
		Count(&completedPodcasts)

	c.JSON(http.StatusOK, gin.H{
		"total_podcasts":         totalPodcasts,
		"completed_podcasts":     completedPodcasts,
		"total_duration_seconds": totalDuration,
		"total_duration_hours":   float64(totalDuration) / 3600,
	})
}
