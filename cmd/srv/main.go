package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"srv.exe.dev/srv"
)

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
	flagBaseURL   = flag.String("base-url", "http://localhost:9100", "public base URL used in manifest/download links")

	// Optional S3 storage. -s3-bucket switches the backend from local FS.
	flagS3Bucket   = flag.String("s3-bucket", "", "S3 bucket for artifacts (switches storage backend to S3)")
	flagS3Region   = flag.String("s3-region", envOr("AWS_REGION", "us-east-1"), "S3 region")
	flagS3Endpoint = flag.String("s3-endpoint", envOr("AWS_ENDPOINT_URL_S3", ""), "custom S3 endpoint (R2/MinIO), optional")
	flagS3Prefix   = flag.String("s3-prefix", "release-hub", "key prefix inside the bucket")
	flagS3Public   = flag.String("s3-public-base", "", "public URL base for bucket objects (set when the bucket is public/CloudFront); otherwise presigned URLs are used")

	// Optional Google Play publishing
	flagPlayCreds = flag.String("play-creds-dir", "", "dir with per-app service-account JSON files named <packageName>.json; .aab uploads for those apps are also pushed to Play (public→production, internal→internal)")
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
	server, err := srv.New(*flagDB, hostname)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}
	server.BaseURL = *flagBaseURL
	ctx := context.Background()
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
		server.Storage = s3s
	} else {
		server.Storage = &srv.LocalStorage{Dir: *flagArtifacts, BaseURL: *flagBaseURL}
	}
	server.PlayCredsDir = *flagPlayCreds
	return server.Serve(*flagListen)
}
