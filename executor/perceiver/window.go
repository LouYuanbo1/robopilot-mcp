package perceiver

import (
	"context"

	"github.com/LouYuanbo1/robopilot-mcp/action"
)

// WindowManager 是可选能力：枚举桌面窗口，把其中一个切到前台，并可选摆放它。
//
// 窗口枚举、激活与移动/缩放在不同系统上要碰完全不同的原生接口（Win32 的
// EnumWindows、SetForegroundWindow 与 SetWindowPos，macOS 的 AX 窗口对象、
// NSRunningApplication 与 AXPosition/AXSize，X11 的 EWMH 客户端列表与
// _NET_MOVERESIZE_WINDOW），这些复杂度由感知器背后的库承担（如 neru-core 的
// window 包）。上层只依赖本接口，因此不需要在 executor 里写任何按平台分文件的
// unsafe 代码。
//
// 未实现本接口的感知器，窗口类动作会明确报错；绝不退回"每个平台各写一份"的老路。
type WindowManager interface {
	// ListWindows 枚举所有可见顶层窗口，前台窗口在最前，含最小化窗口。
	ListWindows(ctx context.Context) ([]Window, error)
	// FrontmostWindow 返回当前前台窗口；没有窗口获得焦点时返回明确错误。
	FrontmostWindow(ctx context.Context) (Window, error)
	// ActivateWindow 把 req 选中的窗口带到前台并聚焦，返回激活后的窗口信息。
	// 实现必须复核切换是否真的生效，不允许"调用了 API 就报告成功"。
	ActivateWindow(ctx context.Context, req WindowRequest) (Window, error)
	// SetWindowBounds 把 req 选中的窗口移动到 bounds 并调整大小，返回调整后的窗口。
	//
	// 窗口若处于最小化或最大化状态，实现会先还原再摆放；实际矩形可能被系统
	// 最小尺寸或窗口管理器修正，返回值是平台实际接受的矩形，而不是请求值。
	SetWindowBounds(ctx context.Context, req WindowRequest, bounds action.Region) (Window, error)
}

// Window 是一个可见顶层窗口的可读信息。
type Window struct {
	// ID 平台给这个窗口的句柄，按字符串透出（Win32 的 HWND、macOS 的 AX 窗口、
	// X11 的窗口 id）。它是同一平台内最精确的定位方式，可直接回填到 WindowRequest.ID。
	ID string `json:"id,omitempty"`
	// PID 窗口所属进程。
	PID int `json:"pid"`
	// Title 窗口标题栏文字，可能为空。
	Title string `json:"title"`
	// Application 应用标识：Windows/Linux 是可执行文件名（如 chrome.exe），
	// macOS 是 bundle id（如 com.google.Chrome）。可直接用于 inspect_screen 的
	// application 参数与 WindowRequest.Application。
	Application string `json:"application,omitempty"`
	// Region 窗口在屏幕上的位置与大小(物理像素、屏幕绝对坐标)；最小化时为空。
	Region action.Region `json:"rect"`
	// Minimized 是否处于最小化状态。
	Minimized bool `json:"minimized,omitempty"`
	// Focused 是否就是当前前台窗口。
	Focused bool `json:"focused,omitempty"`
}

// WindowRequest 一次窗口操作（激活/摆放）的目标参数。
//
// 字段按 ID > PID > Application > Title 的优先级生效，取第一个非空项；
// 至少提供其一，否则执行层会以"参数缺失"拒绝。
type WindowRequest struct {
	// ID 上一次 WindowList 返回的窗口 id，最精确。
	ID string
	// PID 目标窗口进程。
	PID int
	// Application 目标应用标识（可执行文件名或 bundle id）。
	Application string
	// Title 窗口标题的忽略大小写子串。
	Title string
}

// Empty 报告这次请求是否没有指定任何目标。
func (r WindowRequest) Empty() bool {
	return r.ID == "" && r.PID <= 0 && r.Application == "" && r.Title == ""
}
