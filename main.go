package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"hostrans/memory"
	"hostrans/monitor"
	"hostrans/translator"
	"hostrans/ui"
)

const GameProcessName = "HeroesOfTheStorm_x64.exe"

func main() {
	fmt.Println("========================================")
	fmt.Println("  HOSTrans Go  |  无 Key 开箱即用")
	fmt.Println("  引擎: Microsoft → DeepLX → Youdao")
	fmt.Println("========================================")
	fmt.Printf("当前系统: %s/%s\n\n", runtime.GOOS, runtime.GOARCH)

	trans := translator.NewManager()
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println("请选择功能:")
		fmt.Println("  1. 测试翻译（中 → 韩）")
		fmt.Println("  2. 测试翻译（韩 → 中）")
		fmt.Println("  3. 测试翻译（中 → 英）")
		fmt.Println("  4. 启动监控 + 悬浮窗 (Windows)")
		fmt.Println("  5. 退出")
		fmt.Print("输入选项: ")

		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			testTranslate(trans, reader, "zh", "ko")
		case "2":
			testTranslate(trans, reader, "ko", "zh")
		case "3":
			testTranslate(trans, reader, "zh", "en")
		case "4":
			startMonitor(trans)
		case "5", "q", "quit", "exit":
			fmt.Println("再见")
			return
		default:
			fmt.Println("无效选项")
		}
		fmt.Println()
	}
}

func testTranslate(trans *translator.Manager, reader *bufio.Reader, from, to string) {
	fmt.Print("请输入要翻译的文本: ")
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(text)
	if text == "" {
		fmt.Println("文本为空")
		return
	}

	fmt.Printf("正在翻译 [%s → %s] ...\n", from, to)
	start := time.Now()
	result, err := trans.Translate(text, from, to)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("翻译失败: %v\n", err)
		return
	}
	fmt.Printf("结果 (%.2fs):\n%s\n", elapsed.Seconds(), result)
}

func startMonitor(trans *translator.Manager) {
	if runtime.GOOS != "windows" {
		fmt.Println("→ 监控和悬浮窗仅支持 Windows。请交叉编译后在游戏电脑运行。")
		return
	}

	fmt.Println("建议：管理员运行 + 游戏窗口化最大化 + 先进入对局。")
	fmt.Println("Ctrl+1 初始化（会自动发 3 条探测聊天）")
	fmt.Println("Ctrl+Tab 显示悬浮窗  ·  Ctrl+P 输入框中译韩")
	fmt.Println("点 × 关闭悬浮窗并返回菜单")

	memory.EnableDebugPrivilege()
	pid, err := memory.FindProcess(GameProcessName)
	if err != nil {
		fmt.Printf("未找到进程 %s: %v\n请先启动游戏。\n", GameProcessName, err)
		return
	}
	fmt.Printf("找到进程 PID = %d\n", pid)

	proc, err := memory.Open(pid)
	if err != nil {
		fmt.Printf("打开进程失败: %v\n", err)
		return
	}
	defer proc.Close()

	mon := monitor.New(proc, trans)
	ov := ui.NewOverlay()

	ov.OnLocate = func() {
		ov.Status("初始化中，不要操作键鼠…")
		ov.Show()
		if err := mon.Locate(ov.Status); err != nil {
			ov.Status(err.Error())
			ov.Show()
			return
		}
		ov.Status("已开始监控韩语聊天")
		ov.Show()
	}
	ov.OnTranslateInput = func() {
		mon.TranslateInput(ov.Status)
		ov.Show()
	}

	stop := make(chan struct{})
	go mon.Loop(800*time.Millisecond, ov, stop)
	_ = ov.Run()
	close(stop)
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
