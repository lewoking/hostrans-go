# HOSTrans

风暴英雄韩服聊天翻译 · **无 Key · 开箱即用 · 单文件**

```mermaid
flowchart TB
    Start(["hostrans.exe 管理员运行"]) --> Auto["自动定位聊天缓冲"]
    Auto --> Draft[选人阶段]
    Auto --> InGame[游戏内]
    Draft --> Poll["监听多处缓冲"]
    InGame --> Poll
    Poll -->|场景切换地址失效| Auto
    Poll --> ZH["说话人：中文译文"]
    ZH --> OV[透明置顶窗]
    OV -->|"Ctrl+P"| Paste["输入框 中 → 韩"]
```

Windows：[Releases](https://github.com/lewoking/hostrans-go/releases/latest) 下载后 **管理员运行**，游戏用**窗口化最大化**。双击即监控，选人/局内都会跟。Ctrl+P 中译韩成功就说明翻译引擎可用。

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o hostrans.exe .
```
