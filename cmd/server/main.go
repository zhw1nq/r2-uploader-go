package main

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"r2-uploader-go/internal/config"
	"r2-uploader-go/internal/handler"
	"r2-uploader-go/internal/session"
	"r2-uploader-go/internal/storage"
	"r2-uploader-go/web"
)

func main() {
	// 1. Load Configuration
	cfg := config.Load()

	// 2. Initialize SQLite Session Store
	sessionStore, err := session.NewStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize SQLite database (%s): %v", cfg.DBPath, err)
	}
	defer sessionStore.Close()
	sessionStore.SetKeyPrefix(cfg.KeyPrefix)
	log.Printf("[INFO] SQLite database initialized at: %s", cfg.DBPath)

	// Ensure Master Admin Session exists if configured
	if cfg.AdminKey != "" {
		if _, err := sessionStore.EnsureMasterKey(cfg.AdminKey); err != nil {
			log.Printf("[WARNING] EnsureMasterKey: %v", err)
		}
	}

	// 3. Initialize Cloudflare R2 Storage Service
	r2Service, err := storage.NewR2Service(cfg)
	if err != nil {
		log.Printf("[WARNING] Cloudflare R2 initialization: %v", err)
		log.Println("[INFO] Please verify your .env file credentials.")
	} else {
		log.Printf("[INFO] Connected to Cloudflare R2 bucket: %s", cfg.BucketName)
	}

	// 4. Setup Web Assets Filesystem (Direct disk during development, embedded fallback)
	var webFS fs.FS
	if _, err := os.Stat("web/index.html"); err == nil {
		webFS = os.DirFS("web")
	} else {
		webFS = web.FS
	}

	// 5. Initialize Router and HTTP Server
	serverHandler := handler.NewServer(cfg, r2Service, sessionStore, webFS)
	routes := serverHandler.SetupRoutes()

	addr := ":" + cfg.Port
	log.Printf("[INFO] Server listening on %s", addr)
	if cfg.AppURL != "" {
		log.Printf("[INFO] Public Application URL: %s", cfg.AppURL)
		log.Printf("[INFO] Web Dashboard accessible at: %s/", cfg.AppURL)
	} else {
		log.Printf("[INFO] Web Dashboard accessible at: http://localhost%s/", addr)
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      routes,
		ReadTimeout:  30 * time.Minute,
		WriteTimeout: 30 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	// 6. Graceful Shutdown listener
	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-stopCtx.Done()
		log.Println("[INFO] Termination signal received. Shutting down server gracefully...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("[ERROR] Server forced to shutdown: %v", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("[FATAL] Server stopped with error: %v", err)
	}

	log.Println("[INFO] Server stopped cleanly.")
}
