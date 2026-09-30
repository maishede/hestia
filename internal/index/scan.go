package index

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var videoExts = map[string]bool{
	".mp4": true, ".mkv": true, ".webm": true, ".mov": true, ".m4v": true,
	".avi": true, ".ts": true, ".flv": true, ".wmv": true, ".mpg": true,
	".mpeg": true, ".rm": true, ".rmvb": true, ".3gp": true, ".vob": true,
	".ogv": true, ".m2ts": true, ".mts": true,
}

var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".jfif": true, ".png": true, ".webp": true,
	".gif": true, ".bmp": true, ".avif": true, ".heic": true,
}

var subtitleExts = map[string]bool{
	".srt": true, ".ass": true, ".ssa": true,
}

var skipDirNames = map[string]bool{
	"$recycle.bin": true, "system volume information": true, "lost+found": true,
}

type draftFolder struct {
	f        Folder
	childs   []*draftFolder // 子草稿（用于自底向上统计）
	hasCover bool
}

// RescanLibrary 全量扫描一个媒体库并原子替换其索引内容。
func (s *Store) RescanLibrary(libID string) error {
	lib, ok := s.GetLibrary(libID)
	if !ok {
		return fmt.Errorf("库不存在")
	}
	if !lib.Enabled {
		return nil
	}
	s.scanMu.Lock()
	defer s.scanMu.Unlock()

	s.mu.Lock()
	if cur, ok := s.libs[libID]; ok {
		cur.Scanning = true
		cur.Files = 0
		cur.Dirs = 0
	}
	s.mu.Unlock()

	err := s.walkLibrary(libID, lib.Path)

	s.mu.Lock()
	if cur, ok := s.libs[libID]; ok {
		cur.Scanning = false
		cur.LastScan = time.Now()
		cur.LastErr = ""
		if err != nil {
			cur.LastErr = err.Error()
		} else {
			n, d := 0, 0
			for _, m := range s.media {
				if m.LibraryID == libID {
					n++
				}
			}
			for _, f := range s.folders {
				if f.LibraryID == libID {
					d++
				}
			}
			cur.Files, cur.Dirs = n, d
		}
	}
	s.mu.Unlock()
	return err
}

// RescanAll 串行重扫所有已启用库。
func (s *Store) RescanAll() {
	for _, l := range s.Libraries() {
		if l.Enabled {
			_ = s.RescanLibrary(l.ID)
		}
	}
}

func (s *Store) setProgress(libID string, files int) {
	s.mu.Lock()
	if cur, ok := s.libs[libID]; ok {
		cur.Files = files
	}
	s.mu.Unlock()
}

// dirOfSlash 返回斜杠相对路径的父目录（顶层返回 ""）。
func dirOfSlash(rel string) string {
	if i := strings.LastIndexByte(rel, '/'); i >= 0 {
		return rel[:i]
	}
	return ""
}

