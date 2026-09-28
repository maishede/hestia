// Package index 维护媒体库内存索引：库/文件夹/媒体项、分页排序、搜索与探测缓存。
package index

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Kind int

const (
	KindVideo Kind = iota
	KindImage
)

func (k Kind) String() string {
	if k == KindVideo {
		return "video"
	}
	return "image"
}

type Media struct {
	ID       string
	FolderID string
	LibraryID string
	RelPath  string
	Name     string
	Ext      string
	Kind     Kind
	Size     int64
	Mtime    time.Time

	// 外挂字幕（同目录同名或同名.语言.srt/ass）
	SubIDs []string

	// 拼音搜索键
	PyFull string
	PyInit string

	// 探测信息（ffprobe 懒加载）
	Probed       bool
	Duration     float64
	VCodec       string
	ACodec       string
	Width        int
	Height       int
	EmbeddedSubs []SubStream

	CardCoverID string // 同名封面图（视频卡片用）
}

// Subtitle 外挂字幕文件（不进浏览网格，仅随视频提供）。
type Subtitle struct {
	ID        string
	FolderID  string
	LibraryID string
	RelPath   string
	Name      string
	Ext       string
	Size      int64
	Mtime     time.Time
}

// SubStream 内嵌字幕轨（ffprobe 探测）。
type SubStream struct {
	Index int    `json:"index"`
	Codec string `json:"codec"`
	Title string `json:"title,omitempty"`
	Lang  string `json:"lang,omitempty"`
}

type Folder struct {
	ID         string
	LibraryID  string
	ParentID   string
	RelPath    string
	Name       string
	Mtime      time.Time
	CoverID    string
	VideoCount int
	ImageCount int
	SubVideos  int // 含子层视频总数
	PyFull     string
	PyInit     string
}

type LibraryState struct {
	ID       string    `json:"id"`
	Path     string    `json:"path"`
	Label    string    `json:"label"`
	Enabled  bool      `json:"enabled"`
	Scanning bool      `json:"scanning"`
	LastScan time.Time `json:"lastScan"`
	LastErr  string    `json:"lastErr,omitempty"`
	Files    int       `json:"files"`
	Dirs     int       `json:"dirs"`
}

type ProbeInfo struct {
	Size     int64
	Mtime    int64
	Duration float64
	VCodec   string
	ACodec   string
	Width    int
	Height   int
	Subs     []SubStream
}

// NewID 基于库根路径 + 相对路径生成稳定 ID（不泄露物理路径，配置不变则持久稳定）。
func NewID(root, rel string) string {
	h := sha1.Sum([]byte(root + "\x00" + filepath.ToSlash(rel)))
	return base64.RawURLEncoding.EncodeToString(h[:9])
}

type Store struct {
	mu      sync.RWMutex
	dataDir string

	libs    map[string]*LibraryState
	folders map[string]*Folder
	media   map[string]*Media
	subs    map[string]*Subtitle

	childFolders map[string][]string // parentID -> folder ids（自然序）
	childMedia   map[string][]string // folderID -> media ids（自然序）

	probeMu    sync.Mutex
	probeCache map[string]*ProbeInfo
	probeDirty bool
	probeStop  chan struct{}
	probeWG    sync.WaitGroup
	scanMu     sync.Mutex // 串行化扫描
}

func New(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{
		dataDir:      dataDir,
		libs:         map[string]*LibraryState{},
		folders:      map[string]*Folder{},
		media:        map[string]*Media{},
		subs:         map[string]*Subtitle{},
		childFolders: map[string][]string{},
		childMedia:   map[string][]string{},
		probeCache:   map[string]*ProbeInfo{},
		probeStop:    make(chan struct{}),
	}
	s.loadProbeCache()
	s.probeWG.Add(1)
	go s.probeSaver()
	return s, nil
}

func (s *Store) Close() {
	close(s.probeStop)
	s.probeWG.Wait()
	s.SaveProbeCache()
}

// ---------- 库管理 ----------

func (s *Store) LibID(path string) string { return NewID(path, "") }

