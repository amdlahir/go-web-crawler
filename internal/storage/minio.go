package storage

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"path"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/amdlahir/go-web-crawler/internal/config"
)

// MinIOClient wraps the MinIO client.
type MinIOClient struct {
	client *minio.Client
	bucket string
}

// NewMinIOClient creates a new MinIO client.
func NewMinIOClient(cfg config.MinIOConfig) (*MinIOClient, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	return &MinIOClient{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

// EnsureBucket creates the bucket if it doesn't exist.
func (m *MinIOClient) EnsureBucket(ctx context.Context) error {
	exists, err := m.client.BucketExists(ctx, m.bucket)
	if err != nil {
		return fmt.Errorf("check bucket exists: %w", err)
	}

	if exists {
		return nil
	}

	if err := m.client.MakeBucket(ctx, m.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("create bucket: %w", err)
	}

	return nil
}

// htmlPath returns the object path for HTML content.
func htmlPath(contentHash string) string {
	// Use hash prefix for distribution
	prefix := contentHash[:2]
	return path.Join("html", prefix, contentHash+".html.gz")
}

// StoreHTML stores raw HTML content (gzipped).
func (m *MinIOClient) StoreHTML(ctx context.Context, contentHash string, body []byte) error {
	// Compress content
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(body); err != nil {
		return fmt.Errorf("gzip write: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("gzip close: %w", err)
	}

	objectPath := htmlPath(contentHash)
	_, err := m.client.PutObject(
		ctx,
		m.bucket,
		objectPath,
		&buf,
		int64(buf.Len()),
		minio.PutObjectOptions{
			ContentType:     "text/html",
			ContentEncoding: "gzip",
		},
	)
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}

	return nil
}

// GetHTML retrieves raw HTML content.
func (m *MinIOClient) GetHTML(ctx context.Context, contentHash string) ([]byte, error) {
	objectPath := htmlPath(contentHash)
	obj, err := m.client.GetObject(ctx, m.bucket, objectPath, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}
	defer obj.Close()

	// Decompress
	gz, err := gzip.NewReader(obj)
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	content, err := io.ReadAll(gz)
	if err != nil {
		return nil, fmt.Errorf("read content: %w", err)
	}

	return content, nil
}

// Exists checks if HTML content exists.
func (m *MinIOClient) Exists(ctx context.Context, contentHash string) (bool, error) {
	objectPath := htmlPath(contentHash)
	_, err := m.client.StatObject(ctx, m.bucket, objectPath, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("stat object: %w", err)
	}
	return true, nil
}

// Delete removes HTML content.
func (m *MinIOClient) Delete(ctx context.Context, contentHash string) error {
	objectPath := htmlPath(contentHash)
	if err := m.client.RemoveObject(ctx, m.bucket, objectPath, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("remove object: %w", err)
	}
	return nil
}

// GetStats returns storage statistics.
func (m *MinIOClient) GetStats(ctx context.Context) (int64, int64, error) {
	var totalObjects int64
	var totalSize int64

	objectCh := m.client.ListObjects(ctx, m.bucket, minio.ListObjectsOptions{
		Prefix:    "html/",
		Recursive: true,
	})

	for object := range objectCh {
		if object.Err != nil {
			return 0, 0, fmt.Errorf("list objects: %w", object.Err)
		}
		totalObjects++
		totalSize += object.Size
	}

	return totalObjects, totalSize, nil
}

// Close closes the MinIO client.
func (m *MinIOClient) Close() error {
	return nil // MinIO client doesn't need explicit close
}
