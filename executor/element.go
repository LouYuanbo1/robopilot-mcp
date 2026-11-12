package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-vgo/robotgo"

	"github.com/LouYuanbo1/robopilot-mcp/action"
	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver"
)

// 元素级动作的执行落点：把"点名点击某个控件"拆成
// 定位(无障碍树/视觉) → 选优 → 坐标点击 → 恢复光标 四步。
//
// 这样模型不必自己算坐标：界面滚动、窗口缩放、同名控件变化时，
// 只要名称还能匹配上，点击点就由执行层重新算出来。

// clickElementPayload 是 click_element 的回填结构，让模型知道实际点到了什么、点在哪。
type clickElementPayload struct {
	Element        perceiver.Element `json:"element"`
	Click          action.Point      `json:"click"`
	Source         string            `json:"source"`
	CursorRestored bool              `json:"cursor_restored"`
}

// clickElement 按名称/方向定位元素并点击其中心点。
func (e *Executor) clickElement(ctx context.Context, a action.Action) (action.ActionResult, error) {
	res := action.ActionResult{Action: "click_element"}

	v, ok := a.(*action.ClickElement)
	if !ok {
		return res, fmt.Errorf("非元素点击动作: %T", a)
	}
	if e.perceiver == nil {
		return res, errNoPerceiver(res.Action)
	}

	elements, used, err := e.collectElements(ctx, perceiver.ScanRequest{
		Scan:     v.Scan,
		MaxDepth: v.MaxDepth,
	}, v.Source, "")
	if err != nil {
		return res, err
	}

	mode := v.Match
	if mode == "" {
		mode = action.MatchContains
	}

	target, found := e.pickElement(ctx, elements, v, mode)
	// 无障碍树没命中时，按需回退像素视觉（OCR 能读到按钮文字）。
	if !found && used != "vision" && v.Source != action.SourceAX {
		if visionElements, visionErr := e.visionElements(ctx, ""); visionErr == nil {
			if fallback, fallbackOK := e.pickElement(ctx, visionElements, v, mode); fallbackOK {
				target, found, used = fallback, true, "vision"
			}
		}
	}
	if !found {
		return res, fmt.Errorf("%s", e.missingElementHint(v, elements))
	}

	centerX, centerY := target.Center()
	clickX, clickY := centerX+v.OffsetX, centerY+v.OffsetY
	if err := e.guard(clickX, clickY); err != nil {
		return res, err
	}

	originX, originY := robotgo.GetMousePos()
	e.pause()
	if err := e.doMove(clickX, clickY); err != nil {
		return res, err
	}

	// 默认把指针放回原处：直接 warp，不产生指针移动事件，避免被应用读成拖拽。
	// 用 defer 保证点击失败时也会归位。
	restore := !v.KeepCursor
	if restore {
		defer robotgo.Move(originX, originY)
	}

	e.pause()
	if err := robotgo.Click(mouseButton(v.Button), v.Double); err != nil {
		return res, err
	}

	encoded, err := json.Marshal(clickElementPayload{
		Element:        target,
		Click:          action.Point{X: clickX, Y: clickY},
		Source:         used,
		CursorRestored: restore,
	})
	if err != nil {
		return res, err
	}
	res.Value, res.Effective = string(encoded), true
	return res, nil
}

// pickElement 在候选元素中选一个：
// 指定了方向就按空间导航（优先用感知器的成熟算法），否则按文本/角色匹配取最优。
func (e *Executor) pickElement(
	ctx context.Context,
	elements []perceiver.Element,
	v *action.ClickElement,
	mode action.MatchMode,
) (perceiver.Element, bool) {
	if v.Direction != "" {
		x, y := robotgo.GetMousePos()
		from := action.Point{X: x, Y: y}
		if picker, ok := e.perceiver.(perceiver.SpatialPicker); ok {
			return picker.Pick(ctx, elements, from, v.Direction)
		}
		return perceiver.Toward(elements, from, v.Direction)
	}
	return perceiver.Best(elements, v.Text, mode, v.Role)
}

// missingElementHint 生成"没找到目标"的可用提示：说清查询条件、扫到多少元素、
// 前几个元素长什么样，让模型能立刻改参数重试，而不是盲目重复。
func (e *Executor) missingElementHint(v *action.ClickElement, elements []perceiver.Element) string {
	var query strings.Builder
	if v.Text != "" {
		query.WriteString(fmt.Sprintf("text=%q ", v.Text))
	}
	if v.Role != "" {
		query.WriteString(fmt.Sprintf("role=%q ", v.Role))
	}
	if v.Direction != "" {
		query.WriteString(fmt.Sprintf("direction=%q ", v.Direction))
	}

	var b strings.Builder
	b.WriteString("未找到匹配的元素(")
	b.WriteString(strings.TrimSpace(query.String()))
	b.WriteString(fmt.Sprintf(")；本次扫描到 %d 个可交互元素", len(elements)))
	if len(elements) > 0 {
		b.WriteString("，前几个是: ")
		for i, el := range elements {
			if i >= 8 {
				b.WriteString(" ...")
				break
			}
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(el.Text())
		}
	}
	b.WriteString("。可先用 perceive_elements 查看完整列表并核对名称，或改用 click_mouse_at 按坐标点击。")
	return b.String()
}