// AddLibrary 注册媒体库（不触发扫描）。
func (s *Store) AddLibrary(path, label string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("路径不可访问: %v", err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("不是文件夹: %s", path)
	}
	id := s.LibID(path)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.libs[id]; ok {
		return id, nil // 幂等
	}
	if label == "" {
		label = filepath.Base(path)
	}
	s.libs[id] = &LibraryState{ID: id, Path: path, Label: label, Enabled: true}
	return id, nil
}

func (s *Store) RemoveLibrary(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.libs, id)
	s.dropLibraryLocked(id)
}

func (s *Store) dropLibraryLocked(libID string) {
	for fid, f := range s.folders {
		if f.LibraryID == libID {
			delete(s.folders, fid)
		}
	}
	for mid, m := range s.media {
		if m.LibraryID == libID {
			delete(s.media, mid)
		}
	}
	for sid, sub := range s.subs {
		if sub.LibraryID == libID {
			delete(s.subs, sid)
		}
	}
	s.rebuildLocked()
}

func (s *Store) SetLibraryEnabled(id string, en bool) {
	s.mu.Lock()
	if lib, ok := s.libs[id]; ok {
		lib.Enabled = en
		if !en {
			s.dropLibraryLocked(id)
		}
	}
	s.mu.Unlock()
}

func (s *Store) SetLibraryLabel(id, label string) {
	s.mu.Lock()
	if lib, ok := s.libs[id]; ok && label != "" {
		lib.Label = label
	}
	s.mu.Unlock()
}

func (s *Store) Libraries() []LibraryState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LibraryState, 0, len(s.libs))
	for _, l := range s.libs {
		out = append(out, *l)
	}
	sort.Slice(out, func(i, j int) bool { return NaturalLess(out[i].Label, out[j].Label) })
	return out
}

func (s *Store) GetLibrary(id string) (LibraryState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if l, ok := s.libs[id]; ok {
		return *l, true
	}
	return LibraryState{}, false
}

// AnyScanning 报告是否有库正在扫描（管理页轮询用）。
func (s *Store) AnyScanning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, l := range s.libs {
		if l.Scanning {
			return true
		}
	}
	return false
}

func (s *Store) Counts() (folders, media int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.folders), len(s.media)
}

// ---------- 查询 ----------

var newWithin = 14 * 24 * time.Hour

func isNew(mtime time.Time) bool { return time.Since(mtime) < newWithin }

type FolderSummary struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Cover     string    `json:"cover"`
	VideoCount int      `json:"videoCount"`
	ImageCount int      `json:"imageCount"`
	SubVideos int       `json:"subVideos"`
	Mtime     time.Time `json:"mtime"`
	IsNew     bool      `json:"isNew,omitempty"`
}

// 封面统一走文件夹端点：有图重定向原图（带缩放），无图由服务端 ffmpeg 抽帧。
func (s *Store) folderSummary(f *Folder) FolderSummary {
	sum := FolderSummary{
		ID: f.ID, Name: f.Name, VideoCount: f.VideoCount, ImageCount: f.ImageCount,
		SubVideos: f.SubVideos, Mtime: f.Mtime, IsNew: isNew(f.Mtime),
	}
	if f.VideoCount > 0 || f.CoverID != "" {
		sum.Cover = "/api/folders/" + f.ID + "/cover?w=480"
	}
	return sum
}

// Roots 返回所有母文件夹（各库第一层）。
func (s *Store) Roots(sortKey, order string) []FolderSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out = make([]FolderSummary, 0, 16)
	for _, f := range s.folders {
		if f.ParentID == "" {
			out = append(out, s.folderSummary(f))
		}
	}
	sortFolderSummaries(out, sortKey, order)
	return out
}

func (s *Store) GetFolder(id string) (Folder, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if f, ok := s.folders[id]; ok {
		return *f, true
	}
	return Folder{}, false
}

func (s *Store) GetMedia(id string) (Media, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if m, ok := s.media[id]; ok {
		return *m, true
	}
	return Media{}, false
}

// ResolveMedia 返回媒体文件的绝对路径（ID → 路径映射仅存在于内存，接口不暴露路径）。
func (s *Store) ResolveMedia(id string) (string, Media, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.media[id]
	if !ok {
		return "", Media{}, fmt.Errorf("媒体不存在")
	}
	lib, ok := s.libs[m.LibraryID]
	if !ok {
		return "", Media{}, fmt.Errorf("所属媒体库已移除")
	}
	return filepath.Join(lib.Path, filepath.FromSlash(m.RelPath)), *m, nil
}

