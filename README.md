# Cloudflare R2 Uploader & Storage Manager (Production 2026)

A high-performance Cloudflare R2 file storage manager and RESTful API built with Go (AWS SDK v2), SQLite audit logging, and a minimalist Web UI dashboard.

---

## Architecture & Project Structure (Standard Go Project Layout)

The project is structured according to the **Standard Go Project Layout** for production-grade maintainability, security, and scalability:

```text
r2-uploader-go/
├── cmd/
│   └── server/
│       └── main.go                 # Application entrypoint with Graceful Shutdown & signal handling
├── internal/
│   ├── config/                     # Environment configuration loader (.env) & validation
│   │   └── config.go
│   ├── handler/                    # RESTful HTTP endpoints, route multiplexer, SPA router
│   │   ├── handler.go              # Server state, base URL resolution, router wiring
│   │   ├── api_files.go            # Upload, list, streaming (HTTP Range), delete, presign
│   │   ├── api_sessions.go         # Session keys CRUD & activity audit logs
│   │   └── api_health.go           # Healthcheck endpoint (/api/health, /healthz)
│   ├── middleware/                 # Production HTTP middlewares
│   │   ├── logger.go               # Structured HTTP access logging (latency, status, IP)
│   │   ├── recovery.go             # Panic recovery mechanism
│   │   └── security.go             # OWASP security headers (CSP, nosniff, SAMEORIGIN)
│   ├── model/                      # Shared domain models & data transfer objects
│   │   └── models.go
│   ├── session/                    # SQLite session store, WAL concurrency, auto 7-day purge
│   │   └── session.go
│   └── storage/                    # Cloudflare R2 client (AWS SDK v2), multipart upload
│       └── r2.go
├── web/                            # Minimalist dark Web Dashboard SPA
│   ├── app.js                      # Client state, dynamic domain resolution, file viewer
│   ├── index.html                  # HTML structure
│   ├── style.css                   # Responsive dark styling
│   └── web.go                      # Embeds web assets into binary (embed.FS)
├── deployments/                    # Infrastructure & deployment manifests
│   ├── docker/
│   │   ├── Dockerfile              # Multi-stage, non-root user (appuser:10001), healthcheck
│   │   └── compose.yaml            # Compose spec with persistent volume for data/
│   └── systemd/
│       └── r2-uploader.service     # Hardened Linux systemd service unit
├── data/                           # Local SQLite database directory (git-ignored, .gitkeep)
│   └── .gitkeep
├── bin/                            # Output directory for compiled binaries (git-ignored)
├── .env.example                    # Environment variable template with documentation
├── .gitignore                      # Comprehensive git ignore rules for Go, SQLite & OS
├── compose.yaml                    # Root Compose file (docker compose up -d)
├── Dockerfile                      # Root Dockerfile shortcut for PaaS deployment
├── Makefile                        # Automation: build, run, clean, test, vet
├── go.mod
├── go.sum
└── README.md                       # Complete documentation
```

---

## Key Features

- **Standard Go Project Layout (2026)**: Decoupled architecture with internal encapsulation, eliminating circular dependencies.
- **Dynamic Domain Configuration**: Easily configure application domain (`APP_URL=https://r2-upl.vhming.com`) and CDN storage domain (`R2_PUBLIC_DOMAIN`) directly from `.env`.
- **Cloudflare R2 Native**: Powered by AWS SDK Go v2 with concurrent multipart uploads, HTTP Range requests for video/audio streaming, and presigned URL generation.
- **Graceful Shutdown**: Intercepts `SIGINT` / `SIGTERM` signals with a 15-second graceful drain timeout, ensuring zero connection drops and clean SQLite WAL shutdowns.
- **Production Hardened**:
  - Non-root Docker container (`appuser:10001`) with Alpine 3.21 and healthcheck probes.
  - SQLite WAL mode with serialized connection pool (`SetMaxOpenConns(1)`), preventing database locks.
  - Automatic background goroutine lifecycle cancellation on server termination.
  - OWASP Security Headers & panic recovery middleware.
- **Multi-Session Key Management & Audit Logs**:
  - Role-based permissions: `upload`, `read`, `delete`, `admin`.
  - Expiration support (7 days, 30 days, 1 year, or never).
  - Activity audit logs tracking IP, user-agent, file size, action type, and status with automatic 7-day auto-purge.
- **Modern Minimalist Web Dashboard**:
  - Monochrome dark aesthetic powered by Google Font **Be Vietnam Pro** and **Phosphor Icons**.
  - Drag-and-drop upload queue with percentage progress indicators.
  - In-browser media preview (images, HTML5 video/audio player, code/text).

---

## Setup & Running

### 1. Environment Configuration

Copy `.env.example` to `.env`:
```bash
# Linux / macOS
cp .env.example .env

# Windows PowerShell
copy .env.example .env
```

