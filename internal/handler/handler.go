package handler

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"r2-uploader-go/internal/config"
	"r2-uploader-go/internal/middleware"
	"r2-uploader-go/internal/session"
	"r2-uploader-go/internal/storage"
)

type Server struct {
	Cfg       *config.Config
	R2        *storage.R2Service
	Sessions  *session.Store
	WebFS     fs.FS
	StartTime time.Time
}

func NewServer(cfg *config.Config, r2 *storage.R2Service, sessions *session.Store, webFS fs.FS) *Server {
	return &Server{
		Cfg:       cfg,
		R2:        r2,
		Sessions:  sessions,
		WebFS:     webFS,
		StartTime: time.Now(),
	}
}

// GetBaseURL returns the configured public domain (e.g. https://r2-upl.vhming.com) or falls back to request Host
func (s *Server) GetBaseURL(r *http.Request) string {
	if s.Cfg.AppURL != "" {
		return strings.TrimRight(s.Cfg.AppURL, "/")
	}

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "localhost:" + s.Cfg.Port
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func (s *Server) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	// Healthcheck & diagnostics (Kubernetes / Docker probes)
	mux.HandleFunc("/api/health", s.HandleHealth)
	mux.HandleFunc("/healthz", s.HandleHealth)

	// API routes
	mux.HandleFunc("/api/config", s.HandleConfig)
	mux.HandleFunc("/api/stats", s.HandleStats)
	mux.HandleFunc("/api/upload", s.AuthMiddleware("upload", s.HandleUpload))
	mux.HandleFunc("/api/files", s.AuthMiddleware("read", s.HandleListFiles))
	mux.HandleFunc("/api/file", s.HandleFile) // GET (stream), DELETE
	mux.HandleFunc("/api/file/info", s.AuthMiddleware("read", s.HandleFileInfo))
	mux.HandleFunc("/api/folder", s.HandleFolder) // POST (create), DELETE (remove)
	mux.HandleFunc("/api/presign", s.AuthMiddleware("read", s.HandlePresign))

	// Session management & audit logs API
	mux.HandleFunc("/api/sessions", s.AuthMiddleware("admin", s.HandleSessions))
	mux.HandleFunc("/api/sessions/", s.AuthMiddleware("admin", s.HandleSessionItem))
	mux.HandleFunc("/api/logs", s.AuthMiddleware("admin", s.HandleLogs))

	// Web UI SPA
	mux.Handle("/", s.handleWebAssets())

	// Apply production middlewares: Recovery -> Logging -> SecurityHeaders -> Router
	var handler http.Handler = mux
	handler = middleware.SecurityHeaders(handler)
	handler = middleware.Logger(handler)
	handler = middleware.Recovery(handler)

	return handler
}

func (s *Server) AuthMiddleware(requiredPerm string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := getSessionToken(r)

		if s.Cfg.AdminKey != "" && token == s.Cfg.AdminKey {
			next(w, r)
			return
		}

		if s.Cfg.AdminKey == "" && len(s.Sessions.GetAll()) == 0 {
			next(w, r)
			return
		}

		if token == "" {
			writeError(w, http.StatusUnauthorized, "Missing required X-Session-Key or Authorization Bearer header")
			return
		}

		valid, _ := s.Sessions.ValidateKey(token, requiredPerm)
		if !valid {
			writeError(w, http.StatusForbidden, fmt.Sprintf("Access denied: invalid token or missing '%s' permission", requiredPerm))
			return
		}

		next(w, r)
	}
}

func (s *Server) handleWebAssets() http.Handler {
	fileServer := http.FileServer(http.FS(s.WebFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If path doesn't start with /api/, serve SPA files
		fileServer.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func getClientIP(r *http.Request) string {
	rawIP := ""
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		rawIP = strings.TrimSpace(parts[0])
	} else if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		rawIP = strings.TrimSpace(realIP)
	} else {
		rawIP = r.RemoteAddr
	}
	return session.CleanIPv4(rawIP)
}

func getSessionToken(r *http.Request) string {
	token := r.Header.Get("X-Session-Key")
	if token == "" {
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		}
	}
	if token == "" {
		token = r.URL.Query().Get("key")
	}
	return token
}