// GetSubtitle / ResolveSubtitle 外挂字幕查询。
func (s *Store) GetSubtitle(id string) (Subtitle, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if sub, ok := s.subs[id]; ok {
		return *sub, true
	}
	return Subtitle{}, false
}

func (s *Store) ResolveSubtitle(id string) (string, Subtitle, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sub, ok := s.subs[id]
	if !ok {
		return "", Subtitle{}, fmt.Errorf("字幕不存在")
	}
	lib, ok := s.libs[sub.LibraryID]
	if !ok {
		return "", Subtitle{}, fmt.Errorf("所属媒体库已移除")
	}
	return filepath.Join(lib.Path, filepath.FromSlash(sub.RelPath)), *sub, nil
}

// FirstVideoInTree 返回文件夹（含子层，自然序）中第一个视频，供封面抽帧。
func (s *Store) FirstVideoInTree(folderID string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.firstVideoLocked(folderID, 0)
}

func (s *Store) firstVideoLocked(folderID string, depth int) (string, bool) {
	if depth > 8 {
		return "", false
	}
	if ids, ok := s.childMedia[folderID]; ok {
		for _, id := range ids {
			if m, ok := s.media[id]; ok && m.Kind == KindVideo {
				return id, true
			}
		}
	}
	for _, fid := range s.childFolders[folderID] {
		if id, ok := s.firstVideoLocked(fid, depth+1); ok {
			return id, true
		}
	}
	return "", false
}

// SubtitlesOfMedia 返回视频的外挂字幕列表。
func (s *Store) SubtitlesOfMedia(m *Media) []Subtitle {
	out := make([]Subtitle, 0, len(m.SubIDs))
	s.mu.RLock()
	for _, id := range m.SubIDs {
		if sub, ok := s.subs[id]; ok {
			out = append(out, *sub)
		}
	}
	s.mu.RUnlock()
	return out
}

func (s *Store) ResolveFolder(id string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.folders[id]
	if !ok {
		return "", fmt.Errorf("文件夹不存在")
	}
	lib, ok := s.libs[f.LibraryID]
	if !ok {
		return "", fmt.Errorf("所属媒体库已移除")
	}
	return filepath.Join(lib.Path, filepath.FromSlash(f.RelPath)), nil
}

