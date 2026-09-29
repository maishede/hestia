package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testClient(t *testing.T, payload []byte, binary []byte, checksum []byte) *Client {
	t.Helper()
	return &Client{
		latestURL:   latestURL,
		assetHost:   "github.com",
		assetPrefix: assetPrefix,
		http: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			var body []byte
			switch r.URL.String() {
			case latestURL:
				body = payload
			case "https://github.com/maishede/hestia/releases/download/v0.3.7/Hestia.exe":
				body = binary
			case "https://github.com/maishede/hestia/releases/download/v0.3.7/SHA256SUMS.txt":
				body = checksum
			default:
				t.Errorf("unexpected request: %s", r.URL)
				return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
		})},
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.3.7", "0.3.6", 1},
		{"0.3.7", "v0.3.7", 0},
		{"1.0.0", "0.99.99", 1},
		{"0.3.6", "0.3.7", -1},
	}
	for _, tc := range cases {
		got, err := CompareVersions(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Fatalf("CompareVersions(%q, %q) = %d, %v; want %d", tc.a, tc.b, got, err, tc.want)
		}
	}
	if _, err := CompareVersions("v0.3.7-beta", "0.3.6"); err == nil {
		t.Fatal("pre-release tag should not be accepted by the stable updater")
	}
}

func TestCheckAndDownload(t *testing.T) {
	binary := []byte("MZ-test-binary")
	hash := sha256.Sum256(binary)
	digest := hex.EncodeToString(hash[:])
	payload := []byte(`{"tag_name":"v0.3.7","html_url":"https://github.com/maishede/hestia/releases/tag/v0.3.7","assets":[{"name":"Hestia.exe","browser_download_url":"https://github.com/maishede/hestia/releases/download/v0.3.7/Hestia.exe","size":14},{"name":"SHA256SUMS.txt","browser_download_url":"https://github.com/maishede/hestia/releases/download/v0.3.7/SHA256SUMS.txt"}]}`)
	client := testClient(t, payload, binary, []byte(digest+"  Hestia.exe\n"))
	release, err := client.Check(context.Background(), "0.3.6")
	if err != nil || !release.Available || release.Version != "0.3.7" {
		t.Fatalf("Check = %+v, %v", release, err)
	}
	destination := filepath.Join(t.TempDir(), "Hestia-v0.3.7.exe")
	var done int64
	if err := client.Download(context.Background(), release, destination, func(n, _ int64) { done = n }); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(got, binary) || done != int64(len(binary)) {
		t.Fatalf("download = %q, %d, %v", got, done, err)
	}
	current, err := client.Check(context.Background(), "0.3.7")
	if err != nil || current.Available {
		t.Fatalf("current version should not update: %+v, %v", current, err)
	}
}

func TestDownloadRejectsChecksumMismatch(t *testing.T) {
	binary := []byte("MZ-test-binary")
	client := testClient(t, nil, binary, nil)
	release := Release{
		Tag: "v0.3.7", Available: true, Size: int64(len(binary)),
		AssetURL: "https://github.com/maishede/hestia/releases/download/v0.3.7/Hestia.exe",
		Digest:   strings.Repeat("0", 64),
	}
	destination := filepath.Join(t.TempDir(), "Hestia-v0.3.7.exe")
	if err := client.Download(context.Background(), release, destination, nil); err == nil {
		t.Fatal("checksum mismatch should fail")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("unverified binary should not be installed: %v", err)
	}
}

func TestRejectsForeignAssetURL(t *testing.T) {
	client := NewClient()
	if client.validAssetURL("https://example.com/maishede/hestia/releases/download/v0.3.7/Hestia.exe", "v0.3.7") ||
		client.validAssetURL("https://github.com/another/hestia/releases/download/v0.3.7/Hestia.exe", "v0.3.7") {
		t.Fatal("foreign release asset should be rejected")
	}
}

func TestCheckFallsBackWhenAPIRateLimited(t *testing.T) {
	binary := []byte("MZ-test-binary")
	hash := sha256.Sum256(binary)
	checksum := []byte(hex.EncodeToString(hash[:]) + "  Hestia.exe\n")
	client := &Client{
		latestURL: latestURL, assetHost: "github.com", assetPrefix: assetPrefix,
		http: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			status := http.StatusOK
			body := []byte{}
			header := make(http.Header)
			switch r.URL.String() {
			case latestURL:
				status = http.StatusForbidden
			case latestPage:
				status = http.StatusFound
				header.Set("Location", "https://github.com/maishede/hestia/releases/tag/v0.3.7")
			case "https://github.com/maishede/hestia/releases/tag/v0.3.7":
			case "https://github.com/maishede/hestia/releases/download/v0.3.7/SHA256SUMS.txt":
				body = checksum
			case "https://github.com/maishede/hestia/releases/download/v0.3.7/Hestia.exe":
				body = binary
			default:
				t.Errorf("unexpected request: %s", r.URL)
			}
			return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
		})},
	}
	release, err := client.Check(context.Background(), "0.3.6")
	if err != nil || !release.Available || release.Size != 0 || release.Version != "0.3.7" {
		t.Fatalf("fallback Check = %+v, %v", release, err)
	}
	destination := filepath.Join(t.TempDir(), "Hestia-v0.3.7.exe")
	if err := client.Download(context.Background(), release, destination, nil); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(got, binary) {
		t.Fatalf("fallback download = %q, %v", got, err)
	}
}
