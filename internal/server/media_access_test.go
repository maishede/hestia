package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hestia/internal/index"
)

func TestDeleteCancelsOpenDirectStream(t *testing.T) {
	srv, root := libraryFixture(t, "Series/film.mp4", "Other/keep.mp4")
	videoPath := filepath.Join(root, "Series", "film.mp4")
	f, err := os.OpenFile(videoPath, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(64 << 20); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	listener := httptest.NewServer(srv.mux)
	defer listener.Close()
	addr := listener.Listener.Addr().String()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	mediaID := index.NewID(root, "Series/film.mp4")
	if _, err := fmt.Fprintf(conn, "GET /api/media/%s/stream HTTP/1.1\r\nHost: %s\r\nRange: bytes=0-\r\n\r\n", mediaID, addr); err != nil {
		t.Fatal(err)
	}
	// Read the first byte so the stream has opened the source, then stop
	// consuming the response and leave the connection open during deletion.
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var firstByte [1]byte
	if _, err := conn.Read(firstByte[:]); err != nil {
		t.Fatal(err)
	}

	folderURL := "/api/folders/" + index.NewID(root, "Series")
	preview := decodeLibraryResponse[deletionPreviewResponse](t, localLibraryRequest(srv, "GET", folderURL+"/delete-preview", ""))
	data, err := json.Marshal(map[string]string{"reason": "reviewed", "token": preview.Token})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, listener.URL+folderURL+"/delete", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", listener.URL)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("deletion during stalled stream returned %d", response.StatusCode)
	}
	if _, err := os.Stat(videoPath); !os.IsNotExist(err) {
		t.Fatalf("old source path still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Other", "keep.mp4")); err != nil {
		t.Fatal(err)
	}
}
