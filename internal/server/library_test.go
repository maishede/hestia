package server

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"hestia/internal/index"
	"hestia/internal/progress"
)

func libraryFixture(t *testing.T, files ...string) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	for _, name := range files {
		putLibraryFile(t, root, name)
	}
	data := t.TempDir()
	store, err := index.New(data)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	libID, err := store.AddLibrary(root, "测试媒体库")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RescanLibrary(libID); err != nil {
		t.Fatal(err)
	}
	prog, err := progress.New(data)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(prog.Close)
	srv, err := New(nil, store, Toolchain{}, nil, prog, data, os.DirFS("../../web"), log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	return srv, root
}

func putLibraryFile(t *testing.T, root, name string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func libraryRequest(s *Server, method, path, body, host, remote, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(body))
	req.RemoteAddr = remote
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	res := httptest.NewRecorder()
	s.mux.ServeHTTP(res, req)
	return res
}

func localLibraryRequest(s *Server, method, path, body string) *httptest.ResponseRecorder {
	return libraryRequest(s, method, path, body, "127.0.0.1:8080", "127.0.0.1:12345", "http://127.0.0.1:8080")
}

func decodeLibraryResponse[T any](t *testing.T, res *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body)
	}
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCollectionPages(t *testing.T) {
	srv, root := libraryFixture(t, "Film2/a.mp4", "Film10/a.mp4", "Series/Disc2/two.mp4", "Series/Disc10/ten.mp4",
		"Series/ep2.mp4", "Series/ep10.mp4", "Series/ep2.jpg", "Series/ep2.srt")
	type rootsResponse struct {
		Folders []index.FolderSummary
		Total   int
		Page    int
		Size    int
	}
	res := decodeLibraryResponse[rootsResponse](t, localLibraryRequest(srv, "GET", "/api/folders?page=2&size=1&sort=name", ""))
	if len(res.Folders) != 1 || res.Folders[0].Name != "Film10" || res.Total != 3 || res.Page != 2 || res.Size != 1 {
		t.Fatalf("root page = %+v", res)
	}
	path := "/api/folders/" + index.NewID(root, "Series") + "/children?"
	type childrenResponse struct {
		Folders      []index.FolderSummary
		FoldersTotal int
		FoldersPage  int
		Media        index.MediaPage
	}
	children := decodeLibraryResponse[childrenResponse](t, localLibraryRequest(srv, "GET", path+"page=1&folderPage=2&size=1", ""))
	if len(children.Folders) != 1 || children.Folders[0].Name != "Disc10" || children.FoldersPage != 2 || children.FoldersTotal != 2 {
		t.Fatalf("child folder page = %+v", children)
	}
	if len(children.Media.Items) != 1 || children.Media.Items[0].Name != "ep2" || children.Media.Total != 2 {
		t.Fatalf("media page = %+v", children.Media)
	}
	hugePage := strconv.Itoa(int(^uint(0) >> 1))
	res = decodeLibraryResponse[rootsResponse](t, localLibraryRequest(srv, "GET", "/api/folders?page="+hugePage, ""))
	if res.Folders == nil || len(res.Folders) != 0 || res.Total != 3 {
		t.Fatalf("out-of-range page = %+v", res)
	}
	for _, path := range []string{path + "page=" + hugePage, "/api/search?q=ep&page=" + hugePage} {
		if res := localLibraryRequest(srv, "GET", path, ""); res.Code != http.StatusOK {
			t.Fatalf("large page status = %d for %s", res.Code, path)
		}
	}
}

