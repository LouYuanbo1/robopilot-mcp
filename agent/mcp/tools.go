package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// debugTool 包装工具执行函数：将调用参数与返回值打印到终端，便于调试。
// timeout 为每次工具调用的最大执行时长：到点仍未返回时，将 ctx 取消并回传超时错误，
// 让 LLM 反思后重试 / 调整参数 / 换方案，而不是无限卡死或中断整次运行。
// 不影响工具本身的行为，仅在执行前后输出日志。
func debugTool[T any](timeout time.Duration, name string, fn func(ctx context.Context, params *T) (string, error)) func(ctx context.Context, params *T) (string, error) {
	return func(ctx context.Context, params *T) (string, error) {
		argJSON, _ := json.Marshal(params)

		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		out, err := fn(timeoutCtx, params)
		if err != nil {
			if timeoutCtx.Err() != nil {
				log.Printf("[工具超时] %s\n  参数: %s\n  错误: %v", name, argJSON, err)
				return fmt.Sprintf("【%s 执行超时】超过 %v 仍未返回结果。请分析原因，调整参数或换一种方式重新尝试。", name, timeout), nil
			}
			log.Printf("[工具失败] %s\n  参数: %s\n  错误: %v", name, argJSON, err)
			return fmt.Sprintf("【%s 执行失败】%v\n请分析失败原因，调整参数或换一种方式重新尝试。", name, err), nil
		}
		view := out
		if len(view) > 500 {
			view = view[:500] + "\n  ...(输出过长已截断)"
		}
		log.Printf("[工具调用] %s\n  参数: %s\n  返回: %s", name, argJSON, view)
		return out, nil
	}
}

// newMCPTool 用参数类型 T 声明一个 eino 工具：参数经 JSON 序列化后作为 tools/call 的
// arguments 发往远端 MCP 服务，并把返回的文本结果交给模型。
// 每个 T 的字段标签使用 eino-contrib/jsonschema 的旧式 "description=..." 语法，
// 供 eino 的 utils.InferTool 反射出模型可见的入参 schema。
func newMCPTool[T any](c *Client, name, desc string) (tool.InvokableTool, error) {
	return utils.InferTool(name, desc, debugTool(defaultCallTimeout, name, func(ctx context.Context, p *T) (string, error) {
		b, err := json.Marshal(p)
		if err != nil {
			return "", err
		}
		var arguments map[string]any
		if err := json.Unmarshal(b, &arguments); err != nil {
			return "", err
		}
		return c.CallTool(ctx, name, arguments)
	}))
}

// ---- 各工具的参数结构体 ----
// 这里的每个结构体都刻意与 action 包的对应类型（如 action.MouseMove）字段保持一致，
// 但仍单独抄一份，而不是直接复用 action 类型，原因如下：
//
// eino 的 utils.InferTool 通过 jsonschema 反射出模型可见的入参 schema，它对字段标签只认
// "key=value" 形式的写法，例如 `jsonschema:"description=..."`、`jsonschema:"enum=..."`
// （见 eino-contrib/jsonschema 的 structKeywordsFromTags：只有带 '=' 的 token 才会被解析）。
// 而 action 包为了保持"纯协议"的简洁，标签用的是裸文本，例如 `jsonschema:"目标横坐标"`，
// 不带 description= 前缀。直接把 action 类型喂给 InferTool 时，这些裸标签会被当成无法识别的
// token 忽略，导致模型看不到任何字段说明。
//
// 因此在 mcp 侧复制一层带 description=/enum= 前缀的参数结构体，作为"Schema 描述层"：
// 字段与 json key 保持与 action 一致（原样透传给 MCP 服务），标签则按 eino 语法重写，
// 既保证模型能读到字段含义，又与 MCP 服务端接收的 action 结构体完全对齐。
// 坐标均为屏幕绝对坐标(左上角为原点)，单位像素。

