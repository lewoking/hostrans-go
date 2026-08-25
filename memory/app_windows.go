//go:build windows

package memory

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

const quitEventName = "Local\\HOSTransGo.Quit"

var instanceMu windows.Handle

func IsAdmin() bool {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &tok); err != nil {
		return false
	}
	defer tok.Close()
	return tok.IsElevated()
}

// ElevateIfNeeded 非管理员则 UAC 提权重启。用户取消则原进程继续。
func ElevateIfNeeded() {
	if IsAdmin() {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	var params *uint16
	if len(os.Args) > 1 {
		params, _ = windows.UTF16PtrFromString(strings.Join(os.Args[1:], " "))
	}
	if err := windows.ShellExecute(0, verb, file, params, nil, windows.SW_SHOWNORMAL); err != nil {
		return
	}
	os.Exit(0)
}

// EnsureSingleInstance 防止双开导致热键冲突、重复探测刷屏。
func EnsureSingleInstance() error {
	name, err := windows.UTF16PtrFromString("Local\\HOSTransGo.Mutex")
	if err != nil {
		return err
	}
	h, err := windows.CreateMutex(nil, false, name)
	instanceMu = h
	if err == windows.ERROR_ALREADY_EXISTS {
		t, _ := windows.UTF16PtrFromString("HOSTrans 已在运行。是否退出？")
		c, _ := windows.UTF16PtrFromString("HOSTrans")
		r, _ := windows.MessageBox(0, t, c, windows.MB_YESNO|windows.MB_ICONQUESTION)
		if r == 6 { // IDYES
			_ = RequestQuit()
		}
		return fmt.Errorf("已有实例在运行")
	}
	return err
}

func CreateQuitEvent() (QuitHandle, error) {
	name, err := windows.UTF16PtrFromString(quitEventName)
	if err != nil {
		return 0, err
	}
	h, err := windows.CreateEvent(nil, 1, 0, name)
	return QuitHandle(h), err
}

func CloseQuitEvent(h QuitHandle) {
	if h != 0 {
		_ = windows.CloseHandle(windows.Handle(h))
	}
}

func RequestQuit() error {
	name, err := windows.UTF16PtrFromString(quitEventName)
	if err != nil {
		return err
	}
	h, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.SetEvent(h)
}

func WaitQuit(h QuitHandle, onQuit func()) {
	if h == 0 || onQuit == nil {
		return
	}
	_, _ = windows.WaitForSingleObject(windows.Handle(h), windows.INFINITE)
	onQuit()
}