func TestActorListAndVideoPages(t *testing.T) {
	srv, root := libraryFixture(t, "林 青 & 吴 -第2集.MP4", "合集/子目录/林 青 & 吴-第10集-加长版.mkv",
		"合集/林 青 & 吴二号-其他.mp4", "合集/只有文件名.mp4", "合集/-没有主演.mp4",
		"合集/没有片名-.MP4", "合集/海报-剧照.jpg", "合集/字幕-轨道.srt",
		"合集/陈-第1集.mp4", "合集/陈-第3集.mp4", "合集/陈-第4集.mp4")
	type actor struct {
		Name  string
		Count int
	}
	type actorList struct {
		Actors []actor
	}
	readActors := func() []actor {
		return decodeLibraryResponse[actorList](t, localLibraryRequest(srv, "GET", "/api/actors", "")).Actors
	}
	want := []actor{{"陈", 3}, {"林 青 & 吴", 2}, {"林 青 & 吴二号", 1}}
	if got := readActors(); !slices.Equal(got, want) {
		t.Fatalf("actors = %+v, want %+v", got, want)
	}
	path := "/api/actors/videos?actor=" + url.QueryEscape("林 青 & 吴")
	second := decodeLibraryResponse[index.MediaPage](t, localLibraryRequest(srv, "GET", path+"&size=1&page=2&sort=name", ""))
	if second.Page != 2 || second.Size != 1 || second.Total != 2 || len(second.Items) != 1 ||
		second.Items[0].ID != index.NewID(root, "合集/子目录/林 青 & 吴-第10集-加长版.mkv") || second.Items[0].Kind != "video" {
		t.Fatalf("second actor page = %+v", second)
	}
	firstDesc := decodeLibraryResponse[index.MediaPage](t, localLibraryRequest(srv, "GET", path+"&size=1&sort=name&order=desc", ""))
	if firstDesc.Page != 1 || firstDesc.Total != 2 || len(firstDesc.Items) != 1 || firstDesc.Items[0].ID != second.Items[0].ID {
		t.Fatalf("descending actor page = %+v", firstDesc)
	}
	empty := decodeLibraryResponse[index.MediaPage](t, localLibraryRequest(srv, "GET", path+"&size=1&page=3", ""))
	if empty.Total != 2 || empty.Items == nil || len(empty.Items) != 0 {
		t.Fatalf("out-of-range actor page = %+v", empty)
	}
	unknown := decodeLibraryResponse[index.MediaPage](t, localLibraryRequest(srv, "GET", "/api/actors/videos?actor="+url.QueryEscape("林 青"), ""))
	if unknown.Total != 0 || unknown.Items == nil || len(unknown.Items) != 0 {
		t.Fatalf("unknown actor page = %+v", unknown)
	}
	old := filepath.Join(root, "林 青 & 吴 -第2集.MP4")
	if err := os.Rename(old, filepath.Join(root, "陈-第2集.MP4")); err != nil {
		t.Fatal(err)
	}
	libID := srv.store.LibID(root)
	if err := srv.store.RescanLibrary(libID); err != nil {
		t.Fatal(err)
	}
	want = []actor{{"陈", 4}, {"林 青 & 吴", 1}, {"林 青 & 吴二号", 1}}
	if got := readActors(); !slices.Equal(got, want) {
		t.Fatalf("actors after rename and rescan = %+v, want %+v", got, want)
	}
	srv.store.SetLibraryEnabled(libID, false)
	if got := readActors(); got == nil || len(got) != 0 {
		t.Fatalf("actors after disabling library = %+v", got)
	}
}

func TestSearchMatchesMediaNamesWithoutFolders(t *testing.T) {
	srv, _ := libraryFixture(t, "王小明/片段.mp4", "合集/王小明-片段.mp4",
		"合集/王小明-片段.jpg", "合集/王小明-剧照.png", "合集/王小明-字幕.srt")
	for _, query := range []string{"王小明", "wangxiaoming"} {
		t.Run(query, func(t *testing.T) {
			path := "/api/search?q=" + url.QueryEscape(query) + "&size=1"
			first := decodeLibraryResponse[index.SearchResult](t, localLibraryRequest(srv, "GET", path, ""))
			second := decodeLibraryResponse[index.SearchResult](t, localLibraryRequest(srv, "GET", path+"&page=2", ""))
			if first.Total != 2 || second.Total != 2 || len(first.Items) != 1 || len(second.Items) != 1 {
				t.Fatalf("search pages = %+v and %+v", first, second)
			}
			got := map[string]string{first.Items[0].Name: first.Items[0].Kind, second.Items[0].Name: second.Items[0].Kind}
			if got["王小明-片段"] != "video" || got["王小明-剧照"] != "image" {
				t.Fatalf("search media names and kinds = %+v", got)
			}
		})
	}
}

