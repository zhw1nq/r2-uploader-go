package session

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"r2-uploader-go/internal/model"

	_ "modernc.org/sqlite"
)

type Store struct {
	db        *sql.DB
	mu        sync.RWMutex
	adminKey  string
	keyPrefix string
	stopChan  chan struct{}
}

func (s *Store) SetKeyPrefix(prefix string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keyPrefix = prefix
}

func NewStore(dbPath string) (*Store, error) {
	// Auto create parent directory if needed (e.g. data/)
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create sqlite directory '%s': %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Optimize SQLite performance & concurrency (WAL mode + single writer pool)
	db.SetMaxOpenConns(1)
	_, _ = db.Exec("PRAGMA journal_mode = WAL;")
	_, _ = db.Exec("PRAGMA synchronous = NORMAL;")
	_, _ = db.Exec("PRAGMA foreign_keys = ON;")

	store := &Store{
		db:       db,
		stopChan: make(chan struct{}),
	}
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize sqlite schema: %w", err)
	}

	// Initial purge: purge logs older than 7 days
	_, _ = store.PurgeOldLogs(7)

	// Periodic auto-purge ticker: runs every 1 hour in background with clean lifecycle
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_, _ = store.PurgeOldLogs(7)
			case <-store.stopChan:
				return
			}
		}
	}()

	return store, nil
}

func (s *Store) Ping(ctx context.Context) error {
	if s.db == nil {
		return errors.New("database connection is nil")
	}
	return s.db.PingContext(ctx)
}

func (s *Store) initSchema() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			key TEXT UNIQUE NOT NULL,
			permissions TEXT NOT NULL,
			is_active INTEGER NOT NULL DEFAULT 1,
			is_master INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			last_used_at DATETIME,
			expires_at DATETIME
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_key ON sessions(key);`,
		`CREATE TABLE IF NOT EXISTS activity_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'SUCCESS',
			session_id TEXT,
			session_name TEXT,
			target_key TEXT NOT NULL,
			file_name TEXT,
			file_size INTEGER NOT NULL DEFAULT 0,
			ip_address TEXT,
			user_agent TEXT,
			details TEXT,
			created_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_activity_logs_created ON activity_logs(created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_activity_logs_action ON activity_logs(action);`,
		`CREATE TABLE IF NOT EXISTS upload_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT,
			session_name TEXT,
			file_key TEXT NOT NULL,
			file_name TEXT NOT NULL,
			file_size INTEGER NOT NULL,
			content_type TEXT,
			ip_address TEXT,
			created_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_upload_logs_created ON upload_logs(created_at DESC);`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}

	// Ensure is_master column exists on sessions table
	_, _ = s.db.Exec("ALTER TABLE sessions ADD COLUMN is_master INTEGER NOT NULL DEFAULT 0;")

	// Auto migrate existing upload_logs into activity_logs if empty
	var count int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM activity_logs").Scan(&count)
	if count == 0 {
		_, _ = s.db.Exec(`
			INSERT INTO activity_logs (action, status, session_id, session_name, target_key, file_name, file_size, ip_address, details, created_at)
			SELECT 'UPLOAD', 'SUCCESS', session_id, session_name, file_key, file_name, file_size, ip_address, 'Uploaded file (' || COALESCE(content_type, 'unknown') || ')', created_at
			FROM upload_logs
		`)
	}

	return nil
}

func (s *Store) EnsureMasterKey(adminKey string) (*model.SessionKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.adminKey = adminKey
	now := time.Now()
	allPerms := []string{"upload", "read", "delete", "admin"}
	permsJSON, _ := json.Marshal(allPerms)

	if adminKey != "" {
		_, _ = s.db.Exec("DELETE FROM sessions WHERE (name = 'Master Admin Key' OR id = 'sess_master' OR is_master = 1) AND key != ?", adminKey)

		var id string
		err := s.db.QueryRow("SELECT id FROM sessions WHERE key = ?", adminKey).Scan(&id)
		if err == nil {
			_, _ = s.db.Exec("UPDATE sessions SET is_master = 1, is_active = 1, name = 'Master Admin Key', permissions = ? WHERE id = ?", string(permsJSON), id)
			return &model.SessionKey{
				ID:          id,
				Name:        "Master Admin Key",
				Key:         adminKey,
				Permissions: allPerms,
				IsActive:    true,
				IsMaster:    true,
				CreatedAt:   now,
			}, nil
		}

		masterID := "sess_master"
		_, err = s.db.Exec(`
			INSERT OR REPLACE INTO sessions (id, name, key, permissions, is_active, is_master, created_at)
			VALUES (?, ?, ?, ?, 1, 1, ?)
		`, masterID, "Master Admin Key", adminKey, string(permsJSON), now)
		if err != nil {
			return nil, err
		}
		log.Printf("[INFO] Master Admin Key registered and system-protected")
		return &model.SessionKey{
			ID:          masterID,
			Name:        "Master Admin Key",
			Key:         adminKey,
			Permissions: allPerms,
			IsActive:    true,
			IsMaster:    true,
			CreatedAt:   now,
		}, nil
	}

	return nil, nil
}

func (s *Store) ValidateKey(token, requiredPerm string) (bool, *model.SessionKey) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var sk model.SessionKey
	var permsStr string
	var isActiveInt, isMasterInt int
	var lastUsed, expires sql.NullTime

	query := `SELECT id, name, key, permissions, is_active, is_master, created_at, last_used_at, expires_at 
	          FROM sessions WHERE key = ?`
	err := s.db.QueryRow(query, token).Scan(
		&sk.ID, &sk.Name, &sk.Key, &permsStr, &isActiveInt, &isMasterInt, &sk.CreatedAt, &lastUsed, &expires,
	)
	if err != nil {
		return false, nil
	}

	sk.IsActive = isActiveInt == 1
	sk.IsMaster = isMasterInt == 1 || sk.ID == "sess_master" || (s.adminKey != "" && sk.Key == s.adminKey)
	if !sk.IsActive {
		return false, nil
	}

	if expires.Valid {
		sk.ExpiresAt = &expires.Time
		if time.Now().After(expires.Time) {
			return false, nil
		}
	}

	_ = json.Unmarshal([]byte(permsStr), &sk.Permissions)

	hasPerm := false
	for _, p := range sk.Permissions {
		if p == "admin" || p == requiredPerm {
			hasPerm = true
			break
		}
	}

	if !hasPerm {
		return false, nil
	}

	now := time.Now()
	sk.LastUsedAt = &now
	_, _ = s.db.Exec("UPDATE sessions SET last_used_at = ? WHERE id = ?", now, sk.ID)

	return true, &sk
}

func (s *Store) GetAll() []model.SessionKey {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, name, key, permissions, is_active, is_master, created_at, last_used_at, expires_at 
	          FROM sessions ORDER BY is_master DESC, created_at DESC`
	rows, err := s.db.Query(query)
	if err != nil {
		return []model.SessionKey{}
	}
	defer rows.Close()

	var result []model.SessionKey
	for rows.Next() {
		var sk model.SessionKey
		var permsStr string
		var isActiveInt, isMasterInt int
		var lastUsed, expires sql.NullTime

		if err := rows.Scan(
			&sk.ID, &sk.Name, &sk.Key, &permsStr, &isActiveInt, &isMasterInt, &sk.CreatedAt, &lastUsed, &expires,
		); err != nil {
			continue
		}

		sk.IsActive = isActiveInt == 1
		sk.IsMaster = isMasterInt == 1 || sk.ID == "sess_master" || (s.adminKey != "" && sk.Key == s.adminKey)
		if lastUsed.Valid {
			sk.LastUsedAt = &lastUsed.Time
		}
		if expires.Valid {
			sk.ExpiresAt = &expires.Time
		}
		_ = json.Unmarshal([]byte(permsStr), &sk.Permissions)

		result = append(result, sk)
	}

	return result
}

