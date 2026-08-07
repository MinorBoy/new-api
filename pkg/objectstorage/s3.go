package objectstorage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

type Config struct {
	Endpoint        string
	PublicEndpoint  string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool
}

type Store struct {
	api      *s3.Client
	presign  *s3.PresignClient
	uploader *manager.Uploader
	bucket   string
}

func New(cfg Config) (*Store, error) {
	if err := validateClientConfig(cfg); err != nil {
		return nil, err
	}
	api := newS3Client(cfg, cfg.Endpoint)
	presign := s3.NewPresignClient(newS3Client(cfg, cfg.PublicEndpoint))
	return &Store{
		api:      api,
		presign:  presign,
		uploader: manager.NewUploader(api),
		bucket:   cfg.Bucket,
	}, nil
}

func validateClientConfig(cfg Config) error {
	if strings.TrimSpace(cfg.Endpoint) == "" || strings.TrimSpace(cfg.PublicEndpoint) == "" {
		return fmt.Errorf("object storage endpoint and public endpoint are required")
	}
	for field, raw := range map[string]string{"endpoint": cfg.Endpoint, "public_endpoint": cfg.PublicEndpoint} {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("object storage %s must be an absolute HTTP(S) URL", field)
		}
	}
	if strings.TrimSpace(cfg.Region) == "" || strings.TrimSpace(cfg.Bucket) == "" || strings.TrimSpace(cfg.AccessKeyID) == "" || strings.TrimSpace(cfg.SecretAccessKey) == "" {
		return fmt.Errorf("object storage region, bucket, and credentials are required")
	}
	return nil
}

func newS3Client(cfg Config, endpoint string) *s3.Client {
	awsConfig := aws.Config{
		Region:       cfg.Region,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		BaseEndpoint: aws.String(endpoint),
	}
	return s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.UsePathStyle = cfg.UsePathStyle
	})
}

func (s *Store) Put(ctx context.Context, key, contentType string, body io.Reader, size int64) error {
	if s == nil || s.uploader == nil {
		return fmt.Errorf("object storage is not initialized")
	}
	input := &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	}
	if size >= 0 {
		input.ContentLength = aws.Int64(size)
	}
	_, err := s.uploader.Upload(ctx, input)
	return err
}

func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	if s == nil || s.api == nil {
		return false, fmt.Errorf("object storage is not initialized")
	}
	_, err := s.api.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err == nil {
		return true, nil
	}
	var responseErr *smithyhttp.ResponseError
	if errors.As(err, &responseErr) && responseErr.HTTPStatusCode() == 404 {
		return false, nil
	}
	return false, err
}

func (s *Store) Delete(ctx context.Context, key string) error {
	if s == nil || s.api == nil {
		return fmt.Errorf("object storage is not initialized")
	}
	_, err := s.api.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	return err
}

func (s *Store) PresignGet(ctx context.Context, key string, expires time.Duration) (string, error) {
	if s == nil || s.presign == nil {
		return "", fmt.Errorf("object storage is not initialized")
	}
	if expires <= 0 {
		return "", fmt.Errorf("presign expiry must be positive")
	}
	request, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(options *s3.PresignOptions) {
		options.Expires = expires
	})
	if err != nil {
		return "", err
	}
	return request.URL, nil
}

func (s *Store) Probe(ctx context.Context) error {
	key := fmt.Sprintf("__new_api_object_storage_probe/%d", time.Now().UnixNano())
	if err := s.Put(ctx, key, "application/octet-stream", strings.NewReader("probe"), int64(len("probe"))); err != nil {
		return err
	}
	defer func() { _ = s.Delete(context.Background(), key) }()
	exists, err := s.Exists(ctx, key)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("object storage probe object was not found")
	}
	return nil
}