// scanParams 是感知类工具共享的"扫描范围"参数，与服务端 action.Scan 字段一一对应。
type scanParams struct {
	Scope       string `json:"scope,omitempty" jsonschema:"description=看哪里: window 当前窗口(默认)/application 整个前台应用/app 由 application 指定的应用/surface 辅助表面(仅 macOS)"`
	Application string `json:"application,omitempty" jsonschema:"description=scope=app 时的目标应用可执行文件名,如 chrome.exe;可在 list_windows 的返回里找到"`
	Surface     string `json:"surface,omitempty" jsonschema:"description=scope=surface 时的表面名: dock/notification_center/stage_manager/picture_in_picture/screen_capture_ui,仅 macOS 支持"`
}

// ---- 感知类 ----

type screenshotRegionParams struct {
	X      int `json:"x" jsonschema:"description=区域左边界横坐标"`
	Y      int `json:"y" jsonschema:"description=区域上边界纵坐标"`
	Width  int `json:"width" jsonschema:"description=区域宽度"`
	Height int `json:"height" jsonschema:"description=区域高度"`
}

type inspectScreenParams struct {
	scanParams
	Max         int                     `json:"max,omitempty" jsonschema:"description=最多返回的元素数量,<=0 用默认上限(通常64)"`
	MinArea     int                     `json:"min_area,omitempty" jsonschema:"description=过滤面积(平方像素)小于该值的元素,<=0 不过滤"`
	MaxDepth    int                     `json:"max_depth,omitempty" jsonschema:"description=限制无障碍树遍历深度,0 用默认深度"`
	Roles       []string                `json:"roles,omitempty" jsonschema:"description=只保留的 role 列表,如 button/text_field 或 uia:Button;空不过滤"`
	Query       string                  `json:"query,omitempty" jsonschema:"description=按文本过滤元素:在名称/描述/取值里做忽略大小写的子串匹配,空不过滤"`
	Region      *screenshotRegionParams `json:"region,omitempty" jsonschema:"description=只保留中心点在该区域内的元素,并作为 text/screenshot 的默认区域"`
	Source      string                  `json:"source,omitempty" jsonschema:"description=感知来源: ax 仅无障碍树/vision 仅像素视觉/省略先无障碍树再回退视觉"`
	Strategy    string                  `json:"strategy,omitempty" jsonschema:"description=像素视觉策略,enum=vision,enum=contour,enum=segment,default=vision"`
	IncludeText bool                    `json:"include_text,omitempty" jsonschema:"description=是否同时 OCR 读取屏上文字,默认false"`
	Screenshot  bool                    `json:"screenshot,omitempty" jsonschema:"description=是否同时截屏落盘并返回图片路径,默认false"`
	Dir         string                  `json:"artifact_dir,omitempty" jsonschema:"description=截图保存目录,为空用默认目录"`
	Name        string                  `json:"artifact_name,omitempty" jsonschema:"description=截图文件名(不含扩展名),为空自动生成"`
}

func NewInspectScreenTool(c *Client) (tool.InvokableTool, error) {
	return newMCPTool[inspectScreenParams](c, "inspect_screen_tool",
		`Observe the screen in ONE call: desktop geometry, the frontmost window, and every interactive element
		(role, name, bounds, center). Optionally also OCR the visible text and/or save a screenshot.
		Start every task with this call; do not use several separate tools to "look around".
		- scope: window (default) | application | app (with application="chrome.exe") | surface (macOS only)
		- query/roles/min_area: narrow a busy screen
		- source=vision (optional strategy=vision|contour|segment): read pixels when the accessibility tree is empty.
		scope=app reads another application WITHOUT switching to it — use it to answer questions about
		an app that is not in the foreground (e.g. a browser tab title).`)
}

type readTextParams struct {
	Region        *screenshotRegionParams `json:"region,omitempty" jsonschema:"description=只读取该矩形区域,省略表示活动显示器整屏"`
	Languages     []string                `json:"languages,omitempty" jsonschema:"description=期望的识别语言,如 en-US / zh-Hans,省略用系统语言配置"`
	WordLevel     bool                    `json:"word_level,omitempty" jsonschema:"description=是否按词而非按行切分,找单个按钮文字时建议开启"`
	MinConfidence float64                 `json:"min_confidence,omitempty" jsonschema:"description=置信度下限(0到1),<=0 不过滤"`
	Words         bool                    `json:"words,omitempty" jsonschema:"description=为真时返回带坐标的词块列表,否则只返回整段文本"`
	Max           int                     `json:"max,omitempty" jsonschema:"description=词块模式下的最大返回条数,<=0 用默认上限(200)"`
}

