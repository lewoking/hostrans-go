//go:build windows

package memory

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

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
	if err := windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL); err != nil {
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
		t, _ := windows.UTF16PtrFromString("HOSTrans 已在运行")
		c, _ := windows.UTF16PtrFromString("HOSTrans")
		_, _ = windows.MessageBox(0, t, c, windows.MB_OK|windows.MB_ICONINFORMATION)
		return fmt.Errorf("已有实例在运行")
	}
	return err
}
