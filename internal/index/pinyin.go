package index

import (
	"strings"
	"unicode"

	"github.com/mozillazg/go-pinyin"
)

var pyArgs = pinyin.NewArgs()

func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// PinyinKeys 为名称生成全拼与首字母键（无汉字时返回空，避免无谓开销）。
func PinyinKeys(name string) (full, init string) {
	if !hasHan(name) {
		return "", ""
	}
	parts := pinyin.LazyPinyin(name, pyArgs)
	var f, i strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		f.WriteString(p)
		i.WriteByte(p[0])
	}
	return f.String(), i.String()
}
