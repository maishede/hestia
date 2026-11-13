package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hestia/internal/index"
)

// The test binary acts as a slow ffmpeg process until it is killed.
func init() {
	if mode := os.Getenv("HESTIA_TEST_BLOCK_TRANSCODER_CHILD"); mode != "" {
		if mode == "hold-source" {
			for i, arg := range os.Args {
				if arg == "-i" && i+1 < len(os.Args) {
					f, err := os.Open(os.Args[i+1])
					if err != nil {
						os.Exit(2)
					}
					defer f.Close()
					break
				}
			}
		}
		if mode == "ready" || mode == "hold-source" {
			_ = os.WriteFile(os.Args[len(os.Args)-1], []byte("#EXTM3U\n"), 0o644)
		}
		time.Sleep(time.Minute)
		os.Exit(0)
	}
}

func transcodeCancelFixture(t *testing.T, ffmpeg string) (*Transcoder, *httptest.Server, string) {
	t.Helper()
	library := t.TempDir()
	if err := os.WriteFile(filepath.Join(library, "film.mkv"), []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := index.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	libID, err := store.AddLibrary(library, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RescanLibrary(libID); err != nil {
		t.Fatal(err)
	}
	id := index.NewID(library, "film.mkv")
	dataDir := t.TempDir()
	tr := NewTranscoder(ffmpeg, dataDir, func(id string) (string, error) {
		abs, _, err := store.ResolveMedia(id)
		return abs, err
	})
	s, err := New(nil, store, Toolchain{FFmpeg: ffmpeg}, tr, nil, dataDir, os.DirFS("../../web"), log.New(io.Discard, "", 0))
	if err != nil {
		tr.Close()
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(s.mux)
	t.Cleanup(httpServer.Close)
	t.Cleanup(tr.Close)
	return tr, httpServer, id
}

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for transcode state")
}

func TestCancelReservationBeforeStart(t *testing.T) {
	tr, srv, id := transcodeCancelFixture(t, "fake-ffmpeg")
	resp, err := http.Post(srv.URL+"/api/media/"+id+"/transcode/reserve", "application/json", bytes.NewBufferString("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reserve: status %d", resp.StatusCode)
	}
	var reservation struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reservation); err != nil || reservation.SessionID == "" {
		t.Fatalf("invalid reservation: %v", err)
	}
	request, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/transcode/"+reservation.SessionID, nil)
	deleted, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	deleted.Body.Close()
	if deleted.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: status %d", deleted.StatusCode)
	}
	body, _ := json.Marshal(map[string]any{"start": 0, "sessionId": reservation.SessionID})
	started, err := http.Post(srv.URL+"/api/media/"+id+"/transcode", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	started.Body.Close()
	if started.StatusCode < 400 {
		t.Fatalf("canceled reservation started: status %d", started.StatusCode)
	}
	if got := tr.Active(); got != 0 {
		t.Fatalf("started %d canceled sessions", got)
	}
}

func TestCanceledStartRequestStopsFFmpeg(t *testing.T) {
	t.Setenv("HESTIA_TEST_BLOCK_TRANSCODER_CHILD", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, reserved := range []bool{false, true} {
		t.Run(fmt.Sprintf("reserved=%v", reserved), func(t *testing.T) {
			tr, srv, id := transcodeCancelFixture(t, exe)
			body := map[string]any{"start": 0}
			if reserved {
				body["sessionId"] = reserveForTest(t, srv.URL, id)
			}
			data, _ := json.Marshal(body)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/api/media/"+id+"/transcode", bytes.NewReader(data))
			done := make(chan struct{})
			go func() {
				defer close(done)
				if response, err := http.DefaultClient.Do(req); err == nil {
					response.Body.Close()
				}
			}()
			waitUntil(t, func() bool { return tr.Active() == 1 })
			cancel()
			select {
			case <-done:
			case <-time.After(4 * time.Second):
				t.Fatal("canceled HTTP request did not return")
			}
			waitUntil(t, func() bool { return tr.Active() == 0 })
		})
	}
}

func reserveForTest(t *testing.T, base, id string) string {
	t.Helper()
	resp, err := http.Post(base+"/api/media/"+id+"/transcode/reserve", "application/json", bytes.NewBufferString("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reserve: status %d", resp.StatusCode)
	}
	var value struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&value); err != nil || value.SessionID == "" {
		t.Fatalf("invalid reservation: %v", err)
	}
	return value.SessionID
}

func TestReservationStartsOnceAndStopsProcess(t *testing.T) {
	t.Setenv("HESTIA_TEST_BLOCK_TRANSCODER_CHILD", "ready")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tr, srv, id := transcodeCancelFixture(t, exe)
	sid := reserveForTest(t, srv.URL, id)
	body, _ := json.Marshal(map[string]any{"sessionId": sid, "start": 12})
	start := func() *http.Response {
		r, err := http.Post(srv.URL+"/api/media/"+id+"/transcode", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := start()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("start: status %d", r.StatusCode)
	}
	var started struct {
		SessionID string  `json:"sessionId"`
		Start     float64 `json:"start"`
	}
	if err := json.NewDecoder(r.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if started.SessionID != sid || started.Start != 12 || tr.Active() != 1 {
		t.Fatalf("unexpected session: %+v, active=%d", started, tr.Active())
	}
	r = start()
	r.Body.Close()
	if r.StatusCode != http.StatusGone {
		t.Fatalf("reusing reservation: status %d", r.StatusCode)
	}
	tr.mu.Lock()
	session := tr.sessions[sid]
	tr.mu.Unlock()
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/transcode/"+sid, nil)
	r, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != http.StatusNoContent || tr.Active() != 0 {
		t.Fatalf("stop: status=%d active=%d", r.StatusCode, tr.Active())
	}
	select {
	case <-session.done:
	case <-time.After(4 * time.Second):
		t.Fatal("FFmpeg process did not exit")
	}
}

func TestReservationBindingExpiryAndCapacity(t *testing.T) {
	tr, _, id := transcodeCancelFixture(t, "fake-ffmpeg")
	sid, err := tr.Reserve(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Start(context.Background(), "different-media", 0, sid); !errors.Is(err, errReservationGone) {
		t.Fatalf("media mismatch: %v", err)
	}
	tr.mu.Lock()
	reservation, exists := tr.reservations[sid]
	reservation.expiresAt = time.Now().Add(-time.Second)
	tr.reservations[sid] = reservation
	tr.mu.Unlock()
	if !exists {
		t.Fatal("media mismatch consumed another media's reservation")
	}
	if _, err := tr.Start(context.Background(), id, 0, sid); !errors.Is(err, errReservationGone) {
		t.Fatalf("expired reservation: %v", err)
	}
	tr.mu.Lock()
	for i := 0; i < maxReservations; i++ {
		tr.reservations[fmt.Sprint(i)] = transcodeReservation{mediaID: id, expiresAt: time.Now().Add(time.Minute)}
	}
	tr.mu.Unlock()
	if _, err := tr.Reserve(id); err == nil {
		t.Fatal("reservation capacity was not enforced")
	}
	tr.mu.Lock()
	tr.reservations["0"] = transcodeReservation{mediaID: id, expiresAt: time.Now().Add(-time.Second)}
	tr.mu.Unlock()
	if _, err := tr.Reserve(id); err != nil {
		t.Fatalf("expired reservation did not free capacity: %v", err)
	}
}

func TestMalformedStartDoesNotLaunchTranscode(t *testing.T) {
	tr, srv, id := transcodeCancelFixture(t, "fake-ffmpeg")
	sid := reserveForTest(t, srv.URL, id)
	body := `{"start":0,"sessionId":"` + sid // Simulate a truncated request body.
	r, err := http.Post(srv.URL+"/api/media/"+id+"/transcode", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed request: status %d", r.StatusCode)
	}
	if tr.Active() != 0 {
		t.Fatalf("malformed request started %d sessions", tr.Active())
	}
	tr.mu.Lock()
	_, stillReserved := tr.reservations[sid]
	tr.mu.Unlock()
	if !stillReserved {
		t.Fatal("malformed request consumed the reservation")
	}
}

func TestStopDuringReservedStartup(t *testing.T) {
	t.Setenv("HESTIA_TEST_BLOCK_TRANSCODER_CHILD", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tr, _, id := transcodeCancelFixture(t, exe)
	sid, err := tr.Reserve(id)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := tr.Start(context.Background(), id, 0, sid); result <- err }()
	waitUntil(t, func() bool { return tr.Active() == 1 })
	tr.Stop(sid)
	select {
	case err := <-result:
		if !errors.Is(err, errTranscodeStopped) {
			t.Fatalf("stopped startup: %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("stopping startup did not terminate FFmpeg")
	}
	if tr.Active() != 0 {
		t.Fatal("stopped startup remains active")
	}
}

func TestLegacyTranscodeStartWithoutReservation(t *testing.T) {
	t.Setenv("HESTIA_TEST_BLOCK_TRANSCODER_CHILD", "ready")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tr, srv, id := transcodeCancelFixture(t, exe)
	for _, body := range []string{"", "{}"} {
		r, err := http.Post(srv.URL+"/api/media/"+id+"/transcode", "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		var session struct {
			SessionID string `json:"sessionId"`
		}
		if r.StatusCode != http.StatusOK {
			t.Fatalf("legacy start %q: status %d", body, r.StatusCode)
		}
		if err := json.NewDecoder(r.Body).Decode(&session); err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if session.SessionID == "" || tr.Active() != 1 {
			t.Fatalf("legacy session missing for body %q", body)
		}
		tr.Stop(session.SessionID)
	}
}

type failedResponseWriter struct{ header http.Header }

func (w *failedResponseWriter) Header() http.Header       { return w.header }
func (w *failedResponseWriter) WriteHeader(int)           {}
func (w *failedResponseWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestFailedTranscodeResponseWriteCleansUp(t *testing.T) {
	t.Setenv("HESTIA_TEST_BLOCK_TRANSCODER_CHILD", "ready")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tr, srv, id := transcodeCancelFixture(t, exe)
	writer := func() *failedResponseWriter { return &failedResponseWriter{header: make(http.Header)} }
	reserve := httptest.NewRequest(http.MethodPost, "/api/media/"+id+"/transcode/reserve", nil)
	srv.Config.Handler.ServeHTTP(writer(), reserve)
	tr.mu.Lock()
	reservations := len(tr.reservations)
	tr.mu.Unlock()
	if reservations != 0 {
		t.Fatalf("failed reservation response left %d reservations", reservations)
	}

	sid := reserveForTest(t, srv.URL, id)
	body, _ := json.Marshal(map[string]any{"sessionId": sid})
	start := httptest.NewRequest(http.MethodPost, "/api/media/"+id+"/transcode", bytes.NewReader(body))
	srv.Config.Handler.ServeHTTP(writer(), start)
	if tr.Active() != 0 {
		t.Fatalf("failed start response left %d running sessions", tr.Active())
	}
}

func TestUnusedTranscodeExpiresBeforeClaimedAndLegacySessions(t *testing.T) {
	t.Setenv("HESTIA_TEST_BLOCK_TRANSCODER_CHILD", "ready")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tr, srv, id := transcodeCancelFixture(t, exe)
	start := func(sid string) string {
		t.Helper()
		sess, err := tr.Start(context.Background(), id, 0, sid)
		if err != nil {
			t.Fatal(err)
		}
		return sess.ID
	}
	request := func(method, sid, name, byteRange string, want int) {
		t.Helper()
		req, err := http.NewRequest(method, srv.URL+"/api/transcode/"+sid+"/"+name, nil)
		if err != nil {
			t.Fatal(err)
		}
		if byteRange != "" {
			req.Header.Set("Range", byteRange)
		}
		r, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != want {
			t.Fatalf("%s %s/%s: status %d, want %d", method, sid, name, r.StatusCode, want)
		}
	}
	unused := start(reserveForTest(t, srv.URL, id))
	claimed := start(reserveForTest(t, srv.URL, id))
	legacy := start("")
	request(http.MethodHead, unused, "index.m3u8", "", http.StatusOK)
	request(http.MethodGet, unused, "seg99999.ts", "", http.StatusNotFound)
	request(http.MethodGet, unused, "index.m3u8", "bytes=99999-", http.StatusRequestedRangeNotSatisfiable)
	srv.Config.Handler.ServeHTTP(&failedResponseWriter{header: make(http.Header)},
		httptest.NewRequest(http.MethodGet, "/api/transcode/"+unused+"/index.m3u8", nil))
	request(http.MethodGet, claimed, "index.m3u8", "bytes=0-4", http.StatusPartialContent)
	tr.reap(time.Now().Add(26 * time.Second))
	request(http.MethodGet, unused, "index.m3u8", "", http.StatusNotFound)
	request(http.MethodGet, claimed, "index.m3u8", "", http.StatusOK)
	request(http.MethodGet, legacy, "index.m3u8", "", http.StatusOK)
	tr.reap(time.Now().Add(2*time.Minute + time.Second))
	if tr.Active() != 0 {
		t.Fatal("idle sessions were not reclaimed")
	}
}
