package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/vnkhanh/e-podcast-backend/models"
	"gorm.io/gorm"
)

// -----------------------------
// Struct trả về
// -----------------------------
type SearchFullResult struct {
	ID          string `json:"id"`
	Title       string `json:"title,omitempty"`
	Name        string `json:"name,omitempty"`
	Type        string `json:"type"`                  // podcast | subject
	Description string `json:"description,omitempty"` // podcast description
	Slug        string `json:"slug,omitempty"`        // subject slug
}

type SearchFullResponse struct {
	Total   int64              `json:"total"`
	Page    int                `json:"page"`
	PerPage int                `json:"per_page"`
	Results []SearchFullResult `json:"results"`
}

type SearchResult struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"` // podcast
	Name  string `json:"name,omitempty"`  // subject
	Type  string `json:"type"`            // podcast | subject
	Slug  string `json:"slug,omitempty"`  // subject slug
}

// -----------------------------
// Search Full (search page)
// -----------------------------
func SearchFullHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := strings.TrimSpace(c.Query("query"))
		if query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Query không được để trống"})
			return
		}

		// Phân trang
		page := 1
		perPage := 10
		if p := c.Query("page"); p != "" {
			if val, err := strconv.Atoi(p); err == nil && val > 0 {
				page = val
			}
		}
		if pp := c.Query("per_page"); pp != "" {
			if val, err := strconv.Atoi(pp); err == nil && val > 0 {
				perPage = val
			}
		}
		offset := (page - 1) * perPage

		var podcasts []models.Podcast
		var subjects []models.Subject
		var totalPodcasts, totalSubjects int64

		// Tìm podcasts
		podcastQuery := db.Model(&models.Podcast{}).
			Where("LOWER(title) LIKE ?", "%"+strings.ToLower(query)+"%")
		podcastQuery.Count(&totalPodcasts)
		podcastQuery.Offset(offset).Limit(perPage).Find(&podcasts)

		// Tìm subjects
		subjectQuery := db.Model(&models.Subject{}).
			Where("LOWER(name) LIKE ?", "%"+strings.ToLower(query)+"%")
		subjectQuery.Count(&totalSubjects)
		subjectQuery.Offset(offset).Limit(perPage).Find(&subjects)

		total := totalPodcasts + totalSubjects

		// Map results
		var results []SearchFullResult
		for _, p := range podcasts {
			results = append(results, SearchFullResult{
				ID:          p.ID.String(),
				Title:       p.Title,
				Type:        "podcast",
				Description: p.Description,
			})
		}
		for _, s := range subjects {
			results = append(results, SearchFullResult{
				ID:   s.ID.String(),
				Name: s.Name,
				Type: "subject",
				Slug: s.Slug, // <- trả slug
			})
		}

		c.JSON(http.StatusOK, SearchFullResponse{
			Total:   total,
			Page:    page,
			PerPage: perPage,
			Results: results,
		})
	}
}

// -----------------------------
// Search Autocomplete (gợi ý khi nhập)
// -----------------------------
func SearchAutocomplete(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := strings.TrimSpace(c.Query("query"))
		if query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Query không được để trống"})
			return
		}

		limit := 10
		if l := c.Query("limit"); l != "" {
			if val, err := strconv.Atoi(l); err == nil && val > 0 {
				limit = val
			}
		}

		var podcasts []models.Podcast
		var subjects []models.Subject

		// Tìm podcast
		if err := db.Select("id, title").
			Where("LOWER(title) LIKE ?", "%"+strings.ToLower(query)+"%").
			Limit(limit).
			Find(&podcasts).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi khi tìm podcast"})
			return
		}

		// Tìm subject, trả slug
		if err := db.Select("id, name, slug").
			Where("LOWER(name) LIKE ?", "%"+strings.ToLower(query)+"%").
			Limit(limit).
			Find(&subjects).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Lỗi khi tìm môn học"})
			return
		}

		var results []SearchResult
		for _, p := range podcasts {
			results = append(results, SearchResult{
				ID:    p.ID.String(),
				Title: p.Title,
				Type:  "podcast",
			})
		}
		for _, s := range subjects {
			results = append(results, SearchResult{
				ID:   s.ID.String(),
				Name: s.Name,
				Slug: s.Slug, // <- trả slug
				Type: "subject",
			})
		}

		c.JSON(http.StatusOK, results)
	}
}
