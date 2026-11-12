package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/LouYuanbo1/robopilot-mcp/action"
	"github.com/LouYuanbo1/robopilot-mcp/executor"
)

// ---- MCP 服务层 ----
// 本文件把桌面操控能力以 MCP 工具方式暴露：直接基于官方 modelcontextprotocol/go-sdk，
// 用 mcp.NewServer + mcp.AddTool 注册工具，由 SDK 负责 JSON-RPC / Streamable HTTP / 参数解码
// 与校验。这里只保留三件事：
//  1. 把每个 action 值类型注册成一个工具（工具名与 agent 侧 agent/mcp/tools.go 一一对应）；
//  2. 工具入参(SDK 解码好的值类型)取地址转成实现 action.Action 的指针，交给 executor 执行；
//  3. 对声明了 include_image 的产物型动作，把落盘图片作为 MCP 图片内容一并返回，
//     多模态模型可以直接看图，纯文本模型只读第一段文本结果也不受影响。
//
// 工具集刻意保持"一个意图一个工具"：看屏 = inspect_screen，读字 = read_text，
// 点元素 = click_element，点坐标 = click_mouse_at。少一层选择，模型就少一次犹豫。

// defaultCallTimeout 单次工具调用的最大执行时长，超时后报错让模型换种方式重试。
const defaultCallTimeout = 2 * time.Minute

// imageAction 由"希望工具结果附带图片"的动作实现（如 screenshot 的 include_image）。
type imageAction interface {
	WantsImage() bool
}

// toolRegistry 聚合执行器与默认超时，并为每个工具统一桥接 入参→action→执行→ActionResult。
type toolRegistry struct {
	ex          *executor.Executor
	callTimeout time.Duration
}

// NewMCPServer 用给定执行器构造一个已注册好全部桌面工具的 MCP 服务；ex 为空时用默认实例。
func NewMCPServer(ex *executor.Executor, implementation *mcp.Implementation, opts *mcp.ServerOptions) *mcp.Server {
	if ex == nil {
		ex = executor.New()
	}
	if implementation == nil {
		implementation = &mcp.Implementation{Name: "robopilot-mcp", Version: "0.3.0"}
	}
	tr := &toolRegistry{ex: ex, callTimeout: defaultCallTimeout}
	srv := mcp.NewServer(implementation, opts)
	tr.registerTools(srv)
	return srv
}

// register 把值类型 action 注册为工具：SDK 把入参 JSON 解码为 A 的值，
// 这里取地址转成实现 action.Action 的指针（action 的 isAction 均为指针接收者）再执行。
func (tr *toolRegistry) register[A any](srv *mcp.Server, name, desc string) {
	mcp.AddTool(srv, &mcp.Tool{Name: name, Description: desc},
		func(ctx context.Context, _ *mcp.CallToolRequest, input A) (*mcp.CallToolResult, action.ActionResult, error) {
			act, ok := any(&input).(action.Action)
			if !ok {
				return nil, action.ActionResult{}, fmt.Errorf("工具 %s 入参类型 %T 未实现 action.Action", name, input)
			}
			result, err := tr.run(ctx, name, input, act)
			if err != nil {
				return nil, result, err
			}
			return attachImage(result, act), result, nil
		})
}

// run 施加超时、执行动作并统一打日志（成功与失败都记录）。
func (tr *toolRegistry) run[A any](ctx context.Context, name string, input A, act action.Action) (action.ActionResult, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, tr.callTimeout)
	defer cancel()

	results, err := tr.ex.Execute(timeoutCtx, []action.Action{act})
	if err != nil && timeoutCtx.Err() != nil {
		err = fmt.Errorf("【%s 执行超时】超过 %v 仍未返回结果，请调整参数或换一种方式重试", name, tr.callTimeout)
	} else if err != nil {
		log.Printf("[工具失败] %s 入参=%+v 错误=%v", name, input, err)
	}

	var result action.ActionResult
	if len(results) > 0 {
		result = results[0]
	}
	log.Printf("[工具完成] %s action=%s effective=%v 返回=%s", name, result.Action, result.Effective, truncateResult(result.Value))
	return result, err
}

// attachImage 在动作结果需要图片时，把 PNG 作为额外的图片内容返回。
// 优先用执行层直接给的内存字节（output=inline，不落盘），没有再回读落盘文件（output=both）。
// 返回 nil 表示不加图片，让 SDK 走默认的"JSON 文本"结果路径。
func attachImage(result action.ActionResult, act action.Action) *mcp.CallToolResult {
	wants, ok := act.(imageAction)
	if !ok || !wants.WantsImage() {
		return nil
	}

	data := result.ImagePNG
	if len(data) == 0 && result.ArtifactPath != "" {
		fileData, err := os.ReadFile(result.ArtifactPath)
		if err != nil {
			log.Printf("[图片读取失败] %s: %v", result.ArtifactPath, err)
			return nil
		}
		data = fileData
	}
	if len(data) == 0 {
		return nil
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		return nil
	}

	// Content 非空时 SDK 不会再补文本，因此这里自己把 JSON 结果放在第一段，
	// 保证只读文本的客户端（如 eino 侧）行为与不带图片时一致。
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: string(encoded)},
		&mcp.ImageContent{Data: data, MIMEType: "image/png"},
	}}
}

