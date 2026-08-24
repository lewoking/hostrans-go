# HOSTrans Go 版

风暴英雄（Heroes of the Storm）韩服聊天翻译工具  
**无 Key · 开箱即用 · 单文件运行**

---

## 一、快速开始（Windows）

1. 下载 `hostrans.exe`
2. 双击运行，或在命令行执行：
   ```bash
   hostrans.exe
   ```
3. 出现菜单后按数字选择功能：

```
========================================
  HOSTrans Go 版  |  无 Key 开箱即用
  引擎: Microsoft → DeepLX → Youdao
========================================

请选择功能:
  1. 测试翻译（中 → 韩）
  2. 测试翻译（韩 → 中）
  3. 测试翻译（中 → 英）
  4. 初始化游戏内存扫描 (仅 Windows)
  5. 退出
```

### 常用操作

| 选项 | 作用 | 说明 |
|------|------|------|
| 1 | 中文 → 韩语 | 输入中文，返回韩语翻译 |
| 2 | 韩语 → 中文 | 输入韩语，返回中文翻译 |
| 3 | 中文 → 英语 | 输入中文，返回英语翻译 |
| 4 | 初始化内存 | 查找并打开游戏进程（仅 Windows） |
| 5 | 退出 | 关闭程序 |

---

## 二、翻译引擎说明（无 Key）

程序内置三个免费引擎，按顺序自动尝试，成功即返回结果：

1. **Microsoft**（公共接口）
2. **DeepLX**（公共实例）
3. **Youdao**（有道网页接口）

**不需要申请任何 API Key，也不需要配置文件。**

如果公共接口暂时失效，最稳的做法是自己用 Docker 跑一个本地 DeepLX（可选）：

```bash
docker run -d -p 1188:1188 ghcr.io/owo-network/deeplx:latest
```

然后后续可支持把地址改成 `http://127.0.0.1:1188`。

---

## 三、交叉编译（在 Linux / macOS 上）

本项目是纯 Go 实现（无 CGO），可在 Linux 或 macOS 上直接编译出 Windows 可执行文件。

```bash
# 进入项目目录
cd hostrans-go

# 编译 Windows 64 位（推荐）
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o hostrans.exe .

# 可选：编译 Windows 32 位
GOOS=windows GOARCH=386 CGO_ENABLED=0 go build -ldflags="-s -w" -o hostrans-386.exe .
```

编译完成后得到 `hostrans.exe`，复制到 Windows 电脑即可使用。

---

## 四、项目结构

```
hostrans-go/
├── main.go                  # 入口程序 + 菜单
├── translator/
│   └── translator.go        # 无 Key 多引擎翻译实现
├── memory/
│   ├── memory_windows.go    # Windows 内存读取
│   └── memory_stub.go       # 非 Windows 占位
├── go.mod
├── go.sum
├── hostrans.exe             # 已编译好的 Windows 程序
└── README.md                # 本说明
```

---

## 五、当前功能状态

| 功能 | 状态 | 说明 |
|------|------|------|
| 无 Key 翻译 | ✅ 可用 | 多引擎自动回退 |
| 中韩 / 中英互译 | ✅ 可用 | 菜单 1/2/3 |
| 游戏进程查找 | ✅ 可用 | 选项 4 |
| 内存读取底层 | ✅ 已实现 | 支持读字符串、扫描特征 |
| 自动监控聊天 | ⏳ 待完善 | 需要根据当前游戏版本补充特征码 |
| 透明悬浮窗 / 热键 | ⏳ 待完善 | 后续可加 |

---

## 六、注意事项

1. 游戏内存相关功能**仅 Windows** 可用（游戏本身也是 Windows 客户端）。
2. 公共翻译接口有时会变化或限流，属于正常现象。
3. 本工具仅供学习交流，请遵守游戏服务条款。
4. 内存读取功能可能随游戏更新失效，届时需要更新特征码。

---

## 七、后续可扩展方向

- 完善聊天地址自动定位 + 实时监控翻译
- 增加全局热键（Ctrl+P 等）
- 增加透明置顶悬浮窗
- 支持配置文件自定义 DeepLX 地址
- 支持更多无 Key 引擎备用

有需要继续完善的地方直接说即可。