func (s *Store) GetByID(id string) (*model.SessionKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var sk model.SessionKey
	var permsStr string
	var isActiveInt, isMasterInt int
	var lastUsed, expires sql.NullTime

	query := `SELECT id, name, key, permissions, is_active, is_master, created_at, last_used_at, expires_at 
	          FROM sessions WHERE id = ?`
	err := s.db.QueryRow(query, id).Scan(
		&sk.ID, &sk.Name, &sk.Key, &permsStr, &isActiveInt, &isMasterInt, &sk.CreatedAt, &lastUsed, &expires,
	)
	if err != nil {
		return nil, fmt.Errorf("session key with ID '%s' not found", id)
	}

	sk.IsActive = isActiveInt == 1
	sk.IsMaster = isMasterInt == 1 || sk.ID == "sess_master" || (s.adminKey != "" && sk.Key == s.adminKey)
	if lastUsed.Valid {
		sk.LastUsedAt = &lastUsed.Time
	}
	if expires.Valid {
		sk.ExpiresAt = &expires.Time
	}
	_ = json.Unmarshal([]byte(permsStr), &sk.Permissions)

	return &sk, nil
}

func (s *Store) GetByKey(token string) (*model.SessionKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var sk model.SessionKey
	var permsStr string
	var isActiveInt, isMasterInt int
	var lastUsed, expires sql.NullTime

	query := `SELECT id, name, key, permissions, is_active, is_master, created_at, last_used_at, expires_at 
	          FROM sessions WHERE key = ?`
	err := s.db.QueryRow(query, token).Scan(
		&sk.ID, &sk.Name, &sk.Key, &permsStr, &isActiveInt, &isMasterInt, &sk.CreatedAt, &lastUsed, &expires,
	)
	if err != nil {
		return nil, errors.New("session key not found")
	}

	sk.IsActive = isActiveInt == 1
	sk.IsMaster = isMasterInt == 1 || sk.ID == "sess_master" || (s.adminKey != "" && sk.Key == s.adminKey)
	if lastUsed.Valid {
		sk.LastUsedAt = &lastUsed.Time
	}
	if expires.Valid {
		sk.ExpiresAt = &expires.Time
	}
	_ = json.Unmarshal([]byte(permsStr), &sk.Permissions)

	return &sk, nil
}

