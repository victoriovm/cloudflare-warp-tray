package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32                                = syscall.NewLazyDLL("user32.dll")
	shell32                               = syscall.NewLazyDLL("shell32.dll")
	gdi32                                 = syscall.NewLazyDLL("gdi32.dll")
	kernel32                              = syscall.NewLazyDLL("kernel32.dll")
	defWindowProc                         = user32.NewProc("DefWindowProcW")
	postMessage                           = user32.NewProc("PostMessageW")
	shellNotify                           = shell32.NewProc("Shell_NotifyIconW")
	inactiveIcon, activeIcon, neutralIcon uintptr
	window                                uintptr
	trayData                              notifyIconData
	taskbarCreated                        uint32
	cliPath                               string
	busy                                  atomic.Bool
	results                               = make(chan statusResult, 8)
	refresh                               = make(chan struct{}, 1)
	done                                  = make(chan struct{})
)

const (
	wmDestroy     = 0x0002
	wmClose       = 0x0010
	wmTray        = 0x8001
	wmResult      = 0x8002
	wmLButtonUp   = 0x0202
	wmRButtonUp   = 0x0205
	wmContextMenu = 0x007B
)

type point struct{ X, Y int32 }
type message struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          point
	Private        uint32
}
type windowClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	SmallIcon                          uintptr
}
type guid struct {
	A    uint32
	B, C uint16
	D    [8]byte
}
type notifyIconData struct {
	Size                uint32
	Window              uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                guid
	BalloonIcon         uintptr
}
type bitmapInfo struct {
	Size                   uint32
	Width, Height          int32
	Planes, BitCount       uint16
	Compression, ImageSize uint32
	XPels, YPels           int32
	Used, Important        uint32
	Colors                 [1]uint32
}
type iconInfo struct {
	IsIcon             int32
	XHotspot, YHotspot uint32
	Mask, Color        uintptr
}
type rect struct{ Left, Top, Right, Bottom int32 }
type statusResult struct {
	state connectionState
	err   error
}

func utf16(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

func setText(dst []uint16, text string) {
	clear(dst)
	encoded := syscall.StringToUTF16(text)
	if len(encoded) > len(dst) {
		encoded = encoded[:len(dst)-1]
	}
	copy(dst, encoded)
}

func makeIcon(size int, state connectionState) (uintptr, error) {
	dc, _, _ := gdi32.NewProc("CreateCompatibleDC").Call(0)
	if dc == 0 {
		return 0, fmt.Errorf("CreateCompatibleDC failed")
	}
	defer gdi32.NewProc("DeleteDC").Call(dc)
	info := bitmapInfo{Size: 40, Width: int32(size), Height: -int32(size), Planes: 1, BitCount: 32}
	var bits unsafe.Pointer
	bitmap, _, _ := gdi32.NewProc("CreateDIBSection").Call(dc, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 {
		return 0, fmt.Errorf("CreateDIBSection failed")
	}
	defer gdi32.NewProc("DeleteObject").Call(bitmap)
	oldBitmap, _, _ := gdi32.NewProc("SelectObject").Call(dc, bitmap)
	defer gdi32.NewProc("SelectObject").Call(dc, oldBitmap)
	pixels := unsafe.Slice((*byte)(bits), size*size*4)
	copy(pixels, renderIcon(size, state))
	mask, _, _ := gdi32.NewProc("CreateBitmap").Call(uintptr(size), uintptr(size), 1, 1, 0)
	if mask == 0 {
		return 0, fmt.Errorf("CreateBitmap failed")
	}
	defer gdi32.NewProc("DeleteObject").Call(mask)
	icon := iconInfo{IsIcon: 1, Mask: mask, Color: bitmap}
	handle, _, _ := user32.NewProc("CreateIconIndirect").Call(uintptr(unsafe.Pointer(&icon)))
	if handle == 0 {
		return 0, fmt.Errorf("CreateIconIndirect failed")
	}
	return handle, nil
}

func updateTray(result statusResult) {
	trayData.Icon = neutralIcon
	text := "WARP — unknown status"
	if result.err != nil {
		log.Print(result.err)
		text = "WARP — error: " + result.err.Error()
	} else {
		switch result.state {
		case connected:
			trayData.Icon = activeIcon
			text = "WARP connected — click to disconnect"
		case disconnected:
			trayData.Icon = inactiveIcon
			text = "WARP disconnected — click to connect"
		case connecting:
			text = "WARP connection changing — please wait"
		}
	}
	setText(trayData.Tip[:], text)
	shellNotify.Call(1, uintptr(unsafe.Pointer(&trayData)))
}

func deliver(result statusResult) {
	select {
	case results <- result:
		postMessage.Call(window, wmResult, 0, 0)
	case <-done:
	}
}

func queryStatus() statusResult {
	output, err := runCLI(cliPath, "status")
	if err != nil {
		return statusResult{err: err}
	}
	state := parseStatus(output)
	if state == unknown {
		return statusResult{err: fmt.Errorf("unrecognized status: %s", strings.TrimSpace(output))}
	}
	return statusResult{state: state}
}

func toggle() {
	if !busy.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer busy.Store(false)
		result := queryStatus()
		if result.err != nil || result.state == connecting {
			deliver(result)
			return
		}
		command := "connect"
		if result.state == connected {
			command = "disconnect"
		}
		if _, err := runCLI(cliPath, command); err != nil {
			deliver(statusResult{err: err})
			return
		}
		deliver(queryStatus())
		select {
		case refresh <- struct{}{}:
		default:
		}
	}()
}

func poll() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if busy.CompareAndSwap(false, true) {
			result := queryStatus()
			deliver(result)
			busy.Store(false)
		}
		select {
		case <-done:
			return
		case <-ticker.C:
		case <-refresh:
		}
	}
}

