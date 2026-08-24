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
	"hostrans/translator"
)

const (
	GameProcessName = "HeroesOfTheStorm_x64.exe"
)

func main() {
	fmt.Println("========================================")
	fmt.Println("  HOSTrans Go 版  |  无 Key 开箱即用")
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
		fmt.Println("  4. 初始化游戏内存扫描 (仅 Windows)")
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
			if runtime.GOOS != "windows" {
				fmt.Println("→ 内存扫描功能仅支持 Windows 系统")
				continue
			}
			initMemory()
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

func initMemory() {
	fmt.Println("正在查找游戏进程...")
	pid, err := memory.FindProcess(GameProcessName)
	if err != nil {
		fmt.Printf("未找到进程 %s: %v\n", GameProcessName, err)
		fmt.Println("请先启动《风暴英雄》并进入游戏。")
		return
	}
	fmt.Printf("找到进程 PID = %d\n", pid)

	proc, err := memory.Open(pid)
	if err != nil {
		fmt.Printf("打开进程失败: %v\n", err)
		return
	}
	defer proc.Close()
	fmt.Println("进程句柄已打开。")

	fmt.Println("\n注意：完整的聊天地址定位需要根据原版特征码进行扫描。")
	fmt.Println("当前版本已具备内存读取能力，后续可补充具体 pattern。")
	fmt.Println("现在可以先用菜单 1/2/3 测试无 Key 翻译引擎。")
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
