package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// TranscodeSession 一次 ffmpeg 按需转码会话（HLS VOD，写入临时目录）。
type TranscodeSession struct {
	ID        string    `json:"id"`
	MediaID   string    `json:"mediaId"`
	Dir       string    `json:"-"`
	Start     float64   `json:"start"`
	CreatedAt time.Time `json:"createdAt"`

	sourcePath     string
	cmd            *exec.Cmd
	done           chan struct{}
	waitErr        error
	lastAccess     time.Time
	unclaimedUntil time.Time
}

type transcodeReservation struct {
	mediaID   string
	expiresAt time.Time
}

const (
	reservationLifetime       = 2 * time.Minute
	maxReservations           = 1024
	transcodeIdleTimeout      = 2 * time.Minute
	transcodeUnclaimedTimeout = 20 * time.Second
	transcodeCleanupInterval  = 5 * time.Second
	hlsSegmentSeconds         = 4
)

var errReservationGone = errors.New("转码会话已取消或已过期")
var errTranscodeStopped = errors.New("转码会话已停止")

type Transcoder struct {
	mu           sync.Mutex
	sessions     map[string]*TranscodeSession
	stopping     map[string]*TranscodeSession
	reservations map[string]transcodeReservation
	ffmpeg       string
	dataDir      string
	resolve      func(mediaID string) (string, error)
	maxSessions  int

	stop    chan struct{}
	stopped sync.WaitGroup
}

func NewTranscoder(ffmpeg, dataDir string, resolve func(string) (string, error)) *Transcoder {
	t := &Transcoder{
		sessions:     map[string]*TranscodeSession{},
		stopping:     map[string]*TranscodeSession{},
		reservations: map[string]transcodeReservation{},
		ffmpeg:       ffmpeg,
		dataDir:      dataDir,
		resolve:      resolve,
		maxSessions:  3,
		stop:         make(chan struct{}),
	}
	_ = os.MkdirAll(filepath.Join(dataDir, "transcode"), 0o755)
	// 清掉上次异常退出遗留的孤儿目录
	if ents, err := os.ReadDir(filepath.Join(dataDir, "transcode")); err == nil {
		for _, e := range ents {
			if e.IsDir() {
				_ = os.RemoveAll(filepath.Join(dataDir, "transcode", e.Name()))
			}
		}
	}
	t.stopped.Add(1)
	go t.janitor()
	return t
}

func (t *Transcoder) Close() {
	close(t.stop)
	t.stopped.Wait()
	t.StopAll()
}

func (t *Transcoder) janitor() {
	defer t.stopped.Done()
	tk := time.NewTicker(transcodeCleanupInterval)
	defer tk.Stop()
	for {
		select {
		case <-t.stop:
			return
		case <-tk.C:
			t.reap(time.Now())
		}
	}
}

func (t *Transcoder) reap(now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.expireReservationsLocked(now)
	for _, s := range t.sessions {
		unclaimedExpired := !s.unclaimedUntil.IsZero() && !now.Before(s.unclaimedUntil)
		if unclaimedExpired || now.Sub(s.lastAccess) >= transcodeIdleTimeout {
			t.killLocked(s, true)
		}
	}
}

func randSID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (t *Transcoder) expireReservationsLocked(now time.Time) {
	for id, reservation := range t.reservations {
		if !now.Before(reservation.expiresAt) {
			delete(t.reservations, id)
		}
	}
}

