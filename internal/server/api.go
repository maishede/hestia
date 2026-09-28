package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"

	"hestia/internal/config"
	"hestia/internal/index"
	"hestia/internal/progress"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
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

// ensureProbed 返回带探测信息的媒体（必要时同步 ffprobe），失败时已写响应并返回零值。
func (s *Server) ensureProbed(w http.ResponseWriter, r *http.Request, id string) (index.Media, string) {
	abs, m, err := s.store.ResolveMedia(id)
	if err != nil {
		errJSON(w, http.StatusNotFound, "媒体不存在")
		return index.Media{}, ""
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

func (s *Server) handleTranscodeStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Start float64 `json:"start"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	if _, _, err := s.store.ResolveMedia(id); err != nil {
		errJSON(w, http.StatusNotFound, "媒体不存在")
		return
	}
	sess, err := s.tr.Start(id, body.Start)
	if err != nil {
		errJSON(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sessionId": sess.ID,
		"url":       "/api/transcode/" + sess.ID + "/index.m3u8",
		"start":     sess.Start,
	})
}

func (s *Server) handleTranscodeFile(w http.ResponseWriter, r *http.Request) {
	p, err := s.tr.Serve(r.PathValue("sid"), r.PathValue("file"))
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
	st, _ := f.Stat()
	name := r.PathValue("file")
	ct := "video/mp2t"
	if len(name) >= 5 && name[len(name)-5:] == ".m3u8" {
		ct = "application/vnd.apple.mpegurl"
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.Header().Set("Content-Type", ct)
	http.ServeContent(w, r, name, st.ModTime(), f)
}

func (s *Server) handleTranscodeStop(w http.ResponseWriter, r *http.Request) {
	s.tr.Stop(r.PathValue("sid"))
	w.WriteHeader(http.StatusNoContent)
}

// ---------- 管理端（局域网自用，免登录） ----------

func (s *Server) handleAdminLibraries(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": s.store.Libraries()})
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return equalsFoldASCII(a, b)
	}
	return a == b
}

func equalsFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func (s *Server) handleAdminLibAdd(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path  string `json:"path"`
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
		errJSON(w, http.StatusBadRequest, "请填写路径")
		return
	}
	abs := config.AbsPath(body.Path)
	fi, err := os.Stat(abs)
	if err != nil || !fi.IsDir() {
		errJSON(w, http.StatusBadRequest, "路径不存在或不是文件夹: "+abs)
		return
	}
	dup := false
	uerr := s.cfg.Update(func(c *config.Config) {
		for _, l := range c.Libraries {
			if samePath(config.AbsPath(l.Path), abs) {
				dup = true
				return
			}
		}
		label := body.Label
		if label == "" {
			label = fi.Name()
		}
		c.Libraries = append(c.Libraries, config.Library{Path: abs, Label: label, Enabled: true})
	})
	if uerr != nil {
		errJSON(w, http.StatusInternalServerError, uerr.Error())
		return
	}
	if dup {
		errJSON(w, http.StatusConflict, "该路径已添加过")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": s.store.LibID(abs), "path": abs})
}

func (s *Server) handleAdminLibPatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Label   *string `json:"label"`
		Enabled *bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errJSON(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	found := false
	uerr := s.cfg.Update(func(c *config.Config) {
		for i := range c.Libraries {
			if s.store.LibID(config.AbsPath(c.Libraries[i].Path)) == id {
				found = true
				if body.Label != nil && *body.Label != "" {
					c.Libraries[i].Label = *body.Label
				}
				if body.Enabled != nil {
					c.Libraries[i].Enabled = *body.Enabled
				}
			}
		}
	})
	if uerr != nil {
		errJSON(w, http.StatusInternalServerError, uerr.Error())
		return
	}
	if !found {
		errJSON(w, http.StatusNotFound, "媒体库不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAdminLibDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	uerr := s.cfg.Update(func(c *config.Config) {
		out := c.Libraries[:0]
		for _, l := range c.Libraries {
			if s.store.LibID(config.AbsPath(l.Path)) != id {
				out = append(out, l)
			}
		}
		c.Libraries = out
	})
	if uerr != nil {
		errJSON(w, http.StatusInternalServerError, uerr.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAdminRescan(w http.ResponseWriter, r *http.Request) {
	go s.store.RescanAll()
	writeJSON(w, http.StatusAccepted, map[string]any{"started": true})
}

func (s *Server) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	folders, media := s.store.Counts()
	writeJSON(w, http.StatusOK, map[string]any{
		"scanning":   s.store.AnyScanning(),
		"libraries":  s.store.Libraries(),
		"transcodes": s.tr.Active(),
		"totals":     map[string]int{"folders": folders, "media": media},
	})
}

func (s *Server) handleAdminConfigGet(w http.ResponseWriter, r *http.Request) {
	c := s.cfg.Get()
	writeJSON(w, http.StatusOK, map[string]any{
		"port":        c.Port,
		"listen":      c.Listen,
		"openBrowser": c.OpenBrowser,
		"autoStart":   c.AutoStart,
		"tray":        c.UseTray(),
		"ffmpeg":      s.tc.FFmpegOK(),
		"ffprobe":     s.tc.FFprobeOK(),
		"ffmpegPath":  s.tc.FFmpeg,
		"urls":        s.URLs(),
	})
}

func (s *Server) handleAdminConfigPatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Port        *int    `json:"port"`
		Listen      *string `json:"listen"`
		OpenBrowser *bool   `json:"openBrowser"`
		AutoStart   *bool   `json:"autoStart"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errJSON(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if body.Port != nil && (*body.Port < 1 || *body.Port > 65535) {
		errJSON(w, http.StatusBadRequest, "端口取值 1-65535")
		return
	}
	if body.Listen != nil && *body.Listen != "0.0.0.0" && *body.Listen != "127.0.0.1" {
		errJSON(w, http.StatusBadRequest, "监听地址仅支持 0.0.0.0（局域网）或 127.0.0.1（仅本机）")
		return
	}
	uerr := s.cfg.Update(func(c *config.Config) {
		if body.Port != nil {
			c.Port = *body.Port
		}
		if body.Listen != nil {
			c.Listen = *body.Listen
		}
		if body.OpenBrowser != nil {
			c.OpenBrowser = *body.OpenBrowser
		}
		if body.AutoStart != nil {
			c.AutoStart = *body.AutoStart
		}
	})
	if uerr != nil {
		errJSON(w, http.StatusInternalServerError, uerr.Error())
		return
	}
	// 配置变更已通过 onChange 同步应用（端口热切换亦在其中完成）
	c := s.cfg.Get()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "port": c.Port, "listen": c.Listen, "urls": s.URLs(),
		"note": fmt.Sprintf("已生效：http://<本机IP>:%d", c.Port),
	})
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
