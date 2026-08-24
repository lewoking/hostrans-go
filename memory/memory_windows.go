//go:build windows

package memory

import (
	"bytes"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	memCommit     = 0x1000
	memPrivate    = 0x20000
	pageReadWrite = 0x04
	maxScanRegion = 64 << 20 // 64MB，聊天缓冲区不会在巨型映射里
)

// Process 表示目标游戏进程
type Process struct {
	Handle windows.Handle
	PID    uint32
}

// FindProcess 根据进程名查找 PID
func FindProcess(name string) (uint32, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snapshot)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err = windows.Process32First(snapshot, &pe); err != nil {
		return 0, err
	}
	for {
		if windows.UTF16ToString(pe.ExeFile[:]) == name {
			return pe.ProcessID, nil
		}
		if err = windows.Process32Next(snapshot, &pe); err != nil {
			break
		}
	}
	return 0, fmt.Errorf("process %s not found", name)
}

// Open 打开进程句柄（只读内存）
func Open(pid uint32) (*Process, error) {
	access := uint32(windows.PROCESS_QUERY_INFORMATION | windows.PROCESS_VM_READ)
	handle, err := windows.OpenProcess(access, false, pid)
	if err != nil {
		access = windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.PROCESS_VM_READ
		handle, err = windows.OpenProcess(access, false, pid)
	}
	if err != nil {
		return nil, fmt.Errorf("OpenProcess: %w（请用管理员身份运行）", err)
	}
	return &Process{Handle: handle, PID: pid}, nil
}

func (p *Process) Close() {
	if p != nil && p.Handle != 0 {
		windows.CloseHandle(p.Handle)
		p.Handle = 0
	}
}

func (p *Process) ReadMemory(address uintptr, size uint) ([]byte, error) {
	if size == 0 {
		return nil, nil
	}
	buf := make([]byte, size)
	var read uintptr
	err := windows.ReadProcessMemory(p.Handle, address, &buf[0], uintptr(size), &read)
	if err != nil {
		return nil, err
	}
	return buf[:read], nil
}

func (p *Process) ReadString(address uintptr, maxLen int, encoding string) (string, error) {
	if encoding == "utf-16le" || encoding == "utf-16" {
		raw, err := p.ReadMemory(address, uint(maxLen*2))
		if err != nil {
			return "", err
		}
		for i := 0; i+1 < len(raw); i += 2 {
			if raw[i] == 0 && raw[i+1] == 0 {
				raw = raw[:i]
				break
			}
		}
		u16 := make([]uint16, len(raw)/2)
		for i := range u16 {
			u16[i] = uint16(raw[i*2]) | uint16(raw[i*2+1])<<8
		}
		return syscall.UTF16ToString(u16), nil
	}
	raw, err := p.ReadMemory(address, uint(maxLen))
	if err != nil {
		return "", err
	}
	if idx := bytes.IndexByte(raw, 0); idx >= 0 {
		raw = raw[:idx]
	}
	return string(raw), nil
}

func (p *Process) queryMBI(addr uintptr) (windows.MemoryBasicInformation, error) {
	var mbi windows.MemoryBasicInformation
	err := windows.VirtualQueryEx(p.Handle, addr, &mbi, unsafe.Sizeof(mbi))
	return mbi, err
}

// ScanPrivateRW 只扫已提交的私有可写堆，适合定位聊天缓冲区。
func (p *Process) ScanPrivateRW(pattern []byte) ([]uintptr, error) {
	if len(pattern) == 0 {
		return nil, fmt.Errorf("empty pattern")
	}
	var results []uintptr
	var addr uintptr
	for {
		mbi, err := p.queryMBI(addr)
		if err != nil || mbi.RegionSize == 0 {
			break
		}
		next := mbi.BaseAddress + mbi.RegionSize
		if mbi.State == memCommit &&
			mbi.Type == memPrivate &&
			mbi.Protect&pageReadWrite != 0 &&
			mbi.RegionSize > 0 &&
			mbi.RegionSize <= maxScanRegion {
			data, err := p.ReadMemory(mbi.BaseAddress, uint(mbi.RegionSize))
			if err == nil {
				offset := 0
				for {
					idx := bytes.Index(data[offset:], pattern)
					if idx < 0 {
						break
					}
					results = append(results, mbi.BaseAddress+uintptr(offset+idx))
					offset += idx + 1
				}
			}
		}
		if next <= addr {
			break
		}
		addr = next
	}
	return results, nil
}

// FilterContains 从候选地址中筛出当前仍以 pattern 开头的位置。
func (p *Process) FilterContains(addrs []uintptr, pattern []byte) []uintptr {
	out := make([]uintptr, 0, len(addrs))
	n := uint(len(pattern))
	for _, a := range addrs {
		b, err := p.ReadMemory(a, n)
		if err == nil && bytes.Equal(b, pattern) {
			out = append(out, a)
		}
	}
	return out
}

func (p *Process) Alive() bool {
	if p == nil || p.Handle == 0 {
		return false
	}
	var code uint32
	if err := windows.GetExitCodeProcess(p.Handle, &code); err != nil {
		return false
	}
	const stillActive = 259 // STILL_ACTIVE
	return code == stillActive
}

// EnableDebugPrivilege 尽量提升权限，便于读取游戏进程。
func EnableDebugPrivilege() {
	var tok windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok)
	if err != nil {
		return
	}
	defer tok.Close()
	name, err := windows.UTF16PtrFromString("SeDebugPrivilege")
	if err != nil {
		return
	}
	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, name, &luid); err != nil {
		return
	}
	tp := windows.Tokenprivileges{
		PrivilegeCount: 1,
		Privileges:     [1]windows.LUIDAndAttributes{{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}},
	}
	_ = windows.AdjustTokenPrivileges(tok, false, &tp, 0, nil, nil)
}
