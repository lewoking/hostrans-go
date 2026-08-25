//go:build windows

package ui

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	nimAdd     = 0
	nimDelete  = 2
	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

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

type notifyIconData struct {
	Size            uint32
	Wnd             uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
}

var (
	shell32 = windows.NewLazySystemDLL("shell32.dll")

	procLoadIconW           = user32.NewProc("LoadIconW")
	procShellNotifyIconW    = shell32.NewProc("Shell_NotifyIconW")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenuW         = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

var tray = struct {
	hwnd   uintptr
	icon   uintptr
	onQuit func()
}{}

func loadAppIcon(mod uintptr) uintptr {
	ico, _, _ := procLoadIconW.Call(mod, 1) // MAKEINTRESOURCE(1) from embedded .syso
	if ico == 0 {
		ico, _, _ = procLoadIconW.Call(0, idiApplication)
	}
	return ico
}

func startTray(onQuit func()) {
	hwnd := active.hwnd
	if hwnd == 0 {
		return
	}
	mod, _, _ := procGetModuleHandleW.Call(0)
	icon := loadAppIcon(mod)
	nid := notifyIconData{
		Wnd:             hwnd,
		ID:              1,
		Flags:           nifMessage | nifIcon | nifTip,
		CallbackMessage: wmTray,
		Icon:            icon,
	}
	nid.Size = uint32(unsafe.Sizeof(nid))
	tip, _ := windows.UTF16FromString("HOSTrans")
	copy(nid.Tip[:], tip)
	procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	tray.hwnd = hwnd
	tray.icon = icon
	tray.onQuit = onQuit
}

func stopTray() {
	if tray.hwnd == 0 {
		return
	}
	nid := notifyIconData{Wnd: tray.hwnd, ID: 1}
	nid.Size = uint32(unsafe.Sizeof(nid))
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	tray.hwnd = 0
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
