package controllers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vnkhanh/e-podcast-backend/models"
	"gorm.io/gorm"
)

type MonthlyPoint struct {
	Month string `json:"month"` // "2025-01"
	Count int64  `json:"count"`
}

type TopPodcastItem struct {
	PodcastID  string `json:"podcast_id"`
	Title      string `json:"title"`
	AudioURL   string `json:"audio_url"`
	ViewCount  int64  `json:"view_count"`
	LikeCount  int64  `json:"like_count"`
	TotalPlays int64  `json:"total_plays"`
}

type DashboardOverview struct {
	TotalUsers      int64            `json:"total_users"`
	NewUsers30d     int64            `json:"new_users_30d"`
	TotalPodcasts   int64            `json:"total_podcasts"`
	TotalListens30d int64            `json:"total_listens_30d"`
	AvgSessionSec   float64          `json:"avg_session_sec"`
	CompletionRate  float64          `json:"completion_rate"` // 0-100
	TopPodcasts     []TopPodcastItem `json:"top_podcasts"`
}

// Helper to parse ?year=
func getYearParam(c *gin.Context) int {
	yStr := c.Query("year")
	if yStr == "" {
		return time.Now().Year()
	}
	if y, err := strconv.Atoi(yStr); err == nil {
		return y
	}
	return time.Now().Year()
}

// ===================== Lượt nghe theo tháng =====================
func GetMonthlyListens(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	year := getYearParam(c)

	// ép timezone sang +07:00 (Asia/Ho_Chi_Minh)
	rows, err := db.Raw(`
		SELECT TO_CHAR((updated_at AT TIME ZONE 'UTC' AT TIME ZONE 'Asia/Ho_Chi_Minh'), 'YYYY-MM') AS month,
		       SUM(view_count) AS count
		FROM podcasts
		WHERE EXTRACT(YEAR FROM (updated_at AT TIME ZONE 'UTC' AT TIME ZONE 'Asia/Ho_Chi_Minh')) = ?
		GROUP BY month
		ORDER BY month
	`, year).Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var res []MonthlyPoint
	for rows.Next() {
		var m MonthlyPoint
		if err := rows.Scan(&m.Month, &m.Count); err != nil {
			continue
		}
		res = append(res, m)
	}

	// đủ 12 tháng
	months := make(map[string]int64)
	for _, mv := range res {
		months[mv.Month] = mv.Count
	}
	out := []MonthlyPoint{}
	for m := 1; m <= 12; m++ {
		key := time.Date(year, time.Month(m), 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
		out = append(out, MonthlyPoint{Month: key, Count: months[key]})
	}
	c.JSON(http.StatusOK, out)
}

// ===================== Top Podcast =====================
func GetTopPodcasts(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	limitQ := c.DefaultQuery("limit", "10")
	limit, _ := strconv.Atoi(limitQ)
	sort := c.DefaultQuery("sort", "plays") // plays | likes

	if sort == "likes" {
		var items []TopPodcastItem
		db.Raw(`
			SELECT p.id, p.title, p.audio_url, p.view_count, p.like_count, p.view_count as total_plays
			FROM podcasts p
			ORDER BY p.like_count DESC
			LIMIT ?
		`, limit).Scan(&items)
		c.JSON(http.StatusOK, items)
		return
	}

	var items []TopPodcastItem
	db.Raw(`
		SELECT p.id, p.title, p.audio_url, p.view_count, p.like_count, p.view_count as total_plays
		FROM podcasts p
		ORDER BY p.view_count DESC
		LIMIT ?
	`, limit).Scan(&items)
	c.JSON(http.StatusOK, items)
}

// =====================  Người dùng mới =====================
func GetNewUsers(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	daysQ := c.DefaultQuery("days", "30")
	days, _ := strconv.Atoi(daysQ)
	from := time.Now().AddDate(0, 0, -days)

	type Point struct {
		Date  string `json:"date"`
		Count int64  `json:"count"`
	}
	rows, err := db.Raw(`
		SELECT TO_CHAR(created_at, 'YYYY-MM-DD') as date, COUNT(*) as count
		FROM users
		WHERE created_at >= ?
		GROUP BY date
		ORDER BY date
	`, from).Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var out []Point
	for rows.Next() {
		var p Point
		_ = rows.Scan(&p.Date, &p.Count)
		out = append(out, p)
	}
	c.JSON(http.StatusOK, out)
}

// ===================== Tổng quan Dashboard =====================
func GetDashboardOverview(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	var totalUsers int64
	db.Model(&models.User{}).Count(&totalUsers)

	var newUsers30 int64
	db.Model(&models.User{}).Where("created_at >= ?", time.Now().AddDate(0, 0, -30)).Count(&newUsers30)

	var totalPodcasts int64
	db.Model(&models.Podcast{}).Count(&totalPodcasts)

	// Tính tổng lượt nghe 30 ngày gần nhất dựa vào view_count tăng
	var totalListens30 int64
	db.Raw(`SELECT SUM(view_count) FROM podcasts WHERE updated_at >= ?`, time.Now().AddDate(0, 0, -30)).Scan(&totalListens30)

	var avgSession float64
	db.Raw(`SELECT AVG(duration) FROM listening_histories WHERE duration > 0`).Scan(&avgSession)

	var completedCount int64
	var totalCount int64
	db.Model(&models.ListeningHistory{}).Where("completed = ?", true).Count(&completedCount)
	db.Model(&models.ListeningHistory{}).Count(&totalCount)

	var completionRate float64 = 0
	if totalCount > 0 {
		completionRate = float64(completedCount) / float64(totalCount) * 100
	}

	var tops []TopPodcastItem
	db.Raw(`
		SELECT p.id, p.title, p.audio_url, p.view_count, p.like_count, p.view_count as total_plays
		FROM podcasts p
		ORDER BY p.view_count DESC
		LIMIT 5
	`).Scan(&tops)

	overview := DashboardOverview{
		TotalUsers:      totalUsers,
		NewUsers30d:     newUsers30,
		TotalPodcasts:   totalPodcasts,
		TotalListens30d: totalListens30,
		AvgSessionSec:   avgSession,
		CompletionRate:  completionRate,
		TopPodcasts:     tops,
	}
	c.JSON(http.StatusOK, overview)
}

// ===================== Thống kê theo môn học =====================
func GetSubjectBreakdown(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)

	type SB struct {
		SubjectName string `json:"subject"`
		Plays       int64  `json:"plays"`
	}
	var out []SB

	db.Raw(`
		SELECT s.name as subject_name, SUM(p.view_count) as plays
		FROM podcasts p
		JOIN chapters ch ON ch.id = p.chapter_id
		JOIN subjects s ON s.id = ch.subject_id
		GROUP BY s.id, s.name
		ORDER BY plays DESC
		LIMIT 20
	`).Scan(&out)

	c.JSON(http.StatusOK, out)
}