func NewReadTextTool(c *Client) (tool.InvokableTool, error) {
	return newMCPTool[readTextParams](c, "read_text_tool",
		`Read the text on screen (or in a region) with OCR. Use words=true to get each word's box and center,
		which you can then click — this is the way to drive interfaces that expose text only as pixels.`)
}

// screenshotParams 的 JSON 形状与 action.Screenshot 完全一致（region 为嵌套对象），
// 以便 MCP 服务端直接把入参解码为 action.Screenshot，无需再做扁平化映射。
type screenshotParams struct {
	Region *screenshotRegionParams `json:"region,omitempty" jsonschema:"description=要截取的屏幕矩形区域,省略则截全屏"`
	Output string                  `json:"output,omitempty" jsonschema:"description=输出方式,enum=file,enum=inline,enum=both,default=file"`
	Dir    string                  `json:"artifact_dir,omitempty" jsonschema:"description=产物要保存的目录,为空用默认目录"`
	Name   string                  `json:"artifact_name,omitempty" jsonschema:"description=产物文件名(不含扩展名),为空自动生成"`
}

func NewScreenshotTool(c *Client) (tool.InvokableTool, error) {
	return newMCPTool[screenshotParams](c, "screenshot_tool",
		`Capture the screen (or region{x,y,width,height}). output decides what comes back:
		file (default) saves a PNG and returns its path; inline encodes it in memory and returns the image
		without touching the disk; both saves and returns it.
		inline/both only make sense for clients that can see images — a text-only model cannot read them.`)
}

// ---- 窗口类 ----

type listWindowsParams struct{}

func NewListWindowsTool(c *Client) (tool.InvokableTool, error) {
	return newMCPTool[listWindowsParams](c, "list_windows_tool",
		`List every visible top-level window: id, title, pid, application (executable name; bundle id on macOS),
		screen rect, minimized flag and focused flag. Use it to discover a window or to check whether an
		application is running at all. The "application" field feeds inspect_screen_tool's application
		parameter, and the "id" field is the most precise argument to set_active_window_tool.`)
}

// windowRectParams 是 set_active_window 的可选摆放矩形；字段与服务端 action.Region 一一对应。
type windowRectParams struct {
	X      int `json:"x" jsonschema:"description=窗口矩形左边界横坐标(屏幕绝对坐标,像素)"`
	Y      int `json:"y" jsonschema:"description=窗口矩形上边界纵坐标(屏幕绝对坐标,像素)"`
	Width  int `json:"width" jsonschema:"description=窗口宽度(像素,必须为正)"`
	Height int `json:"height" jsonschema:"description=窗口高度(像素,必须为正)"`
}

type setActiveWindowParams struct {
	ID          string            `json:"id,omitempty" jsonschema:"description=目标窗口ID(来自list_windows,最精确,优先使用)"`
	PID         int               `json:"pid,omitempty" jsonschema:"description=目标窗口进程PID(最精确,优先使用)"`
	Application string            `json:"application,omitempty" jsonschema:"description=目标应用可执行文件名,如 chrome.exe;激活该应用的第一个可见窗口"`
	Title       string            `json:"title,omitempty" jsonschema:"description=目标窗口标题关键字;匹配的是窗口标题,不是可执行文件名"`
	Rect        *windowRectParams `json:"rect,omitempty" jsonschema:"description=可选:把窗口移到该矩形并缩放(物理像素);用于左右并排看目标应用与日志。给定时会先还原最大化/最小化"`
}

