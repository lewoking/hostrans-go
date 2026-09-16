//go:build windows

package ui

import (
	"runtime"
	"strings"
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

	lwaColorkey   = 0x00000001
	chromaKey     = 0x00FF00FF // 品红，用作透明色键
	swShow        = 5
	hwndTopmost   = ^uintptr(0) // HWND_TOPMOST = -1
	swpNoMove     = 0x0002
	swpNoSize     = 0x0001
	swpNoActivate = 0x0010
	swpShowWindow = 0x0040
	swpNoZOrder   = 0x0004

	wmPaint      = 0x000F
	wmDestroy    = 0x0002
	wmLButtonUp  = 0x0202
	wmHotkey     = 0x0312
	wmNChitTest  = 0x0084
	wmEraseBk    = 0x0014
	wmClose      = 0x0010
	wmApp        = 0x8000
	wmAppRedraw  = wmApp + 1
	wmAppShow    = wmApp + 2
	wmAppIdleArm = wmApp + 3
	wmTimer      = 0x0113

	idleTimerID = 1
	idleAfterMs = 30000

	htClient  = 1
	htCaption = 2

	modControl  = 0x0002
	modNoRepeat = 0x4000
	vkTab       = 0x09
	vkP         = 0x50

	dtLeft          = 0x0000
	dtWordBreak     = 0x0010
	dtNoPrefix      = 0x0800
	dtCalcRect      = 0x0400
	dtSingleLine    = 0x0020
	transparent     = 1
	fwNormal        = 400
	defaultChar     = 1
	outTTPrecis     = 4
	antiAliasedQual = 4
	idcArrow        = 32512
	idiApplication  = 32512

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
	idleFontDiv = 3

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

	mu         sync.Mutex
	lines      []Line
	idle       bool
	lastActive time.Time

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
	procInvalidateRect             = user32.NewProc("InvalidateRect")
	procSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	procSetWindowPos               = user32.NewProc("SetWindowPos")
	procDestroyWindow              = user32.NewProc("DestroyWindow")
	procLoadCursorW                = user32.NewProc("LoadCursorW")
	procLoadIconW                  = user32.NewProc("LoadIconW")
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
	procGetDC                      = user32.NewProc("GetDC")
	procReleaseDC                  = user32.NewProc("ReleaseDC")
	procSetTimer                   = user32.NewProc("SetTimer")
	procKillTimer                  = user32.NewProc("KillTimer")
	procGetTextExtentPoint32W      = gdi32.NewProc("GetTextExtentPoint32W")

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

func loadAppIcon(mod uintptr) uintptr {
	ico, _, _ := procLoadIconW.Call(mod, 1)
	if ico == 0 {
		ico, _, _ = procLoadIconW.Call(0, idiApplication)
	}
	return ico
}

func NewOverlay() *Overlay {
	return &Overlay{}
}

func (o *Overlay) Push(speaker, text string) {
	o.mu.Lock()
	o.idle = false
	o.lastActive = time.Now()
	o.lines = append(o.lines, Line{Speaker: speaker, Text: text})
	if len(o.lines) > maxChat {
		o.lines = o.lines[len(o.lines)-maxChat:]
	}
	o.mu.Unlock()
	o.redraw()
	o.armIdle()
}

func (o *Overlay) Replace(speaker, from, to string) {
	if to == "" || to == from {
		return
	}
	o.mu.Lock()
	for i := len(o.lines) - 1; i >= 0; i-- {
		if o.lines[i].Alert {
			continue
		}
		if o.lines[i].Speaker == speaker && o.lines[i].Text == from {
			o.lines[i].Text = to
			o.idle = false
			o.lastActive = time.Now()
			o.mu.Unlock()
			o.redraw()
			o.armIdle()
			return
		}
	}
	o.mu.Unlock()
	o.Push(speaker, to)
}

func (o *Overlay) Alert(msg string) {
	if msg == "" {
		return
	}
	o.mu.Lock()
	if n := len(o.lines); n > 0 {
		last := o.lines[n-1]
		if last.Alert && last.Text == msg {
			o.idle = false
			o.lastActive = time.Now()
			o.mu.Unlock()
			o.redraw()
			o.armIdle()
			return
		}
	}
	o.idle = false
	o.lastActive = time.Now()
	o.lines = append(o.lines, Line{Speaker: "错误", Text: msg, Alert: true})
	if len(o.lines) > maxChat {
		o.lines = o.lines[len(o.lines)-maxChat:]
	}
	o.mu.Unlock()
	o.redraw()
	o.armIdle()
}

func (o *Overlay) Show() {
	o.mu.Lock()
	o.idle = false
	o.lastActive = time.Now()
	o.mu.Unlock()
	if o.hwnd != 0 {
		procPostMessageW.Call(o.hwnd, wmAppShow, 0, 0)
		procPostMessageW.Call(o.hwnd, wmAppIdleArm, 0, 0)
	}
}

func (o *Overlay) Stay() {
	o.Show()
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

	face, _ := windows.UTF16PtrFromString("Microsoft YaHei")
	o.fontChat, _, _ = procCreateFontW.Call(
		fontHeight(chatFontPx),
		0, 0, 0, fwNormal, 0, 0, 0,
		defaultChar, outTTPrecis, 0, antiAliasedQual, 0,
		uintptr(unsafe.Pointer(face)),
	)
	o.fontChatIdle, _, _ = procCreateFontW.Call(
		fontHeight(chatFontPx/idleFontDiv),
		0, 0, 0, fwNormal, 0, 0, 0,
		defaultChar, outTTPrecis, 0, antiAliasedQual, 0,
		uintptr(unsafe.Pointer(face)),
	)
	o.fontHint, _, _ = procCreateFontW.Call(
		fontHeight(hintFontPx),
		0, 0, 0, fwNormal, 0, 0, 0,
		defaultChar, outTTPrecis, 0, antiAliasedQual, 0,
		uintptr(unsafe.Pointer(face)),
	)

	modFlags := uintptr(modControl | modNoRepeat)
	procRegisterHotKey.Call(hwnd, hotShow, modFlags, vkTab)
	procRegisterHotKey.Call(hwnd, hotTransIn, modFlags, vkP)

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

func workArea() rect {
	var wa rect
	ok, _, _ := procSystemParametersInfoW.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&wa)), 0)
	if ok == 0 {
		sw, _, _ := procGetSystemMetrics.Call(smCxScreen)
		sh, _, _ := procGetSystemMetrics.Call(smCyScreen)
		wa = rect{Right: int32(sw), Bottom: int32(sh)}
	}
	return wa
}