Configure your `.env` variables:
```ini
# Cloudflare R2 Credentials
R2_ACCOUNT_ID=your_cloudflare_account_id
R2_ACCESS_KEY_ID=your_access_key_id
R2_SECRET_ACCESS_KEY=your_secret_access_key
R2_BUCKET_NAME=your_bucket_name
R2_ROOT_FOLDER=uploader
R2_PUBLIC_DOMAIN=https://cdn.vhming.com/

# Server & Domain Configuration
PORT=8080
APP_URL=https://r2-upl.vhming.com
ADMIN_KEY=your_master_admin_secret_key_here
SQLITE_PATH=data/data.db
KEY_PREFIX=sk_vhming_
```

### 2. Run in Development Mode

```bash
go run ./cmd/server
# or
make run
```
Open your browser at `https://r2-upl.vhming.com` (or `http://localhost:8080` locally).

### 3. Build Standalone Binaries

Using `make`:
```bash
make build              # Build binary for current OS into bin/r2-uploader
make build-linux        # Cross-compile 64-bit Linux binary into bin/r2-uploader-linux-amd64
make build-linux-arm64  # Cross-compile ARM64 Linux binary into bin/r2-uploader-linux-arm64
make build-windows      # Cross-compile Windows 64-bit binary into bin/r2-uploader.exe
make vet                # Run code analysis
make clean              # Remove build artifacts in bin/
```

Or using `go build` directly (zero CGO required):
```bash
# Linux 64-bit
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/r2-uploader ./cmd/server

# Windows 64-bit
$env:CGO_ENABLED="0"; $env:GOOS="windows"; $env:GOARCH="amd64"; go build -ldflags="-s -w" -o bin/r2-uploader.exe ./cmd/server
```

---

## Production Deployment

### 1. Docker & Docker Compose (Recommended)

Run with persistent SQLite data and automatic container healthcheck:
```bash
docker compose up -d
```

Check logs and health status:
```bash
docker compose logs -f
docker compose ps
```

### 2. Linux Production Deployment (systemd)

A hardened systemd unit file is available at `deployments/systemd/r2-uploader.service`:

```bash
# 1. Build and install binary
make build-linux
sudo mkdir -p /opt/r2-uploader/bin /opt/r2-uploader/data
sudo cp bin/r2-uploader-linux-amd64 /opt/r2-uploader/bin/r2-uploader
sudo cp .env /opt/r2-uploader/.env
sudo chown -R www-data:www-data /opt/r2-uploader
sudo chmod +x /opt/r2-uploader/bin/r2-uploader

# 2. Install and enable systemd service
sudo cp deployments/systemd/r2-uploader.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now r2-uploader

# 3. Check status & logs
sudo systemctl status r2-uploader
sudo journalctl -u r2-uploader -f
```

---

## REST API Reference

Pass your session key via the `X-Session-Key` header or `Authorization: Bearer <key>`.

### Health Check (Probes)
- **Method**: `GET /api/health` or `GET /healthz`
- **Response**:
```json
{
  "status": "ok",
  "version": "2026.1",
  "uptime": "1h20m35s",
  "database": "healthy",
  "storage": "healthy",
  "timestamp": "2026-10-05T20:50:00Z"
}
```

### System Configuration
- **Method**: `GET /api/config`
- **Response**: Exposes configured `bucket_name`, `public_domain`, `app_url`, and server status.

### Upload Files
- **Method**: `POST /api/upload`
- **Permission**: `upload`
- **Body**: `multipart/form-data` with `file` and optional `folder`
```bash
curl -X POST "https://r2-upl.vhming.com/api/upload" \
  -H "X-Session-Key: your_session_key" \
  -F "file=@photo.jpg" \
  -F "folder=uploads/"
```

### List Files
- **Method**: `GET /api/files?path=uploads`
- **Permission**: `read`
```bash
curl -X GET "https://r2-upl.vhming.com/api/files?path=uploads" \
  -H "X-Session-Key: your_session_key"
```

### Stream / Download File
- **Method**: `GET /api/file?key={file_key}`
- **Permission**: `read`
```bash
curl -O "https://r2-upl.vhming.com/api/file?key=uploads/photo.jpg" \
  -H "X-Session-Key: your_session_key"
```

### Generate Presigned URL
- **Method**: `GET /api/presign?key={file_key}&expires={seconds}`
- **Permission**: `read`
```bash
curl -X GET "https://r2-upl.vhming.com/api/presign?key=uploads/photo.jpg&expires=86400" \
  -H "X-Session-Key: your_session_key"
```

### Delete File
- **Method**: `DELETE /api/file?key={file_key}`
- **Permission**: `delete`
```bash
curl -X DELETE "https://r2-upl.vhming.com/api/file?key=uploads/photo.jpg" \
  -H "X-Session-Key: your_session_key"
```

### Session Keys & Audit Logs
- `GET /api/sessions` - List session keys (Master Admin sees all; users see their own)
- `POST /api/sessions` - Create new session key (Master Admin only)
- `PUT /api/sessions/{id}` - Update key name, permissions, active status
- `DELETE /api/sessions/{id}` - Revoke session key
- `GET /api/logs` - View activity audit logs (auto-purged after 7 days)