func showMenu(hwnd uintptr) {
	menu, _, _ := user32.NewProc("CreatePopupMenu").Call()
	if menu == 0 {
		return
	}
	defer user32.NewProc("DestroyMenu").Call(menu)
	user32.NewProc("AppendMenuW").Call(menu, 0, 1, uintptr(unsafe.Pointer(utf16("Exit"))))
	var pos point
	user32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&pos)))
	user32.NewProc("SetForegroundWindow").Call(hwnd)
	selected, _, _ := user32.NewProc("TrackPopupMenu").Call(menu, 0x102, uintptr(pos.X), uintptr(pos.Y), 0, hwnd, 0)
	if selected == 1 {
		postMessage.Call(hwnd, wmClose, 0, 0)
	}
	postMessage.Call(hwnd, 0, 0, 0)
}

func windowProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	if msg == taskbarCreated && taskbarCreated != 0 {
		shellNotify.Call(0, uintptr(unsafe.Pointer(&trayData)))
		return 0
	}
	switch msg {
	case wmTray:
		switch uint32(lparam) {
		case wmLButtonUp:
			toggle()
		case wmRButtonUp, wmContextMenu:
			showMenu(hwnd)
		}
		return 0
	case wmResult:
		select {
		case result := <-results:
			updateTray(result)
		default:
		}
		return 0
	case wmClose:
		user32.NewProc("DestroyWindow").Call(hwnd)
		return 0
	case wmDestroy:
		close(done)
		shellNotify.Call(2, uintptr(unsafe.Pointer(&trayData)))
		user32.NewProc("PostQuitMessage").Call(0)
		return 0
	}
	result, _, _ := defWindowProc.Call(hwnd, uintptr(msg), wparam, lparam)
	return result
}

func run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	mutex, _, err := kernel32.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(utf16("Local\\CloudflareWarpMyNetworkTray"))))
	if mutex == 0 {
		return fmt.Errorf("CreateMutexW: %v", err)
	}
	defer syscall.CloseHandle(syscall.Handle(mutex))
	if err == syscall.Errno(183) {
		return nil
	}
	cliPath, err = findCLI()
	if err != nil {
		return err
	}
	user32.NewProc("SetProcessDPIAware").Call()
	size, _, _ := user32.NewProc("GetSystemMetrics").Call(49)
	if size < 16 {
		size = 16
	}
	inactiveIcon, err = makeIcon(int(size), disconnected)
	if err != nil {
		return err
	}
	defer user32.NewProc("DestroyIcon").Call(inactiveIcon)
	activeIcon, err = makeIcon(int(size), connected)
	if err != nil {
		return err
	}
	defer user32.NewProc("DestroyIcon").Call(activeIcon)
	neutralIcon, err = makeIcon(int(size), unknown)
	if err != nil {
		return err
	}
	defer user32.NewProc("DestroyIcon").Call(neutralIcon)
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	className := utf16("CloudflareWarpMyNetworkTray")
	class := windowClass{Proc: syscall.NewCallback(windowProc), Instance: instance, ClassName: className}
	class.Size = uint32(unsafe.Sizeof(class))
	atom, _, err := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return fmt.Errorf("RegisterClassExW: %v", err)
	}
	defer user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(className)), instance)
	registered, _, _ := user32.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(utf16("TaskbarCreated"))))
	taskbarCreated = uint32(registered)
	// A hidden top-level window receives Explorer's TaskbarCreated broadcast, unlike a message-only window.
	window, _, err = user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, instance, 0)
	if window == 0 {
		return fmt.Errorf("CreateWindowExW: %v", err)
	}
	trayData = notifyIconData{Window: window, ID: 1, Flags: 1 | 2 | 4, Callback: wmTray, Icon: neutralIcon}
	trayData.Size = uint32(unsafe.Sizeof(trayData))
	setText(trayData.Tip[:], "WARP — checking connection")
	ok, _, err := shellNotify.Call(0, uintptr(unsafe.Pointer(&trayData)))
	if ok == 0 {
		user32.NewProc("DestroyWindow").Call(window)
		return fmt.Errorf("Shell_NotifyIconW: %v", err)
	}
	go poll()
	var msg message
	for {
		result, _, err := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) == -1 {
			return fmt.Errorf("GetMessageW: %v", err)
		}
		if result == 0 {
			break
		}
		user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
		user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
	}
	return nil
}

func main() {
	if base, err := os.UserCacheDir(); err == nil {
		dir := filepath.Join(base, "WarpTray")
		if os.MkdirAll(dir, 0755) == nil {
			if file, err := os.OpenFile(filepath.Join(dir, "warp-tray.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
				defer file.Close()
				log.SetOutput(file)
			}
		}
	}
	if err := run(); err != nil {
		log.Print(err)
	}
}
