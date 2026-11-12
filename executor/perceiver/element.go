package perceiver

import (
	"strings"

	"github.com/LouYuanbo1/robopilot-mcp/action"
)

// Element 描述感知到的一个 UI 元素。
//
// 坐标均为屏幕绝对坐标(左上角为原点,单位像素)，与鼠标/区域等动作共用同一坐标系。
// 字段带 json 标签，便于把感知结果直接序列化给模型读取。
type Element struct {
	// Label 是本次扫描分配的标签；不同实现语义不同（如每次都重新编号，
	// 仅屏幕不变时有效）。若只用坐标点击，可不依赖它。
	Label string `json:"label"`
	Role  string `json:"role"`
	// Title 是控件的名称/标题（按钮文案、链接文字、输入框标签）。
	Title string `json:"title,omitempty"`
	// Description 是辅助说明（tooltip、ARIA description）。
	Description string `json:"description,omitempty"`
	// Value 是控件当前取值（输入框内容等）。
	Value string `json:"value,omitempty"`
	// Identifier 是开发者给元素设的标识（Windows AutomationId / DOM id 等）。
	Identifier string `json:"identifier,omitempty"`
	// Source 标记元素来源：ax（无障碍树）或 vision（像素视觉）。
	Source string `json:"source,omitempty"`
	// Focused 表示扫描时该元素是否持有键盘焦点。
	Focused bool `json:"focused"`
	// Clickable 表示底层是否认为该元素可点击。
	Clickable bool `json:"clickable"`
	// 元素外接矩形（屏幕绝对坐标）。
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Center 返回元素中心点坐标，可直接用于 move_mouse / click_mouse_at。
func (e Element) Center() (x, y int) {
	return e.X + e.Width/2, e.Y + e.Height/2
}

// Bounds 返回元素外接区域，可与截图区域等动作共用 action.Region。
func (e Element) Bounds() action.Region {
	return action.Region{X: e.X, Y: e.Y, Width: e.Width, Height: e.Height}
}

// Area 返回元素外接矩形面积（平方像素）。
func (e Element) Area() int {
	if e.Width <= 0 || e.Height <= 0 {
		return 0
	}
	return e.Width * e.Height
}

// Text 返回元素对人类/模型可读的摘要文本：Label(Role): Title Description。
func (e Element) Text() string {
	var body []string
	if e.Title != "" {
		body = append(body, e.Title)
	}
	if e.Description != "" {
		body = append(body, e.Description)
	}
	s := e.Label
	if e.Role != "" {
		s += "(" + e.Role + ")"
	}
	if v := strings.Join(body, " "); v != "" {
		s += ": " + v
	}
	return s
}

// Matches 报告元素文本是否匹配查询串，匹配范围是名称、描述、取值与标识。
// 空查询串视为匹配；匹配忽略大小写。
func (e Element) Matches(query string, mode action.MatchMode) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	for _, field := range []string{e.Title, e.Description, e.Value, e.Identifier} {
		if Match(strings.ToLower(field), q, mode) {
			return true
		}
	}
	return false
}

// Match 判断已转小写的文本与查询串是否满足匹配方式；空 mode 按 contains 处理。
func Match(text, queryLower string, mode action.MatchMode) bool {
	if queryLower == "" {
		return true
	}
	switch mode {
	case action.MatchExact:
		return text == queryLower
	case action.MatchPrefix:
		return strings.HasPrefix(text, queryLower)
	default:
		return strings.Contains(text, queryLower)
	}
}
