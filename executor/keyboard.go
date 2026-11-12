package executor

import (
	"github.com/go-vgo/robotgo"

	"github.com/LouYuanbo1/robopilot-mcp/action"
)

// 键盘域动作的执行落点：TypeText/KeyTap 在此聚合实现。

// keyboard 分发所有键盘类动作并组装 ActionResult。
func (e *Executor) keyboard(a action.Action) (action.ActionResult, error) {
	res := action.ActionResult{Action: nameOf(a)}

	switch v := a.(type) {
	case *action.TypeText:
		humanDelay(e.minDelayMS, e.maxDelayMS)
		if err := robotgo.TypeStr(v.Text); err != nil {
			return res, err
		}
		res.Effective = true

	case *action.KeyTap:
		humanDelay(e.minDelayMS, e.maxDelayMS)
		mods := make([]any, 0, len(v.Modifiers))
		for _, m := range v.Modifiers {
			mods = append(mods, m)
		}
		if err := robotgo.KeyTap(v.Key, mods...); err != nil {
			return res, err
		}
		res.Effective = true
	}
	return res, nil
}
