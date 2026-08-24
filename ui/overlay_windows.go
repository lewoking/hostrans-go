//go:build windows

package ui

import (
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wsExLayered    = 0x00080000
	wsExTopmost    = 0x00000008
	wsExToolwindow = 0x00000080
	wsPopup        = 0x80000000
	wsVisible      = 0x10000000

	lwaAlpha      = 0x00000002
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
	wmAppRedraw = wmApp + 1
	wmAppHide   = wmApp + 2
	wmAppShow   = wmApp + 3

	htClient  = 1
	htCaption = 2

	modControl  = 0x0002
	modNoRepeat = 0x4000
	vkTab       = 0x09
	vkP         = 0x50

	dtLeft      = 0x0000
	dtWordBreak = 0x0010
	dtNoPrefix  = 0x0800
	dtCalcRect  = 0x0400
	transparent = 1
	fwNormal    = 400
	defaultChar = 1
	idcArrow    = 32512
	colorWindow = 5

	hotShow    = 2
	hotTransIn = 3

	winW = 360
	winH = 440
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
	hwnd uintptr
	font uintptr

	mu      sync.Mutex
	lines   []Line
	visible bool
	hideT   *time.Timer

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

	wndProcCB = syscall.NewCallback(wndProc)
	active    *Overlay
)

func rgb(r, g, b uint8) uint32 {
	return uint32(r) | uint32(g)<<8 | uint32(b)<<16
}

func NewOverlay() *Overlay {
	return &Overlay{visible: true}
}

func (o *Overlay) Push(speaker, text string) {
	o.mu.Lock()
	o.lines = append(o.lines, Line{Speaker: speaker, Text: text})
	if len(o.lines) > 12 {
		o.lines = o.lines[len(o.lines)-12:]
	}
	o.mu.Unlock()
	o.redraw()
	o.armHide()
}

func (o *Overlay) Status(msg string) {
	o.mu.Lock()
	// 状态去重：覆盖最后一条状态
	if n := len(o.lines); n > 0 && o.lines[n-1].Status && o.lines[n-1].Text == msg {
		o.mu.Unlock()
		return
	}
	o.lines = append(o.lines, Line{Text: msg, Status: true})
	if len(o.lines) > 12 {
		o.lines = o.lines[len(o.lines)-12:]
	}
	o.mu.Unlock()
	o.redraw()
}

func (o *Overlay) Show() {
	if o.hwnd != 0 {
		procPostMessageW.Call(o.hwnd, wmAppShow, 0, 0)
	}
	o.armHide()
}

func (o *Overlay) Stay() {
	if o.hwnd != 0 {
		procPostMessageW.Call(o.hwnd, wmAppShow, 0, 0)
	}
	o.mu.Lock()
	if o.hideT != nil {
		o.hideT.Stop()
		o.hideT = nil
	}
	o.mu.Unlock()
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

func (o *Overlay) armHide() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.hideT != nil {
		o.hideT.Stop()
	}
	o.hideT = time.AfterFunc(4500*time.Millisecond, func() {
		o.Hide()
	})
}

func (o *Overlay) Run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procSetProcessDPIAware.Call()

	className, _ := windows.UTF16PtrFromString("HOSTransOverlay")
	title, _ := windows.UTF16PtrFromString("HOSTrans")
	mod, _, _ := procGetModuleHandleW.Call(0)
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)

	wc := wndClassEx{
		Size:       uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:    wndProcCB,
		Instance:   windows.Handle(mod),
		Cursor:     windows.Handle(cursor),
		ClassName:  className,
		Background: 0,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	active = o
	ex := uintptr(wsExLayered | wsExTopmost | wsExToolwindow)
	style := uintptr(wsPopup | wsVisible)
	hwnd, _, err := procCreateWindowExW.Call(
		ex,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		style,
		80, 80, winW, winH,
		0, 0, mod, 0,
	)
	if hwnd == 0 {
		return err
	}
	o.hwnd = hwnd
	procSetLayeredWindowAttributes.Call(hwnd, 0, 210, lwaAlpha)
	procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow)

	face, _ := windows.UTF16PtrFromString("Microsoft YaHei UI")
	o.font, _, _ = procCreateFontW.Call(
		^uintptr(17)+1, // -18 height
		0, 0, 0, fwNormal, 0, 0, 0,
		defaultChar, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(face)),
	)

	modFlags := uintptr(modControl | modNoRepeat)
	procRegisterHotKey.Call(hwnd, hotShow, modFlags, vkTab)
	procRegisterHotKey.Call(hwnd, hotTransIn, modFlags, vkP)

	o.Status("等待进入对局…")
	o.Status("Ctrl+P 中译韩 / 空框则初始化  ·  Ctrl+Tab 显示")
	o.armHide()

	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	procUnregisterHotKey.Call(hwnd, hotShow)
	procUnregisterHotKey.Call(hwnd, hotTransIn)
	if o.font != 0 {
		procDeleteObject.Call(o.font)
	}
	active = nil
	return nil
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
	brush, _, _ := procCreateSolidBrush.Call(uintptr(rgb(36, 36, 78)))
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), brush)
	procDeleteObject.Call(brush)

	if o.font != 0 {
		procSelectObject.Call(hdc, o.font)
	}
	procSetBkMode.Call(hdc, transparent)

	draw := func(x, y, w, h int32, color uint32, s string) int32 {
		if s == "" {
			return 0
		}
		procSetTextColor.Call(hdc, uintptr(color))
		r := rect{Left: x, Top: y, Right: x + w, Bottom: y + h}
		u, _ := windows.UTF16FromString(s)
		// 计算高度
		calc := r
		procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1),
			uintptr(unsafe.Pointer(&calc)), dtLeft|dtWordBreak|dtNoPrefix|dtCalcRect)
		height := calc.Bottom - calc.Top
		if height < 18 {
			height = 18
		}
		r.Bottom = r.Top + height
		procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1),
			uintptr(unsafe.Pointer(&r)), dtLeft|dtWordBreak|dtNoPrefix)
		return height
	}

	y := int32(10)
	draw(14, y, winW-50, 24, rgb(255, 255, 255), "HOSTrans")
	draw(winW-32, y, 24, 24, rgb(220, 180, 180), "×")
	y += 28
	draw(14, y, winW-28, 18, rgb(140, 140, 190), "说话人：中文译文")
	y += 22

	o.mu.Lock()
	lines := append([]Line(nil), o.lines...)
	o.mu.Unlock()

	maxY := int32(winH - 48)
	for _, ln := range lines {
		if y >= maxY {
			break
		}
		var s string
		var col uint32
		if ln.Status {
			s = ln.Text
			col = rgb(160, 200, 255)
		} else {
			s = ln.Speaker + "：" + ln.Text
			col = rgb(255, 230, 140)
		}
		h := draw(14, y, winW-28, 80, col, s)
		y += h + 6
	}
	draw(14, winH-36, winW-28, 28, rgb(150, 150, 180), "Ctrl+P 译韩/空则初始化")
}
