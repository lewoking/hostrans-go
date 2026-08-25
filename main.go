package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"hostrans/memory"
	"hostrans/monitor"
	"hostrans/translator"
	"hostrans/ui"
)

func main() {
	if runtime.GOOS == "windows" {
		memory.ElevateIfNeeded()
		if memory.WantQuit() {
			if err := memory.RequestQuit(); err != nil {
				os.Exit(1)
			}
			return
		}
		if err := memory.EnsureSingleInstance(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	fmt.Println("HOSTrans  |  无 Key  ·  选人/局内自动翻译")
	if runtime.GOOS != "windows" {
		fmt.Println("请在 Windows 上运行交叉编译出的 hostrans.exe")
		return
	}
	if !memory.IsAdmin() {
		fmt.Println("未取得管理员权限，内存读取可能失败。")
	}

	fmt.Println("窗口化最大化  ·  Ctrl+P 中译韩（空框则初始化）")

	memory.EnableDebugPrivilege()
	mon := monitor.New(translator.NewManager())
	ov := ui.NewOverlay()

	ov.OnTranslateInput = func() {
		ov.Stay()
		mon.TranslateInput(ov.Status)
		ov.Show()
	}

	qh, _ := memory.CreateQuitEvent()
	defer memory.CloseQuitEvent(qh)

	stop := make(chan struct{})
	go mon.RunTranslator(stop, ov)
	go mon.AutoInit(stop, ov)
	go mon.Loop(800*time.Millisecond, ov, stop)
	go memory.WaitQuit(qh, ov.Close)
	_ = ov.Run()
	close(stop)
	mon.CloseProc()
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
