package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"hestia/internal/index"
	"hestia/internal/progress"
)

func writeJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

func errJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func qInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func qStr(r *http.Request, key string) string { return r.URL.Query().Get(key) }

// ---------- 用户端 ----------

func (s *Server) handleServerInfo(w http.ResponseWriter, r *http.Request) {
	folders, media := s.store.Counts()
	writeJSON(w, http.StatusOK, map[string]any{
		"name":         "Hestia",
		"version":      Version,
		"urls":         s.URLs(),
		"ffmpeg":       s.tc.FFmpegOK(),
		"ffmpegPath":   s.tc.FFmpeg,
		"ffprobe":      s.tc.FFprobeOK(),
		"totalFolders": folders,
		"totalMedia":   media,
	})
}

func (s *Server) handleRoots(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"folders": s.store.Roots(qStr(r, "sort"), qStr(r, "order")),
	})
}

func (s *Server) handleChildren(w http.ResponseWriter, r *http.Request) {
	res, err := s.store.Children(r.PathValue("id"), qInt(r, "page", 1), qInt(r, "size", 20), qStr(r, "sort"), qStr(r, "order"))
	if err != nil {
		errJSON(w, http.StatusNotFound, "文件夹不存在")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	res := s.store.Search(qStr(r, "q"), qInt(r, "page", 1), qInt(r, "size", 20), qStr(r, "sort"), qStr(r, "order"))
	writeJSON(w, http.StatusOK, res)
}

// ensureProbed 返回带探测信息的媒体（必要时同步 ffprobe，仅视频），失败时已写响应并返回零值。
func (s *Server) ensureProbed(w http.ResponseWriter, r *http.Request, id string) (index.Media, string) {
	abs, m, err := s.store.ResolveMedia(id)
	if err != nil {
		errJSON(w, http.StatusNotFound, "媒体不存在")
		return index.Media{}, ""
	}
	// 图片会被 ffprobe 当成单帧视频（时长 0.024s 之类），导致前端误显示 0:00，跳过
	if m.Kind != index.KindVideo {
		return m, abs
	}
	if !m.Probed && s.tc.FFprobeOK() {
		lib, _ := s.store.GetLibrary(m.LibraryID)
		if res, err := RunFFprobe(s.tc.FFprobe, abs); err == nil {
			s.store.SetProbeResult(lib.Path, &m, &index.ProbeInfo{
				Duration: res.Duration, VCodec: res.VCodec, ACodec: res.ACodec,
				Width: res.Width, Height: res.Height, Subs: res.Subs,
			})
		}
	}
	return m, abs
}

func (s *Server) handleMediaDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m, _ := s.ensureProbed(w, r, id)
	if m.ID == "" {
		return
	}
	need, reason := DecidePlayback(m, r.UserAgent())
	mode := "direct"
	if need {
		mode = "transcode"
	}
	prev, next := s.store.Siblings(id)
	folderName := ""
	if f, ok := s.store.GetFolder(m.FolderID); ok {
		folderName = f.Name
	}
	// 字幕：外挂（同名 srt/ass）+ 内嵌（文本轨）
	extSubs := []map[string]string{}
	for _, sub := range s.store.SubtitlesOfMedia(&m) {
		extSubs = append(extSubs, map[string]string{"id": sub.ID, "name": sub.Name + sub.Ext})
	}
	embSubs := []map[string]any{}
	for _, st := range m.EmbeddedSubs {
		if textSubCodecs[st.Codec] {
			embSubs = append(embSubs, map[string]any{"index": st.Index, "codec": st.Codec, "title": st.Title, "lang": st.Lang})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": m.ID, "kind": m.Kind.String(), "name": m.Name, "size": m.Size,
		"mtime": m.Mtime, "duration": m.Duration,
		"vcodec": m.VCodec, "acodec": m.ACodec, "width": m.Width, "height": m.Height,
		"folderName": folderName,
		"stream":     "/api/media/" + m.ID + "/stream",
		"playback": map[string]any{
			"mode": mode, "reason": reason,
			"transcodeAvailable": s.tc.FFmpegOK(),
		},
		"subtitles": map[string]any{"external": extSubs, "embedded": embSubs},
		"prev":      prev, "next": next,
	})
}

