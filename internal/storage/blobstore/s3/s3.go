// Package s3 implements blobstore.Store on Amazon S3 (or an S3-compatible
// service via a custom endpoint).
package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/swayamsb50/web_crawler/internal/storage/blobstore"
)

// api is the subset of the S3 client used here, as an interface so tests
// can substitute a fake instead of talking to real AWS.
type api interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// Config controls the S3 store's target bucket and, optionally, a custom
// endpoint for S3-compatible services (e.g. MinIO).
type Config struct {
	Bucket   string
	Region   string
	Prefix   string // optional key prefix, e.g. "pages/"
	Endpoint string // optional; empty uses AWS's default endpoint resolution
}

// Store is a blobstore.Store backed by S3.
type Store struct {
	client api
	bucket string
	prefix string
}

// New builds a Store, loading AWS credentials from the standard SDK
// credential chain (env vars, shared config, instance role, etc).
func New(ctx context.Context, cfg Config) (*Store, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("s3: bucket must not be empty")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true // required by most non-AWS S3-compatible services
		}
	})

	return &Store{client: client, bucket: cfg.Bucket, prefix: cfg.Prefix}, nil
}

// Put implements blobstore.Store.
func (s *Store) Put(ctx context.Context, key string, data []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.prefix + key),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	return nil
}

// Get implements blobstore.Store.
func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.prefix + key),
	})
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return nil, blobstore.ErrNotFound
		}
		return nil, fmt.Errorf("s3 get %s: %w", key, err)
	}
	defer out.Body.Close()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("reading s3 object body: %w", err)
	}
	return data, nil
}
