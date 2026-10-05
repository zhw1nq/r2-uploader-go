package handler

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"r2-uploader-go/internal/model"

	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func (s *Server) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed. Use POST")
		return
	}

	if s.R2 == nil {
		writeError(w, http.StatusServiceUnavailable, "Cloudflare R2 is not configured")
		return
	}

	// 64MB memory limit for multipart parsing
	if err := r.ParseMultipartForm(1024 * 1024 * 64); err != nil {
		writeError(w, http.StatusBadRequest, "Failed to parse multipart form: "+err.Error())
		return
	}

	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		files = r.MultipartForm.File["files"]
	}
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, "No files uploaded (field 'file' or 'files' required)")
		return
	}

	folderPrefix := strings.TrimSpace(r.FormValue("folder"))
	folderPrefix = strings.Trim(path.Clean("/"+folderPrefix), "/")
	if folderPrefix == "." {
		folderPrefix = ""
	}
	if folderPrefix != "" {
		folderPrefix = folderPrefix + "/"
	}

	var results []model.UploadResult
	baseURL := s.GetBaseURL(r)
	clientIP := getClientIP(r)
	token := getSessionToken(r)
	userAgent := r.UserAgent()

	// Process each file with immediate resource cleanup (no defer leak in loop)
	for _, fileHeader := range files {
		res, err := s.processSingleUpload(r, fileHeader, folderPrefix, baseURL, clientIP, token, userAgent)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		results = append(results, *res)
	}

	resp := map[string]any{
		"message": "Files uploaded successfully",
		"count":   len(results),
		"files":   results,
	}
	if len(results) == 1 {
		resp["url"] = results[0].URL
		resp["download_url"] = results[0].DownloadURL
		resp["public_url"] = results[0].PublicURL
		resp["file"] = results[0]
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) processSingleUpload(
	r *http.Request,
	fileHeader *multipart.FileHeader,
	folderPrefix, baseURL, clientIP, token, userAgent string,
) (*model.UploadResult, error) {
	srcFile, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open file '%s': %w", fileHeader.Filename, err)
	}
	defer srcFile.Close()

	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		ext := filepath.Ext(fileHeader.Filename)
		contentType = mime.TypeByExtension(ext)
		if contentType == "" {
			buf := make([]byte, 512)
			n, _ := srcFile.Read(buf)
			contentType = http.DetectContentType(buf[:n])
			_, _ = srcFile.Seek(0, io.SeekStart)
		}
	}

	relKey := folderPrefix + fileHeader.Filename

	h := md5.New()
	tr := io.TeeReader(srcFile, h)

	fullKey, etag, err := s.R2.Upload(r.Context(), relKey, contentType, tr)
	if err != nil {
		return nil, fmt.Errorf("upload to Cloudflare R2 failed: %w", err)
	}

	md5Hex := hex.EncodeToString(h.Sum(nil))
	if etag == "" {
		etag = md5Hex
	}

	fileURL := fmt.Sprintf("%s/api/file?key=%s", baseURL, relKey)

	var publicURL string
	if s.Cfg.PublicDomain != "" {
		publicURL = fmt.Sprintf("%s/%s", strings.TrimRight(s.Cfg.PublicDomain, "/"), fullKey)
	}

	_ = s.Sessions.LogActivity(
		"UPLOAD", "SUCCESS", token, relKey, fileHeader.Filename,
		fileHeader.Size, clientIP, userAgent,
		fmt.Sprintf("Uploaded '%s' (%s, %d bytes)", fileHeader.Filename, contentType, fileHeader.Size),
	)

	return &model.UploadResult{
		Name:         fileHeader.Filename,
		OriginalName: fileHeader.Filename,
		Key:          relKey,
		FullKey:      fullKey,
		Bucket:       s.Cfg.BucketName,
		Size:         fileHeader.Size,
		ContentType:  contentType,
		ETag:         etag,
		MD5Hash:      md5Hex,
		StorageClass: "STANDARD",
		URL:          fileURL,
		DownloadURL:  fileURL,
		PublicURL:    publicURL,
		UploadedAt:   time.Now().UTC(),
	}, nil
}