func overlayOrigin() (x, y int32) {
	wa := workArea()
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
	case wmAppIdleArm:
		procSetTimer.Call(hwnd, idleTimerID, idleAfterMs, 0)
		return 0
	case wmTimer:
		if wParam == idleTimerID && o != nil {
			o.onIdleTimer(hwnd)
		}
		return 0
	case wmAppRedraw:
		if o != nil {
			o.syncSize(hwnd)
		}
		procInvalidateRect.Call(hwnd, 0, 1)
		return 0
	case wmAppShow:
		procShowWindow.Call(hwnd, swShow)
		procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
		if o != nil {
			o.syncSize(hwnd)
		}
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

func (o *Overlay) onIdleTimer(hwnd uintptr) {
	o.mu.Lock()
	waited := time.Since(o.lastActive)
	o.mu.Unlock()
	need := time.Duration(idleAfterMs) * time.Millisecond
	if waited < need {
		remain := int((need - waited) / time.Millisecond)
		if remain < 50 {
			remain = 50
		}
		procSetTimer.Call(hwnd, idleTimerID, uintptr(remain), 0)
		return
	}
	o.mu.Lock()
	o.idle = true
	o.mu.Unlock()
	procKillTimer.Call(hwnd, idleTimerID)
	o.syncSize(hwnd)
	procInvalidateRect.Call(hwnd, 0, 1)
}

type layoutRow struct {
	who    string
	whoW   int32
	body   []string
	indent int32
	lineH  int32
	h      int32
	alert  bool
	font   uintptr
}

type gdiSize struct{ cx, cy int32 }

func textSize(hdc, font uintptr, s string) (w, h int32) {
	if s == "" {
		return 0, 0
	}
	if font != 0 {
		procSelectObject.Call(hdc, font)
	}
	u, _ := windows.UTF16FromString(s)
	n := len(u) - 1
	if n <= 0 {
		return 0, 0
	}
	var sz gdiSize
	procGetTextExtentPoint32W.Call(hdc, uintptr(unsafe.Pointer(&u[0])), uintptr(n), uintptr(unsafe.Pointer(&sz)))
	return sz.cx, sz.cy
}

func wrapPrefix(hdc, font uintptr, s string, maxW int32) (line, rest string) {
	rs := []rune(s)
	if len(rs) == 0 {
		return "", ""
	}
	if maxW < 8 {
		maxW = 8
	}
	lo, hi, fit := 1, len(rs), 1
	for lo <= hi {
		mid := (lo + hi) / 2
		w, _ := textSize(hdc, font, string(rs[:mid]))
		if w <= maxW {
			fit = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	if fit < len(rs) && fit > 1 && isLatinLetter(rs[fit-1]) && isLatinLetter(rs[fit]) {
		for i := fit - 1; i > 0; i-- {
			if rs[i] == ' ' || rs[i] == '	' {
				return strings.TrimRight(string(rs[:i]), " "), strings.TrimLeft(string(rs[i:]), " ")
			}
		}
	}
	return string(rs[:fit]), string(rs[fit:])
}

func isLatinLetter(r rune) bool {
	return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z'
}

func wrapAll(hdc, font uintptr, s string, maxW int32) []string {
	var out []string
	for s != "" {
		line, rest := wrapPrefix(hdc, font, s, maxW)
		if line == "" {
			break
		}
		out = append(out, line)
		s = rest
	}
	return out
}

func (o *Overlay) layoutRows(hdc uintptr) (rows []layoutRow, font uintptr, minH, gap, needed int32) {
	o.mu.Lock()
	lines := append([]Line(nil), o.lines...)
	idle := o.idle
	o.mu.Unlock()

	font = o.fontChat
	minH = int32(lineMinH)
	gap = int32(lineGap)
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

	for _, ln := range lines {
		rowFont := font
		rowMinH := minH
		if ln.Alert {
			if idle {
				if o.fontChatIdle != 0 {
					rowFont = o.fontChatIdle
				}
			} else if o.fontHint != 0 {
				rowFont = o.fontHint
				rowMinH = int32(hintFontPx + 2)
			}
		}
		who := ln.Speaker
		if who != "" {
			who += "："
		}
		contentW := int32(winW - 28)
		_, emH := textSize(hdc, rowFont, "汉")
		if emH < rowMinH {
			emH = rowMinH
		}
		indent, _ := textSize(hdc, rowFont, "的的")
		whoW, _ := textSize(hdc, rowFont, who)
		var body []string
		if ln.Text == "" {
			body = nil
		} else {
			firstW := contentW - whoW
			if who == "" {
				firstW = contentW
			}
			if firstW < 24 {
				body = wrapAll(hdc, rowFont, ln.Text, contentW-indent)
			} else {
				first, rest := wrapPrefix(hdc, rowFont, ln.Text, firstW)
				body = []string{first}
				if rest != "" {
					body = append(body, wrapAll(hdc, rowFont, rest, contentW-indent)...)
				}
			}
		}
		nline := int32(len(body))
		if nline < 1 {
			nline = 1
		}
		if who != "" && whoW > contentW-24 {
			nline = int32(1 + len(body))
			if nline < 1 {
				nline = 1
			}
		}
		h := emH * nline
		rows = append(rows, layoutRow{who: who, whoW: whoW, body: body, indent: indent, lineH: emH, h: h, alert: ln.Alert, font: rowFont})
		if len(rows) >= maxChat {
			break
		}
	}

	needed = int32(padTop + padBot)
	if len(rows) == 0 {
		needed = padTop + minH + padBot
	} else {
		for _, r := range rows {
			needed += r.h + gap
		}
	}
	wa := workArea()
	maxH := wa.Bottom - wa.Top - 48
	if maxH < minH+int32(padTop+padBot) {
		maxH = minH + int32(padTop+padBot)
	}
	if needed > maxH {
		needed = maxH
	}
	for len(rows) > 1 {
		sum := int32(padTop + padBot)
		for _, r := range rows {
			sum += r.h + gap
		}
		if sum <= needed {
			break
		}
		rows = rows[1:]
	}
	if len(rows) > 0 {
		needed = int32(padTop + padBot)
		for _, r := range rows {
			needed += r.h + gap
		}
		if needed > maxH {
			needed = maxH
		}
	}
	return
}

func (o *Overlay) syncSize(hwnd uintptr) {
	hdc, _, _ := procGetDC.Call(hwnd)
	if hdc == 0 {
		return
	}
	_, _, _, _, needed := o.layoutRows(hdc)
	procReleaseDC.Call(hwnd, hdc)
	var rc rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	if rc.Bottom != needed {
		procSetWindowPos.Call(hwnd, 0, 0, 0, uintptr(winW), uintptr(needed), swpNoMove|swpNoActivate|swpNoZOrder)
	}
}

func (o *Overlay) paint(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))

	rows, font, minH, gap, _ := o.layoutRows(hdc)
	var rc rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	brush, _, _ := procCreateSolidBrush.Call(chromaKey)
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), brush)
	procDeleteObject.Call(brush)
	procSetBkMode.Call(hdc, transparent)

	drawLine := func(font uintptr, x, y, w int32, color uint32, s string) {
		if s == "" {
			return
		}
		if font != 0 {
			procSelectObject.Call(hdc, font)
		}
		procSetTextColor.Call(hdc, uintptr(color))
		r := rect{Left: x, Top: y, Right: x + w, Bottom: y + 80}
		u, _ := windows.UTF16FromString(s)
		procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1),
			uintptr(unsafe.Pointer(&r)), dtLeft|dtSingleLine|dtNoPrefix)
	}

	teamBlue := rgb(0x31, 0x84, 0xFF)
	chatWhite := rgb(255, 255, 255)
	alertYellow := rgb(255, 210, 0)
	drawLine(o.fontHint, winW-28, 8, 20, chatWhite, "×")

	y := int32(padTop)
	maxY := rc.Bottom - int32(padBot)
	contentW := int32(winW - 28)
	for _, row := range rows {
		if y >= maxY {
			break
		}
		rowFont := row.font
		if rowFont == 0 {
			rowFont = font
		}
		nameCol, bodyCol := teamBlue, chatWhite
		if row.alert {
			nameCol, bodyCol = alertYellow, alertYellow
		}
		lineH := row.lineH
		if lineH < 1 {
			lineH = minH
		}
		drawLine(rowFont, 14, y, row.whoW+2, nameCol, row.who)
		ownLine := row.who != "" && row.whoW > contentW-24
		if ownLine {
			y += lineH
			for _, part := range row.body {
				if y >= maxY {
					break
				}
				restW := contentW - row.indent
				if restW < 8 {
					restW = 8
				}
				drawLine(rowFont, 14+row.indent, y, restW, bodyCol, part)
				y += lineH
			}
		} else {
			if len(row.body) > 0 {
				firstW := contentW - row.whoW
				if row.who == "" {
					firstW = contentW
				}
				if firstW < 8 {
					firstW = 8
				}
				drawLine(rowFont, 14+row.whoW, y, firstW, bodyCol, row.body[0])
			}
			y += lineH
			for i := 1; i < len(row.body); i++ {
				if y >= maxY {
					break
				}
				restW := contentW - row.indent
				if restW < 8 {
					restW = 8
				}
				drawLine(rowFont, 14+row.indent, y, restW, bodyCol, row.body[i])
				y += lineH
			}
		}
		y += gap
	}
}
