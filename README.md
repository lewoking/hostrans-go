# HOSTrans

风暴英雄韩服聊天翻译 · **无 Key · 开箱即用 · 单文件**

```mermaid
flowchart TB
    Start(["hostrans.exe"]) --> Wait["等待游戏"]
    Wait -->|"空聊天框 Ctrl+P"| Loc["回车→输入探测→回车"]
    Loc --> Poll["监听韩语发言"]
    Poll --> ZH["说话人：中文译文"]
    ZH --> OV["透明常驻悬浮窗"]
    OV -->|"Ctrl+P 有中文"| Paste["回车开框后 Unicode 输入韩文"]
```

Windows：[Releases](https://github.com/lewoking/hostrans-go/releases/latest) 管理员运行，窗口化最大化。

- 悬浮窗常驻、背景全透明
- Ctrl+P：有中文 → 中译韩；空框 → 初始化（回车、打字、回车，不用 Ctrl+V）
- 翻译：Google → DeepLX

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w -H windowsgui" -o hostrans.exe .
```
