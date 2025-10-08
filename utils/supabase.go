package utils

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	storage "github.com/supabase-community/storage-go"
)

// UploadFileToSupabase uploads a document (e.g. .pdf, .txt, .docx) to Supabase Storage
// Path: uploads/documents/<fileID>.<ext>
func UploadFileToSupabase(fileHeader *multipart.FileHeader, fileID string) (string, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_KEY")

	storageClient := storage.NewClient(supabaseURL+"/storage/v1", supabaseKey, nil)

	file, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()

	ext := filepath.Ext(fileHeader.Filename)
	objectPath := fmt.Sprintf("documents/%s%s", fileID, ext) // Path dưới bucket uploads

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, file); err != nil {
		return "", err
	}

	contentType := fileHeader.Header.Get("Content-Type")
	options := storage.FileOptions{
		ContentType: &contentType,
	}

	// Upload to bucket 'uploads', path: documents/<fileID>.<ext>
	_, err = storageClient.UploadFile("uploads", objectPath, &buf, options)
	if err != nil {
		return "", err
	}

	// Public URL: uploads/documents/<filename>
	publicURL := fmt.Sprintf("%s/storage/v1/object/public/uploads/%s", supabaseURL, objectPath)
	return publicURL, nil
}

// UploadBytesToSupabase uploads byte data (e.g. .mp3) to Supabase Storage
// Path: uploads/audio/<filename>.mp3
func UploadBytesToSupabase(data []byte, filename string, contentType string) (string, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_KEY")

	storageClient := storage.NewClient(supabaseURL+"/storage/v1", supabaseKey, nil)

	objectPath := fmt.Sprintf("audio/%s", filename) // Path dưới bucket uploads
	buf := bytes.NewBuffer(data)

	options := storage.FileOptions{
		ContentType: &contentType,
	}

	_, err := storageClient.UploadFile("uploads", objectPath, buf, options)
	if err != nil {
		return "", err
	}

	// Public URL: uploads/audio/<filename>.mp3
	publicURL := fmt.Sprintf("%s/storage/v1/object/public/uploads/%s", supabaseURL, objectPath)
	return publicURL, nil
}

// UploadImageToSupabase uploads an image (e.g. .jpg, .png) to Supabase Storage
// Path: uploads/images/<fileID>.<ext>
func UploadImageToSupabase(fileHeader *multipart.FileHeader, fileID string) (string, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_KEY")

	storageClient := storage.NewClient(supabaseURL+"/storage/v1", supabaseKey, nil)

	file, err := fileHeader.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()

	ext := filepath.Ext(fileHeader.Filename)
	objectPath := fmt.Sprintf("images/%s%s", fileID, ext) // uploads/images/<fileID>.jpg

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, file); err != nil {
		return "", err
	}

	contentType := fileHeader.Header.Get("Content-Type")
	options := storage.FileOptions{
		ContentType: &contentType,
	}

	_, err = storageClient.UploadFile("uploads", objectPath, &buf, options)
	if err != nil {
		return "", err
	}

	publicURL := fmt.Sprintf("%s/storage/v1/object/public/uploads/%s", supabaseURL, objectPath)
	return publicURL, nil
}

// DeleteFileFromSupabase nhận public URL hoặc đường dẫn chứa "/storage/v1/object/"
// và gọi API Supabase Storage để xóa object.
// Yêu cầu: SUPABASE_URL và SUPABASE_KEY (service role / anon key có quyền xóa) đã set trong ENV.
func DeleteFileFromSupabase(publicURL string) error {
	if publicURL == "" {
		return nil
	}

	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_KEY")
	if supabaseURL == "" || supabaseKey == "" {
		return fmt.Errorf("SUPABASE_URL hoặc SUPABASE_KEY chưa cấu hình")
	}

	// Tìm phần "/storage/v1/object/" trong URL
	idx := strings.Index(publicURL, "/storage/v1/object/")
	if idx == -1 {
		// Nếu không chứa đường dẫn chuẩn, thử lấy phần path sau hostname
		// (hoặc trả lỗi để an toàn)
		return fmt.Errorf("không xác định được đường dẫn object trong URL: %s", publicURL)
	}

	rest := publicURL[idx+len("/storage/v1/object/"):]
	// Luôn bỏ prefix "public/" nếu có
	rest = strings.TrimPrefix(rest, "public/")

	// rest => "<bucket>/<path/to/object...>"
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) < 2 {
		return fmt.Errorf("không parse được bucket/object từ URL: %s", publicURL)
	}
	bucket := parts[0]
	object := parts[1]
	// bỏ query params nếu có
	if qIdx := strings.Index(object, "?"); qIdx != -1 {
		object = object[:qIdx]
	}
	// unescape path
	if u, err := url.PathUnescape(object); err == nil {
		object = u
	}

	deleteURL := fmt.Sprintf("%s/storage/v1/object/%s/%s", strings.TrimRight(supabaseURL, "/"), bucket, object)

	req, err := http.NewRequest("DELETE", deleteURL, nil)
	if err != nil {
		return err
	}
	// Supabase expects Authorization: Bearer <SERVICE_KEY> and apikey header
	req.Header.Set("Authorization", "Bearer "+supabaseKey)
	req.Header.Set("apikey", supabaseKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	// Supabase trả 200 hoặc 204 khi xóa thành công
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("xóa file Supabase thất bại: status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}
