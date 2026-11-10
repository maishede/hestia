package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hestia/internal/index"
)

func subtitleFixture(t *testing.T) (*Server, string, string) {
	t.Helper()
	library := t.TempDir()
	for name, content := range map[string]string{
		"film.mkv": "media",
		"film.srt": "1\n00:00:01,000 --> 00:00:10,000\nEARLY\n\n2\n00:00:17,000 --> 00:00:20,000\nSPANNING\n\n3\n00:00:18,500 --> 00:00:28,000\nLATE\n",
	} {
		if err := os.WriteFile(filepath.Join(library, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
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
	mediaID := index.NewID(library, "film.mkv")
	_, media, err := store.ResolveMedia(mediaID)
	if err != nil {
		t.Fatal(err)
	}
	store.SetProbeResult(library, &media, &index.ProbeInfo{Subs: []index.SubStream{{Index: 2, Codec: "subrip"}}})
	return &Server{store: store, tc: Toolchain{FFmpeg: "ffmpeg"}, dataDir: t.TempDir()}, index.NewID(library, "film.srt"), mediaID
}

func subtitleRequest(t *testing.T, s *Server, id, mediaID, query string, embedded bool) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/subtitle"+query, nil)
	w := httptest.NewRecorder()
	if embedded {
		r.SetPathValue("id", mediaID)
		r.SetPathValue("index", "2")
		s.handleEmbeddedSub(w, r)
	} else {
		r.SetPathValue("id", id)
		s.handleSubtitle(w, r)
	}
	return w
}

func TestSubtitleOffsetForExternalAndCachedEmbedded(t *testing.T) {
	s, subID, mediaID := subtitleFixture(t)
	_, media, err := s.store.ResolveMedia(mediaID)
	if err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(s.dataDir, "cache", "sub", fmt.Sprintf("%s-2-%d.vtt", mediaID, media.Size))
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "WEBVTT\n\n1\n00:00:01.000 --> 00:00:10.000\nEARLY\n\n2\n00:00:17.000 --> 00:00:20.000\nSPANNING\n\n3\n00:00:18.500 --> 00:00:28.000 align:start\nLATE\n"
	if err := os.WriteFile(cache, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, embedded := range []bool{false, true} {
		t.Run(fmt.Sprintf("embedded=%v", embedded), func(t *testing.T) {
			w := subtitleRequest(t, s, subID, mediaID, "?offset=18", embedded)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", w.Code, w.Body.String())
			}
			got := w.Body.String()
			for _, want := range []string{"00:00:00.000 --> 00:00:02.000", "SPANNING", "00:00:00.500 --> 00:00:10.000", "LATE"} {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in %q", want, got)
				}
			}
			if strings.Contains(got, "EARLY") {
				t.Errorf("expired cue remains: %q", got)
			}
			unshifted := subtitleRequest(t, s, subID, mediaID, "", embedded)
			if !strings.Contains(unshifted.Body.String(), "00:00:18.500 --> 00:00:28.000") {
				t.Errorf("original was changed: %q", unshifted.Body.String())
			}
		})
	}
	got, err := os.ReadFile(cache)
	if err != nil || string(got) != original {
		t.Fatalf("cached original changed: %v", err)
	}
}

func TestSubtitleRejectsInvalidOffset(t *testing.T) {
	s, subID, mediaID := subtitleFixture(t)
	for _, query := range []string{"?offset=-1", "?offset=NaN", "?offset=Inf", "?offset=oops", "?offset=99999999999999999999"} {
		for _, embedded := range []bool{false, true} {
			w := subtitleRequest(t, s, subID, mediaID, query, embedded)
			if w.Code != http.StatusBadRequest {
				t.Errorf("%s embedded=%v: status = %d", query, embedded, w.Code)
			}
		}
	}
}

func TestShiftVTTKeepsMetadataAndShiftsInlineTimestamps(t *testing.T) {
	raw := "WEBVTT\r\n\r\nNOTE preserve\r\n00:00:01.000 --> 00:00:02.000\r\n\r\nid\r\n00:00:17.000 --> 00:00:20.000 align:start\r\nA<00:00:17.500>B<00:00:18.500>C\r\n"
	got := shiftVTT(raw, 18000)
	for _, want := range []string{"NOTE preserve\n00:00:01.000 --> 00:00:02.000", "00:00:00.000 --> 00:00:02.000 align:start", "AB<00:00:00.500>C"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}
