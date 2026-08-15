package srv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Storage is where release artifacts live. Implementations must stream
// (artifacts are 50MB+); nothing may buffer a whole file in memory.
type Storage interface {
	// Save streams src to the artifact identified by key, returning size and
	// sha256. Implementations must clean up on failure.
	Save(ctx context.Context, key string, src io.Reader) (size int64, sha256Hex string, err error)

	// Open returns a reader for the artifact (for the hub to serve/proxy).
	Open(ctx context.Context, key string) (io.ReadCloser, error)

	// PublicURL returns the URL devices should download from. For local
	// storage this is a hub-relative path; for S3 a presigned GET (or public
	// bucket URL) so traffic bypasses the hub entirely.
	PublicURL(ctx context.Context, key string) (string, error)

	// Delete removes the artifact. Best effort.
	Delete(ctx context.Context, key string) error
}

// ---- local filesystem ----

type LocalStorage struct {
	Dir     string // absolute dir under which artifacts/<slug>/<file> live
	BaseURL string // hub base URL for building download links
}

func (l *LocalStorage) path(key string) string {
	// keys are "<slug>/<file>"; sanitize to prevent traversal
	clean := path.Clean("/" + key)
	return l.Dir + clean
}

func (l *LocalStorage) Save(ctx context.Context, key string, src io.Reader) (int64, string, error) {
	full := l.path(key)
	if err := os.MkdirAll(path.Dir(full), 0o755); err != nil {
		return 0, "", err
	}
	tmp := full + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, "", err
	}
	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(f, h), src)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return 0, "", err
	}
	if err := os.Rename(tmp, full); err != nil {
		_ = os.Remove(tmp)
		return 0, "", err
	}
	return size, hex.EncodeToString(h.Sum(nil)), nil
}

func (l *LocalStorage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	return os.Open(l.path(key))
}

func (l *LocalStorage) PublicURL(ctx context.Context, key string) (string, error) {
	return strings.TrimSuffix(l.BaseURL, "/") + "/artifacts/" + key, nil
}

func (l *LocalStorage) Delete(ctx context.Context, key string) error {
	return os.Remove(l.path(key))
}

// ---- S3 ----

type staticCreds struct {
	v credentials.StaticCredentialsProvider
}

func (c *staticCreds) Retrieve(ctx context.Context) (aws.Credentials, error) {
	return c.v.Retrieve(ctx)
}

type S3Storage struct {
	Client  *s3.Client
	Bucket  string
	Prefix  string        // e.g. "release-hub/" (key becomes prefix+key)
	Expires time.Duration // presign TTL for PublicURL (default 7d)

	publicBase string // optional: if bucket objects are public, use this URL
	// instead of presigning (e.g. CloudFront domain)
}

type S3Options struct {
	Bucket   string
	Region   string
	Endpoint string // custom endpoint (R2, MinIO, ...); optional
	KeyID    string // static creds; optional — falls back to default chain
	Secret   string
	Prefix   string
	// PublicBase, if set, means bucket objects are world-readable via this
	// base URL (bucket policy / CloudFront). PublicURL then returns
	// PublicBase/key instead of a presigned URL.
	PublicBase string
}

func NewS3Storage(ctx context.Context, opt S3Options) (*S3Storage, error) {
	if opt.Bucket == "" {
		return nil, errors.New("s3 storage requires bucket")
	}
	var creds aws.CredentialsProvider
	if opt.KeyID != "" {
		creds = &staticCreds{v: credentials.NewStaticCredentialsProvider(opt.KeyID, opt.Secret, "")}
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(opt.Region),
		func(o *awsconfig.LoadOptions) error {
			if creds != nil {
				o.Credentials = creds
			}
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if opt.Endpoint != "" {
			o.BaseEndpoint = aws.String(opt.Endpoint)
			o.UsePathStyle = true // most S3-compatible stores need this
		}
	})
	return &S3Storage{
		Client:     client,
		Bucket:     opt.Bucket,
		Prefix:     strings.TrimSuffix(opt.Prefix, "/"),
		Expires:    7 * 24 * time.Hour,
		publicBase: strings.TrimSuffix(opt.PublicBase, "/"),
	}, nil
}

func (s *S3Storage) objKey(key string) string { return s.Prefix + "/" + key }

func (s *S3Storage) Save(ctx context.Context, key string, src io.Reader) (int64, string, error) {
	h := sha256.New()
	tee := io.TeeReader(src, h)
	up := manager.NewUploader(s.Client)
	_, err := up.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(s.objKey(key)),
		Body:   tee,
	})
	if err != nil {
		return 0, "", fmt.Errorf("s3 upload: %w", err)
	}
	// manager.Uploader doesn't report size; stat via HEAD.
	head, err := s.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.Bucket), Key: aws.String(s.objKey(key)),
	})
	if err != nil {
		return 0, "", fmt.Errorf("s3 head after upload: %w", err)
	}
	return aws.ToInt64(head.ContentLength), hex.EncodeToString(h.Sum(nil)), nil
}

func (s *S3Storage) S3Open(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket), Key: aws.String(s.objKey(key)),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (s *S3Storage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	return s.S3Open(ctx, key)
}

func (s *S3Storage) PublicURL(ctx context.Context, key string) (string, error) {
	if s.publicBase != "" {
		return s.publicBase + "/" + s.objKey(key), nil
	}
	p := s3.NewPresignClient(s.Client)
	req, err := p.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket), Key: aws.String(s.objKey(key)),
	}, s3.WithPresignExpires(s.Expires))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.Bucket), Key: aws.String(s.objKey(key)),
	})
	return err
}

// publicURLSafe strips the query (signature) from a presigned URL for display.
func publicURLSafe(u string) string {
	if p, err := url.Parse(u); err == nil {
		q := p.Query()
		if len(q) > 0 {
			p.RawQuery = ""
			return p.String() + "?…"
		}
	}
	return u
}