// registerTools 注册全部桌面操控工具；名称必须与 agent 侧保持一致。
//
// 共 14 个：感知 3 + 窗口 2 + 元素/鼠标 6 + 键盘 2 + 剪贴板 1。
func (tr *toolRegistry) registerTools(srv *mcp.Server) {
	// ---- 感知（只读） ----
	tr.register[action.InspectScreen](srv, "inspect_screen_tool",
		"观察屏幕：一次返回桌面几何(分辨率/DPI/虚拟桌面/光标)、前台窗口(标题/PID/应用)、以及当前窗口里的可交互元素(角色/名称/矩形/中心点)；可选同时 OCR 读取屏上文字与截图落盘。默认先调用它。用 scope 指定看哪里(window 当前窗口/application 整个应用/app 按 application 指定的应用/surface 仅 macOS)，用 query 或 roles 收窄元素；无障碍树读不到内容时把 source 设为 vision(可选 strategy=vision/contour/segment)。")
	tr.register[action.ReadText](srv, "read_text_tool",
		"用 OCR 读取屏幕或指定区域上的文字；words 为真时返回带坐标的词块，可据此按文字位置点击。")
	tr.register[action.Screenshot](srv, "screenshot_tool",
		"截取全屏或 region 指定的矩形区域。output 决定产出形态：file(默认,只落盘并返回路径)、inline(不落盘,图片内容直接随结果返回)、both(既落盘又返回图片)；inline/both 仅对支持看图的客户端有意义。")

	// ---- 窗口 ----
	tr.register[action.ListWindows](srv, "list_windows_tool",
		"枚举所有可见顶层窗口：窗口 id、标题、PID、应用标识(application，Windows/Linux 是可执行文件名，macOS 是 bundle id)、屏幕位置、是否最小化、是否前台。用于发现目标窗口；拿到 application 后可直接用 inspect_screen 的 scope=app 在不激活窗口的情况下读取该应用内容，拿到 id 可直接用于 set_active_window。")
	tr.register[action.SetActiveWindow](srv, "set_active_window_tool",
		"把窗口带到前台并聚焦：按 id(来自 list_windows，最精确)、pid、application(可执行文件名，如 chrome.exe)或 title(真正的窗口标题关键字)定位，至少给一个。可选 rect={x,y,width,height} 同时移动/缩放窗口；用 inspect_screen 的 virtual_desktop(dx,dy,dw,dh) 计算半屏：左半屏 {x:dx,y:dy,width:dw/2,height:dh}，右半屏 {x:dx+dw/2,y:dy,width:dw/2,height:dh}，整屏就是 virtual_desktop 本身。例如 virtual_desktop={0,0,2160,1440} 时左半屏为 {x:0,y:0,width:1080,height:1440}。最大化/最小化的窗口会先还原，返回的 rect 是平台实际接受的矩形。Windows 上也可以先激活，再用 key_tap 发 win+left / win+right / win+up 让系统按工作区与多显示器规则贴靠或最大化。")

	// ---- 元素与鼠标 ----
	tr.register[action.ClickElement](srv, "click_element_tool",
		"按名称/角色/方向定位一个可交互元素并点击它，不需要自己算坐标：text 传按钮文字，或用 direction 指定光标某方向上最近的元素。默认点击后把光标放回原处(keep_cursor 可改)。")
	tr.register[action.MouseClickAt](srv, "click_mouse_at_tool",
		"把指针移动到指定坐标并一步完成点击；x/y 可省略，省略则直接在当前光标位置点击。")
	tr.register[action.MouseMove](srv, "move_mouse_tool",
		"把鼠标指针移动到指定屏幕绝对坐标(像素,原点左上)，用于定位或悬停。")
	tr.register[action.MouseButtonAction](srv, "mouse_button_tool",
		"在当前位置按住或松开鼠标键(state=down/up)，配合 move_mouse 实现分段拖拽、框选等单次拖拽表达不了的操作。")
	tr.register[action.MouseDrag](srv, "drag_mouse_tool",
		"按住鼠标从起点拖拽到终点(选区/拖动元素/调整大小)。")
	tr.register[action.MouseScroll](srv, "scroll_mouse_tool",
		"在指针当前位置滚轮滚动指定格数；可选 x/y 先把指针移到目标位置再滚动。")

	// ---- 键盘 ----
	tr.register[action.TypeText](srv, "type_text_tool",
		"向当前聚焦控件输入一段文本，输入前请先点击目标控件使其获焦。")
	tr.register[action.KeyTap](srv, "key_tap_tool",
		"发送单键或组合快捷键，如 key=c 且 modifiers=[ctrl] 即 Ctrl+C；修饰键支持 ctrl/alt/shift/cmd(win)。")

	// ---- 剪贴板 ----
	tr.register[action.Clipboard](srv, "clipboard_tool",
		"读写剪贴板文本：带 text 参数即写入，省略 text 即读取当前剪贴板。")
}

func truncateResult(s string) string {
	const n = 300
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(截断)"
}
