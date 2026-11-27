package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"hestia/internal/index"
)

func TestManualCoverGenerationUsesServingCacheAndSkipsExisting(t *testing.T) {
	srv, root := libraryFixture(t, "Other/poster.jpg")
	path := "Other/poster.jpg"
	writeTestPoster(t, filepath.Join(root, filepath.FromSlash(path)), color.RGBA{R: 255, A: 255})
	lib := srv.store.LibID(root)
	id := index.NewID(root, path)
	if err := srv.store.RescanLibrary(lib); err != nil {
		t.Fatal(err)
	}
	cache := testPosterCache(t, srv, id)
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("cache should start absent: %v", err)
	}

	progress := generateTestCovers(t, srv)
	if !reflect.DeepEqual(progress, CoverProgress{Total: 1, Processed: 1, Generated: 1, Done: true}) {
		t.Fatalf("first generation: %+v", progress)
	}
	first, err := os.ReadFile(cache)
	if err != nil {
		t.Fatal(err)
	}
	if cfg, err := jpeg.DecodeConfig(bytes.NewReader(first)); err != nil || cfg.Width != 480 {
		t.Fatalf("cached image = %+v, %v", cfg, err)
	}
	res := localLibraryRequest(srv, http.MethodGet, "/api/media/"+id+"/image?w=480", "")
	if res.Code != http.StatusOK || !bytes.Equal(first, res.Body.Bytes()) {
		t.Fatalf("HTTP response did not use the generated cache: %d", res.Code)
	}
	progress = generateTestCovers(t, srv)
	if !reflect.DeepEqual(progress, CoverProgress{Total: 1, Processed: 1, Cached: 1, Done: true}) {
		t.Fatalf("second generation: %+v", progress)
	}

	writeTestPoster(t, filepath.Join(root, filepath.FromSlash(path)), color.RGBA{G: 255, A: 255})
	mtime := time.Now().Add(time.Minute)
	if err := os.Chtimes(filepath.Join(root, filepath.FromSlash(path)), mtime, mtime); err != nil {
		t.Fatal(err)
	}
	if err := srv.store.RescanLibrary(lib); err != nil {
		t.Fatal(err)
	}
	changed := testPosterCache(t, srv, id)
	if changed == cache {
		t.Fatal("changed source reused its old cache key")
	}
	if _, err := os.Stat(changed); !os.IsNotExist(err) {
		t.Fatalf("rescan generated a cache file without a request: %v", err)
	}
	progress = generateTestCovers(t, srv)
	if progress.Generated != 1 || progress.Cached != 0 {
		t.Fatalf("generation after rescan: %+v", progress)
	}
	second, err := os.ReadFile(changed)
	if err != nil || bytes.Equal(first, second) {
		t.Fatalf("failed to generate the modified cover: %v", err)
	}
}

func TestManualCoverGenerationCancelsAndRejectsConcurrentRequests(t *testing.T) {
	const file = "Series/poster.jpg"
	srv, root := libraryFixture(t, file)
	writeTestPoster(t, filepath.Join(root, filepath.FromSlash(file)), color.RGBA{A: 255})
	if err := srv.store.RescanLibrary(srv.store.LibID(root)); err != nil {
		t.Fatal(err)
	}
	cache := testPosterCache(t, srv, index.NewID(root, file))
	unlock, _ := srv.imageJobs.lock(context.Background(), cache)
	defer unlock()
	users := func() int {
		srv.imageJobs.mu.Lock()
		defer srv.imageJobs.mu.Unlock()
		return srv.imageJobs.jobs[cache].users
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- srv.GenerateCovers(ctx, func(CoverProgress) error { return nil }) }()
	waitForCover(t, func() bool { return users() == 2 })
	if err := srv.GenerateCovers(t.Context(), func(CoverProgress) error {
		t.Fatal("an overlapping request started reporting progress")
		return nil
	}); !errors.Is(err, ErrCoverGenerationRunning) {
		t.Fatalf("overlapping generation error: %v", err)
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) || users() != 1 {
			t.Fatalf("generation did not cancel: %v (lock users: %d)", err, users())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled generation did not return")
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("canceled generation wrote a cache file: %v", err)
	}
}

