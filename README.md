# HOSTrans

风暴英雄韩服聊天翻译 · **无 Key · 开箱即用 · 单文件**

```mermaid
flowchart TB
    Start(["hostrans.exe 管理员运行"]) --> Wait["等待对局"]
    Wait -->|battlelobby 文件出现| Tip["检测到对局，即将初始化"]
    Tip --> Loc["定位聊天缓冲"]
    Loc --> Poll["监听发言"]
    Poll --> ZH["说话人：中文译文"]
    ZH --> OV[透明置顶窗]
    OV -->|"Ctrl+P 有中文"| Paste["输入框 中 → 韩"]
    OV -->|"Ctrl+P 空框"| Loc
```

Windows：[Releases](https://github.com/lewoking/hostrans-go/releases/latest) 双击后会要管理员。窗口化最大化。新对局写出 battlelobby 后提示并自动初始化（忽略大厅残留旧文件）。Ctrl+P：有中文则译韩，空框则探测定位。日志：`%TEMP%\hostrans.log`。

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w -H windowsgui" -o hostrans.exe .
```
