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

	fmt.Println("窗口化最大化  ·  Ctrl+P 译出  ·  Ctrl+L 韩/英  ·  Ctrl+Tab 显示  ·  Ctrl+1 重定位")

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
	ov.OnSwitchLang = func() {
		name := mon.ToggleOutLang()
		ov.Status("译出语言: 中 → " + name)
		ov.Show()
	}

	stop := make(chan struct{})
	go mon.RunTranslator(stop, ov)
	go mon.AutoInit(stop, ov)
	go mon.Loop(800*time.Millisecond, ov, stop)
	_ = ov.Run()
	close(stop)
	mon.CloseProc()
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
