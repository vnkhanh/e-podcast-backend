package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
)

func CallUploadDocumentAPI(file *multipart.FileHeader, userID string, token string, voice string, speakingRate float64) (map[string]interface{}, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Ghi file upload
	fw, err := writer.CreateFormFile("file", file.Filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %v", err)
	}

	fileContent, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer fileContent.Close()
	if _, err := io.Copy(fw, fileContent); err != nil {
		return nil, fmt.Errorf("failed to copy file content: %v", err)
	}

	// Log dữ liệu file, voice và speakingRate
	fmt.Println("Dữ liệu file: ", file.Filename)
	fmt.Println("Voice: ", voice)
	fmt.Println("Speaking Rate: ", speakingRate)

	// Ghi các trường vào form
	if err := writer.WriteField("voice", voice); err != nil {
		return nil, fmt.Errorf("failed to write voice field: %v", err)
	}
	if err := writer.WriteField("speaking_rate", fmt.Sprintf("%f", speakingRate)); err != nil {
		return nil, fmt.Errorf("failed to write speaking_rate field: %v", err)
	}
	writer.Close()

	baseURL := os.Getenv("API_BASE_URL")
	if baseURL == "" {
		return nil, fmt.Errorf("API_BASE_URL is not set")
	}
	url := fmt.Sprintf("%s/api/admin/documents", baseURL)

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("user_id", userID)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upload failed (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respData, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return result, nil
}
