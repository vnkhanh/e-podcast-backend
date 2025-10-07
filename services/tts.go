package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
)

// GenerateMP3FromText dùng ElevenLabs TTS để sinh audio MP3.
// Trả về []byte (audio MP3) và thời lượng ước lượng theo số ký tự.
func GenerateMP3FromText(ctx context.Context, text, languageCode, voiceID string, speakingRate float32) ([]byte, int, error) {
	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	if apiKey == "" {
		return nil, 0, fmt.Errorf("ELEVENLABS_API_KEY not set")
	}

	if voiceID == "" {
		voiceID = "foH7s9fX31wFFH2yqrFa" // mặc định: Rachel (ElevenLabs sample voice)
	}

	// ElevenLabs endpoint
	url := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s", voiceID)

	// Payload JSON
	payload := fmt.Sprintf(`{
	"text": %q,
	"model_id": "eleven_multilingual_v2",
	"voice_settings": {
		"stability": 0.25,
		"similarity_boost": 0.9
	}
	}`, text)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer([]byte(payload)))
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")
	req.Header.Set("xi-api-key", "sk_0b89c77f2588094dafc614ffbfd80d0821faf07732da323b")

	// Gửi request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("elevenlabs error: %s - %s", resp.Status, string(body))
	}

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	// Ước lượng thời gian đọc
	est := len(text) / 14
	if est < 1 {
		est = 1
	}

	return audio, est, nil
}
