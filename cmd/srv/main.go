package main

import (
	"flag"
	"fmt"
	"os"

	"srv.exe.dev/srv"
)

var (
	flagListen    = flag.String("listen", ":9100", "address to listen on")
	flagDB        = flag.String("db", "db.sqlite3", "sqlite database path")
	flagArtifacts = flag.String("artifacts", "artifacts", "directory to store uploaded artifacts")
	flagBaseURL   = flag.String("base-url", "http://localhost:9100", "public base URL used in manifest/download links")
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
	server.ArtifactsDir = *flagArtifacts
	server.BaseURL = *flagBaseURL
	return server.Serve(*flagListen)
}
