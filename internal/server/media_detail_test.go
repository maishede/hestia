package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hestia/internal/index"
)

func TestMediaDetailExposesFolderDeletion(t *testing.T) {
	srv, root := libraryFixture(t, "Series/film.mp4", "Series/photo.jpg", "loose.mp4")
	type detail struct {
		FolderID  string
		CanDelete bool
	}
	for _, name := range []string{"Series/film.mp4", "Series/photo.jpg"} {
		path := "/api/media/" + index.NewID(root, name)
		local := decodeLibraryResponse[detail](t, localLibraryRequest(srv, "GET", path, ""))
		if local.FolderID != index.NewID(root, "Series") || !local.CanDelete {
			t.Fatalf("local %s detail = %+v", name, local)
		}
		remote := decodeLibraryResponse[detail](t, libraryRequest(srv, "GET", path, "", "127.0.0.1:8080", "192.168.0.8:1234", ""))
		if remote.CanDelete {
			t.Fatalf("remote %s detail enables deletion", name)
		}
	}
	loose := decodeLibraryResponse[detail](t, localLibraryRequest(srv, "GET", "/api/media/"+index.NewID(root, "loose.mp4"), ""))
	if loose.CanDelete {
		t.Fatal("the configured library root cannot be renamed from a media detail")
	}
}

func TestFolderCoverWithPosterDoesNotRequireRedirect(t *testing.T) {
	srv, root := libraryFixture(t, "Series/film.mp4", "Series/poster.jpg")
	folders := srv.store.Roots(1, 20, "name", "asc").Folders
	want := "/api/media/" + index.NewID(root, "Series/poster.jpg") + "/image?w=480"
	if len(folders) != 1 || folders[0].Cover != want {
		t.Fatalf("folders = %+v, want cover %s", folders, want)
	}
}

func TestDeleteStopsOnlyTranscodesInSelectedFolder(t *testing.T) {
	t.Setenv("HESTIA_TEST_BLOCK_TRANSCODER_CHILD", "hold-source")
	srv, root := libraryFixture(t, "Series/film.mp4", "Other/keep.mp4")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	srv.tr = NewTranscoder(exe, t.TempDir(), func(id string) (string, error) {
		abs, _, err := srv.store.ResolveMedia(id)
		return abs, err
	})
	t.Cleanup(srv.tr.Close)
	start := func(rel string) string {
		t.Helper()
		mediaID := index.NewID(root, rel)
		response := decodeLibraryResponse[struct{ SessionID string }](t,
			localLibraryRequest(srv, "POST", "/api/media/"+mediaID+"/transcode", "{}"))
		return response.SessionID
	}
	deletedSession := start("Series/film.mp4")
	otherSession := start("Other/keep.mp4")
	url := "/api/folders/" + index.NewID(root, "Series")
	preview := decodeLibraryResponse[deletionPreviewResponse](t, localLibraryRequest(srv, "GET", url+"/delete-preview", ""))
	body, err := json.Marshal(map[string]string{"reason": "reviewed", "token": preview.Token})
	if err != nil {
		t.Fatal(err)
	}
	res := localLibraryRequest(srv, "POST", url+"/delete", string(body))
	if res.Code != http.StatusOK {
		t.Fatalf("deletion of playing video = %d, %s", res.Code, res.Body)
	}
	if srv.tr.Active() != 1 {
		t.Fatalf("expected only the unrelated transcode to remain, active = %d", srv.tr.Active())
	}
	if _, err := srv.tr.Serve(deletedSession, "index.m3u8"); err == nil {
		t.Error("deleted video session is still accessible")
	}
	if _, err := srv.tr.Serve(otherSession, "index.m3u8"); err != nil {
		t.Errorf("unrelated session was stopped: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Series-reviewed", "film.mp4")); !os.IsNotExist(err) {
		t.Errorf("file should be deleted: %v", err)
	}
	if strings.Contains(res.Body.String(), "warning") {
		t.Errorf("expected deletion without leftover files: %s", res.Body)
	}
}

func TestStopMatchingWaitsForSessionAlreadyStopping(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tr := NewTranscoder(exe, t.TempDir(), nil)
	t.Cleanup(tr.Close)
	done := make(chan struct{})
	// A prior DELETE has removed the session from the live map, but Wait
	// hasn't returned yet. Folder deletion must still wait for that process.
	tr.stopping["closing"] = &TranscodeSession{MediaID: "video", sourcePath: "film.mp4", done: done}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	err = tr.stopMatching(ctx, func(id, path string) bool { return id == "video" && path == "film.mp4" })
	if err != context.DeadlineExceeded {
		t.Fatalf("expected to wait for the source handle, got %v", err)
	}
	close(done)
	if err := tr.stopMatching(context.Background(), func(id, _ string) bool { return id == "video" }); err != nil {
		t.Fatal(err)
	}
}
