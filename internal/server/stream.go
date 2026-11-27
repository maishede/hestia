package server

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

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
	".jpg":  "image/jpeg", ".jpeg": "image/jpeg", ".jfif": "image/jpeg",
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
	ctx, endRead, ok := s.beginMediaRead(w, r, m.FolderID)
	if !ok {
		return
	}
	defer endRead()
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
	serveMediaContent(ctx, w, r, m.Name+m.Ext, st.ModTime(), f)
}

// Close the file on cancellation, and unblock a response stalled on a client
// before the deletion handler waits for this read to finish.
func serveMediaContent(ctx context.Context, w http.ResponseWriter, r *http.Request, name string, mod time.Time, f *os.File) {
	finished := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(finished)
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now())
		_ = f.Close()
	})
	defer func() {
		if !stop() {
			<-finished
		}
	}()
	http.ServeContent(w, r, name, mod, f)
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

	etag := fmt.Sprintf(`"%s-%d-%d-%d"`, id, wq, m.Size, m.Mtime.UnixNano())
	w.Header().Set("Etag", etag)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	ext := strings.ToLower(m.Ext)
	if wq > 0 && canResizeCoverImage(ext) {
		result, err := s.imageThumbnail(r.Context(), abs, wq, m, false)
		if err == errFolderDeleting {
			errJSON(w, http.StatusConflict, err.Error())
			return
		}
		if r.Context().Err() != nil {
			return
		}
		if len(result.data) > 0 {
			serveJPEG(w, result.data)
			return
		}
	}
	ctx, endRead, ok := s.beginMediaRead(w, r, m.FolderID)
	if !ok {
		return
	}
	defer endRead()
	f, err := os.Open(abs)
	if err != nil {
		errJSON(w, http.StatusNotFound, "文件无法读取")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		errJSON(w, http.StatusNotFound, "文件不可用")
		return
	}
	ct := streamMIME[ext]
	if ct == "" {
		ct = "image/jpeg"
	}
	w.Header().Set("Content-Type", ct)
	serveMediaContent(ctx, w, r, m.Name+m.Ext, st.ModTime(), f)
}

func canResizeCoverImage(ext string) bool {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".jfif", ".png":
		return true
	default:
		return false
	}
}

type thumbnail struct {
	data   []byte
	cached bool
}

func (s *Server) imageThumbnail(parent context.Context, abs string, targetW int, m index.Media, batch bool) (thumbnail, error) {
	cacheDir := filepath.Join(s.dataDir, "cache", "img")
	cache := filepath.Join(cacheDir, fmt.Sprintf("%s-%d-%d-%d.jpg", m.ID, targetW, m.Size, m.Mtime.UnixNano()))
	// Preserve already generated previews from earlier builds.
	legacy := filepath.Join(cacheDir, fmt.Sprintf("%s-%d-%d.jpg", m.ID, targetW, m.Mtime.Unix()))
	for _, path := range []string{cache, legacy} {
		if result := cachedThumbnail(path, batch); result.cached {
			return result, nil
		}
	}
	unlock, ok := s.imageJobs.lock(parent, cache)
	if !ok {
		return thumbnail{}, parent.Err()
	}
	defer unlock()
	if result := cachedThumbnail(cache, batch); result.cached {
		return result, nil
	}
	select {
	case s.imageSlots <- struct{}{}:
		defer func() { <-s.imageSlots }()
	case <-parent.Done():
		return thumbnail{}, parent.Err()
	}
	ctx, endRead, err := s.startMediaRead(parent, m.FolderID)
	if err != nil {
		return thumbnail{}, err
	}
	img, err := decodeThumbnailSource(ctx, abs, targetW, batch)
	readErr := thumbnailReadError(parent, ctx)
	// Decoding has closed the original file, so deletion need not wait for encoding.
	endRead()
	if readErr != nil {
		return thumbnail{}, readErr
	}
	if err != nil || img == nil {
		return thumbnail{}, err
	}
	dst := scaleBox(parent, img, targetW)
	if dst == nil {
		return thumbnail{}, parent.Err()
	}
	buf := &bytes.Buffer{}
	if err := jpeg.Encode(buf, dst, &jpeg.Options{Quality: 82}); err != nil {
		return thumbnail{}, err
	}
	if err := parent.Err(); err != nil {
		return thumbnail{}, err
	}
	if err := writeThumbnailCache(cache, buf.Bytes()); batch && err != nil {
		return thumbnail{}, thumbnailFailure{coverCacheWrite, err}
	}
	return thumbnail{data: buf.Bytes()}, nil
}

