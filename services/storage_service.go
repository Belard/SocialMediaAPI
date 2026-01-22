package services

import (
	"SocialMediaAPI/models"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

var allowedTypes = map[string]bool{
	"image/jpeg": true,
	"image/jpg":  true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
	"video/mp4":  true,
}

type StorageService struct {
	uploadDir     string
	baseURL       string
	maxUploadSize int64
}

func NewStorageService(uploadDir, baseURL string) (*StorageService, error) {
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return nil, err
	}

	return &StorageService{
		uploadDir:     uploadDir,
		baseURL:       baseURL,
		maxUploadSize: 10 << 20, // 10 MB
	}, nil
}

func (s *StorageService) SaveFile(file multipart.File, header *multipart.FileHeader, userID string) (*models.Media, error) {
	if header.Size > s.maxUploadSize {
		return nil, fmt.Errorf("file size exceeds maximum allowed size of 10MB")
	}

	buffer := make([]byte, 512)
	_, err := file.Read(buffer)
	if err != nil {
		return nil, err
	}
	file.Seek(0, 0)

	mimeType := http.DetectContentType(buffer)
	if !allowedTypes[mimeType] {
		return nil, fmt.Errorf("file type %s not allowed", mimeType)
	}

	var mediaType models.MediaType
	if strings.HasPrefix(mimeType, "image/") {
		mediaType = models.MediaImage
	} else if strings.HasPrefix(mimeType, "video/") {
		mediaType = models.MediaVideo
	}

	ext := filepath.Ext(header.Filename)
	randomBytes := make([]byte, 16)
	rand.Read(randomBytes)
	filename := hex.EncodeToString(randomBytes) + ext

	userDir := filepath.Join(s.uploadDir, userID)
	if err := os.MkdirAll(userDir, 0755); err != nil {
		return nil, err
	}

	filePath := filepath.Join(userDir, filename)
	dst, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		return nil, err
	}

	media := &models.Media{
		ID:        uuid.New().String(),
		UserID:    userID,
		Filename:  filename,
		Path:      filePath,
		URL:       fmt.Sprintf("%s/uploads/%s/%s", s.baseURL, userID, filename),
		Type:      mediaType,
		Size:      header.Size,
		MimeType:  mimeType,
		CreatedAt: time.Now(),
	}

	return media, nil
}

func (s *StorageService) DeleteFile(media *models.Media) error {
	return os.Remove(media.Path)
}