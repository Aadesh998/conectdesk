package main

import (
	"bytes"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/conectdesk/conectdesk/internal/image"
	"github.com/conectdesk/conectdesk/internal/media/stores/s3"
	"github.com/jmoiron/sqlx"
)

// copyUploadsToS3 copies the files uploaded to the fs store into the s3
// bucket, for a move from a disk to S3 or MinIO. Both stores name a file by
// its media UUID, so the media rows keep working once upload.provider is s3.
// Files already in the bucket are skipped, so it can be run again.
func copyUploadsToS3(db *sqlx.DB) {
	dir := filepath.Clean(ko.String("upload.fs.upload_path"))
	store, err := s3.New(s3.Opt{
		URL:        ko.String("upload.s3.url"),
		AccessKey:  ko.String("upload.s3.access_key"),
		SecretKey:  ko.String("upload.s3.secret_key"),
		Region:     ko.String("upload.s3.region"),
		Bucket:     ko.String("upload.s3.bucket"),
		BucketPath: ko.String("upload.s3.bucket_path"),
		BucketType: "private",
	})
	if err != nil {
		log.Fatalf("error initializing s3 store: %v", err)
	}
	client := store.(*s3.Client)
	if err := client.EnsureBucket(); err != nil {
		log.Fatalf("error checking s3 bucket: %v", err)
	}

	// Content types as recorded at upload; a thumbnail has its image's.
	types := map[string]string{}
	rows, err := db.Query(`SELECT uuid::TEXT, content_type FROM media`)
	if err != nil {
		log.Fatalf("error reading media: %v", err)
	}
	for rows.Next() {
		var id, ct string
		if err := rows.Scan(&id, &ct); err == nil {
			types[id] = ct
		}
	}
	rows.Close()

	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Fatalf("error reading %s: %v", dir, err)
	}
	var copied, skipped, failed int
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		if client.Exists(name) {
			skipped++
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			log.Printf("error reading %s: %v", name, err)
			failed++
			continue
		}
		ct := types[strings.TrimPrefix(name, image.ThumbPrefix)]
		if ct == "" {
			ct = http.DetectContentType(b)
		}
		if _, err := client.Put(name, ct, bytes.NewReader(b)); err != nil {
			log.Printf("error copying %s: %v", name, err)
			failed++
			continue
		}
		copied++
	}
	log.Printf("uploads copied to s3: %d copied, %d already there, %d failed", copied, skipped, failed)
	if failed > 0 {
		os.Exit(1)
	}
}
