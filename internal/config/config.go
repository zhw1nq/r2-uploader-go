package config

import (
	"bufio"
	"os"
	"strings"
)

type Config struct {
	AccountID       string
	AccessKeyID     string
	SecretAccessKey string
	BucketName      string
	PublicDomain    string
	AppURL          string // e.g. "https://r2-upl.vhming.com"
	Port            string
	AdminKey        string
	RootFolder      string
	DBPath          string
	KeyPrefix       string
}

// Load reads .env file if available and loads environment variables
func Load() *Config {
	loadDotEnv(".env")

	root := strings.Trim(getEnv("R2_ROOT_FOLDER", "uploader"), "/")
	if root != "" {
		root = root + "/"
	}

	appURL := strings.TrimSpace(getEnv("APP_URL", "https://r2-upl.vhming.com"))
	if appURL != "" {
		if !strings.HasPrefix(appURL, "http://") && !strings.HasPrefix(appURL, "https://") {
			appURL = "https://" + appURL
		}
		appURL = strings.TrimRight(appURL, "/")
	}

	pubDomain := strings.TrimSpace(getEnv("R2_PUBLIC_DOMAIN", ""))
	if pubDomain != "" {
		pubDomain = strings.TrimRight(pubDomain, "/")
	}

	port := getEnv("PORT", "")
	port = os.ExpandEnv(port)
	if port == "" || strings.HasPrefix(port, "$") {
		port = getEnv("SERVER_PORT", "8080")
	}
	port = os.ExpandEnv(port)
	if port == "" || strings.HasPrefix(port, "$") {
		port = "8080"
	}

	return &Config{
		AccountID:       getEnv("R2_ACCOUNT_ID", ""),
		AccessKeyID:     getEnv("R2_ACCESS_KEY_ID", ""),
		SecretAccessKey: getEnv("R2_SECRET_ACCESS_KEY", ""),
		BucketName:      getEnv("R2_BUCKET_NAME", ""),
		PublicDomain:    pubDomain,
		AppURL:          appURL,
		Port:            port,
		AdminKey:        getEnv("ADMIN_KEY", ""),
		RootFolder:      root,
		DBPath:          getEnv("SQLITE_PATH", "data/data.db"),
		KeyPrefix:       getEnv("KEY_PREFIX", "sk_vhming_"),
	}
}

func loadDotEnv(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			val = strings.Trim(val, `"'`)
			val = os.ExpandEnv(val)
			if strings.HasPrefix(val, "$") {
				// Skip setting unresolved variable template (e.g. ${SERVER_PORT} if not expanded)
				continue
			}
			if os.Getenv(key) == "" && val != "" {
				os.Setenv(key, val)
			}
		}
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		val = os.ExpandEnv(val)
		if !strings.HasPrefix(val, "$") && val != "" {
			return val
		}
	}
	return fallback
}

