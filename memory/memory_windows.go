//go:build windows

package memory

import (
	"bytes"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const GameProcessName = "HeroesOfTheStorm_x64.exe"

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
	hits, err := p.ScanPrivateRWMulti([][]byte{pattern})
	if err != nil {
		return nil, err
	}
	return hits[0], nil
}

// ScanPrivateRWMulti 一次遍历堆，同时搜多组字节串。
func (p *Process) ScanPrivateRWMulti(patterns [][]byte) ([][]uintptr, error) {
	hits := make([][]uintptr, len(patterns))
	if len(patterns) == 0 {
		return hits, nil
	}
	var addr uintptr
	var scanned, skipped int
	var nbytes uint64
	for {
		mbi, err := p.queryMBI(addr)
		if err != nil || mbi.RegionSize == 0 {
			break
		}
		next := mbi.BaseAddress + mbi.RegionSize
		if !ShouldScanRegion(uint32(mbi.State), uint32(mbi.Type), uint32(mbi.Protect), mbi.RegionSize) {
			skipped++
			if next <= addr {
				break
			}
			addr = next
			continue
		}
		scanned++
		base := mbi.BaseAddress
		remain := mbi.RegionSize
		for remain > 0 {
			chunk := remain
			if chunk > scanChunk {
				chunk = scanChunk
			}
			data, err := p.ReadMemory(base, uint(chunk))
			if err == nil && len(data) > 0 {
				nbytes += uint64(len(data))
				for i, pat := range patterns {
					if len(pat) == 0 {
						continue
					}
					offset := 0
					for {
						idx := bytes.Index(data[offset:], pat)
						if idx < 0 {
							break
						}
						hits[i] = append(hits[i], base+uintptr(offset+idx))
						offset += idx + 1
					}
				}
			}
			base += chunk
			remain -= chunk
		}
		if next <= addr {
			break
		}
		addr = next
	}
	n0, n1 := 0, 0
	if len(hits) > 0 {
		n0 = len(hits[0])
	}
	if len(hits) > 1 {
		n1 = len(hits[1])
	}
	debugMem("scan regions=%d skipped=%d bytes=%d hits0=%d hits1=%d", scanned, skipped, nbytes, n0, n1)
	return hits, nil
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
