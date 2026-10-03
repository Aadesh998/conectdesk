package s3

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// TestMinIO checks the store against a MinIO it can reach, as in
// docker-compose: the bucket is created on first use, and a file put is
// found and downloadable from its signed link. Set CONECTDESK_TEST_S3_URL
// (e.g. http://127.0.0.1:9100) with minioadmin credentials to run it. Links
// are signed for localhost, as for a bucket browsers reach at another address
// than the app does.
func TestMinIO(t *testing.T) {
	endpoint := os.Getenv("CONECTDESK_TEST_S3_URL")
	if endpoint == "" {
		t.Skip("CONECTDESK_TEST_S3_URL not set")
	}
	presign := strings.Replace(endpoint, "127.0.0.1", "localhost", 1)
	st, err := New(Opt{URL: endpoint, PresignURL: presign, AccessKey: "minioadmin", SecretKey: "minioadmin", Region: "us-east-1",
		Bucket: "conectdesk-test", BucketType: "private"})
	if err != nil {
		t.Fatal(err)
	}
	c := st.(*Client)
	for i := 0; i < 2; i++ {
		if err := c.EnsureBucket(); err != nil {
			t.Fatalf("ensure bucket, try %d: %v", i+1, err)
		}
	}
	if c.Exists("call.mp3") {
		t.Fatal("file exists before upload")
	}
	if _, err := c.Put("call.mp3", "audio/mpeg", strings.NewReader("ID3 fake audio")); err != nil {
		t.Fatal(err)
	}
	if !c.Exists("call.mp3") {
		t.Fatal("file missing after upload")
	}
	link := c.GetURL(1, "call.mp3", "attachment", "call.mp3")
	if !strings.HasPrefix(link, presign+"/") {
		t.Fatalf("link not signed for %s: %s", presign, link)
	}
	res, err := http.Get(link)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || string(b) != "ID3 fake audio" {
		t.Fatalf("signed download: %s %q", res.Status, b)
	}
	if err := c.Delete("call.mp3"); err != nil {
		t.Fatal(err)
	}
}
