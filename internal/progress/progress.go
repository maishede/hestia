package progress

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Entry 一条播放进度（多设备最后写入者优先合并）。
type Entry struct {
	MediaID   string  `json:"mediaId"`
	Position  float64 `json:"position"`
	Duration  float64 `json:"duration"`
	UpdatedAt int64   `json:"updatedAt"` // 毫秒
	Device    string  `json:"device,omitempty"`
	Name      string  `json:"name,omitempty"`
	Folder    string  `json:"folder,omitempty"`
}

type Store struct {
	mu    sync.Mutex
	path  string
	m     map[string]*Entry
	dirty bool
	stop  chan struct{}
	wg    sync.WaitGroup
}

func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{
		path: filepath.Join(dataDir, "progress.json"),
		m:    map[string]*Entry{},
		stop: make(chan struct{}),
	}
	raw, err := os.ReadFile(s.path)
	if err == nil {
		var m map[string]*Entry
		if json.Unmarshal(raw, &m) == nil && m != nil {
			s.m = m
		}
	}
	s.wg.Add(1)
	go s.saver()
	return s, nil
}

func (s *Store) Close() {
	close(s.stop)
	s.wg.Wait()
	s.save()
}

func (s *Store) saver() {
	defer s.wg.Done()
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.save()
		}
	}
}

func (s *Store) save() {
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return
	}
	raw, err := json.Marshal(s.m)
	s.dirty = false
	s.mu.Unlock()
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if os.WriteFile(tmp, raw, 0o644) == nil {
		_ = os.Rename(tmp, s.path)
	}
}

func (s *Store) Set(e Entry) {
	if e.MediaID == "" {
		return
	}
	e.UpdatedAt = time.Now().UnixMilli()
	s.mu.Lock()
	s.m[e.MediaID] = &e
	s.dirty = true
	s.mu.Unlock()
}

func (s *Store) Get(mediaID string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.m[mediaID]; ok {
		return *e, true
	}
	return Entry{}, false
}

func (s *Store) Delete(mediaID string) {
	s.mu.Lock()
	if _, ok := s.m[mediaID]; ok {
		delete(s.m, mediaID)
		s.dirty = true
	}
	s.mu.Unlock()
}

func (s *Store) Clear() {
	s.mu.Lock()
	s.m = map[string]*Entry{}
	s.dirty = true
	s.mu.Unlock()
}

// Recent 返回最近观看（有效断点：看了 30s 以上且未到结尾）。
func (s *Store) Recent(n int) []Entry {
	if n <= 0 || n > 50 {
		n = 12
	}
	s.mu.Lock()
	out := make([]Entry, 0, len(s.m))
	for _, e := range s.m {
		if e.Position > 30 && (e.Duration <= 0 || e.Position < e.Duration*0.97) {
			out = append(out, *e)
		}
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	if len(out) > n {
		out = out[:n]
	}
	return out
}
