package handler

import (
	"context"
	"net/http"
	"time"

	"r2-uploader-go/internal/model"
)

// HandleHealth provides a standard health check endpoint for monitoring & probes
func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	dbStatus := "healthy"
	if err := s.Sessions.Ping(ctx); err != nil {
		dbStatus = "error: " + err.Error()
	}

	storageStatus := "healthy"
	if s.R2 == nil {
		storageStatus = "unconfigured"
	} else if err := s.R2.Ping(ctx); err != nil {
		storageStatus = "degraded: " + err.Error()
	}

	overallStatus := "ok"
	httpStatusCode := http.StatusOK
	if dbStatus != "healthy" {
		overallStatus = "degraded"
		httpStatusCode = http.StatusServiceUnavailable
	}

	res := model.HealthStatus{
		Status:    overallStatus,
		Version:   "2026.1",
		Uptime:    time.Since(s.StartTime).Round(time.Second).String(),
		Database:  dbStatus,
		Storage:   storageStatus,
		Timestamp: time.Now().UTC(),
	}

	writeJSON(w, httpStatusCode, res)
}