func (s *Server) handleTranscodeReserve(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, _, err := s.store.ResolveMedia(id); err != nil {
		errJSON(w, http.StatusNotFound, "媒体不存在")
		return
	}
	sid, err := s.tr.Reserve(id)
	if err != nil {
		errJSON(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if r.Context().Err() != nil {
		s.tr.Stop(sid)
		return
	}
	if err := writeJSON(w, http.StatusOK, map[string]string{"sessionId": sid}); err != nil {
		s.tr.Stop(sid)
	}
}

func (s *Server) handleTranscodeStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Start     float64 `json:"start"`
		SessionID string  `json:"sessionId"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			errJSON(w, http.StatusBadRequest, "请求格式错误")
			return
		}
	}
	if _, _, err := s.store.ResolveMedia(id); err != nil {
		errJSON(w, http.StatusNotFound, "媒体不存在")
		return
	}
	sess, err := s.tr.Start(r.Context(), id, body.Start, body.SessionID)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		if errors.Is(err, errReservationGone) || errors.Is(err, errTranscodeStopped) {
			errJSON(w, http.StatusGone, err.Error())
			return
		}
		errJSON(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if r.Context().Err() != nil {
		s.tr.Stop(sess.ID)
		return
	}
	if err := writeJSON(w, http.StatusOK, map[string]any{
		"sessionId": sess.ID,
		"url":       "/api/transcode/" + sess.ID + "/index.m3u8",
		"start":     sess.Start,
	}); err != nil {
		s.tr.Stop(sess.ID)
	}
}

func (s *Server) handleTranscodeFile(w http.ResponseWriter, r *http.Request) {
	sid, name := r.PathValue("sid"), r.PathValue("file")
	p, err := s.tr.Serve(sid, name)
	if err != nil {
		errJSON(w, http.StatusNotFound, "转码会话不存在或已结束")
		return
	}
	f, err := os.Open(p)
	if err != nil {
		errJSON(w, http.StatusNotFound, "分片不存在")
		return
	}
	defer f.Close()
	if name == "index.m3u8" {
		manifest, err := io.ReadAll(f)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "无法读取转码清单")
			return
		}
		// 清单尚未结束时，客户端也应从本次转码的起点播放。
		if first, rest, ok := bytes.Cut(manifest, []byte("\n")); ok && string(bytes.TrimSuffix(first, []byte("\r"))) == "#EXTM3U" {
			manifest = append([]byte("#EXTM3U\n#EXT-X-START:TIME-OFFSET=0,PRECISE=YES\n"), rest...)
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
		s.serveTranscodeContent(w, r, sid, name, time.Time{}, bytes.NewReader(manifest))
		return
	}
	st, err := f.Stat()
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "无法读取转码分片")
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	s.serveTranscodeContent(w, r, sid, name, st.ModTime(), f)
}

func (s *Server) serveTranscodeContent(w http.ResponseWriter, r *http.Request, sid, name string, modified time.Time, content io.ReadSeeker) {
	if r.Method != http.MethodGet {
		http.ServeContent(w, r, name, modified, content)
		return
	}
	response := &transcodeReadResponse{ResponseWriter: w}
	http.ServeContent(response, r, name, modified, content)
	if (response.status == http.StatusOK || response.status == http.StatusPartialContent) &&
		response.written > 0 && !response.failed && r.Context().Err() == nil {
		s.tr.markRead(sid)
	}
}

type transcodeReadResponse struct {
	http.ResponseWriter
	status  int
	written int64
	failed  bool
}

func (w *transcodeReadResponse) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *transcodeReadResponse) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	w.written += int64(n)
	w.failed = w.failed || err != nil || n != len(data)
	return n, err
}

// Preserve the underlying HTTP writer's fast path for file responses.
func (w *transcodeReadResponse) ReadFrom(r io.Reader) (int64, error) {
	n, err := io.Copy(w.ResponseWriter, r)
	w.written += n
	w.failed = w.failed || err != nil
	return n, err
}

func (s *Server) handleTranscodeStop(w http.ResponseWriter, r *http.Request) {
	s.tr.Stop(r.PathValue("sid"))
	w.WriteHeader(http.StatusNoContent)
}

// ---------- 播放进度（多设备同步，最后写入优先） ----------

func (s *Server) handleProgressSet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MediaID  string  `json:"mediaId"`
		Position float64 `json:"position"`
		Duration float64 `json:"duration"`
		Device   string  `json:"device"`
		Name     string  `json:"name"`
		Folder   string  `json:"folder"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.MediaID == "" {
		errJSON(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	s.prog.Set(progress.Entry{
		MediaID: body.MediaID, Position: body.Position, Duration: body.Duration,
		Device: body.Device, Name: body.Name, Folder: body.Folder,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleProgressGet(w http.ResponseWriter, r *http.Request) {
	e, ok := s.prog.Get(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"position": 0, "updatedAt": 0})
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleProgressRecent(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": s.prog.Recent(qInt(r, "n", 12))})
}

func (s *Server) handleProgressDelete(w http.ResponseWriter, r *http.Request) {
	s.prog.Delete(r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleProgressClear(w http.ResponseWriter, r *http.Request) {
	s.prog.Clear()
	w.WriteHeader(http.StatusNoContent)
}
