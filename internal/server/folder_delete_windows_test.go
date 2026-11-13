//go:build windows

package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteRestoresMovedFilesWhenWindowsFileIsBusy(t *testing.T) {
	srv, root := libraryFixture(t, "Series/a.mp4", "Series/z.mp4")
	folder := srv.store.Roots(1, 20, "name", "asc").Folders[0]
	url := "/api/folders/" + folder.ID
	preview := decodeLibraryResponse[deletionPreviewResponse](t, localLibraryRequest(srv, "GET", url+"/delete-preview", ""))
	locked, err := os.Open(filepath.Join(root, "Series", "z.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	defer locked.Close()
	body, _ := json.Marshal(map[string]string{"reason": "reviewed", "token": preview.Token})
	response := localLibraryRequest(srv, "POST", url+"/delete", string(body))
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, response = %s", response.Code, response.Body)
	}
	for _, name := range []string{"a.mp4", "z.mp4"} {
		if _, err := os.Stat(filepath.Join(root, "Series", name)); err != nil {
			t.Errorf("expected %s to remain in the original folder: %v", name, err)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "Series" {
		t.Fatalf("rollback left an unexpected directory: %v", entries)
	}
	entries, err = os.ReadDir(filepath.Join(root, "Series"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("rollback left a temporary directory: %v", entries)
	}
}
