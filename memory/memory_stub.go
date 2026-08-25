//go:build !windows

package memory

import "fmt"

const GameProcessName = "HeroesOfTheStorm_x64.exe"

type Process struct {
	PID uint32
}

func FindProcess(name string) (uint32, error) {
	return 0, fmt.Errorf("memory scanning only supported on Windows")
}

func Open(pid uint32) (*Process, error) {
	return nil, fmt.Errorf("memory scanning only supported on Windows")
}

func (p *Process) Close() {}

func (p *Process) ReadMemory(address uintptr, size uint) ([]byte, error) {
	return nil, fmt.Errorf("not supported")
}

func (p *Process) ReadString(address uintptr, maxLen int, encoding string) (string, error) {
	return "", fmt.Errorf("not supported")
}

func (p *Process) ScanPrivateRW(pattern []byte) ([]uintptr, error) {
	return nil, fmt.Errorf("not supported")
}

func (p *Process) ScanPrivateRWMulti(patterns [][]byte) ([][]uintptr, error) {
	return nil, fmt.Errorf("not supported")
}

func (p *Process) Alive() bool { return false }
