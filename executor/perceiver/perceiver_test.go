package perceiver_test

import (
	"encoding/json"
	"testing"

	"github.com/LouYuanbo1/robopilot-mcp/action"
	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver"
)

// elem 构造一个测试元素，第二个包（select_test.go）复用同一份。
func elem(label, role, title string, x, y, w, h int) perceiver.Element {
	return perceiver.Element{
		Label:     label,
		Role:      role,
		Title:     title,
		Source:    "ax",
		Clickable: true,
		X:         x,
		Y:         y,
		Width:     w,
		Height:    h,
	}
}

func TestCleanFiltersAndSorts(t *testing.T) {
	elements := []perceiver.Element{
		elem("c", "button", "保存", 300, 200, 60, 30),
		elem("a", "text_field", "搜索", 10, 10, 200, 30),
		elem("b", "button", "关闭", 100, 10, 40, 40),
		{X: 0, Y: 0, Width: 0, Height: 0}, // 零尺寸应被丢弃
		elem("tiny", "button", "小点", 0, 0, 2, 2),
	}

	out := perceiver.Clean(elements, perceiver.CleanOptions{MinArea: 100})
	if len(out) != 3 {
		t.Fatalf("期望 3 个元素，实际 %d", len(out))
	}
	// 左上→右下：第一行 y=10 的两个，x 小的在前。
	if out[0].Title != "搜索" || out[1].Title != "关闭" || out[2].Title != "保存" {
		t.Errorf("排序不符: %q %q %q", out[0].Title, out[1].Title, out[2].Title)
	}
}

func TestCleanFiltersByRoleQueryAndRegion(t *testing.T) {
	elements := []perceiver.Element{
		elem("1", "button", "保存", 100, 100, 60, 30),
		elem("2", "button", "另存为", 200, 100, 60, 30),
		elem("3", "text_field", "保存路径", 300, 100, 200, 30),
	}

	byRole := perceiver.Clean(append([]perceiver.Element(nil), elements...), perceiver.CleanOptions{Role: "button"})
	if len(byRole) != 2 {
		t.Errorf("按 role 过滤后期望 2 个，实际 %d", len(byRole))
	}

	byQuery := perceiver.Clean(append([]perceiver.Element(nil), elements...), perceiver.CleanOptions{Query: "保存"})
	if len(byQuery) != 2 {
		t.Errorf("按文本过滤后期望 2 个，实际 %d", len(byQuery))
	}

	byExact := perceiver.Clean(append([]perceiver.Element(nil), elements...), perceiver.CleanOptions{
		Query: "保存",
		Match: action.MatchExact,
	})
	if len(byExact) != 1 || byExact[0].Title != "保存" {
		t.Errorf("精确匹配期望只有「保存」，实际 %+v", byExact)
	}

	region := &action.Region{X: 0, Y: 0, Width: 150, Height: 200}
	byRegion := perceiver.Clean(append([]perceiver.Element(nil), elements...), perceiver.CleanOptions{Region: region})
	if len(byRegion) != 1 || byRegion[0].Title != "保存" {
		t.Errorf("按区域过滤期望只有「保存」，实际 %+v", byRegion)
	}

	limited := perceiver.Clean(append([]perceiver.Element(nil), elements...), perceiver.CleanOptions{Max: 2})
	if len(limited) != 2 {
		t.Errorf("截断后期望 2 个，实际 %d", len(limited))
	}
}

func TestCleanDeduplicatesSameRectAndRole(t *testing.T) {
	elements := []perceiver.Element{
		elem("1", "button", "确定", 10, 10, 80, 30),
		elem("2", "button", "确定", 10, 10, 80, 30),
		elem("3", "group", "确定", 10, 10, 80, 30),
	}
	out := perceiver.Clean(elements, perceiver.CleanOptions{})
	if len(out) != 2 {
		t.Fatalf("同位置同 role 应去重，期望 2 个，实际 %d", len(out))
	}
}

func TestRenderIncludesCenter(t *testing.T) {
	rendered := perceiver.RenderElements([]perceiver.Element{elem("1", "button", "确定", 100, 100, 80, 40)})
	if len(rendered) != 1 {
		t.Fatalf("期望 1 个元素，实际 %d", len(rendered))
	}
	if rendered[0].Center.X != 140 || rendered[0].Center.Y != 120 {
		t.Errorf("中心点应为 (140,120)，实际 %+v", rendered[0].Center)
	}

	var decoded []map[string]any
	payload := perceiver.Render([]perceiver.Element{elem("1", "button", "确定", 100, 100, 80, 40)})
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("序列化结果不是合法 JSON: %v", err)
	}
	if _, ok := decoded[0]["center"]; !ok {
		t.Error("序列化结果应包含 center 字段")
	}
}
