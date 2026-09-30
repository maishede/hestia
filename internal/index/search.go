package index

import (
	"sort"
	"strings"
	"unicode"
)

// NaturalLess 数值感知自然排序（"铁2" < "铁10"），忽略大小写；中文按码点序。
func NaturalLess(a, b string) bool {
	ra, rb := []rune(a), []rune(b)
	i, j := 0, 0
	for i < len(ra) && j < len(rb) {
		ca, cb := ra[i], rb[j]
		if isDigitRune(ca) && isDigitRune(cb) {
			ia, ja := i, j
			for ia < len(ra) && isDigitRune(ra[ia]) {
				ia++
			}
			for ja < len(rb) && isDigitRune(rb[ja]) {
				ja++
			}
			na := strings.TrimLeft(string(ra[i:ia]), "0")
			nb := strings.TrimLeft(string(rb[j:ja]), "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			i, j = ia, ja
			continue
		}
		la, lb := unicode.ToLower(ca), unicode.ToLower(cb)
		if la != lb {
			return la < lb
		}
		i++
		j++
	}
	return len(ra)-i < len(rb)-j
}

func isDigitRune(r rune) bool { return r >= '0' && r <= '9' }

type SearchItem struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"` // video | image | folder
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

// Search 全库模糊搜索：空格分词 AND；子串匹配优先，纯 ASCII 词支持子序列匹配；中文子串匹配。
func (s *Store) Search(q string, page, size int, sortKey, order string) SearchResult {
	tokens := tokenize(q)
	res := SearchResult{Items: []SearchItem{}}
	if len(tokens) == 0 {
		return res
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	res.Page, res.Size = page, size

	var hits []scoredHit
	s.mu.RLock()
	for _, f := range s.folders {
		if f.treeEmpty() {
			continue
		}
		if ok, sc := matchItem(strings.ToLower(f.Name), f.PyFull, f.PyInit, tokens); ok {
			hits = append(hits, scoredHit{SearchItem{ID: f.ID, Kind: "folder", Name: f.Name, Where: s.whereLocked(f.ID)}, sc})
		}
	}
	for _, m := range s.media {
		if ok, sc := matchItem(strings.ToLower(m.Name), m.PyFull, m.PyInit, tokens); ok {
			it := SearchItem{ID: m.ID, Kind: m.Kind.String(), Name: m.Name, Size: m.Size, Mtime: m.Mtime.Unix(), Duration: m.Duration}
			if f, ok := s.folders[m.FolderID]; ok {
				it.Where = s.whereLocked(f.ID)
			}
			hits = append(hits, scoredHit{it, sc})
		}
	}
	s.mu.RUnlock()

	res.Total = len(hits)
	if sortKey == "mtime" {
		sort.SliceStable(hits, func(i, j int) bool {
			a, b := hits[i], hits[j]
			if a.item.Mtime == b.item.Mtime {
				if order == "desc" {
					return NaturalLess(b.item.Name, a.item.Name)
				}
				return NaturalLess(a.item.Name, b.item.Name)
			}
			if order == "desc" {
				return a.item.Mtime > b.item.Mtime
			}
			return a.item.Mtime < b.item.Mtime
		})
	} else {
		// 默认按相关度降序，平局自然名升序
		sort.SliceStable(hits, func(i, j int) bool {
			a, b := hits[i], hits[j]
			if a.score != b.score {
				return a.score > b.score
			}
			return NaturalLess(a.item.Name, b.item.Name)
		})
	}

	start := (page - 1) * size
	if start > len(hits) {
		start = len(hits)
	}
	end := start + size
	if end > len(hits) {
		end = len(hits)
	}
	for _, h := range hits[start:end] {
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