func TestRecentProgressIncludesCard(t *testing.T) {
	srv, root := libraryFixture(t, "Series/ep.mp4", "Series/ep.jpg")
	videoID := index.NewID(root, "Series/ep.mp4")
	srv.prog.Set(progress.Entry{MediaID: videoID, Position: 32, Duration: 60})
	type recentResponse struct {
		Items []struct {
			progress.Entry
			CardCover string
			Mtime     time.Time
		}
	}
	res := decodeLibraryResponse[recentResponse](t, localLibraryRequest(srv, "GET", "/api/progress/recent", ""))
	want := "/api/media/" + index.NewID(root, "Series/ep.jpg") + "/image?w=480"
	if len(res.Items) != 1 || res.Items[0].Position != 32 || res.Items[0].CardCover != want ||
		res.Items[0].Folder != "Series" || res.Items[0].Mtime.IsZero() {
		t.Fatalf("recent = %+v, want cover %s", res, want)
	}
}

type deletionPreviewResponse struct {
	Name       string
	Files      int
	Subfolders int
	Token      string
}

func TestDeleteOnlyDirectMediaAndRename(t *testing.T) {
	srv, root := libraryFixture(t, "Series/film.mp4", "Series/film.JPG", "Series/film.srt",
		"Series/notes.nfo", "Series/Disc2/keep.mp4")
	folder := "/api/folders/" + index.NewID(root, "Series")
	videoID := index.NewID(root, "Series/film.mp4")
	childVideoID := index.NewID(root, "Series/Disc2/keep.mp4")
	srv.prog.Set(progress.Entry{MediaID: videoID, Position: 32})
	srv.prog.Set(progress.Entry{MediaID: childVideoID, Position: 42})
	preview := decodeLibraryResponse[deletionPreviewResponse](t, localLibraryRequest(srv, "GET", folder+"/delete-preview", ""))
	if preview.Files != 3 || preview.Subfolders != 1 || preview.Token == "" {
		t.Fatalf("preview = %+v", preview)
	}
	body, err := json.Marshal(map[string]string{"reason": "  已看完  ", "token": preview.Token})
	if err != nil {
		t.Fatal(err)
	}
	res := decodeLibraryResponse[struct{ ID, Name string }](t, localLibraryRequest(srv, "POST", folder+"/delete", string(body)))
	if res.Name != "Series-已看完" || res.ID != index.NewID(root, "Series-已看完") {
		t.Fatalf("result = %+v", res)
	}
	for _, path := range []string{"Series", "Series-已看完/film.mp4", "Series-已看完/film.JPG", "Series-已看完/film.srt"} {
		if _, err := os.Stat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Errorf("%s still exists or was inaccessible: %v", path, err)
		}
	}
	for _, path := range []string{"Series-已看完/notes.nfo", "Series-已看完/Disc2/keep.mp4"} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Errorf("%s not preserved: %v", path, err)
		}
	}
	if _, ok := srv.store.GetMedia(videoID); ok {
		t.Error("deleted video is still indexed")
	}
	if _, ok := srv.prog.Get(videoID); ok {
		t.Error("deleted video's progress remains")
	}
	keptID := index.NewID(root, "Series-已看完/Disc2/keep.mp4")
	if _, ok := srv.store.GetMedia(keptID); !ok {
		t.Error("child video was not reindexed")
	}
	if kept, ok := srv.prog.Get(keptID); !ok || kept.Position != 42 {
		t.Error("child video's progress was not preserved")
	}
	for _, id := range []string{videoID, childVideoID} {
		data, _ := json.Marshal(progress.Entry{MediaID: id, Position: 48})
		if r := localLibraryRequest(srv, "POST", "/api/progress", string(data)); r.Code != http.StatusNoContent {
			t.Fatalf("late progress returned %d", r.Code)
		}
	}
	if kept, ok := srv.prog.Get(keptID); !ok || kept.Position != 48 {
		t.Error("late progress from a buffered child video did not follow the renamed folder")
	}
	if _, ok := srv.prog.Get(videoID); ok {
		t.Error("late progress recreated a deleted video's entry")
	}
}

