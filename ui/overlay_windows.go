//go:build windows

package ui

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wsExLayered    = 0x00080000
	wsExTopmost    = 0x00000008
	wsExToolwindow = 0x00000080
	wsPopup        = 0x80000000
	wsVisible      = 0x10000000

	lwaColorkey   = 0x00000001
	chromaKey     = 0x00FF00FF // 品红，用作透明色键
	swShow        = 5
	swHide        = 0
	hwndTopmost   = ^uintptr(0) // HWND_TOPMOST = -1
	swpNoMove     = 0x0002
	swpNoSize     = 0x0001
	swpNoActivate = 0x0010
	swpShowWindow = 0x0040

	wmPaint     = 0x000F
	wmDestroy   = 0x0002
	wmLButtonUp = 0x0202
	wmHotkey    = 0x0312
	wmNChitTest = 0x0084
	wmEraseBk   = 0x0014
	wmClose     = 0x0010
	wmApp       = 0x8000
	wmAppRedraw  = wmApp + 1
	wmAppHide    = wmApp + 2
	wmAppShow    = wmApp + 3
	wmAppIdleArm = wmApp + 4
	wmTimer      = 0x0113

	idleTimerID = 1
	idleAfterMs = 30000

	htClient  = 1
	htCaption = 2

	modControl  = 0x0002
	modNoRepeat = 0x4000
	vkTab       = 0x09
	vkP         = 0x50

	dtLeft        = 0x0000
	dtNoPrefix    = 0x0800
	dtCalcRect    = 0x0400
	dtSingleLine  = 0x0020
	dtEndEllipsis = 0x00008000
	transparent = 1
	fwNormal    = 400
	defaultChar = 1
	idcArrow    = 32512
	colorWindow = 5

	hotShow    = 2
	hotTransIn = 3

	winW     = 360
	maxChat  = 10
	lineMinH = 22
	lineGap  = 6
	padTop   = 8
	padBot   = 8
	winH     = padTop + maxChat*(lineMinH+lineGap) + padBot

	chatFontPx  = 18
	hintFontPx  = 12
	idleFontDiv = 5

	spiGetWorkArea = 0x0030
	smCxScreen     = 0
	smCyScreen     = 1
)

type rect struct {
	Left, Top, Right, Bottom int32
}

type point struct {
	X, Y int32
}

type paintStruct struct {
	Hdc         uintptr
	Erase       int32
	RcPaint     rect
	Restore     int32
	IncUpdate   int32
	RgbReserved [32]byte
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type Overlay struct {
	hwnd         uintptr
	fontChat     uintptr
	fontChatIdle uintptr
	fontHint     uintptr

	mu      sync.Mutex
	lines   []Line
	visible bool
	idle    bool

	OnLocate         func()
	OnTranslateInput func()
}

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW           = user32.NewProc("RegisterClassExW")
	procCreateWindowExW            = user32.NewProc("CreateWindowExW")
	procDefWindowProcW             = user32.NewProc("DefWindowProcW")
	procGetMessageW                = user32.NewProc("GetMessageW")
	procTranslateMessage           = user32.NewProc("TranslateMessage")
	procDispatchMessageW           = user32.NewProc("DispatchMessageW")
	procPostQuitMessage            = user32.NewProc("PostQuitMessage")
	procShowWindow                 = user32.NewProc("ShowWindow")
	procUpdateWindow               = user32.NewProc("UpdateWindow")
	procInvalidateRect             = user32.NewProc("InvalidateRect")
	procSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	procSetWindowPos               = user32.NewProc("SetWindowPos")
	procDestroyWindow              = user32.NewProc("DestroyWindow")
	procLoadCursorW                = user32.NewProc("LoadCursorW")
	procBeginPaint                 = user32.NewProc("BeginPaint")
	procEndPaint                   = user32.NewProc("EndPaint")
	procFillRect                   = user32.NewProc("FillRect")
	procDrawTextW                  = user32.NewProc("DrawTextW")
	procGetClientRect              = user32.NewProc("GetClientRect")
	procScreenToClient             = user32.NewProc("ScreenToClient")
	procPostMessageW               = user32.NewProc("PostMessageW")
	procRegisterHotKey             = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey           = user32.NewProc("UnregisterHotKey")
	procSetProcessDPIAware         = user32.NewProc("SetProcessDPIAware")
	procGetModuleHandleW           = kernel32.NewProc("GetModuleHandleW")
	procCreateFontW                = gdi32.NewProc("CreateFontW")
	procSelectObject               = gdi32.NewProc("SelectObject")
	procDeleteObject               = gdi32.NewProc("DeleteObject")
	procSetTextColor               = gdi32.NewProc("SetTextColor")
	procSetBkMode                  = gdi32.NewProc("SetBkMode")
	procCreateSolidBrush           = gdi32.NewProc("CreateSolidBrush")
	procGetSystemMetrics           = user32.NewProc("GetSystemMetrics")
	procSystemParametersInfoW      = user32.NewProc("SystemParametersInfoW")
	procSetTimer                   = user32.NewProc("SetTimer")
	procKillTimer                  = user32.NewProc("KillTimer")

	wndProcCB = syscall.NewCallback(wndProc)
	active    *Overlay
)

