package index

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCoverMediaIDsWaitForEnabledLibraryScan(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "library")
	const file = "Series/clip.mp4"
	writeScanFile(t, root, file)
	store, err := New(filepath.Join(base, "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	libID, err := store.AddLibrary(root, "Library")
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.libs[libID].Scanning = true
	store.mu.Unlock()
	if ids, ready := store.CoverMediaIDs(); ready || len(ids) != 0 {
		t.Fatalf("snapshot during scan: ids = %v, ready = %v", ids, ready)
	}

	if err := store.RescanLibrary(libID); err != nil {
		t.Fatal(err)
	}
	want := NewID(root, file)
	if ids, ready := store.CoverMediaIDs(); !ready || len(ids) != 1 || ids[0] != want {
		t.Fatalf("snapshot after scan: ids = %v, ready = %v, want %s", ids, ready, want)
	}
}

func TestRescanUnavailableLibraryHidesStaleContentAndRecovers(t *testing.T) {
	for _, replaceWithFile := range []bool{false, true} {
		name := "missing"
		if replaceWithFile {
			name = "replaced by file"
		}
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "library")
			otherRoot := filepath.Join(base, "other-library")
			writeScanFile(t, root, "Series/Alice - Episode.mp4")
			writeScanFile(t, root, "Series/Alice - Episode.srt")
			writeScanFile(t, otherRoot, "Other/Bob - Film.mp4")

			store, err := New(filepath.Join(base, "data"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(store.Close)
			libID, err := store.AddLibrary(root, "Library")
			if err != nil {
				t.Fatal(err)
			}
			otherID, err := store.AddLibrary(otherRoot, "Other")
			if err != nil {
				t.Fatal(err)
			}
			if err := store.RescanLibrary(libID); err != nil {
				t.Fatal(err)
			}
			if err := store.RescanLibrary(otherID); err != nil {
				t.Fatal(err)
			}
			mediaID := NewID(root, "Series/Alice - Episode.mp4")
			subID := NewID(root, "Series/Alice - Episode.srt")
			if _, ok := store.GetMedia(mediaID); !ok {
				t.Fatal("fixture media was not indexed")
			}
			if _, ok := store.GetSubtitle(subID); !ok {
				t.Fatal("fixture subtitle was not indexed")
			}

			offline := filepath.Join(base, "offline")
			if err := os.Rename(root, offline); err != nil {
				t.Fatal(err)
			}
			if replaceWithFile {
				if err := os.WriteFile(root, []byte("not a folder"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.RescanLibrary(libID); err == nil {
				t.Fatal("rescan of unavailable library unexpectedly succeeded")
			}
			lib, ok := store.GetLibrary(libID)
			if !ok || !lib.Enabled || lib.LastErr == "" || lib.Scanning || lib.Files != 0 || lib.Dirs != 0 {
				t.Fatalf("unavailable library state = %+v, exists = %v", lib, ok)
			}
			if roots := store.Roots(1, 20, "", ""); roots.Total != 1 || roots.Folders[0].Name != "Other" {
				t.Errorf("visible folders after failure = %+v", roots)
			}
			if got := store.Search("Episode", 1, 20, "", ""); got.Total != 0 {
				t.Errorf("unavailable media still searchable: %+v", got)
			}
			if _, ok := store.GetMedia(mediaID); ok {
				t.Error("unavailable media still accessible")
			}
			if _, ok := store.GetSubtitle(subID); ok {
				t.Error("unavailable subtitle still accessible")
			}
			if _, _, ok := store.RecentCard(mediaID); ok {
				t.Error("unavailable media still appears in recent items")
			}
			if actors := store.Actors(); len(actors) != 1 || actors[0].Name != "Bob" {
				t.Errorf("actors after failure = %+v", actors)
			}

			if replaceWithFile {
				if err := os.Remove(root); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Rename(offline, root); err != nil {
				t.Fatal(err)
			}
			if err := store.RescanLibrary(libID); err != nil {
				t.Fatal(err)
			}
			lib, ok = store.GetLibrary(libID)
			if !ok || lib.LastErr != "" || lib.Files != 1 || lib.Dirs != 1 {
				t.Fatalf("restored library state = %+v, exists = %v", lib, ok)
			}
			if _, ok := store.GetMedia(mediaID); !ok {
				t.Error("restored media was not indexed under its original ID")
			}
			if _, ok := store.GetSubtitle(subID); !ok {
				t.Error("restored subtitle was not indexed")
			}
			if roots := store.Roots(1, 20, "", ""); roots.Total != 2 {
				t.Errorf("visible folders after recovery = %+v", roots)
			}
		})
	}
}

func TestRescanRemovesDeletedSubfolderOnly(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "library")
	writeScanFile(t, root, "Removed/Alice - Episode.mp4")
	writeScanFile(t, root, "Kept/Bob - Film.mp4")
	store, err := New(filepath.Join(base, "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	libID, err := store.AddLibrary(root, "Library")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RescanLibrary(libID); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "Removed"), filepath.Join(base, "removed")); err != nil {
		t.Fatal(err)
	}
	if err := store.RescanLibrary(libID); err != nil {
		t.Fatal(err)
	}
	if roots := store.Roots(1, 20, "", ""); roots.Total != 1 || roots.Folders[0].Name != "Kept" {
		t.Errorf("visible folders after rescan = %+v", roots)
	}
	if got := store.Search("Episode", 1, 20, "", ""); got.Total != 0 {
		t.Errorf("removed media still searchable: %+v", got)
	}
	if _, ok := store.GetMedia(NewID(root, "Kept/Bob - Film.mp4")); !ok {
		t.Error("remaining media was not preserved")
	}
}

func writeScanFile(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
}
