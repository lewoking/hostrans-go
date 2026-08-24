//go:build windows

package memory

import (
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	inputKeyboard  = 1
	keyeventfKeyup = 0x0002
	vkReturn       = 0x0D
	vkControl      = 0x11
	vkMenu         = 0x12
	vkA            = 0x41
	vkC            = 0x43
	vkV            = 0x56
	cfUnicodeText  = 13
	gmemMoveable   = 0x0002
	swRestore      = 9
)

type kbInput struct {
	Type      uint32
	_         uint32
	Vk        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
	_         [8]byte
}

type rect struct {
	Left, Top, Right, Bottom int32
}

type hwndCand struct {
	hwnd  windows.HWND
	title string
	area  int32
}

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procSendInput           = user32.NewProc("SendInput")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procAttachThreadInput   = user32.NewProc("AttachThreadInput")
	procBringWindowToTop    = user32.NewProc("BringWindowToTop")
	procShowWindow          = user32.NewProc("ShowWindow")
	procGetWindowTextW      = user32.NewProc("GetWindowTextW")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procOpenClipboard       = user32.NewProc("OpenClipboard")
	procCloseClipboard      = user32.NewProc("CloseClipboard")
	procEmptyClipboard      = user32.NewProc("EmptyClipboard")
	procSetClipboardData    = user32.NewProc("SetClipboardData")
	procGetClipboardData    = user32.NewProc("GetClipboardData")
	procGetCurrentThreadId  = kernel32.NewProc("GetCurrentThreadId")
	procGlobalAlloc         = kernel32.NewProc("GlobalAlloc")
	procGlobalLock          = kernel32.NewProc("GlobalLock")
	procGlobalUnlock        = kernel32.NewProc("GlobalUnlock")
	procGlobalSize          = kernel32.NewProc("GlobalSize")
	procRtlMoveMemory       = kernel32.NewProc("RtlMoveMemory")
)

var (
	enumPID   uint32
	enumCands []hwndCand
	enumCB    = syscall.NewCallback(enumWindowsProc)
)

func enumWindowsProc(hwnd, lparam uintptr) uintptr {
	var pid uint32
	windows.GetWindowThreadProcessId(windows.HWND(hwnd), &pid)
	if pid != enumPID || !windows.IsWindowVisible(windows.HWND(hwnd)) {
		return 1
	}
	title := windowTitle(windows.HWND(hwnd))
	if title == "" {
		return 1
	}
	var rc rect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	w, h := rc.Right-rc.Left, rc.Bottom-rc.Top
	if w < 200 || h < 200 {
		return 1
	}
	enumCands = append(enumCands, hwndCand{windows.HWND(hwnd), title, w * h})
	return 1
}

func windowTitle(hwnd windows.HWND) string {
	buf := make([]uint16, 512)
	n, _, _ := procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), 512)
	if n == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

func FindGameWindow(pid uint32) (windows.HWND, string, error) {
	enumPID = pid
	enumCands = nil
	_ = windows.EnumWindows(enumCB, unsafe.Pointer(nil))
	if len(enumCands) == 0 {
		return 0, "", fmt.Errorf("未找到游戏窗口，请改用窗口化/窗口最大化")
	}
	best := enumCands[0]
	for _, c := range enumCands[1:] {
		if c.area > best.area {
			best = c
		}
	}
	return best.hwnd, best.title, nil
}

func FocusGame(pid uint32) error {
	hwnd, _, err := FindGameWindow(pid)
	if err != nil {
		return err
	}
	procShowWindow.Call(uintptr(hwnd), swRestore)
	fg := windows.GetForegroundWindow()
	fgTid, _ := windows.GetWindowThreadProcessId(fg, nil)
	curTid, _, _ := procGetCurrentThreadId.Call()
	procAttachThreadInput.Call(uintptr(fgTid), curTid, 1)
	// 先点一下 Alt，绕过前台锁
	sendKey(vkMenu, false)
	sendKey(vkMenu, true)
	procSetForegroundWindow.Call(uintptr(hwnd))
	procBringWindowToTop.Call(uintptr(hwnd))
	procAttachThreadInput.Call(uintptr(fgTid), curTid, 0)
	time.Sleep(200 * time.Millisecond)
	return nil
}

