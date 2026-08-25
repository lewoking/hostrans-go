//go:build windows

package ui

import (
	"syscall"
	"unsafe"

	"hostrans/dlog"

	"golang.org/x/sys/windows"
)

const (
	nimAdd    = 0
	nimDelete = 2

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifShowTip = 0x00000080

	wmRButtonUp = 0x0205
	wmTray      = wmApp + 10
	idmQuit     = 1

	tpmRightAlign  = 0x0008
	tpmBottomAlign = 0x0020
	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100
	mfString       = 0

	idiApplication = 32512
)

// 完整 NOTIFYICONDATAW，cbSize 必须对上，否则 Win11 NIM_ADD 会失败。
type notifyIconData struct {
	Size            uint32
	Wnd             uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GuidItem        windows.GUID
	BalloonIcon     uintptr
}

var (
	shell32 = windows.NewLazySystemDLL("shell32.dll")

	procLoadIconW              = user32.NewProc("LoadIconW")
	procShellNotifyIconW       = shell32.NewProc("Shell_NotifyIconW")
	procCreatePopupMenu        = user32.NewProc("CreatePopupMenu")
	procAppendMenuW            = user32.NewProc("AppendMenuW")
	procTrackPopupMenu         = user32.NewProc("TrackPopupMenu")
	procDestroyMenu            = user32.NewProc("DestroyMenu")
	procGetCursorPos           = user32.NewProc("GetCursorPos")
	procSetForegroundWindow    = user32.NewProc("SetForegroundWindow")
	procRegisterWindowMessageW = user32.NewProc("RegisterWindowMessageW")
)

var tray = struct {
	hwnd   uintptr
	icon   uintptr
	onQuit func()
	tbMsg  uint32
}{}

var trayWndProcCB = syscall.NewCallback(trayWndProc)

func loadAppIcon(mod uintptr) uintptr {
	ico, _, _ := procLoadIconW.Call(mod, 1)
	if ico == 0 {
		ico, _, _ = procLoadIconW.Call(0, idiApplication)
	}
	return ico
}

func startTray(onQuit func()) {
	mod, _, _ := procGetModuleHandleW.Call(0)
	className, _ := windows.UTF16PtrFromString("HOSTransTray")
	title, _ := windows.UTF16PtrFromString("HOSTrans")
	icon := loadAppIcon(mod)

	wc := wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:   trayWndProcCB,
		Instance:  windows.Handle(mod),
		Icon:      windows.Handle(icon),
		IconSm:    windows.Handle(icon),
		ClassName: className,
	}
	if atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		dlog.Printf("tray: RegisterClassEx failed: %v", err)
	}

	hwnd, _, err := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsPopup,
		0, 0, 1, 1,
		0, 0, mod, 0,
	)
	if hwnd == 0 {
		dlog.Printf("tray: CreateWindow failed: %v", err)
		return
	}

	tbName, _ := windows.UTF16PtrFromString("TaskbarCreated")
	tbMsg, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(tbName)))

	tray.hwnd = hwnd
	tray.icon = icon
	tray.onQuit = onQuit
	tray.tbMsg = uint32(tbMsg)
	addTrayIcon()
}

func addTrayIcon() {
	if tray.hwnd == 0 {
		return
	}
	nid := notifyIconData{
		Wnd:             tray.hwnd,
		ID:              1,
		Flags:           nifMessage | nifIcon | nifTip | nifShowTip,
		CallbackMessage: wmTray,
		Icon:            tray.icon,
	}
	nid.Size = uint32(unsafe.Sizeof(nid))
	tip, _ := windows.UTF16FromString("HOSTrans")
	copy(nid.Tip[:], tip)

	r, _, err := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	dlog.Printf("tray: NIM_ADD ret=%d err=%v hwnd=%#x icon=%#x size=%d", r, err, tray.hwnd, tray.icon, nid.Size)
	if r == 0 {
		// 自定义图标失败时换系统图标再试
		fallback, _, _ := procLoadIconW.Call(0, idiApplication)
		nid.Icon = fallback
		tray.icon = fallback
		r2, _, err2 := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
		dlog.Printf("tray: NIM_ADD fallback ret=%d err=%v icon=%#x", r2, err2, fallback)
	}
}

func stopTray() {
	if tray.hwnd == 0 {
		return
	}
	nid := notifyIconData{Wnd: tray.hwnd, ID: 1}
	nid.Size = uint32(unsafe.Sizeof(nid))
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	procDestroyWindow.Call(tray.hwnd)
	tray.hwnd = 0
}

func trayWndProc(hwnd, msgID, wParam, lParam uintptr) uintptr {
	if tray.tbMsg != 0 && uint32(msgID) == tray.tbMsg {
		addTrayIcon()
		return 0
	}
	switch msgID {
	case wmTray:
		if lParam == wmRButtonUp || lParam == wmLButtonUp {
			showTrayMenu(hwnd)
		}
		return 0
	case wmDestroy:
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msgID, wParam, lParam)
	return r
}

func showTrayMenu(hwnd uintptr) {
	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	txt, _ := windows.UTF16PtrFromString("退出")
	procAppendMenuW.Call(menu, mfString, idmQuit, uintptr(unsafe.Pointer(txt)))
	procSetForegroundWindow.Call(hwnd)
	cmd, _, _ := procTrackPopupMenu.Call(
		menu,
		tpmRightAlign|tpmBottomAlign|tpmRightButton|tpmReturnCmd,
		uintptr(uint32(pt.X)),
		uintptr(uint32(pt.Y)),
		0, hwnd, 0,
	)
	if cmd == idmQuit && tray.onQuit != nil {
		tray.onQuit()
	}
}
