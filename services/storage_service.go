package services

import (
	"SocialMediaAPI/models"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/h2non/filetype"
	ftypes "github.com/h2non/filetype/types"
)

// allowedFileTypes maps h2non/filetype MIME values to their canonical extensions.
// This is the single source of truth for accepted media types, validated via
// magic-number signatures (not just extensions or Content-Type headers).
var allowedFileTypes = map[string][]string{
	"image/jpeg": {".jpg", ".jpeg"},
	"image/png":  {".png"},
	"image/gif":  {".gif"},
	"image/webp": {".webp"},
	"video/mp4":  {".mp4"},
}

// allowedExtToMIME is the reverse lookup: extension → expected MIME types.
var allowedExtToMIME = map[string][]string{
	".jpg":  {"image/jpeg"},
	".jpeg": {"image/jpeg"},
	".png":  {"image/png"},
	".gif":  {"image/gif"},
	".webp": {"image/webp"},
	".mp4":  {"video/mp4"},
}

// DetectFileType reads the file header and uses magic-number matching to
// determine the real MIME type. Returns the filetype.Type and resets the reader.
// This is exported so the handler layer can also do early content-based checks.
func DetectFileType(file multipart.File) (ftypes.Type, error) {
	// filetype needs at least 262 bytes; we read 512 to be safe.
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil {
		return ftypes.Unknown, fmt.Errorf("unable to read file header for type detection: %w", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		return ftypes.Unknown, fmt.Errorf("unable to reset file reader: %w", err)
	}

	kind, err := filetype.Match(buf[:n])
	if err != nil {
		return ftypes.Unknown, fmt.Errorf("file type detection failed: %w", err)
	}

	return kind, nil
}

// IsAllowedMIME checks whether a MIME string is in the allowed set.
func IsAllowedMIME(mime string) bool {
	_, ok := allowedFileTypes[mime]
	return ok
}

type StorageService struct {
	uploadDir         string
	baseURL           string
	maxImageSize      int64
	maxVideoSize      int64
}

func NewStorageService(uploadDir, baseURL string, maxImageSize, maxVideoSize int64) (*StorageService, error) {
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return nil, err
	}

	return &StorageService{
		uploadDir:    uploadDir,
		baseURL:      baseURL,
		maxImageSize: maxImageSize,
		maxVideoSize: maxVideoSize,
	}, nil
}

func (s *StorageService) SaveFile(file multipart.File, header *multipart.FileHeader, userID string) (*models.Media, error) {
	// Reject empty files
	if header.Size == 0 {
		return nil, fmt.Errorf("empty files are not allowed")
	}

	// --- Extension validation ---
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		return nil, fmt.Errorf("file must have an extension (e.g. .jpg, .png, .mp4)")
	}
	acceptableMIMEs, extAllowed := allowedExtToMIME[ext]
	if !extAllowed {
		return nil, fmt.Errorf("file extension %s is not allowed; accepted: .jpg, .jpeg, .png, .gif, .webp, .mp4", ext)
	}

	// --- Magic-number file type detection (h2non/filetype) ---
	kind, err := DetectFileType(file)
	if err != nil {
		return nil, err
	}

	detectedMIME := kind.MIME.Value
	if kind == ftypes.Unknown || !IsAllowedMIME(detectedMIME) {
		return nil, fmt.Errorf(
			"file content does not match any allowed type (detected: %s); accepted: image/jpeg, image/png, image/gif, image/webp, video/mp4",
			detectedMIME,
		)
	}

	// --- Cross-validate: extension must match the detected MIME type ---
	mimeMatchesExt := false
	for _, m := range acceptableMIMEs {
		if m == detectedMIME {
			mimeMatchesExt = true
			break
		}
	}
	if !mimeMatchesExt {
		return nil, fmt.Errorf("file extension %s does not match detected content type %s; possible file spoofing", ext, detectedMIME)
	}

	// --- Determine media type and enforce per-type size limits ---
	var mediaType models.MediaType
	if strings.HasPrefix(detectedMIME, "image/") {
		mediaType = models.MediaImage
		if header.Size > s.maxImageSize {
			return nil, fmt.Errorf("image size %d bytes exceeds maximum allowed size of %d bytes (%d MB)",
				header.Size, s.maxImageSize, s.maxImageSize/(1<<20))
		}
	} else if strings.HasPrefix(detectedMIME, "video/") {
		mediaType = models.MediaVideo
		if header.Size > s.maxVideoSize {
			return nil, fmt.Errorf("video size %d bytes exceeds maximum allowed size of %d bytes (%d MB)",
				header.Size, s.maxVideoSize, s.maxVideoSize/(1<<20))
		}
	}

	// --- Sanitize filename: use only the validated extension, discard original name ---
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("failed to generate secure filename: %w", err)
	}
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

	// Copy with a size-limited reader to prevent the actual bytes from exceeding the declared size.
	// This guards against tampered Content-Length headers.
	var maxSize int64
	if mediaType == models.MediaVideo {
		maxSize = s.maxVideoSize
	} else {
		maxSize = s.maxImageSize
	}
	limitedReader := io.LimitReader(file, maxSize+1)
	written, err := io.Copy(dst, limitedReader)
	if err != nil {
		os.Remove(filePath)
		return nil, fmt.Errorf("error writing file: %w", err)
	}
	if written > maxSize {
		os.Remove(filePath)
		return nil, fmt.Errorf("file stream exceeded maximum allowed size of %d MB", maxSize/(1<<20))
	}

	media := &models.Media{
		ID:        uuid.New().String(),
		UserID:    userID,
		Filename:  filename,
		Path:      filePath,
		URL:       fmt.Sprintf("%s/uploads/%s/%s", s.baseURL, userID, filename),
		Type:      mediaType,
		Size:      written,
		MimeType:  detectedMIME,
		CreatedAt: time.Now(),
	}

	return media, nil
}

func (s *StorageService) DeleteFile(media *models.Media) error {
	return os.Remove(media.Path)
}