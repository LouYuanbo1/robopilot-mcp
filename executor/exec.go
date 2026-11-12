package executor

import (
	"context"
	"fmt"

	"github.com/LouYuanbo1/robopilot-mcp/action"
)

// 本文件是执行层的路由中心：按动作所属领域把 a 分派给对应的子控制器。
// 输入类动作（鼠标/键盘/截图/剪贴板）由各领域文件通过 robotgo 驱动；
// 感知类与窗口类动作则走注入的 perceiver 能力接口；本文件不对外暴露任何底层类型。

// exec 执行单个动作：先按类型分组路由，再由对应域的子控制器组装 ActionResult。
func (e *Executor) exec(ctx context.Context, a action.Action) (action.ActionResult, error) {
	switch a.(type) {
	case *action.MouseMove, *action.MouseClick, *action.MouseClickAt,
		*action.MouseButtonAction, *action.MouseDrag, *action.MouseScroll:
		return e.mouse(a)
	case *action.ClickElement:
		return e.clickElement(ctx, a)
	case *action.TypeText, *action.KeyTap:
		return e.keyboard(a)
	case *action.Screenshot:
		return e.screen(a)
	case *action.Clipboard:
		return e.clipboard(a)
	case *action.ListWindows, *action.SetActiveWindow:
		return e.window(ctx, a)
	case *action.InspectScreen, *action.ReadText:
		return e.observe(ctx, a)
	default:
		return action.ActionResult{}, fmt.Errorf("未知动作类型: %T", a)
	}
}
