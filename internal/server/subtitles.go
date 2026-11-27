package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
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
var vttBlocks = regexp.MustCompile(`\n[ \t]*\n+`)
var vttTiming = regexp.MustCompile(`^((?:[0-9]{2,}:)?[0-9]{2}:[0-9]{2}\.[0-9]{3})[ \t]+-->[ \t]+((?:[0-9]{2,}:)?[0-9]{2}:[0-9]{2}\.[0-9]{3})(.*)$`)
var vttInlineTime = regexp.MustCompile(`<((?:[0-9]{2,}:)?[0-9]{2}:[0-9]{2}\.[0-9]{3})>`)

// 字幕总是按原视频时间缓存，转码从中间开始时按会话实际起点调整。
func subtitleOffset(r *http.Request) (int64, error) {
	value := r.URL.Query().Get("offset")
	if value == "" {
		return 0, nil
	}
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds > 315360000 {
		return 0, fmt.Errorf("非法字幕时间偏移")
	}
	return int64(math.Round(seconds * 1000)), nil
}

func vttMillis(value string) (int64, bool) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 && len(parts) != 3 {
		return 0, false
	}
	var hours int64
	if len(parts) == 3 {
		var err error
		hours, err = strconv.ParseInt(parts[0], 10, 32)
		if err != nil {
			return 0, false
		}
		parts = parts[1:]
	}
	minutes, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil || minutes >= 60 {
		return 0, false
	}
	seconds := strings.Split(parts[1], ".")
	if len(seconds) != 2 {
		return 0, false
	}
	sec, err := strconv.ParseInt(seconds[0], 10, 32)
	if err != nil || sec >= 60 {
		return 0, false
	}
	ms, err := strconv.ParseInt(seconds[1], 10, 32)
	if err != nil {
		return 0, false
	}
	return ((hours*60+minutes)*60+sec)*1000 + ms, true
}

