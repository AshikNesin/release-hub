package srv

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalStorageSaveOpenURL(t *testing.T) {
	dir := t.TempDir()
	l := &LocalStorage{Dir: dir, BaseURL: "http://hub.test/"}
	content := []byte("artifact-bytes")

	size, sha, err := l.Save(nil, "app/1_file.apk", bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(content)) {
		t.Fatalf("size %d", size)
	}
	sum := sha256.Sum256(content)
	if sha != hex.EncodeToString(sum[:]) {
		t.Fatalf("sha %s", sha)
	}

	rc, err := l.Open(nil, "app/1_file.apk")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	buf := new(bytes.Buffer)
	buf.ReadFrom(rc)
	if !bytes.Equal(buf.Bytes(), content) {
		t.Fatal("content mismatch")
	}

	u, _ := l.PublicURL(nil, "app/1_file.apk")
	if u != "http://hub.test/artifacts/app/1_file.apk" {
		t.Fatalf("url %s", u)
	}

	// no partial file left behind
	if _, err := os.Stat(filepath.Join(dir, "app", "1_file.apk.part")); !os.IsNotExist(err) {
		t.Fatal(".part file left behind")
	}

	// path traversal is contained: "../escape/x.apk" cleans to "/escape/x.apk",
	// which lands INSIDE the storage dir, never outside it.
	if _, _, err := l.Save(nil, "../escape/x.apk", bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "escape", "x.apk")); err != nil {
		t.Fatalf("traversal should be contained inside dir: %v", err)
	}
	if p := l.path("../../etc/passwd"); strings.HasPrefix(p, dir+"/..") || !strings.HasPrefix(p, dir) {
		t.Fatalf("path() escaped dir: %s", p)
	}
}

func TestS3ObjKeyPrefix(t *testing.T) {
	s := &S3Storage{Prefix: "release-hub", Bucket: "b"}
	if got := s.objKey("app/1_x.apk"); got != "release-hub/app/1_x.apk" {
		t.Fatalf("objKey: %s", got)
	}
}

func TestPublicURLSafe(t *testing.T) {
	got := publicURLSafe("https://b.s3.amazonaws.com/k?X-Amz-Signature=abc&X=1")
	if strings.Contains(got, "X-Amz-Signature") {
		t.Fatalf("signature leaked: %s", got)
	}
}
