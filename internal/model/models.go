package model

import "time"

// SessionKey represents an authenticated API session
type SessionKey struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Key         string     `json:"key"`
	Permissions []string   `json:"permissions"` // "upload", "read", "delete", "admin"
	IsActive    bool       `json:"is_active"`
	IsMaster    bool       `json:"is_master"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// UploadLog represents an upload event record
type UploadLog struct {
	ID          int64     `json:"id"`
	SessionID   string    `json:"session_id"`
	SessionName string    `json:"session_name"`
	FileKey     string    `json:"file_key"`
	FileName    string    `json:"file_name"`
	FileSize    int64     `json:"file_size"`
	ContentType string    `json:"content_type"`
	IPAddress   string    `json:"ip_address"`
	CreatedAt   time.Time `json:"created_at"`
}

// ActivityLog represents a detailed audit log entry
type ActivityLog struct {
	ID          int64     `json:"id"`
	Action      string    `json:"action"` // UPLOAD, DELETE_FILE, CREATE_FOLDER, DELETE_FOLDER, DOWNLOAD, PRESIGN, CREATE_KEY, DELETE_KEY, UPDATE_KEY
	Status      string    `json:"status"` // SUCCESS, FAILED
	SessionID   string    `json:"session_id"`
	SessionName string    `json:"session_name"`
	TargetKey   string    `json:"target_key"`
	FileName    string    `json:"file_name,omitempty"`
	FileSize    int64     `json:"file_size"`
	IPAddress   string    `json:"ip_address"`
	UserAgent   string    `json:"user_agent"`
	Details     string    `json:"details"`
	CreatedAt   time.Time `json:"created_at"`
}

// UploadResult represents the response payload for an uploaded file
type UploadResult struct {
	Name         string            `json:"name"`
	OriginalName string            `json:"original_name"`
	Key          string            `json:"key"` // Relative to root folder
	FullKey      string            `json:"full_key"`
	Bucket       string            `json:"bucket"`
	Size         int64             `json:"size"`
	ContentType  string            `json:"content_type"`
	ETag         string            `json:"etag"`
	MD5Hash      string            `json:"md5_hash"`
	StorageClass string            `json:"storage_class"`
	URL          string            `json:"url"`
	DownloadURL  string            `json:"download_url"`
	PublicURL    string            `json:"public_url,omitempty"`
	UploadedAt   time.Time         `json:"uploaded_at"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// FileItem represents an individual object in storage
type FileItem struct {
	Key          string    `json:"key"` // Relative to RootFolder
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	ETag         string    `json:"etag"`
	URL          string    `json:"url"`
	PublicURL    string    `json:"public_url,omitempty"`
}

// FolderItem represents a virtual directory prefix
type FolderItem struct {
	Name string `json:"name"`
	Path string `json:"path"` // Relative to RootFolder
}

// DirectoryListing represents files and folders within a folder scope
type DirectoryListing struct {
	RootFolder  string       `json:"root_folder"`
	CurrentPath string       `json:"current_path"` // Relative to RootFolder
	Folders     []FolderItem `json:"folders"`
	Files       []FileItem   `json:"files"`
}

// StorageStats represents overall storage consumption and file counts
type StorageStats struct {
	Bucket     string `json:"bucket"`
	RootFolder string `json:"root_folder"`
	TotalFiles int64  `json:"total_files"`
	TotalSize  int64  `json:"total_size"`
	Health     string `json:"health"`
}

// HealthStatus represents service health check result
type HealthStatus struct {
	Status    string    `json:"status"`
	Version   string    `json:"version"`
	Uptime    string    `json:"uptime"`
	Database  string    `json:"database"`
	Storage   string    `json:"storage"`
	Timestamp time.Time `json:"timestamp"`
}
