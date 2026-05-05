package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/timmy/emomo/internal/logger"
)

// StorageType defines the type of S3-compatible storage.
type StorageType string

const (
	// StorageTypeR2 identifies Cloudflare R2 storage.
	StorageTypeR2 StorageType = "r2"
	// StorageTypeS3 identifies AWS S3 storage.
	StorageTypeS3 StorageType = "s3"
	// StorageTypeS3Compatible identifies other S3-compatible storage providers.
	StorageTypeS3Compatible StorageType = "s3compatible"
)

// S3Config holds configuration for S3-compatible storage.
type S3Config struct {
	Type      StorageType
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Bucket    string
	Region    string
	PublicURL string // Public URL prefix for R2.dev or custom CDN
}

// S3Storage implements ObjectStorage for S3-compatible services.
type S3Storage struct {
	client    *s3.Client
	bucket    string
	endpoint  string
	useSSL    bool
	storeType StorageType
	publicURL string
	region    string
}

// NewS3Storage creates a new S3-compatible storage client.
// Parameters:
//   - cfg: storage configuration including endpoint, credentials, and bucket.
//
// Returns:
//   - *S3Storage: initialized storage client.
//   - error: non-nil if configuration or client initialization fails.
func NewS3Storage(cfg *S3Config) (*S3Storage, error) {
	// Normalize endpoint: remove protocol prefix and trailing slashes/paths
	endpoint := normalizeEndpoint(cfg.Endpoint)

	// Determine region
	region := cfg.Region
	if region == "" {
		if cfg.Type == StorageTypeR2 {
			region = "auto"
		} else {
			region = "us-east-1" // Default region for S3-compatible services
		}
	}

	// Build endpoint URL
	scheme := "http"
	if cfg.UseSSL {
		scheme = "https"
	}
	endpointURL := fmt.Sprintf("%s://%s", scheme, endpoint)

	// Create AWS config
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKey,
			cfg.SecretKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client with custom endpoint
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpointURL)
		o.UsePathStyle = true // Use path-style for S3-compatible services
	})

	// Normalize public URL (remove trailing slash)
	publicURL := strings.TrimSuffix(cfg.PublicURL, "/")

	return &S3Storage{
		client:    client,
		bucket:    cfg.Bucket,
		endpoint:  endpoint,
		useSSL:    cfg.UseSSL,
		storeType: cfg.Type,
		publicURL: publicURL,
		region:    region,
	}, nil
}

// normalizeEndpoint removes protocol prefix and path from endpoint
func normalizeEndpoint(endpoint string) string {
	// Remove protocol prefix
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")

	// Remove any path (everything after the first /)
	if idx := strings.Index(endpoint, "/"); idx != -1 {
		endpoint = endpoint[:idx]
	}

	// Remove trailing slashes
	endpoint = strings.TrimSuffix(endpoint, "/")

	return endpoint
}

// EnsureBucket ensures the configured bucket exists.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//
// Returns:
//   - error: non-nil if the bucket check/create fails.
func (s *S3Storage) EnsureBucket(ctx context.Context) error {
	// Check if bucket exists
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err == nil {
		return nil
	}

	// R2 doesn't support creating buckets via API - must use dashboard
	if s.storeType == StorageTypeR2 {
		return fmt.Errorf("bucket %s does not exist, please create it in R2 dashboard", s.bucket)
	}

	// Create bucket for S3 and S3-compatible services
	_, err = s.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err != nil {
		return fmt.Errorf("failed to create bucket: %w", err)
	}

	return nil
}

// Upload stores an object at the given key.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - key: storage key (path) for the object.
//   - reader: stream providing the object content.
//   - size: content length in bytes.
//   - contentType: MIME type for the object.
//
// Returns:
//   - error: non-nil if the upload fails.
func (s *S3Storage) Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	startTime := time.Now()

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          reader,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	duration := time.Since(startTime)

	if err != nil {
		logger.With(logger.Fields{
			logger.FieldDurationMs: duration.Milliseconds(),
			logger.FieldSize:       size,
		}).Error(ctx, "Failed to upload: key=%s, error=%v", key, err)
		return fmt.Errorf("failed to upload object: %w", err)
	}

	logger.With(logger.Fields{
		logger.FieldDurationMs: duration.Milliseconds(),
		logger.FieldSize:       size,
	}).Debug(ctx, "Upload completed: key=%s", key)

	return nil
}

// Download retrieves an object by key.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - key: storage key (path) for the object.
//
// Returns:
//   - io.ReadCloser: reader for the object contents.
//   - error: non-nil if the download fails.
func (s *S3Storage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	startTime := time.Now()

	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	duration := time.Since(startTime)

	if err != nil {
		logger.With(logger.Fields{
			logger.FieldDurationMs: duration.Milliseconds(),
		}).Error(ctx, "Failed to download: key=%s, error=%v", key, err)
		return nil, fmt.Errorf("failed to download object: %w", err)
	}

	logger.With(logger.Fields{
		logger.FieldDurationMs: duration.Milliseconds(),
	}).Debug(ctx, "Download completed: key=%s", key)

	return result.Body, nil
}

// GetURL returns a public or signed URL for accessing an object.
// Parameters:
//   - key: storage key (path) for the object.
//
// Returns:
//   - string: URL that can be used to access the object.
func (s *S3Storage) GetURL(key string) string {
	// If public URL is configured (R2.dev or custom CDN), use it
	if s.publicURL != "" {
		return fmt.Sprintf("%s/%s", s.publicURL, key)
	}

	// Generate URL based on storage type
	switch s.storeType {
	case StorageTypeS3:
		// AWS S3 virtual-hosted style URL
		return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.bucket, s.region, key)
	case StorageTypeR2:
		// R2 without public URL configured - use S3 API endpoint (requires signing)
		// Note: It's recommended to always configure public_url for R2
		scheme := "http"
		if s.useSSL {
			scheme = "https"
		}
		return fmt.Sprintf("%s://%s/%s/%s", scheme, s.endpoint, s.bucket, key)
	default:
		// S3-compatible services (path-style)
		scheme := "http"
		if s.useSSL {
			scheme = "https"
		}
		return fmt.Sprintf("%s://%s/%s/%s", scheme, s.endpoint, s.bucket, key)
	}
}

// Delete removes an object by key.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - key: storage key (path) for the object.
//
// Returns:
//   - error: non-nil if the delete fails.
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	return nil
}

// Exists checks if an object exists by key.
// Parameters:
//   - ctx: context for cancellation and deadlines.
//   - key: storage key (path) for the object.
//
// Returns:
//   - bool: true if the object exists.
//   - error: non-nil if the check fails.
func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check object existence: %w", err)
	}
	return true, nil
}