func rgb(r, g, b uint8) uint32 {
	return uint32(r) | uint32(g)<<8 | uint32(b)<<16
}

func fontHeight(px int) uintptr {
	if px < 1 {
		px = 1
	}
	return ^uintptr(px-1) + 1
}

func NewOverlay() *Overlay {
	return &Overlay{visible: true}
}

func (o *Overlay) Push(speaker, text string) {
	o.mu.Lock()
	o.idle = false
	o.lines = append(o.lines, Line{Speaker: speaker, Text: text})
	if len(o.lines) > maxChat {
		o.lines = o.lines[len(o.lines)-maxChat:]
	}
	o.mu.Unlock()
	o.redraw()
	o.armIdle()
}

func (o *Overlay) Status(msg string) {
	// 路径/版本/状态不进悬浮窗
}

func (o *Overlay) Show() {
	o.mu.Lock()
	o.idle = false
	o.mu.Unlock()
	if o.hwnd != 0 {
		procPostMessageW.Call(o.hwnd, wmAppShow, 0, 0)
		procPostMessageW.Call(o.hwnd, wmAppIdleArm, 0, 0)
	}
}

func (o *Overlay) Stay() {
	o.Show()
}

func (o *Overlay) Hide() {
	if o.hwnd != 0 {
		procPostMessageW.Call(o.hwnd, wmAppHide, 0, 0)
	}
}

func (o *Overlay) Close() {
	if o.hwnd != 0 {
		procPostMessageW.Call(o.hwnd, wmClose, 0, 0)
	}
}

func (o *Overlay) redraw() {
	if o.hwnd != 0 {
		procPostMessageW.Call(o.hwnd, wmAppRedraw, 0, 0)
	}
}

func (o *Overlay) armIdle() {
	if o.hwnd != 0 {
		procPostMessageW.Call(o.hwnd, wmAppIdleArm, 0, 0)
	}
}

