package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	texttospeechpb "cloud.google.com/go/texttospeech/apiv1/texttospeechpb"
	"google.golang.org/api/option"
)

const maxChunkBytesLow = 4500

var tmpDir = os.TempDir() // ✅ dùng thư mục tạm hợp lệ của hệ thống

// SynthesizeText - Tổng hợp giọng nói, nén cực mạnh (<50 MB)
func SynthesizeText(text, voice string, rate float64) ([]byte, error) {
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
	client, err := newTTSClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	chunks := splitTextToChunksByByte(text, maxChunkBytesLow)
	fmt.Printf("[LOW-QUALITY] Tổng hợp %d đoạn...\n", len(chunks))

	tmpFiles := make([]string, len(chunks))
	errs := make(chan error, len(chunks))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3) // tối đa 3 luồng song song

	for i, chunk := range chunks {
		wg.Add(1)
		sem <- struct{}{}

		go func(idx int, txt string) {
			defer wg.Done()
			defer func() { <-sem }()

			req := &texttospeechpb.SynthesizeSpeechRequest{
				Input: &texttospeechpb.SynthesisInput{
					InputSource: &texttospeechpb.SynthesisInput_Text{Text: txt},
				},
				Voice: &texttospeechpb.VoiceSelectionParams{
					LanguageCode: "vi-VN",
					Name:         voice,
				},
				AudioConfig: &texttospeechpb.AudioConfig{
					AudioEncoding:   texttospeechpb.AudioEncoding_OGG_OPUS,
					SpeakingRate:    rate,
					SampleRateHertz: 16000,
				},
			}

			resp, err := client.SynthesizeSpeech(ctx, req)
			if err != nil {
				errs <- fmt.Errorf("chunk %d failed: %w", idx+1, err)
				return
			}

			tmpFile := filepath.Join(tmpDir, fmt.Sprintf("chunk_%03d_%d.opus", idx, os.Getpid()))
			if err := os.WriteFile(tmpFile, resp.AudioContent, 0o644); err != nil {
				errs <- fmt.Errorf("write chunk %d failed: %w", idx+1, err)
				return
			}
			tmpFiles[idx] = tmpFile
			fmt.Printf("✓ Chunk %d/%d (%d bytes)\n", idx+1, len(chunks), len(resp.AudioContent))
		}(i, chunk)
	}

	wg.Wait()
	close(errs)
	for e := range errs {
		return nil, e
	}

	merged, err := mergeAudioFilesLow(tmpFiles)
	cleanupTmp(tmpFiles)
	if err != nil {
		return nil, fmt.Errorf("merge failed: %w", err)
	}

	// ✅ FIX HEADER để Supabase nhận đúng thời lượng
	if err := fixMP3Header(filepath.Join(tmpDir, fmt.Sprintf("merged_%d.mp3", os.Getpid()))); err != nil {
		fmt.Println("⚠️  fixMP3Header error:", err)
	}

	compressed, err := ultraCompressAudio(merged)
	if err != nil {
		fmt.Println("Compression failed, returning merged file:", err)
		return merged, nil
	}

	fmt.Printf("[LOW-QUALITY] Final size: %.2f MB\n", float64(len(compressed))/(1024*1024))
	return compressed, nil
}

// ============================= //
//         INTERNAL FUNCS        //
// ============================= //

func newTTSClient(ctx context.Context) (*texttospeech.Client, error) {
	cred := os.Getenv("GOOGLE_CREDENTIALS_JSON")
	if cred == "" {
		return nil, errors.New("GOOGLE_CREDENTIALS_JSON not set")
	}
	if strings.HasPrefix(strings.TrimSpace(cred), "{") {
		return texttospeech.NewClient(ctx, option.WithCredentialsJSON([]byte(cred)))
	}
	return texttospeech.NewClient(ctx, option.WithCredentialsFile(cred))
}

func splitTextToChunksByByte(text string, maxBytes int) []string {
	var chunks []string
	remaining := text
	for len(remaining) > 0 {
		if len(remaining) <= maxBytes {
			chunks = append(chunks, remaining)
			break
		}
		cutPos := maxBytes
		for i := cutPos; i > 0; i-- {
			if strings.ContainsRune(".!?\n", rune(remaining[i-1])) {
				cutPos = i
				break
			}
		}
		for cutPos < len(remaining) && (remaining[cutPos]&0xC0) == 0x80 {
			cutPos++
		}
		chunks = append(chunks, remaining[:cutPos])
		remaining = remaining[cutPos:]
	}
	return chunks
}

// mergeAudioFilesLow - Hợp nhất nhiều file OGG/OPUS thành 1 file MP3 16kHz, mono, bitrate thấp
func mergeAudioFilesLow(files []string) ([]byte, error) {
	listFile := filepath.Join(tmpDir, fmt.Sprintf("merge_list_%d.txt", os.Getpid()))
	outputFile := filepath.Join(tmpDir, fmt.Sprintf("merged_%d.mp3", os.Getpid()))
	defer os.Remove(listFile)
	defer os.Remove(outputFile)

	var listContent strings.Builder
	for _, f := range files {
		if f != "" {
			listContent.WriteString(fmt.Sprintf("file '%s'\n", f))
		}
	}
	if err := os.WriteFile(listFile, []byte(listContent.String()), 0o644); err != nil {
		return nil, err
	}

	// ✅ Thêm -write_xing 1 và -fflags +bitexact để header chính xác
	cmd := exec.Command("ffmpeg",
		"-f", "concat", "-safe", "0",
		"-i", listFile,
		"-acodec", "libmp3lame",
		"-b:a", "32k",
		"-ar", "16000",
		"-ac", "1",
		"-write_xing", "1",
		"-fflags", "+bitexact",
		"-y", outputFile,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg merge error: %v, %s", err, stderr.String())
	}

	return os.ReadFile(outputFile)
}

// ultraCompressAudio - Nén cực mạnh (24kbps, mono, 16kHz)
func ultraCompressAudio(data []byte) ([]byte, error) {
	tmpIn := filepath.Join(tmpDir, fmt.Sprintf("in_%d.opus", os.Getpid()))
	tmpOut := filepath.Join(tmpDir, fmt.Sprintf("out_%d.mp3", os.Getpid()))
	defer os.Remove(tmpIn)
	defer os.Remove(tmpOut)

	if err := os.WriteFile(tmpIn, data, 0o644); err != nil {
		return nil, err
	}

	cmd := exec.Command("ffmpeg",
		"-i", tmpIn,
		"-codec:a", "libmp3lame",
		"-b:a", "24k",
		"-ar", "16000",
		"-ac", "1",
		"-q:a", "9",
		"-write_xing", "1", // ✅ Bảo đảm header chính xác sau nén
		"-fflags", "+bitexact",
		"-y",
		tmpOut,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg compress error: %v, %s", err, stderr.String())
	}

	return os.ReadFile(tmpOut)
}

// fixMP3Header - Rebuild header duration để hiển thị đúng thời lượng
func fixMP3Header(inputPath string) error {
	temp := inputPath + ".fixed.mp3"
	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-acodec", "copy",
		"-write_xing", "1",
		"-fflags", "+bitexact",
		"-y", temp,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("fixMP3Header error: %v, %s", err, stderr.String())
	}
	return os.Rename(temp, inputPath)
}

func cleanupTmp(files []string) {
	for _, f := range files {
		if f != "" {
			os.Remove(f)
		}
	}
}
