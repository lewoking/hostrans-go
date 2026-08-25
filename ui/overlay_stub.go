//go:build !windows

package ui

import (
	"bufio"
	"fmt"
	"os"
	"sync"
)

type Overlay struct {
	mu    sync.Mutex
	lines []Line

	OnLocate         func()
	OnTranslateInput func()
}

func NewOverlay() *Overlay { return &Overlay{} }

func (o *Overlay) Push(speaker, text string) {
	o.mu.Lock()
	o.lines = append(o.lines, Line{Speaker: speaker, Text: text})
	if len(o.lines) > 6 {
		o.lines = o.lines[len(o.lines)-6:]
	}
	o.mu.Unlock()
	fmt.Printf("%s：%s\n", speaker, text)
}

func (o *Overlay) Status(msg string) {
	fmt.Println("[状态]", msg)
}

func (o *Overlay) Show() {}
func (o *Overlay) Stay() {}
func (o *Overlay) Hide() {}

func (o *Overlay) Run() error {
	fmt.Println("当前系统无法显示 Win32 悬浮窗。按 Enter 返回。")
	bufio.NewReader(os.Stdin).ReadString('\n')
	return nil
}

func (o *Overlay) Close() {}
