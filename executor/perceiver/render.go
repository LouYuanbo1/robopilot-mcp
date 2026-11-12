package perceiver

import "encoding/json"

// RenderElement 是序列化感知结果的结构：内嵌 Element(带坐标)并额外附上由 Center
// 计算出的中心点，让模型一读即知"该点多大的可点区域、中心在哪"。
type RenderElement struct {
	Element
	Center Center `json:"center"`
}

// Center 屏幕坐标点，序列化用。
type Center struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// RenderElements 给每个元素补上中心点，供需要内嵌元素列表的复合结果复用。
func RenderElements(elements []Element) []RenderElement {
	out := make([]RenderElement, len(elements))
	for i, e := range elements {
		x, y := e.Center()
		out[i] = RenderElement{Element: e, Center: Center{X: x, Y: y}}
	}
	return out
}

// Render 把可交互元素列表序列化为易读的紧凑 JSON 文本（显式给出中心坐标），
// 适合 agent 侧拼进提示词，让模型知道"屏幕上哪里有可点的元素、点哪里"；失败时返回空串。
func Render(elements []Element) string {
	b, err := json.Marshal(RenderElements(elements))
	if err != nil {
		return ""
	}
	return string(b)
}
