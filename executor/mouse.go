package executor

import (
	"fmt"

	"github.com/go-vgo/robotgo"

	"github.com/LouYuanbo1/robopilot-mcp/action"
)

// guard 校验收发坐标是否在配置的安全区域内。
func (e *Executor) guard(x, y int) error {
	r := e.safeRegion
	if r == nil {
		return nil
	}
	if x < r.X || x >= r.X+r.Width || y < r.Y || y >= r.Y+r.Height {
		return fmt.Errorf("坐标(%d,%d)超出安全区域 %+v，已拦截", x, y, *r)
	}
	return nil
}

// mouseButton 把协议层的鼠标键映射为 robotgo 的字符串。
func mouseButton(b action.MouseButton) string {
	switch b {
	case action.MouseButtonRight:
		return "right"
	case action.MouseButtonMiddle:
		return "center"
	default:
		return "left"
	}
}

// doMove 移动指针：先做安全边界校验，再按拟人开关选择平滑或精确移动。
func (e *Executor) doMove(x, y int) error {
	if err := e.guard(x, y); err != nil {
		return err
	}
	e.pause()
	if e.humanize {
		robotgo.MoveSmooth(x, y)
		return nil
	}
	return robotgo.Move(x, y)
}

func (e *Executor) doScroll(direction action.ScrollDirection, amount int) error {
	e.pause()
	if e.humanize {
		/*
			在 RobotGo 的滚动 API 体系中，一直遵循一个明确的符号规则：
			to 为负数（如 -10）：表示向下滚动。
			to 为正数（如 10）：表示向上滚动。
		*/
		if direction == action.ScrollDown {
			amount = -amount
		}
		robotgo.ScrollSmooth(amount)
		return nil
	}
	return robotgo.ScrollDir(amount, direction)
}

// 鼠标域动作的执行落点：Move/Click/按键相位/Drag/Scroll 在此聚合实现。
// 定位(move)统一吃安全边界校验 + 拟人延迟，Click/Scroll 等也按需加入随机间隔。

// mouse 分发所有鼠标类动作并组装 ActionResult。
func (e *Executor) mouse(a action.Action) (action.ActionResult, error) {
	res := action.ActionResult{Action: nameOf(a)}

	switch v := a.(type) {
	case *action.MouseMove:
		if err := e.doMove(v.X, v.Y); err != nil {
			return res, err
		}
		res.Effective = true

	case *action.MouseClick:
		e.pause()
		if err := robotgo.Click(mouseButton(v.Button), v.Double); err != nil {
			return res, err
		}
		res.Effective = true

	case *action.MouseClickAt:
		// 复合动作：提供了坐标则先定位（走安全边界/拟人校验），再在当前处点击；
		// 未提供坐标（X/Y 为 nil）则直接在当前光标处点击，不移动指针。
		if v.X == nil || v.Y == nil {
			e.pause()
			if err := robotgo.Click(mouseButton(v.Button), v.Double); err != nil {
				return res, err
			}
			res.Effective = true
			return res, nil
		}
		if err := e.doMove(*v.X, *v.Y); err != nil {
			return res, err
		}
		e.pause()
		if err := robotgo.Click(mouseButton(v.Button), v.Double); err != nil {
			return res, err
		}
		res.Effective = true

	case *action.MouseButtonAction:
		e.pause()
		var err error
		if v.State == "up" {
			err = robotgo.MouseUp(mouseButton(v.Button))
		} else {
			err = robotgo.MouseDown(mouseButton(v.Button))
		}
		if err != nil {
			return res, err
		}
		res.Effective = true

	case *action.MouseDrag:
		e.pause()
		if err := e.doMove(v.FromX, v.FromY); err != nil {
			return res, err
		}
		if err := robotgo.DragSmooth(v.ToX, v.ToY); err != nil {
			return res, err
		}
		res.Effective = true

	case *action.MouseScroll:
		// 可选坐标：先把指针移到目标点再滚动（"滚动这个列表"最自然的表达）。
		if v.X != nil && v.Y != nil {
			if err := e.doMove(*v.X, *v.Y); err != nil {
				return res, err
			}
		}
		if err := e.doScroll(v.Direction, v.Amount); err != nil {
			return res, err
		}
		res.Effective = true
	}
	return res, nil
}
