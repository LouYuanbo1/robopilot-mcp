package executor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/LouYuanbo1/robopilot-mcp/action"
	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver"
)

// 窗口域动作的执行落点：枚举可见顶层窗口、把目标窗口切到前台、按矩形移动/缩放。
//
// 平台差异收敛在感知器背后的库（如 neru-core/window）里：executor 只依赖
// perceiver.WindowManager，因此本包不再有任何按系统分文件的 unsafe 代码。
// 枚举结果带"应用标识"，它同时是 InspectScreen 里 scope=app 的入参，
// 因此"发现窗口"到"读取该应用内容"之间不需要模型去猜进程名。

// window 分发所有窗口类动作并组装 ActionResult。
func (e *Executor) window(ctx context.Context, a action.Action) (action.ActionResult, error) {
	res := action.ActionResult{Action: nameOf(a)}

	wm, err := e.windowManager()
	if err != nil {
		return res, err
	}

	switch v := a.(type) {
	case *action.ListWindows:
		windows, err := wm.ListWindows(ctx)
		if err != nil {
			return res, err
		}
		encoded, err := json.Marshal(windows)
		if err != nil {
			return res, err
		}
		res.Value, res.Effective = string(encoded), true

	case *action.SetActiveWindow:
		// 定位优先级（ID > PID > 应用名 > 标题）由感知器实现，这里只做参数完整性检查。
		if v.ID == "" && v.PID <= 0 && v.Application == "" && v.Title == "" {
			return res, fmt.Errorf("id、pid、application、title 至少提供其一")
		}
		req := perceiver.WindowRequest{
			ID:          v.ID,
			PID:         v.PID,
			Application: v.Application,
			Title:       v.Title,
		}

		var (
			activated perceiver.Window
			err       error
		)

		if v.Rect != nil {
			if v.Rect.Empty() {
				return res, fmt.Errorf("rect 的 width 与 height 必须为正数")
			}
			// 带摆放时只解析一次目标：还原、激活、移动、复核都由 SetWindowBounds 完成。
			activated, err = wm.SetWindowBounds(ctx, req, *v.Rect)
		} else {
			activated, err = wm.ActivateWindow(ctx, req)
		}
		if err != nil {
			return res, err
		}
		encoded, err := json.Marshal(activated)
		if err != nil {
			return res, err
		}
		res.Value, res.Effective = string(encoded), true
	}

	return res, nil
}

// windowManager 取出感知器的窗口管理能力。
//
// 没注入感知器、或注入的感知器没有实现 perceiver.WindowManager 时，返回可操作的
// 错误而不是让调用方以为"桌面真的没有窗口"。
func (e *Executor) windowManager() (perceiver.WindowManager, error) {
	if e.perceiver == nil {
		return nil, errNoPerceiver("窗口操作")
	}

	wm, ok := e.perceiver.(perceiver.WindowManager)
	if !ok {
		return nil, fmt.Errorf(
			"当前感知器不支持窗口枚举、激活与摆放；请注入实现了 perceiver.WindowManager 的感知器(如 perceiver/neru)",
		)
	}

	return wm, nil
}