func TestManualCoverGenerationExtractsFramesOnlyForVideosWithoutPosters(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	const withPoster = "With poster/clip.mp4"
	const fallback = "Without poster/clip.mp4"
	const poster = "With poster/poster.jpg"
	srv, root := libraryFixture(t, withPoster, fallback, poster)
	srv.tc.FFmpeg = ffmpeg
	cmd := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-f", "lavfi",
		"-i", "color=c=red:s=64x64:r=10:d=1", "-c:v", "mpeg4", "-y", filepath.Join(root, filepath.FromSlash(fallback)))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create test video: %v: %s", err, output)
	}
	writeTestPoster(t, filepath.Join(root, filepath.FromSlash(poster)), color.RGBA{A: 255})
	lib := srv.store.LibID(root)
	if err := srv.store.RescanLibrary(lib); err != nil {
		t.Fatal(err)
	}

	progress := generateTestCovers(t, srv)
	if !reflect.DeepEqual(progress, CoverProgress{Total: 2, Processed: 2, Generated: 2, Done: true}) {
		t.Fatalf("generation: %+v", progress)
	}

	media, ok := srv.store.GetMedia(index.NewID(root, fallback))
	if !ok {
		t.Fatal("video without poster not indexed")
	}
	want := fmt.Sprintf("%s-%d-%d.jpg", media.ID, media.Size, media.Mtime.Unix())
	actual, err := os.ReadDir(filepath.Join(srv.dataDir, "cache", "thumb"))
	if err != nil || len(actual) != 1 || actual[0].Name() != want {
		t.Fatalf("warmed video frames = %v, %v, want %q only", actual, err, want)
	}
	res := localLibraryRequest(srv, http.MethodGet, "/api/media/"+media.ID+"/thumb", "")
	if cfg, err := jpeg.DecodeConfig(res.Body); res.Code != http.StatusOK || err != nil || cfg.Width != 480 {
		t.Fatalf("cached frame = HTTP %d, %+v, %v", res.Code, cfg, err)
	}
}

func TestManualCoverGenerationReportsSkippedAndFailed(t *testing.T) {
	srv, _ := libraryFixture(t, "Video/clip.mp4", "Corrupt/poster.jpg", "Other/poster.gif")
	srv.tc.FFmpeg = ""
	progress := generateTestCovers(t, srv)
	progress.Issues = nil
	if !reflect.DeepEqual(progress, CoverProgress{Total: 3, Processed: 3, Skipped: 2, Failed: 1, Done: true}) {
		t.Fatalf("generation with unsupported and corrupt items: %+v", progress)
	}
}

