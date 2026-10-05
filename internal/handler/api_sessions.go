package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"r2-uploader-go/internal/model"
)

func (s *Server) HandleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"bucket_name":   s.Cfg.BucketName,
		"public_domain": s.Cfg.PublicDomain,
		"app_url":       s.GetBaseURL(r),
		"has_admin_key": s.Cfg.AdminKey != "",
		"status":        "online",
		"time":          time.Now().Format(time.RFC3339),
	})
}

func (s *Server) HandleStats(w http.ResponseWriter, r *http.Request) {
	if s.R2 == nil {
		writeError(w, http.StatusServiceUnavailable, "Cloudflare R2 is not configured")
		return
	}

	stats, err := s.R2.GetStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to retrieve storage stats: "+err.Error())
		return
	}

	activeKeysCount := 0
	for _, sk := range s.Sessions.GetAll() {
		if sk.IsActive {
			activeKeysCount++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"bucket":      stats.Bucket,
		"total_files": stats.TotalFiles,
		"total_size":  stats.TotalSize,
		"active_keys": activeKeysCount,
		"health":      stats.Health,
	})
}

func (s *Server) HandleSessions(w http.ResponseWriter, r *http.Request) {
	token := getSessionToken(r)
	callerIsMaster := (s.Cfg.AdminKey != "" && token == s.Cfg.AdminKey)
	if !callerIsMaster {
		callerSession, _ := s.Sessions.GetByKey(token)
		if callerSession != nil && callerSession.IsMaster {
			callerIsMaster = true
		}
	}

	switch r.Method {
	case http.MethodGet:
		if callerIsMaster {
			sessions := s.Sessions.GetAll()
			writeJSON(w, http.StatusOK, map[string]any{
				"sessions":        sessions,
				"count":           len(sessions),
				"is_master_admin": true,
			})
			return
		}

		callerSession, err := s.Sessions.GetByKey(token)
		if err != nil || callerSession == nil {
			writeError(w, http.StatusUnauthorized, "Session key not found")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"sessions":        []model.SessionKey{*callerSession},
			"count":           1,
			"is_master_admin": false,
		})

	case http.MethodPost:
		if !callerIsMaster {
			writeError(w, http.StatusForbidden, "Forbidden: Only Master Admin is authorized to create session keys")
			return
		}

		var req struct {
			Name          string   `json:"name"`
			Key           string   `json:"key"`
			Permissions   []string `json:"permissions"`
			ExpiresInDays int      `json:"expires_in_days"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
			return
		}

		if strings.TrimSpace(req.Name) == "" {
			writeError(w, http.StatusBadRequest, "Session name cannot be empty")
			return
		}

		newSession, err := s.Sessions.Add(req.Name, req.Key, req.Permissions, req.ExpiresInDays)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		clientIP := getClientIP(r)
		userAgent := r.UserAgent()
		_ = s.Sessions.LogActivity(
			"CREATE_KEY", "SUCCESS", token, newSession.ID, newSession.Name,
			0, clientIP, userAgent,
			"Created session key '"+newSession.Name+"' with permissions: "+strings.Join(newSession.Permissions, ", "),
		)

		writeJSON(w, http.StatusCreated, newSession)

	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed. Use GET or POST")
	}
}

func (s *Server) HandleSessionItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Missing session ID in URL path")
		return
	}

	token := getSessionToken(r)
	clientIP := getClientIP(r)
	userAgent := r.UserAgent()

	targetSession, err := s.Sessions.GetByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	callerIsMaster := (s.Cfg.AdminKey != "" && token == s.Cfg.AdminKey)
	if !callerIsMaster {
		callerSession, _ := s.Sessions.GetByKey(token)
		if callerSession != nil && callerSession.IsMaster {
			callerIsMaster = true
		}
	}

	if !callerIsMaster && targetSession.Key != token {
		writeError(w, http.StatusForbidden, "Forbidden: You are not authorized to access other users' sessions")
		return
	}

	switch r.Method {
	case http.MethodPut:
		var req struct {
			IsActive    *bool    `json:"is_active"`
			Name        *string  `json:"name"`
			Permissions []string `json:"permissions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
			return
		}

		if targetSession.IsMaster && req.IsActive != nil && !*req.IsActive {
			writeError(w, http.StatusForbidden, "Master Admin Key is system-protected and cannot be deactivated")
			return
		}

		if targetSession.Key == token && req.IsActive != nil && !*req.IsActive {
			writeError(w, http.StatusBadRequest, "Cannot deactivate your own currently active session key")
			return
		}

		if !callerIsMaster && hasAdminPerm(targetSession.Permissions) {
			writeError(w, http.StatusForbidden, "Only Master Admin can modify administrative sessions")
			return
		}

		if err := s.Sessions.Update(id, req.IsActive, req.Name, req.Permissions); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		_ = s.Sessions.LogActivity("UPDATE_KEY", "SUCCESS", token, id, targetSession.Name, 0, clientIP, userAgent, "Updated session key '"+targetSession.Name+"'")
		writeJSON(w, http.StatusOK, map[string]string{"message": "Session updated successfully"})

	case http.MethodDelete:
		if targetSession.IsMaster || (s.Cfg.AdminKey != "" && targetSession.Key == s.Cfg.AdminKey) || id == "sess_master" {
			writeError(w, http.StatusForbidden, "Forbidden: Master Admin Key is system-protected and cannot be deleted")
			return
		}

		if targetSession.Key == token {
			writeError(w, http.StatusBadRequest, "Cannot delete your own currently active session key. Switch to another admin session first.")
			return
		}

		if !callerIsMaster && hasAdminPerm(targetSession.Permissions) {
			writeError(w, http.StatusForbidden, "Only Master Admin can delete other administrative session keys")
			return
		}

		if err := s.Sessions.Delete(id); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		_ = s.Sessions.LogActivity("DELETE_KEY", "SUCCESS", token, id, targetSession.Name, 0, clientIP, userAgent, "Revoked session key '"+targetSession.Name+"'")
		writeJSON(w, http.StatusOK, map[string]string{"message": "Session deleted successfully"})

	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func hasAdminPerm(perms []string) bool {
	for _, p := range perms {
		if p == "admin" {
			return true
		}
	}
	return false
}

func (s *Server) HandleLogs(w http.ResponseWriter, r *http.Request) {
	token := getSessionToken(r)
	callerIsMaster := (s.Cfg.AdminKey != "" && token == s.Cfg.AdminKey)
	var callerSessionID string
	if !callerIsMaster {
		callerSession, _ := s.Sessions.GetByKey(token)
		if callerSession != nil {
			if callerSession.IsMaster {
				callerIsMaster = true
			} else {
				callerSessionID = callerSession.ID
			}
		}
	}

	switch r.Method {
	case http.MethodGet:
		_, _ = s.Sessions.PurgeOldLogs(7)

		limit := 100
		var logs []model.ActivityLog
		var err error

		if callerIsMaster {
			logs, err = s.Sessions.GetActivityLogs(limit)
		} else if callerSessionID != "" {
			logs, err = s.Sessions.GetActivityLogsBySession(callerSessionID, limit)
		} else {
			logs = []model.ActivityLog{}
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to retrieve activity logs: "+err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"logs":      logs,
			"count":     len(logs),
			"retention": "7 days (auto-purged)",
		})

	case http.MethodDelete, http.MethodPost:
		if !callerIsMaster {
			writeError(w, http.StatusForbidden, "Forbidden: Only Master Admin can purge activity logs")
			return
		}

		affected, err := s.Sessions.PurgeOldLogs(7)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to purge logs: "+err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"message": "Logs older than 7 days purged successfully",
			"purged":  affected,
		})

	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}
