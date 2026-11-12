package perceiver_test

import (
	"testing"

	"github.com/LouYuanbo1/robopilot-mcp/action"
	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver"
)

func TestBestPrefersExactClickableSmall(t *testing.T) {
	elements := []perceiver.Element{
		// 外层容器，文本包含「保存」，面积大。
		elem("1", "group", "保存对话框", 0, 0, 800, 600),
		// 真正的按钮。
		elem("2", "button", "保存", 700, 560, 80, 30),
		// 另一个包含「另存」的按钮。
		elem("3", "button", "另存为", 600, 560, 80, 30),
	}

	got, ok := perceiver.Best(elements, "保存", action.MatchContains, "")
	if !ok || got.Label != "2" {
		t.Fatalf("应选中精确匹配的按钮 2，实际 %+v (ok=%v)", got, ok)
	}

	got, ok = perceiver.Best(elements, "保存", action.MatchContains, "button")
	if !ok || got.Label != "2" {
		t.Fatalf("限定 role 后应选中按钮 2，实际 %+v", got)
	}

	if _, ok := perceiver.Best(elements, "不存在", action.MatchContains, ""); ok {
		t.Error("无匹配时不应返回元素")
	}
}

func TestNearestAndToward(t *testing.T) {
	elements := []perceiver.Element{
		elem("1", "button", "左", 10, 100, 40, 30),
		elem("2", "button", "中", 100, 100, 40, 30),
		elem("3", "button", "右", 300, 100, 40, 30),
		elem("4", "button", "上", 100, 10, 40, 30),
		elem("5", "button", "下", 100, 300, 40, 30),
	}

	if got, ok := perceiver.Nearest(elements, action.Point{X: 105, Y: 110}); !ok || got.Label != "2" {
		t.Errorf("最近元素应为 2，实际 %+v", got)
	}
	if got, ok := perceiver.Toward(elements, action.Point{X: 120, Y: 115}, "right"); !ok || got.Label != "3" {
		t.Errorf("向右应为 3，实际 %+v", got)
	}
	if got, ok := perceiver.Toward(elements, action.Point{X: 120, Y: 115}, "down"); !ok || got.Label != "5" {
		t.Errorf("向下应为 5，实际 %+v", got)
	}
	if got, ok := perceiver.Toward(elements, action.Point{X: 120, Y: 115}, "up"); !ok || got.Label != "4" {
		t.Errorf("向上应为 4，实际 %+v", got)
	}
	if _, ok := perceiver.Toward(elements, action.Point{X: 1000, Y: 1000}, "right"); ok {
		t.Error("右侧没有元素时不应返回目标")
	}
}
