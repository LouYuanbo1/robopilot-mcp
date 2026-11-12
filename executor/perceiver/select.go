package perceiver

import (
	"strings"

	"github.com/LouYuanbo1/robopilot-mcp/action"
)

// Best 从候选里挑出最符合"点名某个元素"意图的一个。
//
// 排序依据（依次比较）：文本匹配强度（精确>前缀>包含）、是否可点击、来源偏好
// （无障碍树优先，它带真实控件语义）、面积更小者（同名容器通常比真正的控件大）、
// 以及 Clean 已排好的阅读顺序。query 为空时退化为"可点击且面积最小"。
func Best(elements []Element, query string, mode action.MatchMode, role string) (Element, bool) {
	type scored struct {
		element Element
		score   int
	}
	candidates := make([]scored, 0, len(elements))
	q := strings.ToLower(strings.TrimSpace(query))

	for _, e := range elements {
		if role != "" && e.Role != role {
			continue
		}
		if !e.Matches(query, mode) {
			continue
		}
		score := 0
		switch {
		case q == "":
		case strings.EqualFold(e.Title, q) || strings.EqualFold(e.Value, q):
			score += 3
		case exactPrefix(e, q):
			score += 2
		default:
			score++
		}
		if e.Clickable {
			score += 4
		}
		if e.Source == "ax" {
			score += 2
		}
		candidates = append(candidates, scored{element: e, score: score})
	}

	if len(candidates) == 0 {
		return Element{}, false
	}

	bestIdx := 0
	for i := 1; i < len(candidates); i++ {
		a, b := candidates[bestIdx], candidates[i]
		if a.score != b.score {
			if b.score > a.score {
				bestIdx = i
			}
			continue
		}
		// 同分时取面积更小者；再相同则保留先出现的（阅读顺序更靠前）。
		if b.element.Area() > 0 && (a.element.Area() == 0 || b.element.Area() < a.element.Area()) {
			bestIdx = i
		}
	}
	return candidates[bestIdx].element, true
}

// exactPrefix 报告元素的某个文本字段是否以查询串开头。
func exactPrefix(e Element, queryLower string) bool {
	for _, field := range []string{e.Title, e.Description, e.Value, e.Identifier} {
		if strings.HasPrefix(strings.ToLower(field), queryLower) {
			return true
		}
	}
	return false
}

// Nearest 在 elements 中找离 from 最近的一个元素（按中心点欧氏距离）。
// 供未实现 SpatialPicker 的感知器退化使用。
func Nearest(elements []Element, from action.Point) (Element, bool) {
	var (
		best  Element
		found bool
		bestD int
	)
	for _, e := range elements {
		cx, cy := e.Center()
		d := (cx-from.X)*(cx-from.X) + (cy-from.Y)*(cy-from.Y)
		if !found || d < bestD {
			best, bestD, found = e, d, true
		}
	}
	return best, found
}

// Toward 在 elements 中找 from 指定方向上"最近"的一个元素。
// 计分方式：沿方向的间隙 + 横向漂移(加权)，与 neru-core 的空间导航思路一致。
func Toward(elements []Element, from action.Point, direction string) (Element, bool) {
	weight := 30
	if direction == "up" || direction == "down" {
		weight = 2
	}
	var (
		best  Element
		found bool
		bestS int
	)
	for _, e := range elements {
		cx, cy := e.Center()
		var beyond bool
		var along, across int
		switch direction {
		case "left":
			beyond = cx < from.X
			along = maxInt(from.X-(e.X+e.Width), 0)
			across = absInt(cy - from.Y)
		case "right":
			beyond = cx > from.X
			along = maxInt(e.X-from.X, 0)
			across = absInt(cy - from.Y)
		case "up":
			beyond = cy < from.Y
			along = maxInt(from.Y-(e.Y+e.Height), 0)
			across = absInt(cx - from.X)
		case "down":
			beyond = cy > from.Y
			along = maxInt(e.Y-from.Y, 0)
			across = absInt(cx - from.X)
		}
		if !beyond {
			continue
		}
		score := along + weight*across
		if !found || score < bestS {
			best, bestS, found = e, score, true
		}
	}
	return best, found
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func maxInt(values ...int) int {
	m := values[0]
	for _, v := range values[1:] {
		if v > m {
			m = v
		}
	}
	return m
}
