//go:build windows

package main

import (
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	startupRegisterClass     = tUser32.NewProc("RegisterClassW")
	startupDefaultWindowProc = tUser32.NewProc("DefWindowProcW")
	startupCreateWindow      = tUser32.NewProc("CreateWindowExW")
	startupGetSystemMetrics  = tUser32.NewProc("GetSystemMetrics")
	startupSystemParameters  = tUser32.NewProc("SystemParametersInfoW")
	startupSetTimer          = tUser32.NewProc("SetTimer")
	startupKillTimer         = tUser32.NewProc("KillTimer")
	startupGetMessage        = tUser32.NewProc("GetMessageW")
	startupDispatchMessage   = tUser32.NewProc("DispatchMessageW")
	startupPostThreadMessage = tUser32.NewProc("PostThreadMessageW")
	startupGetThreadID       = tKernel32.NewProc("GetCurrentThreadId")
	startupGetModule         = tKernel32.NewProc("GetModuleHandleW")
)

type startupWindowClass struct {
	style                   uint32
	proc                    uintptr
	classExtra, windowExtra int32
	instance, icon, cursor  uintptr
	background              uintptr
	menu, name              *uint16
}

type startupMessage struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	point   pointT
	private uint32
}

// showStartup draws the splash on a dedicated UI thread so the animation
// keeps moving while the main thread initializes the app and WebView2.
func showStartup() func() {
	ready := make(chan uintptr)
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)

		visual := newStartupVisual()
		if visual == nil {
			ready <- 0
			return
		}
		defer visual.close()

		module, _, _ := startupGetModule.Call(0)
		class := startupWindowClass{
			proc: syscall.NewCallback(func(hwnd, msg, wp, lp uintptr) uintptr {
				if msg == 0x0317 || msg == 0x0318 { // WM_PRINT, WM_PRINTCLIENT
					visual.print(wp)
					return 0
				}
				result, _, _ := startupDefaultWindowProc.Call(hwnd, msg, wp, lp)
				return result
			}),
			instance: module,
			name:     t16o("HestiaStartup"),
		}
		if atom, _, _ := startupRegisterClass.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
			ready <- 0
			return
		}
		screenWidth, _, _ := startupGetSystemMetrics.Call(0)  // SM_CXSCREEN
		screenHeight, _, _ := startupGetSystemMetrics.Call(1) // SM_CYSCREEN
		wnd, _, _ := startupCreateWindow.Call(0x80080,        // WS_EX_LAYERED | WS_EX_TOOLWINDOW
			uintptr(unsafe.Pointer(class.name)), uintptr(unsafe.Pointer(t16o("正在启动 Hestia"))),
			0x80000000, // WS_POPUP
			(screenWidth-startupWidth)/2, (screenHeight-startupHeight)/2, startupWidth, startupHeight, 0, 0, module, 0)
		if wnd == 0 {
			ready <- 0
			return
		}
		defer pTDestroyWindow.Call(wnd)

		var animate int32 = 1
		startupSystemParameters.Call(0x1042, 0, uintptr(unsafe.Pointer(&animate)), 0) // SPI_GETCLIENTAREAANIMATION
		if !visual.present(wnd, 0, animate != 0) {
			ready <- 0
			return
		}
		if animate != 0 {
			started := time.Now()
			timerProc := syscall.NewCallback(func(_, _, _, _ uintptr) uintptr {
				visual.present(wnd, time.Since(started).Seconds(), true)
				return 0
			})
			if timer, _, _ := startupSetTimer.Call(wnd, 1, 16, timerProc); timer != 0 {
				defer startupKillTimer.Call(wnd, timer)
			}
		}
		pTShowWindow.Call(wnd, 5) // SW_SHOW
		thread, _, _ := startupGetThreadID.Call()
		ready <- thread

		var msg startupMessage
		for {
			result, _, _ := startupGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(result) <= 0 {
				return
			}
			startupDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}()
	thread := <-ready
	var once sync.Once
	return func() {
		once.Do(func() {
			if thread != 0 {
				startupPostThreadMessage.Call(thread, 0x0012, 0, 0) // WM_QUIT
			}
			<-done
		})
	}
}
