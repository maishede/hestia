package server

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/gif"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"hestia/internal/index"
)

var streamMIME = map[string]string{
	".mp4": "video/mp4", ".m4v": "video/mp4", ".mkv": "video/x-matroska",
	".webm": "video/webm", ".mov": "video/quicktime", ".avi": "video/x-msvideo",
	".ts": "video/mp2t", ".m2ts": "video/mp2t", ".mts": "video/mp2t",
	".flv": "video/x-flv", ".wmv": "video/x-ms-wmv", ".mpg": "video/mpeg",
	".mpeg": "video/mpeg", ".vob": "video/mpeg", ".3gp": "video/3gpp",
	".ogv": "video/ogg", ".rm": "application/vnd.rn-realmedia",
	".rmvb": "application/vnd.rn-realmedia-vbr",
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".jfif": "image/jpeg",
	".png": "image/png", ".gif": "image/gif", ".webp": "image/webp",
	".bmp": "image/bmp", ".avif": "image/avif", ".heic": "image/heic",
}

// handleStream 直链播放（http.ServeContent 原生支持 Range/206/断点）。
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	abs, m, err := s.store.ResolveMedia(id)
	if err != nil {
		errJSON(w, http.StatusNotFound, "媒体不存在")
		return
	}
	f, err := os.Open(abs)
	if err != nil {
		errJSON(w, http.StatusNotFound, "文件无法读取（磁盘未挂载？）")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		errJSON(w, http.StatusNotFound, "文件不可用")
		return
	}
	ct := streamMIME[strings.ToLower(m.Ext)]
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	http.ServeContent(w, r, m.Name+m.Ext, st.ModTime(), f)
}

// handleImage 输出图片；带 ?w= 参数时对 jpeg/png 生成盒采样缩略图（带磁盘缓存）。
func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	abs, m, err := s.store.ResolveMedia(id)
	if err != nil {
		errJSON(w, http.StatusNotFound, "媒体不存在")
		return
	}
	if m.Kind != index.KindImage {
		errJSON(w, http.StatusBadRequest, "不是图片")
		return
	}
	wq := 0
	if v := r.URL.Query().Get("w"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			wq = n
		}
	}
	if wq < 16 {
		wq = 0
	}
	if wq > 1920 {
		wq = 1920
	}

	etag := fmt.Sprintf(`"%s-%d-%d-%d"`, id, wq, m.Size, m.Mtime.Unix())
	w.Header().Set("Etag", etag)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	ext := strings.ToLower(m.Ext)
	if wq > 0 && (ext == ".jpg" || ext == ".jpeg" || ext == ".jfif" || ext == ".png") {
		if s.serveThumb(w, id, abs, wq, m) {
			return
		}
	}
	f, err := os.Open(abs)
	if err != nil {
		errJSON(w, http.StatusNotFound, "文件无法读取")
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	ct := streamMIME[ext]
	if ct == "" {
		ct = "image/jpeg"
	}
	w.Header().Set("Content-Type", ct)
	if st != nil {
		http.ServeContent(w, r, m.Name+m.Ext, st.ModTime(), f)
		return
	}
	buf := &bytes.Buffer{}
	_, _ = buf.ReadFrom(f)
	w.Header().Set("Content-Type", ct)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) serveThumb(w http.ResponseWriter, id, abs string, targetW int, m index.Media) bool {
	cacheDir := filepath.Join(s.dataDir, "cache", "img")
	cache := filepath.Join(cacheDir, fmt.Sprintf("%s-%d-%d.jpg", id, targetW, m.Mtime.Unix()))
	if b, err := os.ReadFile(cache); err == nil {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(b)
		return true
	}
	f, err := os.Open(abs)
	if err != nil {
		return false
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return false
	}
	b := img.Bounds()
	sw := b.Dx()
	if sw <= targetW { // 原图更小，无需放大
		return false
	}
	dst := scaleBox(img, targetW)
	buf := &bytes.Buffer{}
	if err := jpeg.Encode(buf, dst, &jpeg.Options{Quality: 82}); err != nil {
		return false
	}
	_ = os.MkdirAll(cacheDir, 0o755)
	_ = os.WriteFile(cache, buf.Bytes(), 0o644)
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(buf.Bytes())
	return true
}

// scaleBox 盒平均降采样（仅用于封面缩略图，质量足够、零依赖）。
func scaleBox(src image.Image, targetW int) *image.RGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	th := sh * targetW / sw
	if th < 1 {
		th = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, targetW, th))
	for y := 0; y < th; y++ {
		sy0, sy1 := y*sh/th, (y+1)*sh/th
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for x := 0; x < targetW; x++ {
			sx0, sx1 := x*sw/targetW, (x+1)*sw/targetW
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var rr, gg, bb, nn uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					pr, pg, pb, _ := src.At(b.Min.X+sx, b.Min.Y+sy).RGBA()
					rr += uint64(pr)
					gg += uint64(pg)
					bb += uint64(pb)
					nn++
				}
			}
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(rr / nn >> 8),
				G: uint8(gg / nn >> 8),
				B: uint8(bb / nn >> 8),
				A: 255,
			})
		}
	}
	return dst
}