type Crumb struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Breadcrumb 返回 文件夹链（首项为库标签，仅名称不含物理路径）。
func (s *Store) Breadcrumb(folderID string) []Crumb {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var chain []Crumb
	cur, ok := s.folders[folderID]
	for ok {
		chain = append(chain, Crumb{ID: cur.ID, Name: cur.Name})
		if cur.ParentID == "" {
			if lib, ok := s.libs[cur.LibraryID]; ok {
				chain = append(chain, Crumb{ID: "", Name: lib.Label})
			}
			break
		}
		cur, ok = s.folders[cur.ParentID]
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

type MediaSummary struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Mtime    time.Time `json:"mtime"`
	Duration float64   `json:"duration,omitempty"`
	CardCover string   `json:"cardCover,omitempty"`
	IsNew    bool      `json:"isNew,omitempty"`
	Subs     int       `json:"subs,omitempty"`
}

func mediaSummary(m *Media) MediaSummary {
	sum := MediaSummary{ID: m.ID, Kind: m.Kind.String(), Name: m.Name, Size: m.Size, Mtime: m.Mtime, Duration: m.Duration, IsNew: isNew(m.Mtime), Subs: len(m.SubIDs)}
	if m.CardCoverID != "" {
		sum.CardCover = "/api/media/" + m.CardCoverID + "/image?w=480"
	}
	return sum
}

type ChildrenResult struct {
	Breadcrumb []Crumb         `json:"breadcrumb"`
	Folders    []FolderSummary `json:"folders"`
	Media      MediaPage       `json:"media"`
}

type MediaPage struct {
	Page  int            `json:"page"`
	Size  int            `json:"size"`
	Total int            `json:"total"`
	Items []MediaSummary `json:"items"`
}

func (s *Store) Children(folderID string, page, size int, sortKey, order string) (ChildrenResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.folders[folderID]; !ok {
		return ChildrenResult{}, fmt.Errorf("文件夹不存在")
	}
	res := ChildrenResult{Breadcrumb: s.breadcrumbLocked(folderID), Folders: []FolderSummary{}}
	if ids, ok := s.childFolders[folderID]; ok {
		for _, id := range ids {
			res.Folders = append(res.Folders, s.folderSummary(s.folders[id]))
		}
	}
	sortFolderSummaries(res.Folders, sortKey, order)

	mediaIDs := s.childMedia[folderID]
	total := len(mediaIDs)
	sums := make([]MediaSummary, 0, total)
	for _, id := range mediaIDs {
		sums = append(sums, mediaSummary(s.media[id]))
	}
	sortMediaSummaries(sums, sortKey, order)
	res.Media = paginate(sums, page, size)
	return res, nil
}

func paginate(sums []MediaSummary, page, size int) MediaPage {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	total := len(sums)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return MediaPage{Page: page, Size: size, Total: total, Items: sums[start:end]}
}

func (s *Store) breadcrumbLocked(folderID string) []Crumb {
	var chain []Crumb
	cur, ok := s.folders[folderID]
	for ok {
		chain = append(chain, Crumb{ID: cur.ID, Name: cur.Name})
		if cur.ParentID == "" {
			if lib, ok := s.libs[cur.LibraryID]; ok {
				chain = append(chain, Crumb{ID: "", Name: lib.Label})
			}
			break
		}
		cur, ok = s.folders[cur.ParentID]
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

type Sibling struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Siblings 返回同文件夹内同类型媒体的前后项。
func (s *Store) Siblings(mediaID string) (prev, next *Sibling) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.media[mediaID]
	if !ok {
		return nil, nil
	}
	var same []string
	for _, id := range s.childMedia[m.FolderID] {
		if s.media[id].Kind == m.Kind {
			same = append(same, id)
		}
	}
	for i, id := range same {
		if id != mediaID {
			continue
		}
		if i > 0 {
			prev = &Sibling{ID: same[i-1], Name: s.media[same[i-1]].Name}
		}
		if i+1 < len(same) {
			next = &Sibling{ID: same[i+1], Name: s.media[same[i+1]].Name}
		}
		return prev, next
	}
	return nil, nil
}

// ---------- 排序 ----------

func normSort(sortKey, order string) (string, string) {
	switch sortKey {
	case "mtime", "size":
	default:
		sortKey = "name"
	}
	if order != "desc" {
		order = "asc"
	}
	return sortKey, order
}

func sortFolderSummaries(list []FolderSummary, sortKey, order string) {
	sortKey, order = normSort(sortKey, order)
	if sortKey == "size" {
		sortKey = "name" // 文件夹无大小维度，降级为名称
	}
	desc := order == "desc"
	if sortKey == "name" {
		sort.SliceStable(list, func(i, j int) bool {
			if desc {
				return NaturalLess(list[j].Name, list[i].Name)
			}
			return NaturalLess(list[i].Name, list[j].Name)
		})
		return
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.Mtime.Equal(b.Mtime) {
			return NaturalLess(a.Name, b.Name)
		}
		if desc {
			return a.Mtime.After(b.Mtime)
		}
		return a.Mtime.Before(b.Mtime)
	})
}

func sortMediaSummaries(list []MediaSummary, sortKey, order string) {
	sortKey, order = normSort(sortKey, order)
	desc := order == "desc"
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		var less bool
		switch sortKey {
		case "mtime":
			if a.Mtime.Equal(b.Mtime) {
				less = NaturalLess(a.Name, b.Name)
			} else {
				less = a.Mtime.Before(b.Mtime)
			}
			if desc && !a.Mtime.Equal(b.Mtime) {
				less = a.Mtime.After(b.Mtime)
			}
		case "size":
			if a.Size == b.Size {
				less = NaturalLess(a.Name, b.Name)
			} else {
				less = a.Size < b.Size
			}
			if desc && a.Size != b.Size {
				less = a.Size > b.Size
			}
		default:
			if desc {
				less = NaturalLess(b.Name, a.Name)
			} else {
				less = NaturalLess(a.Name, b.Name)
			}
		}
		return less
	})
}

// ---------- 探测缓存 ----------