// HandleListFiles returns directory listing with subfolders and files
func (s *Server) HandleListFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed. Use GET")
		return
	}

	if s.R2 == nil {
		writeError(w, http.StatusServiceUnavailable, "Cloudflare R2 is not configured")
		return
	}

	subPath := r.URL.Query().Get("path")
	listing, err := s.R2.ListDirectory(r.Context(), subPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list directory from R2: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, listing)
}

// HandleFolder creates or deletes folders
func (s *Server) HandleFolder(w http.ResponseWriter, r *http.Request) {
	if s.R2 == nil {
		writeError(w, http.StatusServiceUnavailable, "Cloudflare R2 is not configured")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
			return
		}

		cleanPath := strings.Trim(path.Clean("/"+req.Path), "/")
		if cleanPath == "" || cleanPath == "." {
			writeError(w, http.StatusBadRequest, "Folder path cannot be empty")
			return
		}

		if err := s.R2.CreateFolder(r.Context(), cleanPath); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to create folder on R2: "+err.Error())
			return
		}

		_ = s.Sessions.LogActivity("CREATE_FOLDER", "SUCCESS", getSessionToken(r), cleanPath, cleanPath, 0, getClientIP(r), r.UserAgent(), "Created folder '/"+cleanPath+"'")

		writeJSON(w, http.StatusCreated, map[string]string{
			"message": "Folder created successfully",
			"path":    cleanPath,
		})

	case http.MethodDelete:
		token := getSessionToken(r)
		valid, _ := s.Sessions.ValidateKey(token, "delete")
		if !valid && s.Cfg.AdminKey != "" && token != s.Cfg.AdminKey {
			writeError(w, http.StatusForbidden, "Session key lacks 'delete' permission")
			return
		}

		folderPath := r.URL.Query().Get("path")
		cleanPath := strings.Trim(path.Clean("/"+folderPath), "/")
		if cleanPath == "" || cleanPath == "." {
			writeError(w, http.StatusBadRequest, "Invalid folder path")
			return
		}

		if err := s.R2.DeleteFolder(r.Context(), cleanPath); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to delete folder on R2: "+err.Error())
			return
		}

		_ = s.Sessions.LogActivity("DELETE_FOLDER", "SUCCESS", token, cleanPath, cleanPath, 0, getClientIP(r), r.UserAgent(), "Deleted folder '/"+cleanPath+"' and all contents")

		writeJSON(w, http.StatusOK, map[string]string{
			"message": fmt.Sprintf("Folder '%s' deleted successfully", cleanPath),
		})

	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// HandleFile streams or deletes a single file
