package executor

import (
	"fmt"

	"github.com/go-vgo/robotgo"

	"github.com/LouYuanbo1/robopilot-mcp/action"
)

// 剪贴板域动作的执行落点：读写文本合并成一个动作，用是否提供文本区分。

// clipboard 执行剪贴板读写并组装 ActionResult。
func (e *Executor) clipboard(a action.Action) (action.ActionResult, error) {
	res := action.ActionResult{Action: nameOf(a)}

	v, ok := a.(*action.Clipboard)
	if !ok {
		return res, fmt.Errorf("非剪贴板动作: %T", a)
	}

	if v.Text == "" {
		txt, err := robotgo.ReadAll()
		if err != nil {
			return res, err
		}
		res.Value, res.Effective = txt, true
		return res, nil
	}

	if err := robotgo.WriteAll(v.Text); err != nil {
		return res, err
	}
	res.Effective = true
	return res, nil
}
