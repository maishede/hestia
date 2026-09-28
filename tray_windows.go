//go:build windows

package main

// 窗口与托盘行为：点 X 隐藏到托盘（服务继续运行），
// 托盘左键/双击恢复窗口，右键菜单「显示主窗口 / 退出」。

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	tUser32   = syscall.NewLazyDLL("user32.dll")
	tShell32  = syscall.NewLazyDLL("shell32.dll")
	tKernel32 = syscall.NewLazyDLL("kernel32.dll")

	pTSetWindowLongPtrW = tUser32.NewProc("SetWindowLongPtrW")
	pTCallWindowProcW   = tUser32.NewProc("CallWindowProcW")
	pTShellNotifyIconW  = tShell32.NewProc("Shell_NotifyIconW")
	pTCreatePopupMenu   = tUser32.NewProc("CreatePopupMenu")
	pTAppendMenuW       = tUser32.NewProc("AppendMenuW")
	pTDestroyMenu       = tUser32.NewProc("DestroyMenu")
	pTGetCursorPos      = tUser32.NewProc("GetCursorPos")
	pTSetForegroundWnd  = tUser32.NewProc("SetForegroundWindow")
	pTTrackPopupMenu    = tUser32.NewProc("TrackPopupMenu")
	pTCreateIconFromRes = tUser32.NewProc("CreateIconFromResourceEx")
	pTCreateMutexW      = tKernel32.NewProc("CreateMutexW")
	pTLoadIconW         = tUser32.NewProc("LoadIconW")
	pTMessageBoxW       = tUser32.NewProc("MessageBoxW")
	pTSendMessageW      = tUser32.NewProc("SendMessageW")
	pTDestroyWindow     = tUser32.NewProc("DestroyWindow")
	pTShowWindow        = tUser32.NewProc("ShowWindow")
)

const (
	wmCloseTray    = 0x0010       // WM_CLOSE
	wmTrayCallback = 0x8000 + 1   // WM_APP+1 托盘回调
	wmSetIconT     = 0x0080       // WM_SETICON
	gwlpWndProc    = -4

	nimAdd    = 0
	nimModify = 1
	nimDelete = 2

	nifMessage = 0x1
	nifIcon    = 0x2
	nifTip     = 0x4
	nifInfo    = 0x10

	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205

	tpmRightButton = 0x0002
	tpmReturNCmd   = 0x0100

	swHide    = 0
	swRestore = 9
)

type pointT struct{ X, Y int32 }

type notifyIconDataW struct {
	CbSize           uint32
	HWnd             syscall.Handle
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            syscall.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     syscall.Handle
}

var (
	trayMu         sync.Mutex
	guiHwnd        syscall.Handle
	oldWndProc     uintptr
	newWndProcPtr  uintptr
	trayIconData   notifyIconDataW
	trayIconLoaded bool
	balloonShown   bool
)

func t16(s string) uintptr {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return 0
	}
	return uintptr(unsafe.Pointer(p))
}

func utf16Buf(dst []uint16, s string) {
	i := 0
	for _, r := range s {
		if i >= len(dst)-1 {
			break
		}
		dst[i] = uint16(r)
		i++
	}
}

func shellNotify(dwMessage uint32, data *notifyIconDataW) bool {
	r, _, _ := pTShellNotifyIconW.Call(uintptr(dwMessage), uintptr(unsafe.Pointer(data)))
	return r != 0
}

func loadAppIcon() syscall.Handle {
	loadAppIcons()
	return appIconSmall
}