func NewSetActiveWindowTool(c *Client) (tool.InvokableTool, error) {
	return newMCPTool[setActiveWindowParams](c, "set_active_window_tool",
		`Bring a window to the foreground and give it focus. Locate it by id (from list_windows, most precise),
		by pid (most precise), by application (executable name, e.g. chrome.exe) or by title (a substring of
		the window title). Provide at least one of id/pid/application/title.
		Optionally pass rect={x,y,width,height} to move and resize the window in the same call. Derive it from
		inspect_screen_tool's virtual_desktop (dx,dy,dw,dh): left half = {x:dx, y:dy, width:dw/2, height:dh},
		right half = {x:dx+dw/2, y:dy, width:dw/2, height:dh}, full screen = virtual_desktop itself.
		Example: with virtual_desktop {0,0,2160,1440}, the left half is {x:0,y:0,width:1080,height:1440}.
		A maximized or minimized window is restored first, and the returned rect is the rectangle it actually
		got. On Windows you can alternatively snap with key_tap_tool (win+left / win+right / win+up), which
		uses the OS's own work-area and multi-monitor rules.`)
}

// ---- 元素与鼠标 ----

type clickElementParams struct {
	scanParams
	Text       string `json:"text,omitempty" jsonschema:"description=目标元素的名称关键字,与 direction 至少提供其一"`
	Role       string `json:"role,omitempty" jsonschema:"description=限定元素 role 以收窄匹配,如 button/link/menu_item,空不限定"`
	Match      string `json:"match,omitempty" jsonschema:"description=文本匹配方式,enum=contains,enum=exact,enum=prefix,default=contains"`
	Direction  string `json:"direction,omitempty" jsonschema:"description=从当前光标出发的空间方向,enum=up,enum=down,enum=left,enum=right"`
	MaxDepth   int    `json:"max_depth,omitempty" jsonschema:"description=限制无障碍树遍历深度,0 用默认深度"`
	Button     string `json:"button,omitempty" jsonschema:"description=鼠标键,enum=left,enum=right,enum=middle,default=left"`
	Double     bool   `json:"double,omitempty" jsonschema:"description=是否双击,默认false"`
	OffsetX    int    `json:"offset_x,omitempty" jsonschema:"description=点击点相对元素中心的横向偏移(像素)"`
	OffsetY    int    `json:"offset_y,omitempty" jsonschema:"description=点击点相对元素中心的纵向偏移(像素)"`
	KeepCursor bool   `json:"keep_cursor,omitempty" jsonschema:"description=是否把光标留在目标处,默认false即点击后放回原处"`
	Source     string `json:"source,omitempty" jsonschema:"description=感知来源: ax 仅无障碍树/vision 仅视觉/省略先无障碍树再回退视觉"`
}

func NewClickElementTool(c *Client) (tool.InvokableTool, error) {
	return newMCPTool[clickElementParams](c, "click_element_tool",
		`Find an element by name (or by spatial direction from the cursor) and click it, without computing coordinates.
		Example: text="Save" clicks the Save button; direction="right" clicks the nearest element to the right of the cursor.
		This is the preferred way to click: prefer it over click_mouse_at_tool whenever you know the element's name.
		The pointer is restored to its original position after the click unless keep_cursor is true.`)
}

type clickMouseAtParams struct {
	X      *int   `json:"x,omitempty" jsonschema:"description=目标横坐标(屏幕绝对坐标,像素),省略则点击当前光标位置"`
	Y      *int   `json:"y,omitempty" jsonschema:"description=目标纵坐标(屏幕绝对坐标,像素),省略则点击当前光标位置"`
	Button string `json:"button,omitempty" jsonschema:"description=要点击的鼠标键,enum=left,enum=right,enum=middle,default=left"`
	Double bool   `json:"double,omitempty" jsonschema:"description=是否双击,default=false"`
}

func NewClickMouseAtTool(c *Client) (tool.InvokableTool, error) {
	return newMCPTool[clickMouseAtParams](c, "click_mouse_at_tool",
		`Click at an absolute coordinate (optionally moving the pointer there first).
		Use it only when you already know the coordinate — e.g. the "center" of an element returned by
		inspect_screen_tool. If you know the element's name, use click_element_tool instead.`)
}

type moveMouseParams struct {
	X int `json:"x" jsonschema:"description=目标横坐标(屏幕绝对坐标,像素)"`
	Y int `json:"y" jsonschema:"description=目标纵坐标(屏幕绝对坐标,像素)"`
}

func NewMoveMouseTool(c *Client) (tool.InvokableTool, error) {
	return newMCPTool[moveMouseParams](c, "move_mouse_tool",
		`Move the mouse pointer to an absolute screen coordinate without clicking (hover / position the cursor).`)
}

