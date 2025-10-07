package utils

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"

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
