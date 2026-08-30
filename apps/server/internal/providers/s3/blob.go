package s3

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	awss3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/httpurl"
	"yujian.me/server/internal/ports"
)

type Config struct {
	Endpoint        string
	PublicBaseURL   string
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
	bucket        string
	publicBaseURL *url.URL
	client        *awss3.Client
	presigner     *awss3.PresignClient
	now           func() time.Time
}

const defaultHTTPTimeout = 30 * time.Second

func NewBlobStore(config Config) (*BlobStore, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	publicBaseURL, err := parsePublicBaseURL(config.PublicBaseURL, config.RequireHTTPS)
	if err != nil {
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
		bucket:        config.Bucket,
		publicBaseURL: publicBaseURL,
		client:        client,
		presigner:     awss3.NewPresignClient(client),
		now:           time.Now,
	}, nil
}

func s3HTTPClient(client *http.Client, requireHTTPS bool) *http.Client {
	if client == nil {
		if !requireHTTPS {
			return nil
		}
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	if !requireHTTPS {
		return client
	}
	secured := *client
	if secured.Timeout <= 0 {
		secured.Timeout = defaultHTTPTimeout
	}
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
	checksum, checksumErr := encodeProviderChecksum(request.Checksum)
	if keyErr := validateKey(request.BlobKey); keyErr != nil || request.ContentType == "" || request.Size <= 0 || request.ExpiresIn <= 0 || checksumErr != nil {
		return ports.SignedUpload{}, domain.ErrInvalidInput
	}
	result, err := store.presigner.PresignPutObject(ctx, &awss3.PutObjectInput{
		Bucket:         aws.String(store.bucket),
		Key:            aws.String(request.BlobKey),
		ContentLength:  aws.Int64(request.Size),
		ContentType:    aws.String(request.ContentType),
		ChecksumSHA256: aws.String(checksum),
	}, func(options *awss3.PresignOptions) {
		options.Expires = request.ExpiresIn
	})
	if err != nil {
		return ports.SignedUpload{}, mapBlobError(err)
	}
	headers := map[string]string{"Content-Type": request.ContentType}
	headers["X-Amz-Checksum-Sha256"] = checksum
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
		Bucket:       aws.String(store.bucket),
		Key:          aws.String(key),
		ChecksumMode: awss3types.ChecksumModeEnabled,
	})
	if err != nil {
		return ports.BlobMetadata{}, mapBlobError(err)
	}
	return ports.BlobMetadata{
		ContentType: aws.ToString(result.ContentType),
		Size:        aws.ToInt64(result.ContentLength),
		Checksum:    decodeProviderChecksum(aws.ToString(result.ChecksumSHA256)),
	}, nil
}

func (store *BlobStore) Put(ctx context.Context, key string, reader io.Reader, metadata ports.BlobMetadata) error {
	if err := validateKey(key); err != nil || reader == nil || metadata.ContentType == "" || metadata.Size < 0 {
		return domain.ErrInvalidInput
	}
	checksum, err := encodeProviderChecksum(metadata.Checksum)
	if err != nil {
		return domain.ErrInvalidInput
	}
	_, err = store.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket:         aws.String(store.bucket),
		Key:            aws.String(key),
		Body:           reader,
		ContentLength:  aws.Int64(metadata.Size),
		ContentType:    aws.String(metadata.ContentType),
		ChecksumSHA256: aws.String(checksum),
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

func (store *BlobStore) PublicURL(_ context.Context, key string) (string, error) {
	if err := validateKey(key); err != nil {
		return "", domain.ErrInvalidInput
	}
	return store.publicBaseURL.JoinPath(strings.Split(key, "/")...).String(), nil
}

func validateConfig(config Config) error {
	if strings.TrimSpace(config.Endpoint) == "" || strings.TrimSpace(config.Region) == "" ||
		strings.TrimSpace(config.Bucket) == "" || strings.TrimSpace(config.AccessKeyID) == "" ||
		strings.TrimSpace(config.SecretAccessKey) == "" {
		return errors.New("S3 endpoint, region, bucket, and credentials are required")
	}
	endpoint, err := httpurl.ParseAbsolute(config.Endpoint)
	if err != nil {
		return errors.New("S3 endpoint must be an absolute HTTP URL without credentials")
	}
	if config.RequireHTTPS && endpoint.Scheme != "https" {
		return errors.New("S3 endpoint must use HTTPS")
	}
	if _, err := parsePublicBaseURL(config.PublicBaseURL, config.RequireHTTPS); err != nil {
		return err
	}
	return nil
}

func parsePublicBaseURL(value string, requireHTTPS bool) (*url.URL, error) {
	publicBaseURL, err := httpurl.ParseAbsolute(strings.TrimSpace(value))
	if err != nil || publicBaseURL.RawQuery != "" || publicBaseURL.Fragment != "" {
		return nil, errors.New("media public base URL must be an absolute HTTP URL without credentials, query, or fragment")
	}
	if requireHTTPS && publicBaseURL.Scheme != "https" {
		return nil, errors.New("media public base URL must use HTTPS")
	}
	return publicBaseURL, nil
}

func validateKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") || path.Clean(key) != key || key == "." {
		return domain.ErrInvalidInput
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == ".." {
			return domain.ErrInvalidInput
		}
	}
	return nil
}

func encodeProviderChecksum(checksum string) (string, error) {
	if !strings.HasPrefix(checksum, "sha256:") {
		return "", errors.New("SHA-256 checksum is required")
	}
	digest, err := hex.DecodeString(strings.TrimPrefix(checksum, "sha256:"))
	if err != nil || len(digest) != sha256.Size {
		return "", errors.New("invalid SHA-256 checksum")
	}
	return base64.StdEncoding.EncodeToString(digest), nil
}

func decodeProviderChecksum(checksum string) string {
	digest, err := base64.StdEncoding.DecodeString(checksum)
	if err != nil || len(digest) != sha256.Size {
		return ""
	}
	return "sha256:" + hex.EncodeToString(digest)
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
