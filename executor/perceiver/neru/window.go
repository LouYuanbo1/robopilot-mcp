package neru

import (
	"context"
	"fmt"
	"image"

	"github.com/LouYuanbo1/neru-core/window"

	"github.com/LouYuanbo1/robopilot-mcp/action"
	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver"
)

// 本文件把 neru-core/window 的窗口管理能力翻译成 perceiver.WindowManager：
// 枚举、前台窗口、激活与摆放。平台的差异（Win32 的 EnumWindows/SetForegroundWindow/
// SetWindowPos、macOS 的 AX 窗口与 NSRunningApplication、X11 的 EWMH 客户端消息）
// 全部留在 neru-core 内，本包只做"中立窗口信息 → perceiver.Window"的拷贝。

// windowManagerOnce 惰性创建并复用 neru-core 的窗口管理器；创建失败会缓存错误并返回。
func (p *Perceiver) windowManagerOnce() (window.Manager, error) {
	p.windowMu.Lock()
	defer p.windowMu.Unlock()

	if p.windows != nil || p.windowErr != nil {
		return p.windows, p.windowErr
	}

	manager, err := window.New(window.Options{})
	if err != nil {
		p.windowErr = err
		return nil, err
	}

	p.windows = manager

	return p.windows, nil
}

// ListWindows 枚举所有可见顶层窗口，前台窗口在最前，含最小化窗口。
func (p *Perceiver) ListWindows(ctx context.Context) ([]perceiver.Window, error) {
	manager, err := p.windowManagerOnce()
	if err != nil {
		return nil, fmt.Errorf("初始化窗口管理失败: %w", err)
	}

	infos, err := manager.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("枚举窗口失败: %w", err)
	}

	windows := make([]perceiver.Window, 0, len(infos))
	for _, info := range infos {
		windows = append(windows, fromWindowInfo(info))
	}

	return windows, nil
}

// FrontmostWindow 返回当前前台窗口；没有窗口获得焦点时返回明确错误。
func (p *Perceiver) FrontmostWindow(ctx context.Context) (perceiver.Window, error) {
	manager, err := p.windowManagerOnce()
	if err != nil {
		return perceiver.Window{}, fmt.Errorf("初始化窗口管理失败: %w", err)
	}

	info, err := manager.Frontmost(ctx)
	if err != nil {
		return perceiver.Window{}, fmt.Errorf("读取前台窗口失败: %w", err)
	}

	return fromWindowInfo(info), nil
}

// ActivateWindow 把请求选中的窗口带到前台并聚焦，返回激活后的窗口信息。
//
// 目标按 ID > PID > Application > Title 的优先级解析；neru-core 的 Activate 会
// 复核窗口是否真的切到前台，因此这里不需要再补一次"Alt 解锁 + 重试"的平台技巧。
func (p *Perceiver) ActivateWindow(
	ctx context.Context,
	req perceiver.WindowRequest,
) (perceiver.Window, error) {
	if req.Empty() {
		return perceiver.Window{}, fmt.Errorf("pid、application、title 至少提供其一")
	}

	manager, err := p.windowManagerOnce()
	if err != nil {
		return perceiver.Window{}, fmt.Errorf("初始化窗口管理失败: %w", err)
	}

	info, err := manager.Activate(ctx, window.Target{
		ID:          req.ID,
		PID:         req.PID,
		Application: req.Application,
		Title:       req.Title,
	})
	if err != nil {
		return perceiver.Window{}, fmt.Errorf("激活窗口失败: %w", err)
	}

	return fromWindowInfo(info), nil
}

// SetWindowBounds 把窗口移动到 bounds 并调整大小，返回平台实际接受的矩形。
//
// 还原最小化/最大化、绕开平台限制这些细节都在 neru-core 的 SetBounds 里；
// 这里只把中立 Region 换算成 image.Rectangle，并做参数校验。
func (p *Perceiver) SetWindowBounds(
	ctx context.Context,
	req perceiver.WindowRequest,
	bounds action.Region,
) (perceiver.Window, error) {
	if req.Empty() {
		return perceiver.Window{}, fmt.Errorf("id、pid、application、title 至少提供其一")
	}

	if bounds.Empty() {
		return perceiver.Window{}, fmt.Errorf("rect 的 width 与 height 必须为正数")
	}

	manager, err := p.windowManagerOnce()
	if err != nil {
		return perceiver.Window{}, fmt.Errorf("初始化窗口管理失败: %w", err)
	}

	info, err := manager.SetBounds(ctx, window.Target{
		ID:          req.ID,
		PID:         req.PID,
		Application: req.Application,
		Title:       req.Title,
	}, image.Rect(bounds.X, bounds.Y, bounds.X+bounds.Width, bounds.Y+bounds.Height))
	if err != nil {
		return perceiver.Window{}, fmt.Errorf("调整窗口大小失败: %w", err)
	}

	return fromWindowInfo(info), nil
}

// fromWindowInfo 把 neru-core 的窗口读数拷贝成 perceiver 的中立窗口信息。
// 空矩形保持为全零 Region（最小化窗口），不编造坐标。
func fromWindowInfo(info window.Info) perceiver.Window {
	return perceiver.Window{
		ID:          info.ID,
		PID:         info.PID,
		Title:       info.Title,
		Application: info.Application,
		Region: action.Region{
			X:      info.Bounds.Min.X,
			Y:      info.Bounds.Min.Y,
			Width:  info.Bounds.Dx(),
			Height: info.Bounds.Dy(),
		},
		Minimized: info.Minimized,
		Focused:   info.Focused,
	}
}
