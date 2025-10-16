// package services
package services

import (
	"context"
	"errors"
	"fmt"
	"os"

	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	texttospeechpb "cloud.google.com/go/texttospeech/apiv1/texttospeechpb"
	"google.golang.org/api/option"
)

// SynthesizeText chuyển text thành audio []byte
func SynthesizeText(text string, voice string, rate float64) ([]byte, error) {
	if len(text) == 0 {
		return nil, errors.New("text is empty")
	}
	if voice == "" {
		voice = "vi-VN-Chirp3-HD-Puck"
	}
	if rate <= 0 {
		rate = 1.0
	}

	ctx := context.Background()
	credPath := os.Getenv("GOOGLE_CREDENTIALS_JSON")
	if credPath == "" {
		return nil, errors.New("GOOGLE_CREDENTIALS_JSON environment variable is not set")
	}

	// client, err := texttospeech.NewClient(ctx, option.WithCredentialsJSON([]byte(jsonCreds)))//Nào deploy mới dùng
	client, err := texttospeech.NewClient(ctx, option.WithCredentialsFile(credPath))

	if err != nil {
		return nil, err
	}
	defer client.Close()

	chunks := splitTextToChunksByByte(text, 4500) // Dưới ngưỡng 5000 bytes
	var allAudio []byte

	for idx, chunk := range chunks {
		fmt.Printf("Synthesizing chunk %d/%d: %d bytes\n", idx+1, len(chunks), len(chunk))

		req := &texttospeechpb.SynthesizeSpeechRequest{
			Input: &texttospeechpb.SynthesisInput{
				InputSource: &texttospeechpb.SynthesisInput_Text{
					Text: chunk,
				},
			},
			Voice: &texttospeechpb.VoiceSelectionParams{
				LanguageCode: "vi-VN",
				Name:         voice,
			},
			AudioConfig: &texttospeechpb.AudioConfig{
				AudioEncoding: texttospeechpb.AudioEncoding_MP3,
				SpeakingRate:  rate,
			},
		}

		resp, err := client.SynthesizeSpeech(ctx, req)
		if err != nil {
			return nil, err
		}
		allAudio = append(allAudio, resp.AudioContent...)
	}

	return allAudio, nil
}

// splitTextToChunksByByte chia text theo giới hạn byte + dấu câu
func splitTextToChunksByByte(text string, maxBytes int) []string {
	var chunks []string
	remaining := text

	for len(remaining) > 0 {
		if len(remaining) <= maxBytes {
			chunks = append(chunks, remaining)
			break
		}

		cutPos := maxBytes
		// Tìm dấu câu trong đoạn cắt được
		for i := cutPos; i > 0; i-- {
			if remaining[i-1] == '.' || remaining[i-1] == '!' || remaining[i-1] == '?' || remaining[i-1] == '\n' {
				cutPos = i
				break
			}
		}

		// Nếu không tìm thấy dấu câu, đảm bảo không cắt giữa ký tự UTF-8
		for cutPos < len(remaining) && (remaining[cutPos]&0xC0) == 0x80 {
			cutPos++
		}

		chunks = append(chunks, remaining[:cutPos])
		remaining = remaining[cutPos:]
	}

	return chunks
}