func (s *Store) Add(name, customKey string, permissions []string, expireDays int) (*model.SessionKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	keyToken := strings.TrimSpace(customKey)
	if keyToken == "" {
		prefix := s.keyPrefix
		if prefix == "" {
			prefix = "sk_vhming_"
		}
		keyToken = prefix + generateRandomHex(16)
	}

	now := time.Now()
	var expiresAt *time.Time
	if expireDays > 0 {
		exp := now.Add(time.Duration(expireDays) * 24 * time.Hour)
		expiresAt = &exp
	}

	if len(permissions) == 0 {
		permissions = []string{"read", "upload"}
	}

	permsJSON, _ := json.Marshal(permissions)
	newSession := model.SessionKey{
		ID:          "sess_" + generateRandomHex(6),
		Name:        name,
		Key:         keyToken,
		Permissions: permissions,
		IsActive:    true,
		IsMaster:    false,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
	}

	_, err := s.db.Exec(`
		INSERT INTO sessions (id, name, key, permissions, is_active, is_master, created_at, expires_at)
		VALUES (?, ?, ?, ?, 1, 0, ?, ?)
	`, newSession.ID, newSession.Name, newSession.Key, string(permsJSON), now, expiresAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, errors.New("session key token already exists")
		}
		return nil, err
	}

	return &newSession, nil
}

func (s *Store) Update(id string, isActive *bool, name *string, permissions []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var isMasterInt int
	var keyToken string
	err := s.db.QueryRow("SELECT is_master, key FROM sessions WHERE id = ?", id).Scan(&isMasterInt, &keyToken)
	if err != nil {
		return fmt.Errorf("session key with ID '%s' not found", id)
	}

	isMaster := isMasterInt == 1 || id == "sess_master" || (s.adminKey != "" && keyToken == s.adminKey)
	if isMaster && isActive != nil && !*isActive {
		return errors.New("cannot deactivate Master Admin Key: master key must remain active")
	}

	if isActive != nil {
		_, err := s.db.Exec("UPDATE sessions SET is_active = ? WHERE id = ?", boolToInt(*isActive), id)
		if err != nil {
			return err
		}
	}
	if name != nil && *name != "" {
		_, err := s.db.Exec("UPDATE sessions SET name = ? WHERE id = ?", *name, id)
		if err != nil {
			return err
		}
	}
	if len(permissions) > 0 {
		if isMaster {
			permissions = []string{"upload", "read", "delete", "admin"}
		}
		permsJSON, _ := json.Marshal(permissions)
		_, err := s.db.Exec("UPDATE sessions SET permissions = ? WHERE id = ?", string(permsJSON), id)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var isMasterInt int
	var keyToken string
	err := s.db.QueryRow("SELECT is_master, key FROM sessions WHERE id = ?", id).Scan(&isMasterInt, &keyToken)
	if err != nil {
		return fmt.Errorf("session key with ID '%s' not found", id)
	}

	if isMasterInt == 1 || id == "sess_master" || (s.adminKey != "" && keyToken == s.adminKey) {
		return errors.New("cannot delete Master Admin Key: protected system key")
	}

	res, err := s.db.Exec("DELETE FROM sessions WHERE id = ?", id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session key with ID '%s' not found", id)
	}
	return nil
}

func CleanIPv4(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}
	raw = strings.Trim(raw, "[]")

	ip := net.ParseIP(raw)
	if ip != nil {
		if ip.IsLoopback() {
			return "127.0.0.1"
		}
		if ip4 := ip.To4(); ip4 != nil {
			return ip4.String()
		}
		return ""
	}

	if strings.Contains(raw, ".") && !strings.Contains(raw, ":") {
		return raw
	}

	return ""
}