func thumbnailReadError(parent, readCtx context.Context) error {
	if err := parent.Err(); err != nil {
		return err
	}
	if readCtx.Err() != nil {
		return errFolderDeleting
	}
	return nil
}

func cachedThumbnail(path string, batch bool) thumbnail {
	if batch {
		st, err := os.Stat(path)
		return thumbnail{cached: err == nil && st.Mode().IsRegular() && st.Size() > 0}
	}
	b, err := os.ReadFile(path)
	return thumbnail{data: b, cached: err == nil && len(b) > 0}
}

type thumbnailReader struct {
	ctx context.Context
	io.Reader
}

func (r thumbnailReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}

func decodeThumbnailSource(ctx context.Context, abs string, targetW int, batch bool) (image.Image, error) {
	f, err := os.Open(abs)
	if err != nil {
		return nil, thumbnailFailure{coverSourceMissing, err}
	}
	defer f.Close()
	src := thumbnailReader{ctx: ctx, Reader: f}
	config, _, err := image.DecodeConfig(src)
	if err != nil {
		return nil, thumbnailFailure{coverImageRead, err}
	}
	if config.Width <= targetW {
		return nil, nil
	}
	// Keep batch memory bounded. Larger originals can still be viewed on demand.
	const maxBatchPixels = 16 * 1024 * 1024
	if batch && (config.Height <= 0 || config.Width > maxBatchPixels/config.Height) {
		return nil, nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, thumbnailFailure{coverImageRead, err}
	}
	img, _, err := image.Decode(src)
	if err != nil {
		return nil, thumbnailFailure{coverImageRead, err}
	}
	return img, nil
}

func writeThumbnailCache(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "thumbnail-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp.Name(), path)
}

type imageJob struct {
	busy  chan struct{}
	users int
}

type imageJobs struct {
	mu   sync.Mutex
	jobs map[string]*imageJob
}

func (j *imageJobs) lock(ctx context.Context, key string) (func(), bool) {
	j.mu.Lock()
	if j.jobs == nil {
		j.jobs = make(map[string]*imageJob)
	}
	job := j.jobs[key]
	if job == nil {
		job = &imageJob{busy: make(chan struct{}, 1)}
		j.jobs[key] = job
	}
	job.users++
	j.mu.Unlock()
	unregister := func() {
		j.mu.Lock()
		defer j.mu.Unlock()
		job.users--
		if job.users == 0 {
			delete(j.jobs, key)
		}
	}
	select {
	case job.busy <- struct{}{}:
		return func() {
			<-job.busy
			unregister()
		}, true
	case <-ctx.Done():
		unregister()
		return nil, false
	}
}

// scaleBox 盒平均降采样（仅用于封面缩略图，质量足够、零依赖）。
func scaleBox(ctx context.Context, src image.Image, targetW int) *image.RGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	th := sh * targetW / sw
	if th < 1 {
		th = 1
	}
	if th > 1920 {
		targetW = max(1, targetW*1920/th)
		th = 1920
	}
	dst := image.NewRGBA(image.Rect(0, 0, targetW, th))
	for y := 0; y < th; y++ {
		if ctx.Err() != nil {
			return nil
		}
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
			// Limit samples per output pixel so very large posters don't
			// perform a separate color conversion for every source pixel.
			stepX, stepY := (sx1-sx0+2)/3, (sy1-sy0+2)/3
			for sy := sy0; sy < sy1; sy += stepY {
				for sx := sx0; sx < sx1; sx += stepX {
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