type mouseButtonParams struct {
	State  string `json:"state" jsonschema:"description=相位: down 按住/up 松开,enum=down,enum=up"`
	Button string `json:"button,omitempty" jsonschema:"description=鼠标键,enum=left,enum=right,enum=middle,default=left"`
}

func NewMouseButtonTool(c *Client) (tool.InvokableTool, error) {
	return newMCPTool[mouseButtonParams](c, "mouse_button_tool",
		`Press and hold (state=down) or release (state=up) a mouse button at the current pointer position.
		Combine with move_mouse_tool for multi-step drags, rubber-band selections or press-drag-over-several-points
		operations that a single drag_mouse_tool call cannot express.`)
}

type dragMouseParams struct {
	FromX int `json:"from_x" jsonschema:"description=拖拽起点横坐标(屏幕绝对坐标)"`
	FromY int `json:"from_y" jsonschema:"description=拖拽起点纵坐标(屏幕绝对坐标)"`
	ToX   int `json:"to_x" jsonschema:"description=拖拽终点横坐标(屏幕绝对坐标)"`
	ToY   int `json:"to_y" jsonschema:"description=拖拽终点纵坐标(屏幕绝对坐标)"`
}

func NewDragMouseTool(c *Client) (tool.InvokableTool, error) {
	return newMCPTool[dragMouseParams](c, "drag_mouse_tool",
		`Hold the mouse button and drag from the start coordinate to the end coordinate;
		use for text selection, moving/resizing elements or other drag operations.`)
}

type scrollMouseParams struct {
	Direction string `json:"direction" jsonschema:"description=滚动方向,enum=up,enum=down"`
	Amount    int    `json:"amount" jsonschema:"description=滚动量(格数,正整数)"`
	X         *int   `json:"x,omitempty" jsonschema:"description=可选:滚动前把指针移到该横坐标(屏幕绝对坐标)"`
	Y         *int   `json:"y,omitempty" jsonschema:"description=可选:滚动前把指针移到该纵坐标(屏幕绝对坐标)"`
}

func NewScrollMouseTool(c *Client) (tool.InvokableTool, error) {
	return newMCPTool[scrollMouseParams](c, "scroll_mouse_tool",
		`Scroll the mouse wheel at the current pointer position by the given number of notches, up or down.
		Pass x/y to move the pointer there first, e.g. to scroll a specific list or panel.`)
}

// ---- 键盘类 ----

type typeTextParams struct {
	Text string `json:"text" jsonschema:"description=要输入的文本内容(必填)"`
}

func NewTypeTextTool(c *Client) (tool.InvokableTool, error) {
	return newMCPTool[typeTextParams](c, "type_text_tool",
		`Type a piece of text into the currently focused control/input. Ensure the target field is focused first
		(e.g. by clicking it) before calling this tool.`)
}

type keyTapParams struct {
	Key       string   `json:"key" jsonschema:"description=要发送的按键名,如 a/enter/F5/space/backspace 等"`
	Modifiers []string `json:"modifiers,omitempty" jsonschema:"description=组合修饰键,如 ctrl/alt/shift/cmd(win),按任意顺序"`
	Count     int      `json:"count,omitempty" jsonschema:"description=连按次数,default=1"`
}

func NewKeyTapTool(c *Client) (tool.InvokableTool, error) {
	return newMCPTool[keyTapParams](c, "key_tap_tool",
		`Send a single key press; combine with Modifiers for shortcuts, e.g. key=c with Modifiers=[ctrl] for Ctrl+C.
		Windows key is spelled "cmd" (alias "win"). Use Count>1 for repeated presses.`)
}

// ---- 剪贴板 ----

type clipboardParams struct {
	Text string `json:"text,omitempty" jsonschema:"description=要写入剪贴板的文本;省略则读取当前剪贴板"`
}

func NewClipboardTool(c *Client) (tool.InvokableTool, error) {
	return newMCPTool[clipboardParams](c, "clipboard_tool",
		`Read or write the clipboard text: pass text to write it, omit text to read the current clipboard.
		Typical use: write a long text, then key_tap_tool ctrl+v to paste it into the focused input.`)
}
