package main

import (
	"fmt"
	"log"
	"runtime"
	"time"

	"hostrans/memory"
	"hostrans/monitor"
	"hostrans/translator"
	"hostrans/ui"
)

func main() {
	fmt.Println("HOSTrans  |  无 Key  ·  选人/局内自动翻译")
	if runtime.GOOS != "windows" {
		fmt.Println("请在 Windows 上运行交叉编译出的 hostrans.exe")
		return
	}

	fmt.Println("管理员 + 窗口化最大化  ·  Ctrl+P 中译韩  ·  Ctrl+Tab 显示  ·  Ctrl+1 重定位  ·  × 退出")

	memory.EnableDebugPrivilege()
	mon := monitor.New(translator.NewManager())
	ov := ui.NewOverlay()

	ov.OnLocate = func() {
		ov.Status("强制重定位当前界面…")
		ov.Stay()
		if err := mon.Locate(ov.Status); err != nil {
			ov.Status(err.Error())
			ov.Stay()
			return
		}
		ov.Status("已监听当前场景")
		ov.Show()
	}
	ov.OnTranslateInput = func() {
		mon.TranslateInput(ov.Status)
		ov.Show()
	}

	stop := make(chan struct{})
	go mon.AutoInit(stop, ov)
	go mon.Loop(800*time.Millisecond, ov, stop)
	_ = ov.Run()
	close(stop)
	mon.CloseProc()
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
