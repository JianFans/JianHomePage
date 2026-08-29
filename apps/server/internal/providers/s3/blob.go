package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
)

type Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	UsePathStyle    bool
	RequireHTTPS    bool
	HTTPClient      *http.Client
}

type BlobStore struct {
	bucket    string
	client    *awss3.Client
	presigner *awss3.PresignClient
	now       func() time.Time
}

func NewBlobStore(config Config) (*BlobStore, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(config.Region),
		awsconfig.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			config.AccessKeyID, config.SecretAccessKey, config.SessionToken,
		)),
	}
	httpClient := s3HTTPClient(config.HTTPClient, config.RequireHTTPS)
	if httpClient != nil {
		loadOptions = append(loadOptions, awsconfig.WithHTTPClient(httpClient))
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(context.Background(), loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load S3 configuration: %w", err)
	}
	client := awss3.NewFromConfig(awsConfig, func(options *awss3.Options) {
		options.BaseEndpoint = aws.String(config.Endpoint)
		options.UsePathStyle = config.UsePathStyle
	})
	return &BlobStore{
		bucket:    config.Bucket,
		client:    client,
		presigner: awss3.NewPresignClient(client),
		now:       time.Now,
	}, nil
}

func s3HTTPClient(client *http.Client, requireHTTPS bool) *http.Client {
	if client == nil {
		if !requireHTTPS {
			return nil
		}
		client = http.DefaultClient
	}
	if !requireHTTPS {
		return client
	}
	secured := *client
	checkRedirect := client.CheckRedirect
	secured.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if request.URL.Scheme != "https" {
			return errors.New("S3 redirect must use HTTPS")
		}
		if checkRedirect != nil {
			return checkRedirect(request, via)
		}
		return nil
	}
	return &secured
}

func (store *BlobStore) CreateUpload(ctx context.Context, request ports.UploadRequest) (ports.SignedUpload, error) {
	if err := validateKey(request.BlobKey); err != nil || request.ContentType == "" || request.Size <= 0 || request.ExpiresIn <= 0 {
		return ports.SignedUpload{}, domain.ErrInvalidInput
	}
	metadata := checksumMetadata(request.Checksum)
	result, err := store.presigner.PresignPutObject(ctx, &awss3.PutObjectInput{
		Bucket:        aws.String(store.bucket),
		Key:           aws.String(request.BlobKey),
		ContentLength: aws.Int64(request.Size),
		ContentType:   aws.String(request.ContentType),
		Metadata:      metadata,
	}, func(options *awss3.PresignOptions) {
		options.Expires = request.ExpiresIn
	})
	if err != nil {
		return ports.SignedUpload{}, mapBlobError(err)
	}
	headers := map[string]string{"Content-Type": request.ContentType}
	if request.Checksum != "" {
		headers["X-Amz-Meta-Checksum"] = request.Checksum
	}
	return ports.SignedUpload{
		URL:       result.URL,
		Headers:   headers,
		ExpiresAt: store.now().UTC().Add(request.ExpiresIn),
	}, nil
}

func (store *BlobStore) Stat(ctx context.Context, key string) (ports.BlobMetadata, error) {
	if err := validateKey(key); err != nil {
		return ports.BlobMetadata{}, domain.ErrInvalidInput
	}
	result, err := store.client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(store.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return ports.BlobMetadata{}, mapBlobError(err)
	}
	return ports.BlobMetadata{
		ContentType: aws.ToString(result.ContentType),
		Size:        aws.ToInt64(result.ContentLength),
		Checksum:    result.Metadata["checksum"],
	}, nil
}

func (store *BlobStore) Put(ctx context.Context, key string, reader io.Reader, metadata ports.BlobMetadata) error {
	if err := validateKey(key); err != nil || reader == nil || metadata.ContentType == "" || metadata.Size < 0 {
		return domain.ErrInvalidInput
	}
	_, err := store.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket:        aws.String(store.bucket),
		Key:           aws.String(key),
		Body:          reader,
		ContentLength: aws.Int64(metadata.Size),
		ContentType:   aws.String(metadata.ContentType),
		Metadata:      checksumMetadata(metadata.Checksum),
	})
	return mapBlobError(err)
}

func (store *BlobStore) Delete(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return domain.ErrInvalidInput
	}
	_, err := store.client.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: aws.String(store.bucket),
		Key:    aws.String(key),
	})
	return mapBlobError(err)
}

func (store *BlobStore) SignedReadURL(ctx context.Context, key string, expiresIn time.Duration) (string, error) {
	if err := validateKey(key); err != nil || expiresIn <= 0 {
		return "", domain.ErrInvalidInput
	}
	result, err := store.presigner.PresignGetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(store.bucket),
		Key:    aws.String(key),
	}, func(options *awss3.PresignOptions) {
		options.Expires = expiresIn
	})
	if err != nil {
		return "", mapBlobError(err)
	}
	return result.URL, nil
}

func validateConfig(config Config) error {
	if strings.TrimSpace(config.Endpoint) == "" || strings.TrimSpace(config.Region) == "" ||
		strings.TrimSpace(config.Bucket) == "" || strings.TrimSpace(config.AccessKeyID) == "" ||
		strings.TrimSpace(config.SecretAccessKey) == "" {
		return errors.New("S3 endpoint, region, bucket, and credentials are required")
	}
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil {
		return errors.New("S3 endpoint must be an absolute HTTP URL without credentials")
	}
	if config.RequireHTTPS && endpoint.Scheme != "https" {
		return errors.New("S3 endpoint must use HTTPS")
	}
	return nil
}

func validateKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") || path.Clean(key) != key || key == "." {
		return domain.ErrInvalidInput
	}
	return nil
}

func checksumMetadata(checksum string) map[string]string {
	if checksum == "" {
		return nil
	}
	return map[string]string{"checksum": checksum}
}

func mapBlobError(err error) error {
	if err == nil {
		return nil
	}
	var statusError interface{ HTTPStatusCode() int }
	if errors.As(err, &statusError) {
		switch statusError.HTTPStatusCode() {
		case http.StatusNotFound:
			return errors.Join(domain.ErrNotFound, err)
		case http.StatusConflict, http.StatusPreconditionFailed:
			return errors.Join(domain.ErrConflict, err)
		}
	}
	var apiError smithy.APIError
	if errors.As(err, &apiError) {
		switch apiError.ErrorCode() {
		case "NoSuchKey", "NotFound", "NoSuchBucket":
			return errors.Join(domain.ErrNotFound, err)
		case "Conflict", "PreconditionFailed":
			return errors.Join(domain.ErrConflict, err)
		}
	}
	return err
}

var _ ports.BlobStore = (*BlobStore)(nil)