func (s *Store) walkLibrary(libID, root string) error {
	drafts := map[string]*draftFolder{} // rel -> 文件夹草稿（不含库根 ""）
	var medias []*Media
	var subtitles []*Subtitle

	relOf := func(p string) string {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return ""
		}
		return filepath.ToSlash(rel)
	}
	folderIDFor := func(rel string) string {
		if rel == "" {
			return ""
		}
		if d, ok := drafts[rel]; ok {
			return d.f.ID
		}
		return ""
	}

	visited := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 跳过不可读项，继续扫描其余内容
		}
		name := d.Name()
		if d.IsDir() {
			if p == root {
				return nil
			}
			if strings.HasPrefix(name, ".") || skipDirNames[strings.ToLower(name)] {
				return fs.SkipDir
			}
			rel := relOf(p)
			if rel == "" || rel == ".." || strings.HasPrefix(rel, "../") {
				return nil
			}
			info, ie := d.Info()
			if ie != nil {
				return nil
			}
			df := &draftFolder{f: Folder{
				ID: NewID(root, rel), LibraryID: libID,
				ParentID: folderIDFor(dirOfSlash(rel)), RelPath: rel,
				Name: name, Mtime: info.ModTime(),
			}}
			drafts[rel] = df
			if parent, ok := drafts[dirOfSlash(rel)]; ok {
				parent.childs = append(parent.childs, df)
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		isVideo, isImage, isSub := videoExts[ext], imageExts[ext], subtitleExts[ext]
		if !isVideo && !isImage && !isSub {
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		info, ie := d.Info()
		if ie != nil {
			return nil
		}
		rel := relOf(p)
		visited++
		if visited%200 == 0 {
			s.setProgress(libID, visited)
		}
		if isSub {
			subtitles = append(subtitles, &Subtitle{
				ID: NewID(root, rel), LibraryID: libID, FolderID: folderIDFor(dirOfSlash(rel)),
				RelPath: rel, Name: strings.TrimSuffix(name, ext), Ext: ext,
				Size: info.Size(), Mtime: info.ModTime(),
			})
			return nil
		}
		kind := KindImage
		if isVideo {
			kind = KindVideo
		}
		medias = append(medias, &Media{
			ID: NewID(root, rel), LibraryID: libID, FolderID: folderIDFor(dirOfSlash(rel)),
			RelPath: rel, Name: strings.TrimSuffix(name, ext), Ext: ext,
			Kind: kind, Size: info.Size(), Mtime: info.ModTime(),
		})
		return nil
	})
	if err != nil {
		return err
	}

	// ---- 分组统计 / 封面 / 同名字幕 ----
	type folderAgg struct {
		videos, images []*Media
		subs           []*Subtitle
	}
	aggs := map[string]*folderAgg{}
	getAgg := func(rel string) *folderAgg {
		if a, ok := aggs[rel]; ok {
			return a
		}
		a := &folderAgg{}
		aggs[rel] = a
		return a
	}
	for _, m := range medias {
		a := getAgg(dirOfSlash(m.RelPath))
		if m.Kind == KindVideo {
			a.videos = append(a.videos, m)
		} else {
			a.images = append(a.images, m)
		}
	}
	for _, sub := range subtitles {
		getAgg(dirOfSlash(sub.RelPath)).subs = append(getAgg(dirOfSlash(sub.RelPath)).subs, sub)
	}

	for rel, a := range aggs {
		cover := pickCover(a.images)
		byStem := map[string]*Media{}
		for _, img := range a.images {
			byStem[strings.ToLower(img.Name)] = img
		}
		// 字幕挂到同名视频（或 "视频名.语言.srt" 前缀匹配）；同名图片并入视频卡片做海报
		for _, v := range a.videos {
			lv := strings.ToLower(v.Name)
			if img, ok := byStem[lv]; ok {
				v.CardCoverID = img.ID
				img.CoverOf = v.ID
			}
			for _, sub := range a.subs {
				ls := strings.ToLower(sub.Name)
				if ls == lv || strings.HasPrefix(ls, lv+".") {
					v.SubIDs = append(v.SubIDs, sub.ID)
				}
			}
			sort.Strings(v.SubIDs)
		}
		// 单视频文件夹：poster/cover/folder/fanart 命名的图片视为该片海报，同样并入视频卡片
		if len(a.videos) == 1 && a.videos[0].CardCoverID == "" {
			v := a.videos[0]
			var cand *Media
			candRank := 9
			for _, img := range a.images {
				if img.CoverOf != "" {
					continue
				}
				if r := coverRank(img.Name); r < candRank || (r == candRank && cand != nil && NaturalLess(img.Name, cand.Name)) {
					cand, candRank = img, r
				}
			}
			if cand != nil {
				v.CardCoverID = cand.ID
				cand.CoverOf = v.ID
			}
		}
		if df, ok := drafts[rel]; ok { // 库根层媒体无文件夹节点
			df.f.VideoCount = len(a.videos)
			visible := 0
			for _, img := range a.images {
				if img.CoverOf == "" {
					visible++
				}
			}
			df.f.ImageCount = visible // 已并入视频卡片的封面图不计入列表展示数
			if cover != nil {
				df.f.CoverID = cover.ID
				df.hasCover = true
			}
		}
	}

	// 递归视频/图片数 + 子层封面继承（自底向上）
	rels := make([]string, 0, len(drafts))
	for rel := range drafts {
		rels = append(rels, rel)
	}
	sort.Slice(rels, func(i, j int) bool {
		return strings.Count(rels[i], "/") > strings.Count(rels[j], "/")
	})
	for _, rel := range rels {
		df := drafts[rel]
		sum, sumImg := df.f.VideoCount, df.f.ImageCount
		for _, c := range df.childs {
			sum += c.f.SubVideos
			sumImg += c.f.SubImages
			if !df.hasCover && c.hasCover {
				df.f.CoverID = c.f.CoverID
				df.hasCover = true
			}
		}
		df.f.SubVideos = sum
		df.f.SubImages = sumImg
	}

	// 拼音键
	for _, m := range medias {
		m.PyFull, m.PyInit = PinyinKeys(m.Name)
	}
	for _, df := range drafts {
		df.f.PyFull, df.f.PyInit = PinyinKeys(df.f.Name)
	}

	// ---- 原子提交 ----
	s.mu.Lock()
	s.dropLibraryLocked(libID)
	for _, df := range drafts {
		f := df.f
		s.folders[f.ID] = &f
	}
	for _, m := range medias {
		s.media[m.ID] = m
	}
	for _, sub := range subtitles {
		s.subs[sub.ID] = sub
	}
	s.rebuildLocked()
	s.mu.Unlock()

	// 探测缓存回填（时长/编码/字幕轨）
	if root != "" {
		for _, m := range medias {
			s.LookupProbeCache(root, m)
		}
	}
	s.setProgress(libID, len(medias))
	return nil
}

// pickCover 按命名优先级选封面：poster > cover > folder > fanart > 自然序第一张。
func pickCover(images []*Media) *Media {
	if len(images) == 0 {
		return nil
	}
	best := images[0]
	bestRank := coverRank(images[0].Name)
	for _, img := range images[1:] {
		r := coverRank(img.Name)
		if r < bestRank || (r == bestRank && NaturalLess(img.Name, best.Name)) {
			best, bestRank = img, r
		}
	}
	return best
}

func coverRank(name string) int {
	stem := strings.ToLower(name)
	if i := strings.LastIndexByte(stem, '.'); i > 0 {
		stem = stem[:i]
	}
	for _, p := range []struct {
		prefix string
		rank   int
	}{{"poster", 0}, {"cover", 1}, {"folder", 2}, {"fanart", 3}} {
		if strings.HasPrefix(stem, p.prefix) {
			return p.rank
		}
	}
	return 9
}