func (o *Overlay) Run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procSetProcessDPIAware.Call()

	className, _ := windows.UTF16PtrFromString("HOSTransOverlay")
	title, _ := windows.UTF16PtrFromString("HOSTrans")
	mod, _, _ := procGetModuleHandleW.Call(0)
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	appIcon := loadAppIcon(mod)

	wc := wndClassEx{
		Size:       uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:    wndProcCB,
		Instance:   windows.Handle(mod),
		Icon:       windows.Handle(appIcon),
		Cursor:     windows.Handle(cursor),
		ClassName:  className,
		IconSm:     windows.Handle(appIcon),
		Background: 0,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	active = o
	ex := uintptr(wsExLayered | wsExTopmost | wsExToolwindow)
	style := uintptr(wsPopup | wsVisible)
	x, y := overlayOrigin()
	hwnd, _, err := procCreateWindowExW.Call(
		ex,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		style,
		uintptr(x), uintptr(y), winW, winH,
		0, 0, mod, 0,
	)
	if hwnd == 0 {
		t, _ := windows.UTF16PtrFromString("无法创建悬浮窗")
		c, _ := windows.UTF16PtrFromString("HOSTrans")
		_, _ = windows.MessageBox(0, t, c, windows.MB_OK|windows.MB_ICONERROR)
		return err
	}
	o.hwnd = hwnd
	procSetLayeredWindowAttributes.Call(hwnd, chromaKey, 255, lwaColorkey)
	procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow)

	face, _ := windows.UTF16PtrFromString("Microsoft YaHei UI")
	o.fontChat, _, _ = procCreateFontW.Call(
		fontHeight(chatFontPx),
		0, 0, 0, fwNormal, 0, 0, 0,
		defaultChar, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(face)),
	)
	o.fontChatIdle, _, _ = procCreateFontW.Call(
		fontHeight(chatFontPx/idleFontDiv),
		0, 0, 0, fwNormal, 0, 0, 0,
		defaultChar, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(face)),
	)
	o.fontHint, _, _ = procCreateFontW.Call(
		fontHeight(hintFontPx),
		0, 0, 0, fwNormal, 0, 0, 0,
		defaultChar, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(face)),
	)

	modFlags := uintptr(modControl | modNoRepeat)
	procRegisterHotKey.Call(hwnd, hotShow, modFlags, vkTab)
	procRegisterHotKey.Call(hwnd, hotTransIn, modFlags, vkP)
	startTray(func() { o.Close() })

	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	stopTray()
	procUnregisterHotKey.Call(hwnd, hotShow)
	procUnregisterHotKey.Call(hwnd, hotTransIn)
	if o.fontChat != 0 {
		procDeleteObject.Call(o.fontChat)
	}
	if o.fontChatIdle != 0 {
		procDeleteObject.Call(o.fontChatIdle)
	}
	if o.fontHint != 0 {
		procDeleteObject.Call(o.fontHint)
	}
	active = nil
	return nil
}

func overlayOrigin() (x, y int32) {
	var wa rect
	ok, _, _ := procSystemParametersInfoW.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&wa)), 0)
	if ok == 0 {
		sw, _, _ := procGetSystemMetrics.Call(smCxScreen)
		sh, _, _ := procGetSystemMetrics.Call(smCyScreen)
		wa = rect{Right: int32(sw), Bottom: int32(sh)}
	}
	const margin = 24
	x = wa.Right - winW - margin
	if x < wa.Left {
		x = wa.Left
	}
	y = wa.Top + (wa.Bottom-wa.Top-winH)/2
	if y < wa.Top {
		y = wa.Top
	}
	return
}

func wndProc(hwnd, msgID, wParam, lParam uintptr) uintptr {
	o := active
	switch msgID {
	case wmEraseBk:
		return 1
	case wmPaint:
		if o != nil {
			o.paint(hwnd)
		}
		return 0
	case wmNChitTest:
		if hitCloseScreen(hwnd, lParam) {
			return htClient
		}
		return htCaption
	case wmLButtonUp:
		x := int32(int16(lParam))
		y := int32(int16(lParam >> 16))
		if inClose(hwnd, x, y) {
			procDestroyWindow.Call(hwnd)
		}
		return 0
	case wmHotkey:
		if o == nil {
			break
		}
		switch wParam {
		case hotShow:
			o.Show()
		case hotTransIn:
			if o.OnTranslateInput != nil {
				go o.OnTranslateInput()
			}
		}
		return 0
	case wmTray:
		if lParam == wmRButtonUp || lParam == wmLButtonUp {
			showTrayMenu(hwnd)
		}
		return 0
	case wmAppIdleArm:
		procSetTimer.Call(hwnd, idleTimerID, idleAfterMs, 0)
		return 0
	case wmTimer:
		if wParam == idleTimerID && o != nil {
			o.mu.Lock()
			o.idle = true
			o.mu.Unlock()
			procKillTimer.Call(hwnd, idleTimerID)
			procInvalidateRect.Call(hwnd, 0, 1)
		}
		return 0
	case wmAppRedraw:
		procInvalidateRect.Call(hwnd, 0, 1)
		return 0
	case wmAppHide:
		if o != nil {
			o.visible = false
		}
		procShowWindow.Call(hwnd, swHide)
		return 0
	case wmAppShow:
		if o != nil {
			o.visible = true
		}
		procShowWindow.Call(hwnd, swShow)
		procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
		procInvalidateRect.Call(hwnd, 0, 1)
		return 0
	case wmDestroy:
		procKillTimer.Call(hwnd, idleTimerID)
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msgID, wParam, lParam)
	return r
}