func TestManualCoverGenerationIdentifiesAndLogsFailures(t *testing.T) {
	srv, root := libraryFixture(t, "Corrupt/poster.jpg", "Missing/poster.jpg", "Cache/poster.jpg")
	corrupt := filepath.Join(root, "Corrupt", "poster.jpg")
	missing := filepath.Join(root, "Missing", "poster.jpg")
	valid := filepath.Join(root, "Cache", "poster.jpg")
	writeTestPoster(t, valid, color.RGBA{A: 255})
	if err := srv.store.RescanLibrary(srv.store.LibID(root)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(missing); err != nil {
		t.Fatal(err)
	}
	// A regular file in place of the cache directory makes the cache write fail.
	if err := os.WriteFile(filepath.Join(srv.dataDir, "cache"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	srv.logger = log.New(&logs, "", 0)
	seen := map[string]string{}
	err := srv.GenerateCovers(t.Context(), func(progress CoverProgress) error {
		// Decode the same JSON fields consumed by the console.
		payload, err := json.Marshal(progress)
		if err != nil {
			return err
		}
		var update struct {
			Issues []struct {
				Path   string `json:"path"`
				Reason string `json:"reason"`
			} `json:"issues"`
		}
		if err := json.Unmarshal(payload, &update); err != nil {
			return err
		}
		for _, issue := range update.Issues {
			if _, duplicate := seen[issue.Path]; duplicate {
				t.Errorf("failure reported twice: %q", issue.Path)
			}
			seen[issue.Path] = issue.Reason
		}
		if progress.Failed != len(seen) {
			t.Errorf("failure count %d and details %v differ", progress.Failed, seen)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for file, reason := range map[string]string{
		corrupt: "imageRead",
		missing: "sourceMissing",
		valid:   "cacheWrite",
	} {
		if seen[file] != reason {
			t.Errorf("%q: reason = %q, want %q", file, seen[file], reason)
		}
		if !strings.Contains(logs.String(), filepath.Dir(file)) {
			t.Errorf("missing path in log: %q", file)
		}
	}
	if !strings.Contains(logs.String(), "unknown format") {
		t.Errorf("missing decoder error from log: %s", logs.String())
	}
}

func TestManualCoverGenerationLogsFFmpegFailure(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	const corrupt = "Video/corrupt.mp4"
	srv, root := libraryFixture(t, corrupt)
	srv.tc.FFmpeg = ffmpeg
	var logs bytes.Buffer
	srv.logger = log.New(&logs, "", 0)

	progress := generateTestCovers(t, srv)
	want := CoverIssue{Path: filepath.Join(root, "Video", "corrupt.mp4"), Reason: "videoFrame"}
	if progress.Failed != 1 || !reflect.DeepEqual(progress.Issues, []CoverIssue{want}) {
		t.Fatalf("frame failure: %+v", progress)
	}
	if !strings.Contains(logs.String(), "FFmpeg: exit status") ||
		!strings.Contains(logs.String(), "Invalid data found") {
		t.Fatalf("missing FFmpeg diagnostic: %s", logs.String())
	}
}

func generateTestCovers(t *testing.T, srv *Server) CoverProgress {
	t.Helper()
	var updates []CoverProgress
	err := srv.GenerateCovers(t.Context(), func(progress CoverProgress) error {
		updates = append(updates, progress)
		return nil
	})
	if err != nil || len(updates) < 2 {
		t.Fatalf("generation progress: %v, %v", updates, err)
	}
	first, last := updates[0], updates[len(updates)-1]
	if first.Done || first.Processed != 0 || !last.Done || last.Total != first.Total {
		t.Fatalf("invalid first or final progress: %+v, %+v", first, last)
	}
	for i, update := range updates {
		if (i > 0 && update.Processed < updates[i-1].Processed) ||
			update.Processed != update.Cached+update.Generated+update.Skipped+update.Failed ||
			update.Processed > update.Total {
			t.Fatalf("invalid progress: %+v", updates)
		}
	}
	return last
}

func testPosterCache(t *testing.T, srv *Server, id string) string {
	t.Helper()
	media, ok := srv.store.GetMedia(id)
	if !ok {
		t.Fatal("poster not indexed")
	}
	return filepath.Join(srv.dataDir, "cache", "img",
		fmt.Sprintf("%s-480-%d-%d.jpg", id, media.Size, media.Mtime.UnixNano()))
}

func writeTestPoster(t *testing.T, path string, fill color.Color) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	poster := image.NewRGBA(image.Rect(0, 0, 600, 300))
	draw.Draw(poster, poster.Bounds(), image.NewUniform(fill), image.Point{}, draw.Src)
	err = jpeg.Encode(f, poster, nil)
	if closeErr := f.Close(); err != nil || closeErr != nil {
		t.Fatalf("writing poster: encode = %v, close = %v", err, closeErr)
	}
}

func waitForCover(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for cover generation")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
