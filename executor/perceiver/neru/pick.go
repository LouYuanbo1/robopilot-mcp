package neru

import (
	"context"
	"image"
	"strconv"

	"github.com/LouYuanbo1/neru-core/element"

	"github.com/LouYuanbo1/robopilot-mcp/action"
	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver"
)

// Pick 按空间关系挑元素：direction 为空时取离 from 最近的，
// 否则沿该方向取"最近的一步"。几何计算委托给 neru-core 的 Nearest / Toward，
// 它们已经处理了容器/子元素、裁剪与方向漂移等细节。
func (p *Perceiver) Pick(
	_ context.Context,
	elements []perceiver.Element,
	from action.Point,
	direction string,
) (perceiver.Element, bool) {
	if len(elements) == 0 {
		return perceiver.Element{}, false
	}

	byID := make(map[element.ID]perceiver.Element, len(elements))
	converted := make([]*element.Element, 0, len(elements))
	union := image.Rectangle{}

	for i, e := range elements {
		bounds := image.Rect(e.X, e.Y, e.X+e.Width, e.Y+e.Height)
		if bounds.Empty() {
			continue
		}
		id := element.ID(strconv.Itoa(i))
		source := element.SourceAccessibilityTree
		if e.Source == "vision" {
			source = element.SourceVision
		}
		convertedElem, err := element.NewElement(
			id,
			bounds,
			element.Role(e.Role),
			element.WithTitle(e.Title),
			element.WithDescription(e.Description),
			element.WithValue(e.Value),
			element.WithClickable(e.Clickable),
			element.WithFocused(e.Focused),
			element.WithSource(source),
		)
		if err != nil {
			continue
		}
		byID[id] = e
		converted = append(converted, convertedElem)
		union = union.Union(bounds)
	}

	if len(converted) == 0 {
		return perceiver.Element{}, false
	}

	origin := image.Pt(from.X, from.Y)
	if direction == "" {
		target, ok := element.Nearest(converted, origin, union)
		if !ok {
			return perceiver.Element{}, false
		}
		return byID[target.Element.ID()], true
	}

	dir, err := element.ParseDirection(direction)
	if err != nil {
		// 未知方向交给调用方的退化实现处理。
		return perceiver.Toward(elements, from, direction)
	}
	target, ok := element.Toward(converted, origin, union, dir)
	if !ok {
		return perceiver.Element{}, false
	}
	return byID[target.Element.ID()], true
}