// Reserve 仅分配可取消的 ID，不启动进程，也不占用转码并发槽位。
func (t *Transcoder) Reserve(mediaID string) (string, error) {
	if t.ffmpeg == "" {
		return "", errors.New("未找到 ffmpeg，无法转码（可将 ffmpeg 放到程序同目录或加入 PATH）")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	t.expireReservationsLocked(now)
	if len(t.reservations) >= maxReservations {
		return "", errors.New("待启动的转码会话过多")
	}
	sid := t.newSIDLocked()
	t.reservations[sid] = transcodeReservation{mediaID: mediaID, expiresAt: now.Add(reservationLifetime)}
	return sid, nil
}

func (t *Transcoder) newSIDLocked() string {
	for {
		id := randSID()
		if _, exists := t.sessions[id]; exists {
			continue
		}
		if _, exists := t.stopping[id]; exists {
			continue
		}
		if _, exists := t.reservations[id]; !exists {
			return id
		}
	}
}

// Start 为媒体创建会话：加锁段只做会话登记/驱逐，等待首分片在锁外进行，
// 避免一个起播阻塞其他用户长达 30 秒。
// reservationID 为空时兼容旧客户端；非空时只能启动未取消、未过期的预留会话。
func (t *Transcoder) Start(ctx context.Context, mediaID string, startSec float64, reservationID string) (*TranscodeSession, error) {
	if t.ffmpeg == "" {
		return nil, errors.New("未找到 ffmpeg，无法转码（可将 ffmpeg 放到程序同目录或加入 PATH）")
	}
	if startSec < 0 {
		startSec = 0
	}

	// ---- 加锁段：驱逐最旧会话、创建进程、登记 ----
	t.mu.Lock()
	if err := ctx.Err(); err != nil {
		t.mu.Unlock()
		return nil, err
	}
	sid := reservationID
	if sid != "" {
		reservation, ok := t.reservations[sid]
		if !ok || reservation.mediaID != mediaID || !time.Now().Before(reservation.expiresAt) {
			if ok && !time.Now().Before(reservation.expiresAt) {
				delete(t.reservations, sid)
			}
			t.mu.Unlock()
			return nil, errReservationGone
		}
		delete(t.reservations, sid)
	} else {
		sid = t.newSIDLocked()
	}
	if len(t.sessions) >= t.maxSessions {
		var oldest *TranscodeSession
		for _, s := range t.sessions {
			if oldest == nil || s.lastAccess.Before(oldest.lastAccess) {
				oldest = s
			}
		}
		if oldest != nil {
			t.killLocked(oldest, true)
		}
	}
	abs, err := t.resolve(mediaID)
	if err != nil {
		t.mu.Unlock()
		return nil, err
	}
	dir := filepath.Join(t.dataDir, "transcode", sid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.mu.Unlock()
		return nil, err
	}
	segmentSeconds := strconv.Itoa(hlsSegmentSeconds)
	args := []string{
		"-nostdin", "-hide_banner", "-loglevel", "error",
		"-ss", strconv.FormatFloat(startSec, 'f', 2, 64),
		"-i", abs,
		"-map", "0:v:0", "-map", "0:a:0?",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23",
		// 浏览器兼容的 8 位 4:2:0 要求偶数宽高，填充避免缩放原画面。
		"-vf", "pad=ceil(iw/2)*2:ceil(ih/2)*2", "-pix_fmt", "yuv420p",
		"-force_key_frames", "expr:gte(t,n_forced*" + segmentSeconds + ")",
		"-c:a", "aac", "-ac", "2", "-ar", "48000",
		"-sn", "-dn",
		"-f", "hls",
		"-hls_time", segmentSeconds, "-hls_list_size", "0", "-start_number", "0",
		"-hls_segment_filename", filepath.Join(dir, "seg%05d.ts"),
		filepath.Join(dir, "index.m3u8"),
	}
	cmd := exec.Command(t.ffmpeg, args...)
	hideChildWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(dir)
		t.mu.Unlock()
		return nil, err
	}
	s := &TranscodeSession{
		ID: sid, MediaID: mediaID, Dir: dir, Start: startSec,
		CreatedAt: time.Now(), lastAccess: time.Now(),
		sourcePath: abs, cmd: cmd, done: make(chan struct{}),
	}
	go func() {
		s.waitErr = cmd.Wait()
		close(s.done)
		t.mu.Lock()
		delete(t.stopping, s.ID)
		t.mu.Unlock()
	}()
	t.sessions[sid] = s
	t.mu.Unlock()

	// ---- 锁外段：等待首个 playlist 落盘 ----
	playlist := filepath.Join(dir, "index.m3u8")
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			t.Stop(sid)
			return nil, err
		}
		if _, err := os.Stat(playlist); err == nil {
			return t.finishStart(ctx, s, reservationID != "")
		}
		select {
		case <-s.done:
			// 小文件可能在首次轮询前就全部转完并正常退出
			if _, statErr := os.Stat(playlist); statErr == nil {
				return t.finishStart(ctx, s, reservationID != "")
			}
			t.mu.Lock()
			active := t.sessions[sid] == s
			if active {
				delete(t.sessions, sid)
			}
			t.mu.Unlock()
			if !active {
				return nil, errTranscodeStopped
			}
			tail := stderr.String()
			if len(tail) > 400 {
				tail = tail[len(tail)-400:]
			}
			_ = os.RemoveAll(dir)
			if s.waitErr != nil {
				return nil, fmt.Errorf("ffmpeg 失败: %s", tail)
			}
			return nil, fmt.Errorf("转码异常结束: %s", tail)
		case <-ctx.Done():
			t.Stop(sid)
			return nil, ctx.Err()
		case <-ticker.C:
		case <-deadline.C:
			t.Stop(sid)
			return nil, errors.New("转码启动超时")
		}
	}
}