// setupWindowChrome 子类化窗口（拦截 X）、注册托盘图标、设置图标。
// 返回 true 表示已有实例在运行（应退出本次启动）。
func setupWindowChrome(hwnd syscall.Handle) bool {
	guiHwnd = hwnd

	// 单实例：已运行则提示（旧实例可能在托盘里）
	_, _, err := pTCreateMutexW.Call(0, 1, t16(`Local\HestiaSingleton`))
	if errno, ok := err.(syscall.Errno); ok && errno == 183 { // ERROR_ALREADY_EXISTS
		pTMessageBoxW.Call(0, t16("Hestia 已在运行（可能最小化到了托盘）。\n\n左键托盘图标可恢复窗口，右键图标可选择退出。"), t16("Hestia"), 0x40)
		return true
	}

	loadAppIcons()
	pTSendMessageW.Call(uintptr(hwnd), wmSetIconT, 1, uintptr(appIconLarge)) // ICON_BIG
	pTSendMessageW.Call(uintptr(hwnd), wmSetIconT, 0, uintptr(appIconSmall)) // ICON_SMALL
	icon := appIconSmall

	trayIconData = notifyIconDataW{
		CbSize:           uint32(unsafe.Sizeof(trayIconData)),
		HWnd:             hwnd,
		UID:              1,
		UFlags:           nifMessage | nifIcon | nifTip,
		UCallbackMessage: wmTrayCallback,
		HIcon:            icon,
	}
	utf16Buf(trayIconData.SzTip[:], "Hestia 家庭影视服务器（左键打开，右键退出）")
	if shellNotify(nimAdd, &trayIconData) {
		trayIconLoaded = true
	}

	newWndProcPtr = syscall.NewCallback(trayWndProc)
	r, _, _ := pTSetWindowLongPtrW.Call(uintptr(hwnd), uintptr(^uintptr(3)), newWndProcPtr)
	oldWndProc = r
	return false
}

func removeTray() {
	trayMu.Lock()
	defer trayMu.Unlock()
	if trayIconLoaded {
		shellNotify(nimDelete, &trayIconData)
		trayIconLoaded = false
	}
}

func showGuiWindow() {
	pTShowWindow.Call(uintptr(guiHwnd), swRestore)
	pTSetForegroundWnd.Call(uintptr(guiHwnd))
}

func hideGuiWindow() {
	pTShowWindow.Call(uintptr(guiHwnd), swHide)
	if !balloonShown { // 首次隐藏时气泡提示
		balloonShown = true
		d := trayIconData
		d.UFlags = nifInfo
		d.DwInfoFlags = 1 // NIIF_INFO
		utf16Buf(d.SzInfoTitle[:], "Hestia 仍在运行")
		utf16Buf(d.SzInfo[:], "服务已最小化到托盘继续运行；左键图标恢复窗口，右键可退出。")
		shellNotify(nimModify, &d)
	}
}

func trayWndProc(hwnd syscall.Handle, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case wmCloseTray: // 点 X：隐藏到托盘
		hideGuiWindow()
		return 0
	case wmTrayCallback:
		switch lp & 0xFFFF {
		case wmLButtonUp, wmLButtonDblClk:
			showGuiWindow()
		case wmRButtonUp:
			trayMenu()
		}
		return 0
	}
	r, _, _ := pTCallWindowProcW.Call(oldWndProc, uintptr(hwnd), uintptr(msg), wp, lp)
	return r
}

// trayMenu 托盘右键菜单：显示主窗口 / 退出。
func trayMenu() {
	menu, _, _ := pTCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer pTDestroyMenu.Call(menu)
	pTAppendMenuW.Call(menu, 0, 1001, t16("显示主窗口"))
	pTAppendMenuW.Call(menu, 0, 1002, t16("退出"))
	var pt pointT
	pTGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pTSetForegroundWnd.Call(uintptr(guiHwnd))
	cmd, _, _ := pTTrackPopupMenu.Call(menu, uintptr(tpmRightButton|tpmReturNCmd),
		uintptr(pt.X), uintptr(pt.Y), 0, uintptr(guiHwnd), 0)
	switch cmd {
	case 1001:
		showGuiWindow()
	case 1002: // 真正退出：销毁窗口 → 消息循环结束 → shutdown
		removeTray()
		pTDestroyWindow.Call(uintptr(guiHwnd))
	}
}
