package controllers

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vnkhanh/e-podcast-backend/utils" // nơi chứa UploadBytesToSupabase
)

// TTSRequest struct để bind JSON request
type TTSRequest struct {
	Text string `json:"text" binding:"required"`
}

// GenerateTTS gọi HuggingFace API và upload audio lên Supabase
func GenerateTTS(c *gin.Context) {
	var reqBody TTSRequest
	if err := c.ShouldBindJSON(&reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Thiếu trường text"})
		return
	}

	apiKey := os.Getenv("HUGGINGFACE_API_KEY")
	if apiKey == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Chưa set HUGGINGFACE_API_KEY trong .env"})
		return
	}

	// Chuẩn bị request gửi Hugging Face
	payload := []byte(fmt.Sprintf(`{"inputs": "%s"}`, reqBody.Text))
	req, _ := http.NewRequest("POST",
		"https://api-inference.huggingface.co/models/capleaf/viXTTS",
		bytes.NewBuffer(payload))

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không gọi được API Hugging Face"})
		return
	}
	defer resp.Body.Close()

	// Đọc toàn bộ audio response thành []byte
	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không đọc được dữ liệu audio"})
		return
	}

	// Tạo tên file duy nhất
	filename := fmt.Sprintf("tts_%d.wav", time.Now().Unix())

	// Upload lên Supabase
	audioURL, err := utils.UploadBytesToSupabase(audioBytes, filename, "audio/wav")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Upload audio lên Supabase thất bại", "detail": err.Error()})
		return
	}

	// Trả về JSON response thay vì file trực tiếp
	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"audio_url": audioURL,
	})
}