func hitCloseScreen(hwnd, lParam uintptr) bool {
	pt := point{X: int32(int16(lParam)), Y: int32(int16(lParam >> 16))}
	procScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
	return inClose(hwnd, pt.X, pt.Y)
}

func inClose(hwnd uintptr, x, y int32) bool {
	var rc rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	return x >= rc.Right-36 && x <= rc.Right-8 && y >= 8 && y <= 32
}

func (o *Overlay) paint(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))

	var rc rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	brush, _, _ := procCreateSolidBrush.Call(chromaKey)
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), brush)
	procDeleteObject.Call(brush)
	procSetBkMode.Call(hdc, transparent)

	draw := func(font uintptr, x, y, w, minH int32, color uint32, s string) int32 {
		if s == "" {
			return minH
		}
		if font != 0 {
			procSelectObject.Call(hdc, font)
		}
		procSetTextColor.Call(hdc, uintptr(color))
		r := rect{Left: x, Top: y, Right: x + w, Bottom: y + minH}
		u, _ := windows.UTF16FromString(s)
		procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1),
			uintptr(unsafe.Pointer(&r)), dtLeft|dtSingleLine|dtNoPrefix|dtEndEllipsis)
		return minH
	}

	o.mu.Lock()
	lines := append([]Line(nil), o.lines...)
	idle := o.idle
	o.mu.Unlock()

	teamBlue := rgb(0x31, 0x84, 0xFF)
	chatWhite := rgb(255, 255, 255)
	font := o.fontChat
	minH := int32(lineMinH)
	gap := int32(lineGap)
	if idle && o.fontChatIdle != 0 {
		font = o.fontChatIdle
		minH = int32(lineMinH / idleFontDiv)
		if minH < 1 {
			minH = 1
		}
		gap = int32(lineGap / idleFontDiv)
		if gap < 1 {
			gap = 1
		}
	}
	measureW := func(font uintptr, s string) int32 {
		if s == "" {
			return 0
		}
		if font != 0 {
			procSelectObject.Call(hdc, font)
		}
		r := rect{Right: winW, Bottom: 40}
		u, _ := windows.UTF16FromString(s)
		procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1),
			uintptr(unsafe.Pointer(&r)), dtLeft|dtNoPrefix|dtCalcRect|dtSingleLine)
		return r.Right - r.Left
	}

	draw(o.fontHint, winW-28, 8, 20, 14, rgb(255, 255, 255), "×")

	y := int32(padTop)
	maxY := int32(winH - padBot)
	shown := 0
	for _, ln := range lines {
		if y >= maxY || shown >= maxChat {
			break
		}
		if ln.Status {
			continue
		}
		who := ln.Speaker
		if who != "" {
			who += "："
		}
		ww := measureW(font, who)
		if ww > winW-80 {
			ww = winW - 80
		}
		h1 := draw(font, 14, y, ww+2, minH, teamBlue, who)
		h2 := draw(font, 14+ww, y, winW-28-ww, minH, chatWhite, ln.Text)
		if h2 > h1 {
			h1 = h2
		}
		y += h1 + gap
		shown++
	}
}