func (s *Store) probeKey(libPath, rel string) string {
	return libPath + "\x00" + rel
}

func (s *Store) loadProbeCache() {
	raw, err := os.ReadFile(filepath.Join(s.dataDir, "probe-cache.json"))
	if err != nil {
		return
	}
	var m map[string]*ProbeInfo
	if json.Unmarshal(raw, &m) == nil && m != nil {
		s.probeCache = m
	}
}

func (s *Store) SaveProbeCache() {
	s.probeMu.Lock()
	if !s.probeDirty {
		s.probeMu.Unlock()
		return
	}
	raw, err := json.Marshal(s.probeCache)
	s.probeDirty = false
	s.probeMu.Unlock()
	if err != nil {
		return
	}
	tmp := filepath.Join(s.dataDir, "probe-cache.json.tmp")
	if os.WriteFile(tmp, raw, 0o644) == nil {
		_ = os.Rename(tmp, filepath.Join(s.dataDir, "probe-cache.json"))
	}
}

func (s *Store) probeSaver() {
	defer s.probeWG.Done()
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.probeStop:
			return
		case <-t.C:
			s.SaveProbeCache()
		}
	}
}

// LookupProbeCache 命中（size+mtime 校验）则填充探测信息，返回是否命中。
func (s *Store) LookupProbeCache(libPath string, m *Media) bool {
	s.probeMu.Lock()
	p, ok := s.probeCache[s.probeKey(libPath, m.RelPath)]
	s.probeMu.Unlock()
	if !ok || p == nil || p.Size != m.Size || p.Mtime != m.Mtime.Unix() {
		return false
	}
	m.Duration, m.VCodec, m.ACodec = p.Duration, p.VCodec, p.ACodec
	m.Width, m.Height, m.Probed = p.Width, p.Height, true
	if len(p.Subs) > 0 {
		m.EmbeddedSubs = append([]SubStream(nil), p.Subs...)
	}
	return true
}

// SetProbeResult 写入探测结果并更新缓存。
func (s *Store) SetProbeResult(libPath string, m *Media, info *ProbeInfo) {
	m.Duration, m.VCodec, m.ACodec = info.Duration, info.VCodec, info.ACodec
	m.Width, m.Height, m.Probed = info.Width, info.Height, true
	if len(info.Subs) > 0 {
		m.EmbeddedSubs = append([]SubStream(nil), info.Subs...)
	}
	s.mu.Lock()
	if cur, ok := s.media[m.ID]; ok {
		cur.Duration, cur.VCodec, cur.ACodec = info.Duration, info.VCodec, info.ACodec
		cur.Width, cur.Height, cur.Probed = info.Width, info.Height, true
		if len(info.Subs) > 0 {
			cur.EmbeddedSubs = append([]SubStream(nil), info.Subs...)
		}
	}
	s.mu.Unlock()
	s.probeMu.Lock()
	s.probeCache[s.probeKey(libPath, m.RelPath)] = &ProbeInfo{
		Size: m.Size, Mtime: m.Mtime.Unix(),
		Duration: info.Duration, VCodec: info.VCodec, ACodec: info.ACodec,
		Width: info.Width, Height: info.Height, Subs: info.Subs,
	}
	s.probeDirty = true
	s.probeMu.Unlock()
}

// ---------- 内部 ----------

func (s *Store) rebuildLocked() {
	s.childFolders = map[string][]string{}
	s.childMedia = map[string][]string{}
	for id, f := range s.folders {
		s.childFolders[f.ParentID] = append(s.childFolders[f.ParentID], id)
	}
	for id, m := range s.media {
		s.childMedia[m.FolderID] = append(s.childMedia[m.FolderID], id)
	}
	for _, ids := range s.childFolders {
		sortIDsByName(ids, s.foldersIDName)
	}
	for _, ids := range s.childMedia {
		sortIDsByName(ids, s.mediaIDName)
	}
}

type nameGetter func(id string) string

func (s *Store) foldersIDName(id string) string { return s.folders[id].Name }
func (s *Store) mediaIDName(id string) string   { return s.media[id].Name }

func sortIDsByName(ids []string, name nameGetter) {
	sort.SliceStable(ids, func(i, j int) bool {
		return NaturalLess(name(ids[i]), name(ids[j]))
	})
}