func TestDeleteDirectSubtitlesWithOnlyNestedVideos(t *testing.T) {
	srv, root := libraryFixture(t, "Series/clip.vtt", "Series/Disc2/keep.mp4")
	folder := "/api/folders/" + index.NewID(root, "Series")
	children := decodeLibraryResponse[index.ChildrenResult](t, localLibraryRequest(srv, "GET", folder+"/children", ""))
	if !children.CanDelete || children.Media.Total != 0 {
		t.Fatalf("local deletion unavailable for a folder with direct subtitles: %+v", children)
	}
	preview := decodeLibraryResponse[deletionPreviewResponse](t, localLibraryRequest(srv, "GET", folder+"/delete-preview", ""))
	if preview.Files != 1 || preview.Subfolders != 1 {
		t.Fatalf("subtitle preview = %+v", preview)
	}
}

func TestFolderDeleteLocalAddressOnSpecificLANBind(t *testing.T) {
	address := &net.TCPAddr{IP: net.ParseIP("192.168.50.1"), Port: 8080}
	req := httptest.NewRequest(http.MethodGet, "http://192.168.50.1:8080/api/folders/x/delete-preview", nil)
	req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, address))
	req.RemoteAddr = "192.168.50.1:12345"
	if !localLibraryAccess(req) {
		t.Fatal("a request from the listening host was rejected")
	}
	req.RemoteAddr = "192.168.50.2:12345"
	if localLibraryAccess(req) {
		t.Fatal("a different LAN device gained file access")
	}
	req.RemoteAddr = "192.168.50.1:12345"
	req.Host = "external.invalid"
	if localLibraryAccess(req) {
		t.Fatal("an external host gained file access")
	}
}

func TestDeleteRefusesUnsafeOrStaleRequests(t *testing.T) {
	srv, root := libraryFixture(t, "Series/film.mp4", "Series-occupied/other.mp4")
	folder := "/api/folders/" + index.NewID(root, "Series")
	preview := decodeLibraryResponse[deletionPreviewResponse](t, localLibraryRequest(srv, "GET", folder+"/delete-preview", ""))
	postBody := func(reason string) string {
		body, _ := json.Marshal(map[string]string{"reason": reason, "token": preview.Token})
		return string(body)
	}
	for _, tc := range []struct {
		name, host, remote, origin string
	}{
		{"remote client", "127.0.0.1:8080", "192.168.0.8:1234", "http://127.0.0.1:8080"},
		{"external origin", "127.0.0.1:8080", "127.0.0.1:1234", "http://external.invalid"},
		{"external host", "external.invalid", "127.0.0.1:1234", "http://external.invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := libraryRequest(srv, "POST", folder+"/delete", postBody("reviewed"), tc.host, tc.remote, tc.origin)
			if res.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", res.Code)
			}
		})
	}
	for _, tc := range []struct {
		reason string
		status int
	}{{"../outside", http.StatusBadRequest}, {"", http.StatusBadRequest}, {"occupied", http.StatusConflict}} {
		res := localLibraryRequest(srv, "POST", folder+"/delete", postBody(tc.reason))
		if res.Code != tc.status {
			t.Fatalf("reason %q status = %d, want %d", tc.reason, res.Code, tc.status)
		}
	}
	putLibraryFile(t, root, "Series/new.jpg")
	res := localLibraryRequest(srv, "POST", folder+"/delete", postBody("changed"))
	if res.Code != http.StatusConflict {
		t.Fatalf("stale preview status = %d, want 409", res.Code)
	}
	for _, name := range []string{"Series/film.mp4", "Series/new.jpg", "Series-occupied/other.mp4"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Errorf("%s was affected: %v", name, err)
		}
	}
}

func TestConcurrentDeleteUsesConfirmationOnce(t *testing.T) {
	srv, root := libraryFixture(t, "Series/film.mp4", "Series/notes.txt")
	folder := "/api/folders/" + index.NewID(root, "Series")
	preview := decodeLibraryResponse[deletionPreviewResponse](t, localLibraryRequest(srv, "GET", folder+"/delete-preview", ""))
	body, _ := json.Marshal(map[string]string{"reason": "reviewed", "token": preview.Token})
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for range 2 {
		wg.Go(func() {
			statuses <- localLibraryRequest(srv, "POST", folder+"/delete", string(body)).Code
		})
	}
	wg.Wait()
	first, second := <-statuses, <-statuses
	if first != http.StatusOK || second != http.StatusNotFound {
		if second != http.StatusOK || first != http.StatusNotFound {
			t.Fatalf("delete statuses = %d and %d", first, second)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "Series-reviewed", "notes.txt")); err != nil {
		t.Fatal(err)
	}
}