func (s *Store) LogActivity(action, status, token, targetKey, fileName string, size int64, ip, userAgent, details string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ip = CleanIPv4(ip)

	var sessionID, sessionName string
	var isMasterInt int
	if token != "" {
		_ = s.db.QueryRow("SELECT id, name, is_master FROM sessions WHERE key = ?", token).Scan(&sessionID, &sessionName, &isMasterInt)
	}

	if sessionName == "" {
		if s.adminKey != "" && token == s.adminKey {
			sessionName = "Master Admin Key"
			sessionID = "sess_master"
		} else if token != "" {
			sessionName = "API Session"
		} else {
			sessionName = "Web Dashboard"
		}
	} else if isMasterInt == 1 {
		sessionName = "Master Admin Key"
	}

	if status == "" {
		status = "SUCCESS"
	}

	_, err := s.db.Exec(`
		INSERT INTO activity_logs (action, status, session_id, session_name, target_key, file_name, file_size, ip_address, user_agent, details, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, action, status, sessionID, sessionName, targetKey, fileName, size, ip, userAgent, details, time.Now())

	return err
}

func (s *Store) PurgeOldLogs(retentionDays int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if retentionDays <= 0 {
		retentionDays = 7
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	res, err := s.db.Exec("DELETE FROM activity_logs WHERE created_at < ?", cutoff)
	if err != nil {
		return 0, err
	}
	_, _ = s.db.Exec("DELETE FROM upload_logs WHERE created_at < ?", cutoff)

	affected, _ := res.RowsAffected()
	if affected > 0 {
		log.Printf("[INFO] Auto-purged %d activity logs older than %d days", affected, retentionDays)
	}
	return affected, nil
}

func (s *Store) GetActivityLogs(limit int) ([]model.ActivityLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > 300 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT id, action, status, session_id, session_name, target_key, file_name, file_size, ip_address, user_agent, details, created_at 
		FROM activity_logs 
		ORDER BY created_at DESC 
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []model.ActivityLog
	for rows.Next() {
		var l model.ActivityLog
		var sessID, sessName, fName, ip, ua, details sql.NullString
		if err := rows.Scan(
			&l.ID, &l.Action, &l.Status, &sessID, &sessName, &l.TargetKey, &fName, &l.FileSize, &ip, &ua, &details, &l.CreatedAt,
		); err != nil {
			continue
		}
		if sessID.Valid {
			l.SessionID = sessID.String
		}
		if sessName.Valid {
			l.SessionName = sessName.String
		}
		if fName.Valid {
			l.FileName = fName.String
		}
		if ip.Valid {
			l.IPAddress = ip.String
		}
		if ua.Valid {
			l.UserAgent = ua.String
		}
		if details.Valid {
			l.Details = details.String
		}
		logs = append(logs, l)
	}
	return logs, nil
}

func (s *Store) GetActivityLogsBySession(sessionID string, limit int) ([]model.ActivityLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > 300 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT id, action, status, session_id, session_name, target_key, file_name, file_size, ip_address, user_agent, details, created_at 
		FROM activity_logs 
		WHERE session_id = ?
		ORDER BY created_at DESC 
		LIMIT ?
	`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []model.ActivityLog
	for rows.Next() {
		var l model.ActivityLog
		var sessID, sessName, fName, ip, ua, details sql.NullString
		if err := rows.Scan(
			&l.ID, &l.Action, &l.Status, &sessID, &sessName, &l.TargetKey, &fName, &l.FileSize, &ip, &ua, &details, &l.CreatedAt,
		); err != nil {
			continue
		}
		if sessID.Valid {
			l.SessionID = sessID.String
		}
		if sessName.Valid {
			l.SessionName = sessName.String
		}
		if fName.Valid {
			l.FileName = fName.String
		}
		if ip.Valid {
			l.IPAddress = ip.String
		}
		if ua.Valid {
			l.UserAgent = ua.String
		}
		if details.Valid {
			l.Details = details.String
		}
		logs = append(logs, l)
	}
	return logs, nil
}

func (s *Store) LogUpload(token, fileKey, fileName string, size int64, contentType, ip string) error {
	return s.LogActivity("UPLOAD", "SUCCESS", token, fileKey, fileName, size, ip, "", "Uploaded file ("+contentType+")")
}

func (s *Store) GetUploadLogs(limit int) ([]model.UploadLog, error) {
	actLogs, err := s.GetActivityLogs(limit)
	if err != nil {
		return nil, err
	}

	var uploadLogs []model.UploadLog
	for _, a := range actLogs {
		if a.Action == "UPLOAD" {
			uploadLogs = append(uploadLogs, model.UploadLog{
				ID:          a.ID,
				SessionID:   a.SessionID,
				SessionName: a.SessionName,
				FileKey:     a.TargetKey,
				FileName:    a.FileName,
				FileSize:    a.FileSize,
				IPAddress:   a.IPAddress,
				CreatedAt:   a.CreatedAt,
			})
		}
	}
	return uploadLogs, nil
}

func (s *Store) Close() error {
	// Signal background ticker to stop
	select {
	case <-s.stopChan:
		// already closed
	default:
		close(s.stopChan)
	}

	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func generateRandomHex(n int) string {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}
