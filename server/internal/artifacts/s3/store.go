package s3store

import (
	"context"
	"fmt"
	"io"

	"xmdm/server/internal/artifacts"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Config struct {
	Endpoint        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	UsePathStyle    bool
}

type Store struct {
	client *s3.Client
	bucket string
}

var _ artifacts.Store = (*Store)(nil)

func New(ctx context.Context, cfg Config) (*Store, error) {
	if cfg.Endpoint == "" || cfg.Region == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" || cfg.Bucket == "" {
		return nil, fmt.Errorf("missing object storage configuration")
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = cfg.UsePathStyle
	})
	store := &Store{client: client, bucket: cfg.Bucket}
	if err := store.ensureBucket(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Put(ctx context.Context, key string, body io.Reader, contentType string, contentLength int64) error {
	if key == "" || body == nil {
		return fmt.Errorf("missing artifact key or body")
	}
	input := &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(contentLength),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	_, err := s.client.PutObject(ctx, input)
	return err
}

func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if key == "" {
		return nil, fmt.Errorf("missing artifact key")
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("missing artifact key")
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

func (s *Store) ensureBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	if err == nil {
		return nil
	}
	if _, createErr := s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.bucket)}); createErr == nil {
		return nil
	}
	return err
}
