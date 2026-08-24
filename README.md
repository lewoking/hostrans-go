# HOSTrans

风暴英雄韩服聊天翻译 · **无 Key · 开箱即用 · 单文件**

```mermaid
flowchart TB
    Start(["hostrans.exe 管理员运行"]) --> Menu{菜单}

    Menu -->|"1/2/3 测试翻译"| In[输入文本]
    In --> Eng[Microsoft → DeepLX → Youdao]
    Eng --> Out([译文])

    Menu -->|"4 监控 + 悬浮窗"| OV["透明置顶窗"]
    OV -->|"Ctrl+1"| Loc["发 3 次探测串 → 私有堆求交"]
    Loc --> Poll["每 0.8s 读聊天缓冲"]
    Poll --> Parse["解析 说话人 + 正文"]
    Parse --> KO{含韩文?}
    KO -->|是| ZH["说话人：中文译文"]
    ZH --> OV
    OV -->|"Ctrl+P"| Paste["输入框 中 → 韩"]
    OV -->|"Ctrl+Tab / 4.5s"| Hide[显示 / 自动隐藏]

    Menu -->|5| End([退出])
```

Windows：[Releases](https://github.com/lewoking/hostrans-go/releases/latest) 下载后 **管理员运行**。游戏请用**窗口化最大化**，进对局后按 **Ctrl+1** 初始化。

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o hostrans.exe .
```