func sendInput(in kbInput) {
	procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
}

func sendKey(vk uint16, up bool) {
	in := kbInput{Type: inputKeyboard, Vk: vk}
	if up {
		in.Flags = keyeventfKeyup
	}
	sendInput(in)
}

func tap(vk uint16) {
	sendKey(vk, false)
	time.Sleep(20 * time.Millisecond)
	sendKey(vk, true)
}

func chord(mod, vk uint16) {
	sendKey(mod, false)
	time.Sleep(15 * time.Millisecond)
	sendKey(vk, false)
	time.Sleep(20 * time.Millisecond)
	sendKey(vk, true)
	time.Sleep(15 * time.Millisecond)
	sendKey(mod, true)
}

func SetClipboardText(s string) error {
	u16, err := windows.UTF16FromString(s)
	if err != nil {
		return err
	}
	bytes := len(u16) * 2
	mem, _, err2 := procGlobalAlloc.Call(gmemMoveable, uintptr(bytes))
	if mem == 0 {
		return fmt.Errorf("GlobalAlloc: %v", err2)
	}
	ptr, _, err2 := procGlobalLock.Call(mem)
	if ptr == 0 {
		return fmt.Errorf("GlobalLock: %v", err2)
	}
	procRtlMoveMemory.Call(ptr, uintptr(unsafe.Pointer(&u16[0])), uintptr(len(u16)*2))
	runtime.KeepAlive(u16)
	procGlobalUnlock.Call(mem)

	for i := 0; i < 10; i++ {
		r, _, _ := procOpenClipboard.Call(0)
		if r != 0 {
			break
		}
		if i == 9 {
			return fmt.Errorf("OpenClipboard failed")
		}
		time.Sleep(30 * time.Millisecond)
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()
	r, _, err2 := procSetClipboardData.Call(cfUnicodeText, mem)
	if r == 0 {
		return fmt.Errorf("SetClipboardData: %v", err2)
	}
	return nil
}

func GetClipboardText() (string, error) {
	for i := 0; i < 10; i++ {
		r, _, _ := procOpenClipboard.Call(0)
		if r != 0 {
			break
		}
		if i == 9 {
			return "", fmt.Errorf("OpenClipboard failed")
		}
		time.Sleep(30 * time.Millisecond)
	}
	defer procCloseClipboard.Call()
	h, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return "", nil
	}
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return "", fmt.Errorf("GlobalLock failed")
	}
	defer procGlobalUnlock.Call(h)
	size, _, _ := procGlobalSize.Call(h)
	if size < 2 {
		return "", nil
	}
	n := int(size / 2)
	buf := make([]uint16, n)
	procRtlMoveMemory.Call(uintptr(unsafe.Pointer(&buf[0])), ptr, size)
	runtime.KeepAlive(buf)
	return windows.UTF16ToString(buf), nil
}

// SendChat 打开聊天框、粘贴并发送。初始化定位时使用。
func SendChat(pid uint32, text string) error {
	if err := FocusGame(pid); err != nil {
		return err
	}
	if err := SetClipboardText(text); err != nil {
		return err
	}
	tap(vkReturn)
	time.Sleep(180 * time.Millisecond)
	chord(vkControl, vkV)
	time.Sleep(120 * time.Millisecond)
	tap(vkReturn)
	time.Sleep(450 * time.Millisecond)
	return nil
}

// CaptureChatInput 全选并复制当前输入框。
func CaptureChatInput(pid uint32) (string, error) {
	if err := FocusGame(pid); err != nil {
		return "", err
	}
	chord(vkControl, vkA)
	time.Sleep(40 * time.Millisecond)
	chord(vkControl, vkC)
	time.Sleep(80 * time.Millisecond)
	return GetClipboardText()
}

func PasteToGame(pid uint32, text string) error {
	if err := FocusGame(pid); err != nil {
		return err
	}
	if err := SetClipboardText(text); err != nil {
		return err
	}
	chord(vkControl, vkA)
	time.Sleep(40 * time.Millisecond)
	chord(vkControl, vkV)
	return nil
}
