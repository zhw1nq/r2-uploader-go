package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"r2-uploader-go/internal/config"
	"r2-uploader-go/internal/model"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type R2Service struct {
	cfg        *config.Config
	client     *s3.Client
	presign    *s3.PresignClient
	uploader   *manager.Uploader
	BucketName string
	RootFolder string // e.g. "uploader/"
}

func NewR2Service(cfg *config.Config) (*R2Service, error) {
	if cfg.AccountID == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, fmt.Errorf("missing Cloudflare R2 credentials in environment")
	}

	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID)

	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL:               endpoint,
			HostnameImmutable: true,
			SigningRegion:     "auto",
		}, nil
	})

	awsCfg, err := awsconfig.LoadDefaultConfig(
		context.Background(),
		awsconfig.WithEndpointResolverWithOptions(customResolver),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		)),
		awsconfig.WithRegion("auto"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AWS SDK v2 config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	presignClient := s3.NewPresignClient(client)
	uploader := manager.NewUploader(client)

	return &R2Service{
		cfg:        cfg,
		client:     client,
		presign:    presignClient,
		uploader:   uploader,
		BucketName: cfg.BucketName,
		RootFolder: cfg.RootFolder,
	}, nil
}

// Ping checks if the R2 bucket is accessible
func (s *R2Service) Ping(ctx context.Context) error {
	if s.client == nil {
		return fmt.Errorf("R2 client not initialized")
	}
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.BucketName),
	})
	return err
}

// ResolveKey safely converts a relative path to the full R2 object key inside RootFolder
func (s *R2Service) ResolveKey(relPath string) string {
	clean := path.Clean("/" + strings.TrimSpace(relPath))
	clean = strings.TrimPrefix(clean, "/")
	if clean == "." {
		clean = ""
	}
	if s.RootFolder != "" {
		if clean == "" {
			return s.RootFolder
		}
		return s.RootFolder + clean
	}
	return clean
}

func (s *R2Service) Upload(ctx context.Context, relKey, contentType string, body io.Reader) (string, string, error) {
	fullKey := s.ResolveKey(relKey)
	out, err := s.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.BucketName),
		Key:         aws.String(fullKey),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fullKey, "", err
	}
	etag := ""
	if out != nil && out.ETag != nil {
		etag = strings.Trim(*out.ETag, `"`)
	}
	return fullKey, etag, nil
}

func (s *R2Service) CreateFolder(ctx context.Context, relFolderPath string) error {
	folderKey := s.ResolveKey(relFolderPath)
	if !strings.HasSuffix(folderKey, "/") {
		folderKey += "/"
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.BucketName),
		Key:    aws.String(folderKey),
		Body:   bytes.NewReader([]byte{}),
	})
	return err
}

