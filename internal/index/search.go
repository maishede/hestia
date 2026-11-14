package index

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// NaturalLess 数值感知自然排序（"铁2" < "铁10"），忽略大小写；中文按码点序。
func NaturalLess(a, b string) bool {
	return naturalCompare(a, b) < 0
}

func naturalCompare(a, b string) int {
	for len(a) > 0 && len(b) > 0 {
		if isDigit(a[0]) && isDigit(b[0]) {
			ia, ib := digitCount(a), digitCount(b)
			na := strings.TrimLeft(a[:ia], "0")
			nb := strings.TrimLeft(b[:ib], "0")
			if len(na) != len(nb) {
				return cmp.Compare(len(na), len(nb))
			}
			if na != nb {
				return cmp.Compare(na, nb)
			}
			a, b = a[ia:], b[ib:]
			continue
		}
		ca, wa := utf8.DecodeRuneInString(a)
		cb, wb := utf8.DecodeRuneInString(b)
		if comparison := cmp.Compare(unicode.ToLower(ca), unicode.ToLower(cb)); comparison != 0 {
			return comparison
		}
		a, b = a[wa:], b[wb:]
	}
	return cmp.Compare(len(a), len(b))
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func digitCount(s string) int {
	i := 0
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	return i
}

type SearchItem struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"` // video | image
	Name     string  `json:"name"`
	Where    string  `json:"where"` // 所属文件夹面包屑（仅名称，不含物理路径）
	Size     int64   `json:"size,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	Mtime    int64   `json:"mtime,omitempty"`
}

type SearchResult struct {
	Items []SearchItem `json:"items"`
	Total int          `json:"total"`
	Page  int          `json:"page"`
	Size  int          `json:"size"`
}

type scoredHit struct {
	item  SearchItem
	score int
}

// Search 按媒体文件名模糊搜索，支持空格分词、子串、拼音和 ASCII 子序列匹配。
func (s *Store) Search(q string, page, size int, sortKey, order string) SearchResult {
	tokens := tokenize(q)
	page, size = normalizePage(page, size)
	res := SearchResult{Items: []SearchItem{}, Page: page, Size: size}
	if len(tokens) == 0 {
		return res
	}

	var hits []scoredHit
	s.mu.RLock()
	for _, m := range s.media {
		if m.Kind == KindImage && m.CoverOf != "" {
			continue // 已并入视频卡片做海报的图片不参与搜索
		}
		if ok, sc := matchItem(strings.ToLower(m.Name), m.PyFull, m.PyInit, tokens); ok {
			it := SearchItem{ID: m.ID, Kind: m.Kind.String(), Name: m.Name, Size: m.Size, Mtime: m.Mtime.Unix(), Duration: m.Duration}
			hits = append(hits, scoredHit{it, sc})
		}
	}
	s.mu.RUnlock()

	res.Total = len(hits)
	slices.SortFunc(hits, func(a, b scoredHit) int {
		comparison, direction := cmp.Compare(a.score, b.score), "desc"
		if sortKey == "mtime" {
			comparison, direction = cmp.Compare(a.item.Mtime, b.item.Mtime), order
		}
		return compareSort(comparison, direction, a.item.Name, b.item.Name, a.item.ID, b.item.ID)
	})

	start, end := pageBounds(len(hits), page, size)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, h := range hits[start:end] {
		if m := s.media[h.item.ID]; m != nil {
			h.item.Where = s.whereLocked(m.FolderID)
		}
		res.Items = append(res.Items, h.item)
	}
	return res
}

func (s *Store) whereLocked(folderID string) string {
	var parts []string
	cur, ok := s.folders[folderID]
	for ok {
		parts = append(parts, cur.Name)
		if cur.ParentID == "" {
			if lib, ok := s.libs[cur.LibraryID]; ok {
				parts = append(parts, lib.Label)
			}
			break
		}
		cur, ok = s.folders[cur.ParentID]
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, " / ")
}

func tokenize(q string) []string {
	fields := strings.Fields(q)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.ToLower(strings.TrimSpace(f))
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// matchItem 对 名称/全拼/首字母 三个键做 AND 匹配（任一键命中即可）。
func matchItem(lowerName, pyFull, pyInit string, tokens []string) (bool, int) {
	total := 0
	for _, t := range tokens {
		if idx := strings.Index(lowerName, t); idx >= 0 {
			total += 100 - min(idx, 50)
			continue
		}
		if pyFull != "" && strings.Contains(pyFull, t) {
			total += 80
			continue
		}
		if pyInit != "" && strings.Contains(pyInit, t) {
			total += 60
			continue
		}
		if isASCII(t) && isSubsequence(lowerName, t) {
			total += 40
			continue
		}
		return false, 0
	}
	return true, total
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func isSubsequence(hay, needle string) bool {
	i := 0
	for j := 0; j < len(hay) && i < len(needle); j++ {
		if hay[j] == needle[i] {
			i++
		}
	}
	return i == len(needle)
}
