package controllers

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"strings"

	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	"cloud.google.com/go/texttospeech/apiv1/texttospeechpb"
	"github.com/gin-gonic/gin"
	"github.com/vnkhanh/e-podcast-backend/services"
	"google.golang.org/api/option"
)

type TTSRequest struct {
	Text         string  `json:"text" binding:"required"`
	Voice        string  `json:"voice"`
	SpeakingRate float64 `json:"speaking_rate"`
	Pitch        float64 `json:"pitch"`
}

func TextToSpeechHandler(c *gin.Context) {
	var req TTSRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	audioContent, err := services.SynthesizeText(req.Text, req.Voice, req.SpeakingRate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"voice_used":    req.Voice,
		"audio_content": base64.StdEncoding.EncodeToString(audioContent),
		"message":       "Text converted to speech successfully",
	})
}

type VoiceInfo struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	Gender       string `json:"gender"`
	Language     string `json:"language"`
	VoiceType    string `json:"voice_type"`
	SampleRateHz int32  `json:"sample_rate_hz"`
}

// GetVietnameseVoices trả danh sách toàn bộ voice tiếng Việt
func GetVietnameseVoices(c *gin.Context) {
	ctx := context.Background()
	credPath := os.Getenv("GOOGLE_CREDENTIALS_JSON")

	client, err := texttospeech.NewClient(ctx, option.WithCredentialsFile(credPath))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Không thể khởi tạo client TTS",
			"details": err.Error(),
		})
		return
	}
	defer client.Close()

	resp, err := client.ListVoices(ctx, &texttospeechpb.ListVoicesRequest{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Không thể lấy danh sách voice",
			"details": err.Error(),
		})
		return
	}

	var voices []VoiceInfo
	for _, v := range resp.Voices {
		if !contains(v.LanguageCodes, "vi-VN") {
			continue
		}

		name := v.Name
		gender := v.SsmlGender.String()
		voiceType := detectVoiceType(name)
		label := formatLabel(name, gender)

		voices = append(voices, VoiceInfo{
			Name:         name,
			Label:        label,
			Gender:       gender,
			Language:     "vi-VN",
			VoiceType:    voiceType,
			SampleRateHz: v.NaturalSampleRateHertz,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"count":  len(voices),
		"voices": voices,
	})
}

// ====== Hàm phụ ======

func contains(list []string, item string) bool {
	for _, i := range list {
		if i == item {
			return true
		}
	}
	return false
}

func detectVoiceType(name string) string {
	switch {
	case strings.Contains(name, "Neural2"):
		return "Neural2"
	case strings.Contains(name, "Wavenet"):
		return "Wavenet"
	case strings.Contains(name, "Chirp3"):
		return "Chirp3"
	case strings.Contains(name, "Standard"):
		return "Standard"
	default:
		return "Unknown"
	}
}

func formatLabel(name, gender string) string {
	short := strings.TrimPrefix(name, "vi-VN-")
	return short + " (" + strings.ToLower(gender) + ")"
}