func (t *Transcoder) finishStart(ctx context.Context, s *TranscodeSession, reserved bool) (*TranscodeSession, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sessions[s.ID] != s {
		return nil, errTranscodeStopped
	}
	if err := ctx.Err(); err != nil {
		t.killLocked(s, true)
		return nil, err
	}
	s.lastAccess = time.Now()
	// 启动响应未送达且取消请求丢失时，尽快回收未读取过媒体的会话。
	if reserved {
		s.unclaimedUntil = s.lastAccess.Add(transcodeUnclaimedTimeout)
	}
	return s, nil
}

var segNameRe = regexp.MustCompile(`^(index\.m3u8|seg\d+\.ts)$`)

// Serve 向客户端回传会话内的 playlist / 分片。
func (t *Transcoder) Serve(sid, name string) (string, error) {
	if !segNameRe.MatchString(name) {
		return "", errors.New("非法文件名")
	}
	t.mu.Lock()
	s, ok := t.sessions[sid]
	t.mu.Unlock()
	if !ok {
		return "", errors.New("转码会话不存在或已结束")
	}
	return filepath.Join(s.Dir, name), nil
}

func (t *Transcoder) markRead(sid string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s := t.sessions[sid]; s != nil {
		s.lastAccess = time.Now()
		s.unclaimedUntil = time.Time{}
	}
}

// Stop 结束会话并清理。
func (t *Transcoder) Stop(sid string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.reservations, sid)
	if s, ok := t.sessions[sid]; ok {
		t.killLocked(s, true)
	}
}

// stopMatching cancels pending starts and waits for the matching FFmpeg
// processes to release their source files.
func (t *Transcoder) stopMatching(ctx context.Context, matches func(mediaID, sourcePath string) bool) error {
	t.mu.Lock()
	for id, reservation := range t.reservations {
		if matches(reservation.mediaID, "") {
			delete(t.reservations, id)
		}
	}
	var stopped []*TranscodeSession
	for _, session := range t.stopping {
		if matches(session.MediaID, session.sourcePath) {
			stopped = append(stopped, session)
		}
	}
	for _, session := range t.sessions {
		if matches(session.MediaID, session.sourcePath) {
			t.killLocked(session, true)
			stopped = append(stopped, session)
		}
	}
	t.mu.Unlock()
	for _, session := range stopped {
		select {
		case <-session.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (t *Transcoder) StopAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, s := range t.sessions {
		t.killLocked(s, false)
	}
	clear(t.reservations)
}

func (t *Transcoder) Active() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.sessions)
}

// killLocked 结束进程并延迟清理目录（持锁调用）。
func (t *Transcoder) killLocked(s *TranscodeSession, async bool) {
	delete(t.sessions, s.ID)
	if s.cmd != nil && s.cmd.Process != nil {
		select {
		case <-s.done:
		default:
			t.stopping[s.ID] = s
			_ = s.cmd.Process.Kill()
		}
	}
	dir := s.Dir
	if async {
		go func() {
			if s.done != nil {
				<-s.done
			}
			time.Sleep(2 * time.Second)
			_ = os.RemoveAll(dir)
		}()
	} else {
		_ = os.RemoveAll(dir)
	}
}
