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
	PROCESS_VM_READ           = 0x0010
	PROCESS_QUERY_INFORMATION = 0x0400
	MEM_COMMIT                = 0x1000
	PAGE_READWRITE            = 0x04
	PAGE_READONLY             = 0x02
	PAGE_EXECUTE_READ         = 0x20
	PAGE_EXECUTE_READWRITE    = 0x40
)

var (
	modKernel32          = windows.NewLazySystemDLL("kernel32.dll")
	procOpenProcess      = modKernel32.NewProc("OpenProcess")
	procReadProcessMemory = modKernel32.NewProc("ReadProcessMemory")
	procVirtualQueryEx   = modKernel32.NewProc("VirtualQueryEx")
	procCloseHandle      = modKernel32.NewProc("CloseHandle")
)

type MEMORY_BASIC_INFORMATION struct {
	BaseAddress       uintptr
	AllocationBase    uintptr
	AllocationProtect uint32
	RegionSize        uintptr
	State             uint32
	Protect           uint32
	Type              uint32
}

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
	err = windows.Process32First(snapshot, &pe)
	if err != nil {
		return 0, err
	}
	for {
		exe := windows.UTF16ToString(pe.ExeFile[:])
		if exe == name {
			return pe.ProcessID, nil
		}
		err = windows.Process32Next(snapshot, &pe)
		if err != nil {
			break
		}
	}
	return 0, fmt.Errorf("process %s not found", name)
}

// Open 打开进程句柄
func Open(pid uint32) (*Process, error) {
	handle, _, err := procOpenProcess.Call(
		uintptr(PROCESS_QUERY_INFORMATION|PROCESS_VM_READ),
		0,
		uintptr(pid),
	)
	if handle == 0 {
		return nil, fmt.Errorf("OpenProcess failed: %v", err)
	}
	return &Process{Handle: windows.Handle(handle), PID: pid}, nil
}

// Close 关闭句柄
func (p *Process) Close() {
	if p.Handle != 0 {
		procCloseHandle.Call(uintptr(p.Handle))
		p.Handle = 0
	}
}

// ReadMemory 读取指定地址的内存
func (p *Process) ReadMemory(address uintptr, size uint) ([]byte, error) {
	buf := make([]byte, size)
	var bytesRead uintptr
	ret, _, err := procReadProcessMemory.Call(
		uintptr(p.Handle),
		address,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(size),
		uintptr(unsafe.Pointer(&bytesRead)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("ReadProcessMemory failed: %v", err)
	}
	return buf[:bytesRead], nil
}

// ReadString 读取以 null 结尾的字符串（支持 utf-8 / utf-16le）
func (p *Process) ReadString(address uintptr, maxLen int, encoding string) (string, error) {
	if encoding == "utf-16le" || encoding == "utf-16" {
		raw, err := p.ReadMemory(address, uint(maxLen*2))
		if err != nil {
			return "", err
		}
		// 找 \x00\x00 结束
		for i := 0; i+1 < len(raw); i += 2 {
			if raw[i] == 0 && raw[i+1] == 0 {
				raw = raw[:i]
				break
			}
		}
		// 简单 utf16 转 string
		u16 := make([]uint16, len(raw)/2)
		for i := 0; i < len(u16); i++ {
			u16[i] = uint16(raw[i*2]) | uint16(raw[i*2+1])<<8
		}
		return syscall.UTF16ToString(u16), nil
	}

	// utf-8
	raw, err := p.ReadMemory(address, uint(maxLen))
	if err != nil {
		return "", err
	}
	if idx := bytes.IndexByte(raw, 0); idx >= 0 {
		raw = raw[:idx]
	}
	return string(raw), nil
}

// ScanPattern 在可读写内存区域搜索字节模式
func (p *Process) ScanPattern(pattern []byte) ([]uintptr, error) {
	var results []uintptr
	var address uintptr = 0
	mbi := MEMORY_BASIC_INFORMATION{}

	for {
		ret, _, _ := procVirtualQueryEx.Call(
			uintptr(p.Handle),
			address,
			uintptr(unsafe.Pointer(&mbi)),
			unsafe.Sizeof(mbi),
		)
		if ret == 0 {
			break
		}
		if mbi.RegionSize == 0 {
			break
		}

		// 只扫描已提交且可读的区域
		if mbi.State == MEM_COMMIT &&
			(mbi.Protect&PAGE_READWRITE != 0 ||
				mbi.Protect&PAGE_READONLY != 0 ||
				mbi.Protect&PAGE_EXECUTE_READ != 0 ||
				mbi.Protect&PAGE_EXECUTE_READWRITE != 0) {

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

		address = mbi.BaseAddress + mbi.RegionSize
		if address < mbi.BaseAddress { // overflow
			break
		}
	}
	return results, nil
}
