package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

func CallVITSTTS(text string) (string, error) {
	baseURL := "http://localhost:5004/tts"

	params := url.Values{}
	params.Add("text", text)
	params.Add("speed", "normal")

	reqURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())
	resp, err := http.Get(reqURL)
	if err != nil {
		return "", fmt.Errorf("lỗi gọi VITS: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("VITS lỗi %d: %s", resp.StatusCode, string(body))
	}

	var data struct {
		AudioURL string `json:"audio_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("lỗi đọc JSON từ VITS: %v", err)
	}

	if data.AudioURL == "" {
		return "", fmt.Errorf("VITS không trả về audio_url")
	}

	return data.AudioURL, nil
}
