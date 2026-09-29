//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"hestia/internal/server"
	"hestia/internal/update"
)

type updateState struct {
	Current  string `json:"current"`
	Latest   string `json:"latest"`
	URL      string `json:"url"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	Error    string `json:"error"`
}

type desktopUpdater struct {
	mu      sync.Mutex
	client  *update.Client
	release update.Release
	state   updateState
}

var guiUpdater = &desktopUpdater{
	client: update.NewClient(),
	state:  updateState{Current: server.Version, Status: "idle"},
}

func (u *desktopUpdater) snapshot() updateState {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state
}

func (u *desktopUpdater) check(ctx context.Context) (updateState, error) {
	u.mu.Lock()
	if u.state.Status == "checking" || u.state.Status == "downloading" || u.state.Status == "restarting" {
		state := u.state
		u.mu.Unlock()
		return state, nil
	}
	u.state.Status, u.state.Error = "checking", ""
	u.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	release, err := u.client.Check(ctx, server.Version)
	u.mu.Lock()
	defer u.mu.Unlock()
	if err != nil {
		u.state.Status, u.state.Error = "error", err.Error()
		return u.state, err
	}
	u.release = release
	u.state.Latest, u.state.URL = release.Version, release.URL
	u.state.Progress, u.state.Error = 0, ""
	if release.Available {
		u.state.Status = "available"
	} else {
		u.state.Status = "current"
	}
	return u.state, nil
}

func (u *desktopUpdater) start(quit func()) error {
	u.mu.Lock()
	if u.state.Status != "available" {
		u.mu.Unlock()
		return errors.New("没有可安装的新版本")
	}
	release := u.release
	u.state.Status, u.state.Progress, u.state.Error = "downloading", 0, ""
	u.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		exe, err := os.Executable()
		if err != nil {
			u.fail(err)
			return
		}
		stage := filepath.Join(updateDir(exe), "Hestia-v"+release.Version+".exe")
		err = u.client.Download(ctx, release, stage, func(done, total int64) {
			u.mu.Lock()
			if total > 0 {
				u.state.Progress = int(done * 100 / total)
			} else {
				u.state.Progress = -1
			}
			u.mu.Unlock()
		})
		if err != nil {
			u.fail(err)
			return
		}
		// Run a copy of the current binary as the helper. The new release need not
		// know the helper protocol before its first normal launch.
		helper := filepath.Join(updateDir(exe), "Hestia-updater.exe")
		if err := copyFile(exe, helper); err != nil {
			u.fail(fmt.Errorf("准备更新程序失败: %w", err))
			return
		}
		cmd := exec.Command(helper, "--apply-update", strconv.Itoa(os.Getpid()), exe, stage)
		cmd.Dir = filepath.Dir(exe)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if err := cmd.Start(); err != nil {
			u.fail(fmt.Errorf("无法启动更新程序: %w", err))
			return
		}
		_ = cmd.Process.Release()
		u.mu.Lock()
		u.state.Status = "restarting"
		u.mu.Unlock()
		// Give the HTTP response time to reach the WebView before closing the GUI.
		time.Sleep(300 * time.Millisecond)
		quit()
	}()
	return nil
}

func (u *desktopUpdater) fail(err error) {
	u.mu.Lock()
	u.state.Status, u.state.Error = "error", err.Error()
	u.mu.Unlock()
}

func updateDir(exe string) string {
	return filepath.Join(filepath.Dir(exe), "data", "updates")
}

// The staged *new* binary runs this mode, waits for the old process to exit,
// swaps the executable and starts it at its original path.
func runUpdateHelperIfRequested() bool {
	if len(os.Args) < 2 || os.Args[1] != "--apply-update" {
		return false
	}
	if len(os.Args) != 5 {
		os.Exit(2)
	}
	pid, err := strconv.Atoi(os.Args[2])
	if err == nil && pid <= 0 {
		err = errors.New("无效的旧进程 ID")
	}
	if err == nil {
		err = applyUpdate(pid, os.Args[3], os.Args[4])
	}
	if err != nil {
		target := os.Args[3]
		_ = os.MkdirAll(updateDir(target), 0o755)
		_ = os.WriteFile(filepath.Join(updateDir(target), "last-error.txt"), []byte(err.Error()), 0o644)
		// On failure, reopen whichever version still occupies the original path.
		if _, statErr := os.Stat(target); statErr == nil {
			_ = launchUpdatedApp(target)
		}
		os.Exit(1)
	}
	os.Exit(0)
	return true
}

func applyUpdate(parentPID int, target, stage string) error {
	helper, err := os.Executable()
	if err != nil {
		return err
	}
	helper, err = filepath.Abs(helper)
	if err != nil {
		return err
	}
	stage, err = filepath.Abs(stage)
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	if !strings.EqualFold(helper, filepath.Join(updateDir(target), "Hestia-updater.exe")) ||
		!strings.EqualFold(filepath.Dir(stage), updateDir(target)) ||
		!strings.HasPrefix(strings.ToLower(filepath.Base(stage)), "hestia-v") ||
		!strings.EqualFold(filepath.Ext(stage), ".exe") ||
		strings.EqualFold(stage, target) {
		return errors.New("更新程序路径无效")
	}
	if err := waitForProcess(parentPID); err != nil {
		return err
	}
	backup := filepath.Join(updateDir(target), "Hestia.previous.exe")
	_ = os.Remove(filepath.Join(updateDir(target), "last-error.txt"))
	if err := replaceExecutable(stage, target, backup, launchUpdatedApp); err != nil {
		return err
	}
	return nil
}

func replaceExecutable(stage, target, backup string, launch func(string) error) error {
	pending := target + ".new"
	defer os.Remove(pending)
	if err := copyFile(stage, pending); err != nil {
		return fmt.Errorf("复制新版本失败: %w", err)
	}
	if err := os.Remove(backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("清理旧备份失败: %w", err)
	}
	// A file lock may persist briefly after the process exit notification.
	var moveErr error
	for i := 0; i < 20; i++ {
		moveErr = os.Rename(target, backup)
		if moveErr == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if moveErr != nil {
		return fmt.Errorf("备份旧版本失败: %w", moveErr)
	}
	if err := os.Rename(pending, target); err != nil {
		return rollbackUpdate(backup, target, fmt.Errorf("替换程序失败: %w", err))
	}
	if err := launch(target); err != nil {
		_ = os.Remove(target)
		return rollbackUpdate(backup, target, fmt.Errorf("启动新版本失败: %w", err))
	}
	return nil
}

func rollbackUpdate(backup, target string, cause error) error {
	if err := os.Rename(backup, target); err != nil {
		if copyErr := copyFile(backup, target); copyErr != nil {
			return fmt.Errorf("%w；恢复旧版本也失败：%v", cause, copyErr)
		}
	}
	return cause
}

func waitForProcess(pid int) error {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return nil // The parent has already exited.
	}
	if err != nil {
		return fmt.Errorf("等待旧版本退出失败: %w", err)
	}
	defer windows.CloseHandle(handle)
	result, err := windows.WaitForSingleObject(handle, 120000)
	if err != nil {
		return err
	}
	if result != windows.WAIT_OBJECT_0 {
		return errors.New("旧版本未能在两分钟内退出")
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func launchUpdatedApp(target string) error {
	cmd := exec.Command(target)
	cmd.Dir = filepath.Dir(target)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func cleanupUpdateStages(exe string) {
	time.Sleep(5 * time.Second)
	state := guiUpdater.snapshot().Status
	if state == "downloading" || state == "restarting" {
		return
	}
	paths, _ := filepath.Glob(filepath.Join(updateDir(exe), "Hestia-v*.exe"))
	for _, path := range paths {
		_ = os.Remove(path)
	}
	_ = os.Remove(filepath.Join(updateDir(exe), "Hestia-updater.exe"))
}
