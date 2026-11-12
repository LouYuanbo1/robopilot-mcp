package perceiver

import (
	"slices"
	"strings"

	"github.com/LouYuanbo1/robopilot-mcp/action"
)

// CleanOptions 是 Clean 的过滤/截断配置。
type CleanOptions struct {
	// MinArea 过滤掉外接矩形面积小于该值的元素(平方像素)；<=0 表示不过滤。
	MinArea int
	// KeepRoles 只保留这些 role；为空表示不过滤。
	KeepRoles map[string]bool
	// Role 只保留该单个 role；为空不限定（与 KeepRoles 取交集）。
	Role string
	// Query 文本关键字，匹配名称/描述/取值，空表示不过滤。
	Query string
	// Match 文本匹配方式，空按 contains 处理。
	Match action.MatchMode
	// Region 只保留中心点落在该区域内的元素；nil 表示不过滤。
	Region *action.Region
	// Source 只保留该来源的元素（ax / vision）；空表示不过滤。
	Source string
	// Max 单次最多保留的元素个数；<=0 表示不截断。
	Max int
}

// Keep 报告元素是否通过全部过滤条件（不含去重/排序/截断）。
func (o CleanOptions) Keep(e Element) bool {
	if e.Width <= 0 || e.Height <= 0 {
		return false
	}
	if o.MinArea > 0 && e.Area() < o.MinArea {
		return false
	}
	if len(o.KeepRoles) > 0 && !o.KeepRoles[e.Role] {
		return false
	}
	if o.Role != "" && e.Role != o.Role {
		return false
	}
	if o.Source != "" && e.Source != o.Source {
		return false
	}
	if o.Region != nil {
		x, y := e.Center()
		if !o.Region.Contains(x, y) {
			return false
		}
	}
	return e.Matches(o.Query, o.Match)
}

// Clean 归一化一批感知结果（供各实现复用）：
//  1. 过滤零尺寸/过小、非目标 role/来源/区域、文本不匹配的元素；
//  2. 按外接矩形与 role 去重（同一元素可能被多策略各报一次，保留首个）；
//  3. 左上→右下排序，使编号与屏幕位置一致；
//  4. 按 Max 截断。
//
// 返回并排序原切片（原地操作）。
func Clean(elements []Element, opts CleanOptions) []Element {
	// 1) 过滤
	filtered := elements[:0]
	for _, e := range elements {
		if opts.Keep(e) {
			filtered = append(filtered, e)
		}
	}

	// 2) 去重：同位置不同 role 的嵌套元素都应保留，故 role 也进键。
	seen := make(map[string]bool, len(filtered))
	out := filtered[:0]
	for _, e := range filtered {
		key := rectKey(e)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}

	// 3) 左上→右下排序（稳定，坐标相同的保持原次序）。
	slices.SortStableFunc(out, func(a, b Element) int {
		if a.Y != b.Y {
			if a.Y < b.Y {
				return -1
			}
			return 1
		}
		if a.X != b.X {
			if a.X < b.X {
				return -1
			}
			return 1
		}
		return 0
	})

	// 4) 截断。
	if opts.Max > 0 && len(out) > opts.Max {
		out = out[:opts.Max]
	}
	return out
}

// rectKey 生成"矩形 + role"去重键。
func rectKey(e Element) string {
	var b strings.Builder
	b.Grow(len(e.Role) + 32)
	b.WriteString(e.Role)
	b.WriteByte('|')
	for _, v := range [4]int{e.X, e.Y, e.Width, e.Height} {
		b.WriteString(itoa(v))
		b.WriteByte(',')
	}
	return b.String()
}

// itoa 是 strconv.Itoa 的零分配替代，只为拼去重键服务。
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
