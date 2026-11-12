# robopilot-mcp

[![English](https://img.shields.io/badge/Language-English-blue.svg)](./README.md) [![中文](https://img.shields.io/badge/Language-%E4%B8%AD%E6%96%87-red.svg)](./README_zh.md)

一个用 Go 编写的**桌面自动化 MCP 服务器**：既能"看见"屏幕（无障碍树 / OCR / 像素视觉），
也能"动手"操作（鼠标 / 键盘 / 窗口 / 剪贴板），并通过
[Model Context Protocol](https://modelcontextprotocol.io) 把这些能力暴露给任意 MCP 客户端
（Claude Desktop、自带的 RobotPilot 智能体，或你自己的 Agent）。

感知与窗口管理部分基于 [`neru-core`](https://github.com/LouYuanbo1/neru-core)：纯 Go、无 CGo，
在进程内直连系统接口（Windows UIA / macOS AX / Linux AT-SPI + 屏幕捕获 + OCR + 轮廓/分割检测 +
窗口枚举/激活/摆放）

## 工具集（14 个，一个意图一个工具）

### 感知（只读）

| 工具 | 说明 |
| --- | --- |
| `inspect_screen_tool` | **先调用它**。一次返回：桌面几何（分辨率/DPI/虚拟桌面/光标）、前台窗口（标题/PID/应用）、当前窗口的可交互元素（角色/名称/矩形/中心点）；可选 OCR 文字与截图落盘 |
| `read_text_tool` | OCR 读屏幕或区域；`words=true` 返回带坐标词块，可按文字位置点击 |
| `screenshot_tool` | 截图。`output` 决定产出形态：`file`（默认，落盘到 `~/robopilot-artifacts`）/ `inline`（**不落盘**，图片字节直接随结果返回）/ `both`（两者都要） |

> `inline`/`both` 只对支持看图的客户端有意义；纯文本模型（如当前内置 agent）保持默认 `file` 即可，
> 路径可直接给用户，也不会让几 MB 的 base64 白白经过一次传输。

`inspect_screen_tool` 的三个关键参数：

| 参数 | 用途 |
| --- | --- |
| `scope` | `window`（默认）当前窗口 / `application` 整个前台应用 / `app` 按 `application` 指定的应用 / `surface` 辅助表面（仅 macOS） |
| `query` / `roles` / `region` | 在元素很多的界面上收窄结果 |
| `source` / `strategy` | `source=vision` 改走像素视觉（`strategy=vision/contour/segment`），用于游戏、画布、远程桌面等读不到无障碍信息的界面 |

其中 `scope=app` 是"读别的应用"的关键：传入 `list_windows_tool` 返回的 `application`（如 `chrome.exe`），
**不切换窗口**就能读到该应用的内容，例如浏览器标签页标题。

### 窗口

| 工具 | 说明 |
| --- | --- |
| `list_windows_tool` | 列出所有可见顶层窗口：标题、PID、应用标识（Windows/Linux 是可执行文件名，macOS 是 bundle id）、窗口 id、屏幕位置、是否最小化、是否前台 |
| `set_active_window_tool` | 按 `id`（来自 list_windows，最精确）/ `pid`（最精确）/ `application`（如 chrome.exe）/ `title`（真正的窗口标题）激活；可选 `rect={x,y,width,height}` 同时把窗口移动到指定矩形，常用于左右并排看目标应用与日志 |

左右半屏不要自己猜屏幕尺寸：用 `inspect_screen_tool` 返回的 `virtual_desktop`（`dx,dy,dw,dh`）计算，
左半屏是 `{x:dx, y:dy, width:dw/2, height:dh}`，右半屏是 `{x:dx+dw/2, y:dy, width:dw/2, height:dh}`。
Windows 上也可以先激活窗口，再用 `key_tap_tool` 发 `win+left` / `win+right` / `win+up`，让系统按工作区与
多显示器规则贴靠或最大化；需要精确矩形时才用 `rect`。

### 操作

| 工具 | 说明 |
| --- | --- |
| `click_element_tool` | **优先用它**：按名称/角色/方向定位元素并点击，默认点完把光标放回原处 |
| `click_mouse_at_tool` | 按坐标点击（元素返回的 `center` 可直接用） |
| `move_mouse_tool` | 移动/悬停指针 |
| `mouse_button_tool` | `state=down/up` 按住或松开，配合移动实现分段拖拽、框选 |
| `drag_mouse_tool` / `scroll_mouse_tool` | 拖拽 / 滚轮（可选先移动指针到目标位置再滚） |
| `type_text_tool` / `key_tap_tool` | 键入文本 / 单键与组合快捷键 |
| `clipboard_tool` | 带 `text` 写入，不带则读取 |

### 推荐工作流

```
看：inspect_screen_tool()                       ← 一次拿全（含 virtual_desktop）
摆：set_active_window_tool(application="chrome.exe",
        rect={x:0, y:0, width:1080, height:1440})  ← 左半屏，与日志并排
点：click_element_tool(text="保存")             ← 知道名字就点名
    或 click_mouse_at_tool(x=…, y=…center…)     ← 只知道坐标
验证：inspect_screen_tool() / read_text_tool()
```

## 快速开始

```bash
# 只启动 MCP 服务器（默认监听 :8080）
go run ./cmd/server

# 自定义端口与感知参数
go run ./cmd/server -addr :9090 -perceive-max 32 -perceive-depth 12 -ocr-langs zh-Hans,en-US

# 终端 REPL 智能体（进程内自动拉起 MCP 服务器，需要 LLM_API_KEY）
set LLM_API_KEY=sk-xxxx && go run ./cmd/agent

# 不接模型，直接试某个动作
go run ./cmd/demo -act '{"actions":[{"type":"inspect_screen","max":10}]}'
go run ./cmd/demo -act '{"actions":[{"type":"inspect_screen","scope":"app","application":"chrome.exe"}]}'
go run ./cmd/demo -act '{"actions":[{"type":"click_element","text":"保存"}]}'
go run ./cmd/demo -act '{"actions":[{"type":"read_text","region":{"x":0,"y":0,"width":600,"height":200},"words":true}]}'
```

MCP 端点：`http://127.0.0.1:8080/mcp`（Streamable HTTP），健康检查：`GET /healthz`。

旧动作名（`perceive_elements` / `describe_screen` / `detect_targets` / `mouse_down` / `mouse_up` /
`get_clipboard` / `set_clipboard`）仍可解析，会映射到合并后的动作上。

### 接入 MCP 客户端

以 Claude Desktop 为例：

```json
{
  "mcpServers": {
    "robopilot": {
      "url": "http://127.0.0.1:8080/mcp"
    }
  }
}
```

### 平台权限

| 平台 | 需要做的事 |
| --- | --- |
| Windows | 无需额外授权（UIA 与 GDI 捕获都不弹窗） |
| macOS | 系统设置 → 隐私与安全性 → **辅助功能** 与 **屏幕录制**：勾选本程序 |
| Linux | 无特殊权限；Wayland 下部分合成器首次注入输入可能弹出一次授权 |

## 架构

```
action/                     纯协议层：Action / ActionResult / 声明式 JSON 解析与校验（零平台依赖）
executor/                   执行层：动作路由 + robotgo 驱动（鼠标/键盘/屏幕/剪贴板）
executor/perceiver/         感知契约层（纯逻辑，不碰平台）：
                            perceiver.go 能力接口 / element.go 元素模型 / request.go 请求与读数
                            window.go 窗口能力契约 / filter.go 过滤去重排序 / select.go 选优与空间导航
                            render.go 序列化 / ax.go 无障碍角色词汇解析
executor/perceiver/neru/    感知与窗口实现层（neru-core 适配）：
                            neru.go 构造与生命周期 / perceive.go 无障碍树扫描 / info.go 桌面状态
                            window.go 窗口枚举、激活与摆放 / text.go OCR / vision.go 视觉检测
                            pick.go 空间导航 / capture.go 屏幕捕获
server/                     MCP 服务层：注册工具、超时、日志、图片内容
agent/                      内置智能体：eino 工具 + 提示词，经 MCP 客户端调用服务器
cmd/server                  MCP 服务器入口
cmd/agent                   REPL 智能体入口
cmd/demo                    动作协议调试入口
```

设计要点：

- **协议与实现分离**：`action` 只描述"做什么"，不依赖任何平台库；感知类与窗口类动作由 `perceiver` 的实现承担，输入类动作由 `executor` 通过 robotgo 驱动。
- **能力按接口切分**：`perceiver` 用多个小接口描述能力（`Scoper` / `InfoProvider` / `TextReader` /
  `TargetDetector` / `SpatialPicker` / `WindowManager`），实现方多实现一个接口就自动多一项可用的动作，
  缺哪一项就明确报错。
- **窗口复杂度在 neru-core 里**：枚举、激活与摆放不写在各平台文件里，而是调用 neru-core 的 `window` 包；
  Win32 的 `EnumWindows`/`SetForegroundWindow`/`SetWindowPos`、macOS 的 AX 窗口与 `NSRunningApplication`、
  X11 的 EWMH 客户端消息都由它承担，executor 只依赖 `perceiver.WindowManager`。
- **包名 `perceiver` 覆盖"感知 + 窗口"**：契约、读数（元素/文字/桌面信息/窗口），以及窗口激活/摆放这类
  窗口动作；鼠标/键盘/截图/剪贴板等输入类动作仍在 executor。具体后端按上面的能力接口插拔。
- **眼睛与手分离**：感知与窗口管理走 neru-core（无障碍树 + 像素 + 窗口），输入走 robotgo
  （SendInput / XTest / CGEvent），两条路径都收敛在 `executor` 内部，对上层只暴露统一坐标系。
- **无障碍优先、视觉兜底**：默认先读无障碍树；树为空或失败时自动回退 OCR + 视觉检测。
  但如果显式指定了目标（`source=ax`、`scope=app`、`scope=surface`），失败就直接报错，
  绝不退回前台窗口答非所问。

## 坐标系

所有坐标都是**屏幕绝对坐标**，原点在虚拟桌面左上角。Windows/Linux 上是物理像素；macOS 的辅助功能
与鼠标注入坐标同为系统逻辑点。多显示器时，位于主屏左侧或上方的屏幕会出现负坐标；
`inspect_screen_tool` 返回的 `virtual_desktop` 就是可用的完整范围。

Windows 上进程会在首次感知时设为 per-monitor DPI aware，因此无障碍树/窗口返回的矩形与鼠标注入的
坐标是同一套物理像素。

## 开发与测试

```bash
go build ./...
go vet ./...
go test ./...              # 纯逻辑单测，不需要桌面

# 真实桌面验证：默认只读感知（不点击、不输入）；TestLiveSetWindowBounds 会创建并摆放一个 winver 窗口
$env:ROBOPILOT_LIVE=1      # PowerShell
go test ./executor/perceiver/neru -run TestLive -v
```

窗口枚举、激活与摆放依赖 neru-core 的 `window` 包。robopilot-mcp 与本仓库同级的 `neru-core/` 一起检出时，
项目根目录的 `go.work` 会让构建直接使用本地 neru-core，改完源码即可联调，不需要先发布版本。
单独构建 robopilot-mcp 时，`go.mod` 里要求的 neru-core 必须已包含 `window` 包；否则请保留这个
`go.work`，或把 `go.mod` 的依赖指向包含该包的 neru-core 版本。

## 许可

MIT。