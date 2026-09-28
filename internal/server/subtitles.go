package server

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"hestia/internal/index"
)

// 支持转 VTT 的文本字幕轨（PGS 图形字幕不支持）
var textSubCodecs = map[string]bool{
	"subrip": true, "srt": true, "ass": true, "ssa": true,
	"mov_text": true, "webvtt": true, "sami": true,
}

var assTagRe = regexp.MustCompile(`\{[^}]*\}`)

// ---------- 外挂字幕 ----------

// handleSubtitle 输出外挂字幕，统一转换为 WebVTT（浏览器原生 <track> 渲染）。
func (s *Server) handleSubtitle(w http.ResponseWriter, r *http.Request) {
	abs, sub, err := s.store.ResolveSubtitle(r.PathValue("id"))
	if err != nil {
		errJSON(w, http.StatusNotFound, "字幕不存在")
		return
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		errJSON(w, http.StatusNotFound, "字幕文件无法读取")
		return
	}
	if len(raw) > 16<<20 {
		errJSON(w, http.StatusRequestEntityTooLarge, "字幕文件过大")
		return
	}
	var vtt string
	switch strings.ToLower(sub.Ext) {
	case ".srt":
		vtt = srtToVtt(string(raw))
	case ".ass", ".ssa":
		vtt = assToVtt(string(raw))
	default:
		vtt = strings.TrimPrefix(string(raw), "\ufeff")
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write([]byte(vtt))
}

func srtToVtt(raw string) string {
	s := strings.TrimPrefix(strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n"), "\ufeff")
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "-->") {
			line = strings.ReplaceAll(line, ",", ".")
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func assToVtt(raw string) string {
	s := strings.TrimPrefix(strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n"), "\ufeff")
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(line, "Dialogue:") {
			continue
		}
		parts := strings.SplitN(strings.TrimPrefix(line, "Dialogue:"), ",", 10)
		if len(parts) < 10 {
			continue
		}
		start, end := assTime(parts[1]), assTime(parts[2])
		if start == "" || end == "" {
			continue
		}
		text := assTagRe.ReplaceAllString(parts[9], "")
		text = strings.NewReplacer(`\N`, "\n", `\n`, "\n", `\h`, " ").Replace(text)
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		n++
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", n, start, end, text)
	}
	if n == 0 {
		return "WEBVTT\n\n"
	}
	return b.String()
}

// assTime "0:00:01.23" → "00:00:01.230"
func assTime(v string) string {
	v = strings.TrimSpace(v)
	parts := strings.Split(v, ":")
	if len(parts) != 3 {
		return ""
	}
	sec, frac := parts[2], "000"
	if i := strings.IndexByte(sec, '.'); i >= 0 {
		frac = sec[i+1:]
		sec = sec[:i]
	}
	for len(frac) < 3 {
		frac += "0"
	}
	if len(frac) > 3 {
		frac = frac[:3]
	}
	return fmt.Sprintf("%02s:%02s:%02s.%s", parts[0], parts[1], sec, frac)
}

// ---------- 内嵌字幕提取 ----------

// handleEmbeddedSub 用 ffmpeg 把指定字幕轨提取为 VTT（结果缓存到磁盘）。
func (s *Server) handleEmbeddedSub(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	idx, err := strconv.Atoi(r.PathValue("index"))
	if err != nil || idx < 0 {
		errJSON(w, http.StatusBadRequest, "非法字幕轨")
		return
	}
	m, abs := s.ensureProbed(w, r, id)
	if m.ID == "" {
		return // ensureProbed 已写错误响应
	}
	var stream *index.SubStream
	for i := range m.EmbeddedSubs {
		if m.EmbeddedSubs[i].Index == idx {
			stream = &m.EmbeddedSubs[i]
			break
		}
	}
	if stream == nil {
		errJSON(w, http.StatusNotFound, "字幕轨不存在")
		return
	}
	if !textSubCodecs[stream.Codec] {
		errJSON(w, http.StatusUnsupportedMediaType, "该字幕轨为图形字幕（"+stream.Codec+"），暂不支持")
		return
	}
	if !s.tc.FFmpegOK() {
		errJSON(w, http.StatusServiceUnavailable, "未找到 ffmpeg")
		return
	}
	cacheDir := filepath.Join(s.dataDir, "cache", "sub")
	cache := filepath.Join(cacheDir, fmt.Sprintf("%s-%d-%d.vtt", id, idx, m.Size))
	if b, err := os.ReadFile(cache); err == nil {
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
		_, _ = w.Write(b)
		return
	}
	_ = os.MkdirAll(cacheDir, 0o755)
	s.subMu.Lock()
	defer s.subMu.Unlock()
	if b, err := os.ReadFile(cache); err == nil { // 双重检查
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
		_, _ = w.Write(b)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, s.tc.FFmpeg,
		"-nostdin", "-hide_banner", "-loglevel", "error",
		"-i", abs, "-map", "0:"+strconv.Itoa(idx), "-y", cache)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(cache)
		tail := stderr.String()
		if len(tail) > 300 {
			tail = tail[len(tail)-300:]
		}
		errJSON(w, http.StatusBadGateway, "字幕提取失败: "+tail)
		return
	}
	b, err := os.ReadFile(cache)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "字幕读取失败")
		return
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	_, _ = w.Write(b)
}

// ---------- 无图封面：视频抽帧 ----------

// handleFolderCover 文件夹封面统一入口：有图 → 图片端点；无图 → 抽首个视频帧。
func (s *Server) handleFolderCover(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	f, ok := s.store.GetFolder(id)
	if !ok {
		errJSON(w, http.StatusNotFound, "文件夹不存在")
		return
	}
	if f.CoverID != "" {
		target := "/api/media/" + f.CoverID + "/image"
		if q := r.URL.RawQuery; q != "" {
			target += "?" + q
		}
		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
		return
	}
	vid, has := s.store.FirstVideoInTree(id)
	if !has || !s.tc.FFmpegOK() {
		errJSON(w, http.StatusNotFound, "无可用封面")
		return
	}
	abs, m, err := s.store.ResolveMedia(vid)
	if err != nil {
		errJSON(w, http.StatusNotFound, "无可用封面")
		return
	}
	cacheDir := filepath.Join(s.dataDir, "cache", "fcover")
	cache := filepath.Join(cacheDir, fmt.Sprintf("%s-%d.jpg", id, m.Mtime.Unix()))
	if b, err := os.ReadFile(cache); err == nil {
		serveJPEG(w, b)
		return
	}
	_ = os.MkdirAll(cacheDir, 0o755)
	s.subMu.Lock()
	defer s.subMu.Unlock()
	if b, err := os.ReadFile(cache); err == nil {
		serveJPEG(w, b)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, s.tc.FFmpeg,
		"-nostdin", "-hide_banner", "-loglevel", "error",
		"-ss", "3", "-i", abs, "-frames:v", "1", "-vf", "scale=480:-2", "-q:v", "5", "-y", cache)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(cache)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("封面生成失败"))
		return
	}
	b, err := os.ReadFile(cache)
	if err != nil {
		errJSON(w, http.StatusNotFound, "封面生成失败")
		return
	}
	serveJPEG(w, b)
}

func serveJPEG(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(b)
}
