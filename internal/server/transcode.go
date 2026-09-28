package server

import (
	"bytes"
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

	cmd        *exec.Cmd
	done       chan error
	lastAccess time.Time
}

type Transcoder struct {
	mu          sync.Mutex
	sessions    map[string]*TranscodeSession
	ffmpeg      string
	dataDir     string
	resolve     func(mediaID string) (string, error)
	maxSessions int

	stop     chan struct{}
	stopped  sync.WaitGroup
}

func NewTranscoder(ffmpeg, dataDir string, resolve func(string) (string, error)) *Transcoder {
	t := &Transcoder{
		sessions:    map[string]*TranscodeSession{},
		ffmpeg:      ffmpeg,
		dataDir:     dataDir,
		resolve:     resolve,
		maxSessions: 3,
		stop:        make(chan struct{}),
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
	tk := time.NewTicker(20 * time.Second)
	defer tk.Stop()
	for {
		select {
		case <-t.stop:
			return
		case <-tk.C:
			t.mu.Lock()
			for _, s := range t.sessions {
				if time.Since(s.lastAccess) > 2*time.Minute {
					t.killLocked(s, true)
				}
			}
			t.mu.Unlock()
		}
	}
}

func randSID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Start 为媒体创建转码会话，startSec 为起播偏移（秒）。
func (t *Transcoder) Start(mediaID string, startSec float64) (*TranscodeSession, error) {
	if t.ffmpeg == "" {
		return nil, errors.New("未找到 ffmpeg，无法转码（可将 ffmpeg 放到程序同目录或加入 PATH）")
	}
	if startSec < 0 {
		startSec = 0
	}

	t.mu.Lock()
	defer t.mu.Unlock()
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
		return nil, err
	}
	sid := randSID()
	dir := filepath.Join(t.dataDir, "transcode", sid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	args := []string{
		"-nostdin", "-hide_banner", "-loglevel", "error",
		"-ss", strconv.FormatFloat(startSec, 'f', 2, 64),
		"-i", abs,
		"-map", "0:v:0", "-map", "0:a:0?",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23",
		"-c:a", "aac", "-ac", "2", "-ar", "48000",
		"-sn", "-dn",
		"-f", "hls",
		"-hls_time", "4", "-hls_list_size", "0", "-start_number", "0",
		"-hls_segment_filename", filepath.Join(dir, "seg%05d.ts"),
		filepath.Join(dir, "index.m3u8"),
	}
	cmd := exec.Command(t.ffmpeg, args...)
	hideChildWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	s := &TranscodeSession{
		ID: sid, MediaID: mediaID, Dir: dir, Start: startSec,
		CreatedAt: time.Now(), lastAccess: time.Now(),
		cmd: cmd, done: make(chan error, 1),
	}
	go func() { s.done <- cmd.Wait() }()
	t.sessions[sid] = s

	// 等待首个 playlist 落盘（首个分片完成）
	playlist := filepath.Join(dir, "index.m3u8")
	deadline := time.After(30 * time.Second)
	for {
		if _, err := os.Stat(playlist); err == nil {
			return s, nil
		}
		select {
		case err := <-s.done:
			// 小文件可能在首次轮询前就全部转完并正常退出
			if _, statErr := os.Stat(playlist); statErr == nil {
				return s, nil
			}
			delete(t.sessions, sid)
			tail := stderr.String()
			if len(tail) > 400 {
				tail = tail[len(tail)-400:]
			}
			_ = os.RemoveAll(dir)
			if err != nil {
				return nil, fmt.Errorf("ffmpeg 失败: %s", tail)
			}
			return nil, fmt.Errorf("转码异常结束: %s", tail)
		case <-time.After(300 * time.Millisecond):
		case <-deadline:
			t.killLocked(s, true)
			return nil, errors.New("转码启动超时")
		}
	}
}

var segNameRe = regexp.MustCompile(`^(index\.m3u8|seg\d+\.ts)$`)

// Serve 向客户端回传会话内的 playlist / 分片。
func (t *Transcoder) Serve(sid, name string) (string, error) {
	t.mu.Lock()
	s, ok := t.sessions[sid]
	if ok {
		s.lastAccess = time.Now()
	}
	t.mu.Unlock()
	if !ok {
		return "", errors.New("转码会话不存在或已结束")
	}
	if !segNameRe.MatchString(name) {
		return "", errors.New("非法文件名")
	}
	return filepath.Join(s.Dir, name), nil
}

// Stop 结束会话并清理。
func (t *Transcoder) Stop(sid string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s, ok := t.sessions[sid]; ok {
		t.killLocked(s, true)
	}
}

func (t *Transcoder) StopAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, s := range t.sessions {
		t.killLocked(s, false)
	}
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
		_ = s.cmd.Process.Kill()
	}
	dir := s.Dir
	if async {
		go func() {
			time.Sleep(2 * time.Second)
			_ = os.RemoveAll(dir)
		}()
	} else {
		_ = os.RemoveAll(dir)
	}
}