func (s *R2Service) ListDirectory(ctx context.Context, relPath string) (*model.DirectoryListing, error) {
	cleanRel := path.Clean("/" + strings.TrimSpace(relPath))
	cleanRel = strings.TrimPrefix(cleanRel, "/")
	if cleanRel == "." {
		cleanRel = ""
	}

	prefix := s.RootFolder
	if cleanRel != "" {
		prefix = s.RootFolder + cleanRel + "/"
	}

	input := &s3.ListObjectsV2Input{
		Bucket:    aws.String(s.BucketName),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
		MaxKeys:   aws.Int32(1000),
	}

	output, err := s.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, err
	}

	listing := &model.DirectoryListing{
		RootFolder:  strings.Trim(s.RootFolder, "/"),
		CurrentPath: cleanRel,
		Folders:     []model.FolderItem{},
		Files:       []model.FileItem{},
	}

	// 1. Process Subfolders (CommonPrefixes)
	for _, cp := range output.CommonPrefixes {
		if cp.Prefix == nil {
			continue
		}
		fullPrefix := *cp.Prefix
		relFolder := strings.TrimPrefix(fullPrefix, s.RootFolder)
		relFolder = strings.TrimSuffix(relFolder, "/")
		parts := strings.Split(relFolder, "/")
		folderName := parts[len(parts)-1]

		if folderName != "" {
			listing.Folders = append(listing.Folders, model.FolderItem{
				Name: folderName,
				Path: relFolder,
			})
		}
	}

	// 2. Process Files
	for _, obj := range output.Contents {
		if obj.Key == nil {
			continue
		}
		fullKey := *obj.Key

		if fullKey == prefix || (strings.HasSuffix(fullKey, "/") && *obj.Size == 0) {
			continue
		}

		relKey := strings.TrimPrefix(fullKey, s.RootFolder)
		parts := strings.Split(relKey, "/")
		fileName := parts[len(parts)-1]

		var publicURL string
		if s.cfg.PublicDomain != "" {
			publicURL = fmt.Sprintf("%s/%s", strings.TrimRight(s.cfg.PublicDomain, "/"), fullKey)
		}

		listing.Files = append(listing.Files, model.FileItem{
			Key:          relKey,
			Name:         fileName,
			Size:         *obj.Size,
			LastModified: *obj.LastModified,
			ETag:         strings.Trim(*obj.ETag, `"`),
			URL:          "/api/file?key=" + relKey,
			PublicURL:    publicURL,
		})
	}

	return listing, nil
}

func (s *R2Service) GetObject(ctx context.Context, relKey, rangeHeader string) (*s3.GetObjectOutput, error) {
	fullKey := s.ResolveKey(relKey)
	input := &s3.GetObjectInput{
		Bucket: aws.String(s.BucketName),
		Key:    aws.String(fullKey),
	}
	if rangeHeader != "" {
		input.Range = aws.String(rangeHeader)
	}
	return s.client.GetObject(ctx, input)
}

func (s *R2Service) HeadObject(ctx context.Context, relKey string) (*s3.HeadObjectOutput, error) {
	fullKey := s.ResolveKey(relKey)
	input := &s3.HeadObjectInput{
		Bucket: aws.String(s.BucketName),
		Key:    aws.String(fullKey),
	}
	return s.client.HeadObject(ctx, input)
}

func (s *R2Service) DeleteObject(ctx context.Context, relKey string) error {
	fullKey := s.ResolveKey(relKey)
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.BucketName),
		Key:    aws.String(fullKey),
	})
	return err
}

func (s *R2Service) DeleteFolder(ctx context.Context, relFolderPath string) error {
	folderKey := s.ResolveKey(relFolderPath)
	if !strings.HasSuffix(folderKey, "/") {
		folderKey += "/"
	}

	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.BucketName),
		Prefix: aws.String(folderKey),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, obj := range page.Contents {
			_, _ = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(s.BucketName),
				Key:    obj.Key,
			})
		}
	}

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.BucketName),
		Key:    aws.String(folderKey),
	})
	return err
}

func (s *R2Service) GeneratePresignedURL(ctx context.Context, relKey string, expiresSec int) (string, error) {
	fullKey := s.ResolveKey(relKey)
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.BucketName),
		Key:    aws.String(fullKey),
	}, s3.WithPresignExpires(time.Duration(expiresSec)*time.Second))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func (s *R2Service) GetStats(ctx context.Context) (*model.StorageStats, error) {
	var totalFiles int64 = 0
	var totalSize int64 = 0

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.BucketName),
	}
	if s.RootFolder != "" {
		input.Prefix = aws.String(s.RootFolder)
	}

	paginator := s3.NewListObjectsV2Paginator(s.client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			if strings.HasSuffix(*obj.Key, "/") && *obj.Size == 0 {
				continue
			}
			totalFiles++
			totalSize += *obj.Size
		}
	}

	return &model.StorageStats{
		Bucket:     s.BucketName,
		RootFolder: strings.Trim(s.RootFolder, "/"),
		TotalFiles: totalFiles,
		TotalSize:  totalSize,
		Health:     "Online",
	}, nil
}