func (s *Server) HandleFile(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "Missing required 'key' query parameter")
		return
	}

	if s.R2 == nil {
		writeError(w, http.StatusServiceUnavailable, "Cloudflare R2 is not configured")
		return
	}

	switch r.Method {
	case http.MethodGet, http.MethodHead:
		rangeHeader := r.Header.Get("Range")
		obj, err := s.R2.GetObject(r.Context(), key, rangeHeader)
		if err != nil {
			var notFound *s3types.NoSuchKey
			if errors.As(err, &notFound) {
				writeError(w, http.StatusNotFound, "Object not found on Cloudflare R2")
				return
			}
			writeError(w, http.StatusInternalServerError, "Failed to fetch object: "+err.Error())
			return
		}
		defer obj.Body.Close()

		if obj.ContentType != nil {
			w.Header().Set("Content-Type", *obj.ContentType)
		}
		if obj.ContentLength != nil {
			w.Header().Set("Content-Length", strconv.FormatInt(*obj.ContentLength, 10))
		}
		if obj.ContentRange != nil {
			w.Header().Set("Content-Range", *obj.ContentRange)
			w.WriteHeader(http.StatusPartialContent)
		}
		if obj.ETag != nil {
			w.Header().Set("ETag", *obj.ETag)
		}

		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Cache-Control", "public, max-age=86400")

		if r.Method == http.MethodGet {
			_, _ = io.Copy(w, obj.Body)
		}

	case http.MethodDelete:
		token := getSessionToken(r)
		valid, _ := s.Sessions.ValidateKey(token, "delete")
		if !valid && s.Cfg.AdminKey != "" && token != s.Cfg.AdminKey {
			writeError(w, http.StatusForbidden, "Session key lacks 'delete' permission")
			return
		}

		if err := s.R2.DeleteObject(r.Context(), key); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to delete object: "+err.Error())
			return
		}

		_ = s.Sessions.LogActivity("DELETE_FILE", "SUCCESS", token, key, filepath.Base(key), 0, getClientIP(r), r.UserAgent(), "Permanently deleted object '"+key+"'")

		writeJSON(w, http.StatusOK, map[string]string{
			"message": fmt.Sprintf("Object '%s' deleted successfully", key),
		})

	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) HandlePresign(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "Missing required 'key' query parameter")
		return
	}

	expiresSec := 3600
	if expStr := r.URL.Query().Get("expires"); expStr != "" {
		if parsed, err := strconv.Atoi(expStr); err == nil && parsed > 0 && parsed <= 604800 {
			expiresSec = parsed
		}
	}

	url, err := s.R2.GeneratePresignedURL(r.Context(), key, expiresSec)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate presigned URL: "+err.Error())
		return
	}

	token := getSessionToken(r)
	_ = s.Sessions.LogActivity("PRESIGN", "SUCCESS", token, key, filepath.Base(key), 0, getClientIP(r), r.UserAgent(), fmt.Sprintf("Generated presigned URL valid for %ds", expiresSec))

	writeJSON(w, http.StatusOK, map[string]any{
		"key":        key,
		"url":        url,
		"expires_in": expiresSec,
	})
}

// HandleFileInfo returns detailed metadata, ETag/MD5 hash, size, and R2 storage info for a file
func (s *Server) HandleFileInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if s.R2 == nil {
		writeError(w, http.StatusServiceUnavailable, "Cloudflare R2 is not configured")
		return
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "Missing required 'key' parameter")
		return
	}

	head, err := s.R2.HeadObject(r.Context(), key)
	if err != nil {
		writeError(w, http.StatusNotFound, "Object not found on Cloudflare R2: "+err.Error())
		return
	}

	fullKey := s.R2.ResolveKey(key)
	var publicURL string
	if s.Cfg.PublicDomain != "" {
		publicURL = fmt.Sprintf("%s/%s", strings.TrimRight(s.Cfg.PublicDomain, "/"), fullKey)
	}

	var size int64
	if head.ContentLength != nil {
		size = *head.ContentLength
	}

	var contentType string
	if head.ContentType != nil {
		contentType = *head.ContentType
	}
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(key))
	}

	var lastModified time.Time
	if head.LastModified != nil {
		lastModified = *head.LastModified
	}

	etag := ""
	if head.ETag != nil {
		etag = strings.Trim(*head.ETag, `"`)
	}

	storageClass := "STANDARD"
	if head.StorageClass != "" {
		storageClass = string(head.StorageClass)
	}

	baseURL := s.GetBaseURL(r)

	writeJSON(w, http.StatusOK, map[string]any{
		"name":          filepath.Base(key),
		"key":           key,
		"full_key":      fullKey,
		"bucket":        s.Cfg.BucketName,
		"size":          size,
		"content_type":  contentType,
		"etag":          etag,
		"md5_hash":      etag,
		"last_modified": lastModified,
		"storage_class": storageClass,
		"public_url":    publicURL,
		"download_url":  fmt.Sprintf("%s/api/file?key=%s", baseURL, key),
		"metadata":      head.Metadata,
	})
}
