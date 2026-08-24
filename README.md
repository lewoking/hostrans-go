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

Windows：[Releases](https://github.com/lewoking/hostrans-go/releases/latest) 双击后会要管理员。窗口化最大化。选人/局内自动跟。Ctrl+P 中译韩。

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w -H windowsgui" -o hostrans.exe .
```
