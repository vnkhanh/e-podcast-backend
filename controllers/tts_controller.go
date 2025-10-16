package controllers

import (
	"encoding/base64"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vnkhanh/e-podcast-backend/services"
)

type TTSRequest struct {
	Text         string  `json:"text" binding:"required"`
	Voice        string  `json:"voice"`
	SpeakingRate float64 `json:"speaking_rate"`
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
