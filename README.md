# HOSTrans

风暴英雄韩服聊天翻译 · **无 Key · 开箱即用 · 单文件**

```mermaid
flowchart TB
    Start(["hostrans.exe 管理员运行"]) --> Menu{菜单}

    Menu -->|"1/2/3 测试翻译"| In[输入文本]
    In --> Eng[Microsoft → DeepLX → Youdao]
    Eng --> Out([译文])

    Menu -->|"4 监控 + 悬浮窗"| Auto["自动定位聊天缓冲"]
    Auto --> Draft[选人阶段]
    Auto --> InGame[游戏内]
    Draft --> Poll["监听多处缓冲"]
    InGame --> Poll
    Poll -->|场景切换地址失效| Auto
    Poll --> Parse["说话人：中文译文"]
    Parse --> OV[透明置顶窗]
    OV -->|"Ctrl+P"| Paste["输入框 中 → 韩"]

    Menu -->|5| End([退出])
```

Windows：[Releases](https://github.com/lewoking/hostrans-go/releases/latest) 下载后 **管理员运行**，游戏用**窗口化最大化**。选人、局内都会跟；进局或回选人会自动重定位。Ctrl+1 仅在跟丢时手动补一次。

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o hostrans.exe .
```
