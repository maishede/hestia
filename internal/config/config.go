// Package config 负责 config.json 的加载、原子写入与热重载（轮询 mtime）。
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type Library struct {
	Path    string `json:"path"`
	Label   string `json:"label"`
	Enabled bool   `json:"enabled"`
}

type Config struct {
	Port        int       `json:"port"`
	Listen      string    `json:"listen"`
	OpenBrowser bool      `json:"openBrowser"`
	Tray        *bool     `json:"tray"` // 托盘常驻（nil = Windows 默认开，其他平台默认关）
	AutoStart   bool      `json:"autoStart"`
	Libraries   []Library `json:"libraries"`
}

// UseTray 解析托盘默认值。
func (c Config) UseTray() bool {
	if c.Tray != nil {
		return *c.Tray
	}
	return runtime.GOOS == "windows"
}

func Default() Config {
	return Config{Port: 8080, Listen: "0.0.0.0", OpenBrowser: true}
}

// Manager 维护当前配置；外部修改（API）与手工修改（编辑文件）统一走 onChange 回调生效。
type Manager struct {
	mu       sync.Mutex
	path     string
	cfg      Config
	lastRaw  []byte
	lastMod  time.Time
	lastSize int64
	onChange []func(Config)
	stop     chan struct{}
	stopped  sync.WaitGroup
}

func Load(path string) (*Manager, error) {
	m := &Manager{path: path, stop: make(chan struct{})}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			m.cfg = Default()
			if err := m.persistLocked(); err != nil {
				return nil, err
			}
			m.startWatch()
			return m, nil
		}
		return nil, err
	}
	m.lastRaw = raw
	if err := json.Unmarshal(raw, &m.cfg); err != nil {
		return nil, errors.New("配置文件解析失败: " + err.Error())
	}
	if m.cfg.Listen == "" {
		m.cfg.Listen = "0.0.0.0"
	}
	if m.cfg.Port == 0 {
		m.cfg.Port = 8080
	}
	if fi, err := os.Stat(path); err == nil {
		m.lastMod, m.lastSize = fi.ModTime(), fi.Size()
	}
	m.startWatch()
	return m, nil
}

func (m *Manager) Close() {
	close(m.stop)
	m.stopped.Wait()
}

func (m *Manager) OnChange(fn func(Config)) {
	m.mu.Lock()
	m.onChange = append(m.onChange, fn)
	m.mu.Unlock()
}

// Get 返回配置副本。
func (m *Manager) Get() Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.cfg
	out := make([]Library, len(c.Libraries))
	copy(out, c.Libraries)
	c.Libraries = out
	return c
}

// Update 以回调方式修改并持久化配置，然后触发 onChange。
func (m *Manager) Update(mutate func(*Config)) error {
	m.mu.Lock()
	m.cfg = applyDefaults(m.cfg)
	mutate(&m.cfg)
	if err := m.persistLocked(); err != nil {
		return err
	}
	cfg := m.snapshotLocked()
	fns := make([]func(Config), len(m.onChange))
	copy(fns, m.onChange)
	m.mu.Unlock()
	fire(fns, cfg)
	return nil
}

func (m *Manager) persistLocked() error {
	raw, err := json.MarshalIndent(m.cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, m.path); err != nil {
		return err
	}
	m.lastRaw = raw
	if fi, err := os.Stat(m.path); err == nil {
		m.lastMod, m.lastSize = fi.ModTime(), fi.Size()
	}
	return nil
}

func (m *Manager) snapshotLocked() Config {
	c := m.cfg
	out := make([]Library, len(c.Libraries))
	copy(out, c.Libraries)
	c.Libraries = out
	return c
}

func (m *Manager) startWatch() {
	m.stopped.Add(1)
	go func() {
		defer m.stopped.Done()
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-m.stop:
				return
			case <-t.C:
				m.checkFile()
			}
		}
	}()
}

func (m *Manager) checkFile() {
	fi, err := os.Stat(m.path)
	if err != nil {
		return
	}
	m.mu.Lock()
	if fi.ModTime().Equal(m.lastMod) && fi.Size() == m.lastSize {
		m.mu.Unlock()
		return
	}
	raw, err := os.ReadFile(m.path)
	if err != nil {
		m.mu.Unlock()
		return
	}
	if string(raw) == string(m.lastRaw) {
		m.lastMod, m.lastSize = fi.ModTime(), fi.Size()
		m.mu.Unlock()
		return
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		m.lastMod, m.lastSize = fi.ModTime(), fi.Size() // 跳过坏文件，下轮再看
		m.mu.Unlock()
		return
	}
	m.cfg = applyDefaults(cfg)
	m.lastRaw, m.lastMod, m.lastSize = raw, fi.ModTime(), fi.Size()
	snap := m.snapshotLocked()
	fns := make([]func(Config), len(m.onChange))
	copy(fns, m.onChange)
	m.mu.Unlock()
	fire(fns, snap)
}

func fire(fns []func(Config), cfg Config) {
	for _, fn := range fns {
		fn(cfg)
	}
}

func applyDefaults(c Config) Config {
	if c.Listen == "" {
		c.Listen = "0.0.0.0"
	}
	if c.Port == 0 {
		c.Port = 8080
	}
	return c
}

// AbsPath 展开用户输入的库路径（支持 ~ 与相对路径）。
func AbsPath(p string) string {
	if p == "" {
		return ""
	}
	if p[0] == '~' {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, p[1:])
		}
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return abs
}
