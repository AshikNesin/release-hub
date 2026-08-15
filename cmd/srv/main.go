package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"srv.exe.dev/srv"
)

// envOr lets each flag be overridden by an environment variable:
// RELEASE_HUB_<UPPERCASE_NAME>. Flags win when both are given explicitly.
func envOr(k, def string) string {
	if v := os.Getenv(k); strings.TrimSpace(v) != "" {
		return v
	}
	return def
}

var (
	flagListen    = flag.String("listen", ":9100", "address to listen on")
	flagDB        = flag.String("db", "db.sqlite3", "sqlite database path")
	flagArtifacts = flag.String("artifacts", "artifacts", "local storage dir (ignored with -s3-bucket)")
	flagBaseURL   = flag.String("base-url", envOr("RELEASE_HUB_BASE_URL", "http://localhost:9100"), "public base URL used in manifest/download links (env: RELEASE_HUB_BASE_URL)")

	// Optional S3 storage. -s3-bucket switches the backend from local FS.
	flagS3Bucket   = flag.String("s3-bucket", "", "S3 bucket for artifacts (switches storage backend to S3)")
	flagS3Region   = flag.String("s3-region", envOr("AWS_REGION", "us-east-1"), "S3 region")
	flagS3Endpoint = flag.String("s3-endpoint", envOr("AWS_ENDPOINT_URL_S3", ""), "custom S3 endpoint (R2/MinIO), optional")
	flagS3Prefix   = flag.String("s3-prefix", "release-hub", "key prefix inside the bucket")
	flagS3Public   = flag.String("s3-public-base", "", "public URL base for bucket objects (set when the bucket is public/CloudFront); otherwise presigned URLs are used")
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	flag.Parse()
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	// Storage backend: local FS by default, S3-compatible when -s3-bucket set.
	ctx := context.Background()
	var st srv.Storage = &srv.LocalStorage{Dir: *flagArtifacts, BaseURL: *flagBaseURL}
	if *flagS3Bucket != "" {
		s3s, err := srv.NewS3Storage(ctx, srv.S3Options{
			Bucket: *flagS3Bucket, Region: *flagS3Region, Endpoint: *flagS3Endpoint,
			Prefix: *flagS3Prefix, PublicBase: *flagS3Public,
			KeyID:  os.Getenv("AWS_ACCESS_KEY_ID"),
			Secret: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		})
		if err != nil {
			return fmt.Errorf("s3 storage: %w", err)
		}
		st = s3s
	}
	server, err := srv.New(srv.Options{
		DBPath: *flagDB, Hostname: hostname, BaseURL: *flagBaseURL, Storage: st,
	})
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}
	return server.Serve(*flagListen)
}