func formatVTTMillis(ms int64) string {
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

func shiftVTT(raw string, offset int64) string {
	if offset == 0 {
		return raw
	}
	raw = strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n")
	blocks := vttBlocks.Split(raw, -1)
	kept := make([]string, 0, len(blocks))
	for _, block := range blocks {
		lines := strings.Split(block, "\n")
		first := strings.TrimPrefix(lines[0], "\ufeff")
		if strings.HasPrefix(first, "WEBVTT") || first == "STYLE" || first == "REGION" || first == "NOTE" || strings.HasPrefix(first, "NOTE ") {
			kept = append(kept, block)
			continue
		}
		timing := 0
		match := vttTiming.FindStringSubmatch(lines[timing])
		if match == nil && len(lines) > 1 {
			timing = 1
			match = vttTiming.FindStringSubmatch(lines[timing])
		}
		if match == nil {
			kept = append(kept, block)
			continue
		}
		start, okStart := vttMillis(match[1])
		end, okEnd := vttMillis(match[2])
		if !okStart || !okEnd {
			kept = append(kept, block)
			continue
		}
		if end <= offset {
			continue
		}
		start -= offset
		if start < 0 {
			start = 0
		}
		lines[timing] = formatVTTMillis(start) + " --> " + formatVTTMillis(end-offset) + match[3]
		for i := timing + 1; i < len(lines); i++ {
			lines[i] = vttInlineTime.ReplaceAllStringFunc(lines[i], func(tag string) string {
				stamp, ok := vttMillis(tag[1 : len(tag)-1])
				if !ok {
					return tag
				}
				if stamp <= offset+start {
					return ""
				}
				return "<" + formatVTTMillis(stamp-offset) + ">"
			})
		}
		kept = append(kept, strings.Join(lines, "\n"))
	}
	return strings.Join(kept, "\n\n")
}

func writeSubtitle(w http.ResponseWriter, vtt []byte, offset int64) {
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	_, _ = w.Write([]byte(shiftVTT(string(vtt), offset)))
}

// ---------- 外挂字幕 ----------

// handleSubtitle 输出外挂字幕，统一转换为 WebVTT（浏览器原生 <track> 渲染）。
func (s *Server) handleSubtitle(w http.ResponseWriter, r *http.Request) {
	offset, err := subtitleOffset(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	abs, sub, err := s.store.ResolveSubtitle(r.PathValue("id"))
	if err != nil {
		errJSON(w, http.StatusNotFound, "字幕不存在")
		return
	}
	ctx, endRead, ok := s.beginMediaRead(w, r, sub.FolderID)
	if !ok {
		return
	}
	raw, err := os.ReadFile(abs)
	canceled := ctx.Err() != nil
	endRead()
	if canceled {
		mediaReadCanceled(ctx, w, r)
		return
	}
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
	w.Header().Set("Cache-Control", "public, max-age=86400")
	writeSubtitle(w, []byte(vtt), offset)
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
	offset, err := subtitleOffset(r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
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
		writeSubtitle(w, b, offset)
		return
	}
	_ = os.MkdirAll(cacheDir, 0o755)
	s.subMu.Lock()
	defer s.subMu.Unlock()
	if b, err := os.ReadFile(cache); err == nil { // 双重检查
		writeSubtitle(w, b, offset)
		return
	}
	readCtx, endRead, ok := s.beginMediaRead(w, r, m.FolderID)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(readCtx, 60*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, s.tc.FFmpeg,
		"-nostdin", "-hide_banner", "-loglevel", "error",
		"-i", abs, "-map", "0:"+strconv.Itoa(idx), "-y", cache)
	hideChildWindow(cmd)
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	canceled := readCtx.Err() != nil
	endRead()
	if canceled {
		_ = os.Remove(cache)
		mediaReadCanceled(readCtx, w, r)
		return
	}
	if runErr != nil {
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
	writeSubtitle(w, b, offset)
}

// ---------- 无图封面：视频抽帧 ----------

// handleFolderCover retains the folder endpoint for older clients.
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
	vid, ok := s.store.FirstVideoInTree(id)
	if !ok {
		errJSON(w, http.StatusNotFound, "无可用封面")
		return
	}
	s.serveVideoFrame(w, r, vid)
}

func (s *Server) handleMediaThumb(w http.ResponseWriter, r *http.Request) {
	s.serveVideoFrame(w, r, r.PathValue("id"))
}

func (s *Server) serveVideoFrame(w http.ResponseWriter, r *http.Request, id string) {
	abs, m, err := s.store.ResolveMedia(id)
	if err != nil || m.Kind != index.KindVideo {
		errJSON(w, http.StatusNotFound, "视频不存在")
		return
	}
	if !s.tc.FFmpegOK() {
		errJSON(w, http.StatusServiceUnavailable, "未找到 ffmpeg")
		return
	}
	result, err := s.videoThumbnail(r.Context(), abs, m, false)
	if r.Context().Err() != nil {
		return
	}
	if err == errFolderDeleting {
		errJSON(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		errJSON(w, http.StatusNotFound, "缩略图生成失败")
		return
	}
	if len(result.data) > 0 {
		serveJPEG(w, result.data)
	}
}

func (s *Server) videoThumbnail(parent context.Context, abs string, m index.Media, batch bool) (thumbnail, error) {
	cacheDir := filepath.Join(s.dataDir, "cache", "thumb")
	cache := filepath.Join(cacheDir, fmt.Sprintf("%s-%d-%d.jpg", m.ID, m.Size, m.Mtime.Unix()))
	if result := cachedThumbnail(cache, batch); result.cached {
		return result, nil
	}
	if batch && !s.tc.FFmpegOK() {
		return thumbnail{}, nil
	}
	unlock, ok := s.imageJobs.lock(parent, cache)
	if !ok {
		return thumbnail{}, parent.Err()
	}
	defer unlock()
	if result := cachedThumbnail(cache, batch); result.cached {
		return result, nil
	}
	readCtx, endRead, err := s.startMediaRead(parent, m.FolderID)
	if err != nil {
		return thumbnail{}, err
	}
	defer endRead()
	select {
	case s.frameSlots <- struct{}{}:
		defer func() { <-s.frameSlots }()
	case <-readCtx.Done():
		return thumbnail{}, thumbnailReadError(parent, readCtx)
	}

	ctx, cancel := context.WithTimeout(readCtx, 15*time.Second)
	defer cancel()
	run := func(seek bool) ([]byte, error) {
		args := []string{"-nostdin", "-hide_banner", "-loglevel", "error"}
		threads := "2"
		if batch {
			threads = "1"
			args = append(args, "-filter_threads", "1")
		}
		args = append(args, "-threads", threads)
		if seek {
			args = append(args, "-ss", "3")
		}
		args = append(args, "-i", abs, "-frames:v", "1", "-vf", fmt.Sprintf("scale=%d:-2", index.CardCoverWidth), "-q:v", "5",
			"-threads", "1", "-c:v", "mjpeg", "-f", "image2pipe", "pipe:1")
		cmd := exec.CommandContext(ctx, s.tc.FFmpeg, args...)
		hideChildWindow(cmd)
		return cmd.Output()
	}
	seek := m.Duration <= 0 || m.Duration >= 3
	data, err := run(seek)
	if seek && ctx.Err() == nil && (err != nil || len(data) == 0) {
		data, err = run(false)
	}
	if cancelErr := thumbnailReadError(parent, readCtx); cancelErr != nil {
		return thumbnail{}, cancelErr
	}
	if err != nil || len(data) == 0 {
		if batch && ctx.Err() == nil {
			if _, sourceErr := os.Stat(abs); sourceErr != nil {
				return thumbnail{}, thumbnailFailure{coverSourceMissing, sourceErr}
			}
		}
		return thumbnail{}, thumbnailFailure{coverVideoFrame, thumbnailFrameError(ctx, err)}
	}
	if err := writeThumbnailCache(cache, data); batch && err != nil {
		return thumbnail{}, thumbnailFailure{coverCacheWrite, err}
	}
	return thumbnail{data: data}, nil
}

func thumbnailFrameError(ctx context.Context, err error) error {
	if timeout := ctx.Err(); timeout != nil {
		return fmt.Errorf("FFmpeg: %w", timeout)
	}
	if err == nil {
		return errors.New("FFmpeg returned no video frame")
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		detail := []rune(strings.Join(strings.Fields(string(exit.Stderr)), " "))
		if len(detail) > 360 {
			detail = append(detail[:360], '…')
		}
		if len(detail) != 0 {
			return fmt.Errorf("FFmpeg: %w: %s", err, string(detail))
		}
	}
	return fmt.Errorf("FFmpeg: %w", err)
}

func serveJPEG(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(b)
}
