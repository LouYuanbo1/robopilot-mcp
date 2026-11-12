package action_test

import (
	"strings"
	"testing"

	"github.com/LouYuanbo1/robopilot-mcp/action"
)

// parseOne 解析单条内联动作 JSON，返回动作或错误。
func parseOne(t *testing.T, doc string) (action.Action, error) {
	t.Helper()
	acts, err := action.ParseJSON(strings.NewReader(doc))
	if err != nil {
		return nil, err
	}
	if len(acts) != 1 {
		t.Fatalf("期望 1 个动作，实际 %d 个", len(acts))
	}
	return acts[0], nil
}

func TestParseMouseButton(t *testing.T) {
	cases := []struct {
		doc   string
		state string
	}{
		{`{"actions":[{"type":"mouse_button","state":"down"}]}`, "down"},
		{`{"actions":[{"type":"mouse_button","state":"up","button":"right"}]}`, "up"},
		{`{"actions":[{"type":"mouse_down"}]}`, "down"},
		{`{"actions":[{"type":"mouse_up"}]}`, "up"},
	}
	for _, tc := range cases {
		act, err := parseOne(t, tc.doc)
		if err != nil {
			t.Fatalf("%s 解析失败: %v", tc.doc, err)
		}
		button, ok := act.(*action.MouseButtonAction)
		if !ok {
			t.Fatalf("%s 类型不符: %T", tc.doc, act)
		}
		if button.State != tc.state {
			t.Errorf("%s 期望 state=%s，实际 %s", tc.doc, tc.state, button.State)
		}
		if button.Button == "" {
			t.Errorf("%s 鼠标键应有默认值", tc.doc)
		}
	}

	if _, err := parseOne(t, `{"actions":[{"type":"mouse_button","state":"press"}]}`); err == nil {
		t.Error("非法 state 应报错")
	}
}

func TestParseMouseScrollValidation(t *testing.T) {
	if _, err := parseOne(t, `{"actions":[{"type":"mouse_scroll","direction":"down","amount":3}]}`); err != nil {
		t.Fatalf("合法滚动被拒绝: %v", err)
	}
	if _, err := parseOne(t, `{"actions":[{"type":"mouse_scroll","direction":"left","amount":3}]}`); err == nil {
		t.Error("非法方向应报错")
	}
	if _, err := parseOne(t, `{"actions":[{"type":"mouse_scroll","direction":"up","amount":0}]}`); err == nil {
		t.Error("amount 为 0 应报错")
	}
	if _, err := parseOne(t, `{"actions":[{"type":"mouse_scroll","direction":"up","amount":2,"x":10}]}`); err == nil {
		t.Error("只给 x 不给 y 应报错")
	}
}

func TestParseInspectScreen(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		ok   bool
	}{
		{"默认", `{"actions":[{"type":"inspect_screen"}]}`, true},
		{"按应用", `{"actions":[{"type":"inspect_screen","scope":"app","application":"chrome.exe"}]}`, true},
		{"视觉策略", `{"actions":[{"type":"inspect_screen","source":"vision","strategy":"contour"}]}`, true},
		{"未知范围", `{"actions":[{"type":"inspect_screen","scope":"screen"}]}`, false},
		{"缺应用标识", `{"actions":[{"type":"inspect_screen","scope":"app"}]}`, false},
		{"缺表面名", `{"actions":[{"type":"inspect_screen","scope":"surface"}]}`, false},
		{"非法来源", `{"actions":[{"type":"inspect_screen","source":"ocr"}]}`, false},
		{"非法策略", `{"actions":[{"type":"inspect_screen","strategy":"magic"}]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseOne(t, tc.doc)
			if tc.ok && err != nil {
				t.Errorf("应通过却报错: %v", err)
			}
			if !tc.ok && err == nil {
				t.Error("应报错却通过")
			}
		})
	}
}

// TestParseLegacyAliases 确认旧动作名仍然解码到合并后的新动作上。
func TestParseLegacyAliases(t *testing.T) {
	for _, legacy := range []string{"perceive_elements", "describe_screen", "detect_targets"} {
		act, err := parseOne(t, `{"actions":[{"type":"`+legacy+`","max":5}]}`)
		if err != nil {
			t.Fatalf("%s 应作为 inspect_screen 的别名被接受: %v", legacy, err)
		}
		inspect, ok := act.(*action.InspectScreen)
		if !ok {
			t.Fatalf("%s 应解码为 InspectScreen，实际 %T", legacy, act)
		}
		if inspect.Max != 5 {
			t.Errorf("%s 的 max 未解码: %d", legacy, inspect.Max)
		}
	}
}

func TestParseReadTextValidation(t *testing.T) {
	if _, err := parseOne(t, `{"actions":[{"type":"read_text","words":true}]}`); err != nil {
		t.Fatalf("合法参数被拒绝: %v", err)
	}
	if _, err := parseOne(t, `{"actions":[{"type":"read_text","min_confidence":1.5}]}`); err == nil {
		t.Error("置信度越界应报错")
	}
	if _, err := parseOne(t, `{"actions":[{"type":"read_text","region":{"x":0,"y":0,"width":0,"height":10}}]}`); err == nil {
		t.Error("空区域应报错")
	}
}

func TestParseClickElementValidation(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		ok   bool
	}{
		{"按名称", `{"actions":[{"type":"click_element","text":"保存"}]}`, true},
		{"按方向", `{"actions":[{"type":"click_element","direction":"right"}]}`, true},
		{"非法方向", `{"actions":[{"type":"click_element","direction":"north"}]}`, false},
		{"缺定位条件", `{"actions":[{"type":"click_element","role":"button"}]}`, false},
		{"非法匹配方式", `{"actions":[{"type":"click_element","text":"ok","match":"regex"}]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			act, err := parseOne(t, tc.doc)
			if tc.ok && err != nil {
				t.Errorf("应通过却报错: %v", err)
			}
			if !tc.ok && err == nil {
				t.Error("应报错却通过")
			}
			if tc.ok && err == nil {
				click, isClick := act.(*action.ClickElement)
				if !isClick {
					t.Fatalf("类型不符: %T", act)
				}
				if click.Button != action.MouseButtonLeft {
					t.Errorf("默认按键应为 left，实际 %q", click.Button)
				}
			}
		})
	}
}

func TestParseWindowAndClipboard(t *testing.T) {
	if _, err := parseOne(t, `{"actions":[{"type":"list_windows"}]}`); err != nil {
		t.Fatalf("list_windows 解析失败: %v", err)
	}

	if _, err := parseOne(t, `{"actions":[{"type":"set_active_window","title":"记事本"}]}`); err != nil {
		t.Fatalf("按标题激活解析失败: %v", err)
	}
	if _, err := parseOne(t, `{"actions":[{"type":"set_active_window","application":"chrome.exe"}]}`); err != nil {
		t.Fatalf("按应用激活解析失败: %v", err)
	}
	if _, err := parseOne(t, `{"actions":[{"type":"set_active_window","id":"0x3f0b58"}]}`); err != nil {
		t.Fatalf("按窗口 id 激活解析失败: %v", err)
	}
	if _, err := parseOne(t, `{"actions":[{"type":"set_active_window"}]}`); err == nil {
		t.Error("四者都没给时应报错")
	}

	withRect, err := parseOne(t, `{"actions":[{"type":"set_active_window","application":"chrome.exe","rect":{"x":0,"y":0,"width":960,"height":1440}}]}`)
	if err != nil {
		t.Fatalf("带 rect 的激活解析失败: %v", err)
	}
	windowAction, ok := withRect.(*action.SetActiveWindow)
	if !ok {
		t.Fatalf("带 rect 的激活类型不符: %T", withRect)
	}
	wantRect := action.Region{X: 0, Y: 0, Width: 960, Height: 1440}
	if windowAction.Rect == nil || *windowAction.Rect != wantRect {
		t.Fatalf("rect = %+v, want %+v", windowAction.Rect, wantRect)
	}

	if _, err := parseOne(t, `{"actions":[{"type":"set_active_window","title":"x","rect":{"x":0,"y":0,"width":0,"height":100}}]}`); err == nil {
		t.Error("rect 宽为 0 时应报错")
	}

	read, err := parseOne(t, `{"actions":[{"type":"clipboard"}]}`)
	if err != nil {
		t.Fatalf("读剪贴板解析失败: %v", err)
	}
	if read.(*action.Clipboard).Text != "" {
		t.Error("不带 text 时应为空（读模式）")
	}

	write, err := parseOne(t, `{"actions":[{"type":"clipboard","text":"你好"}]}`)
	if err != nil {
		t.Fatalf("写剪贴板解析失败: %v", err)
	}
	if write.(*action.Clipboard).Text != "你好" {
		t.Error("text 未解码")
	}

	// 旧名字仍然可用
	if _, err := parseOne(t, `{"actions":[{"type":"set_clipboard","text":"x"}]}`); err != nil {
		t.Fatalf("set_clipboard 别名解析失败: %v", err)
	}
}

func TestParseScreenshotOutput(t *testing.T) {
	cases := []struct {
		output string
		doc    string
	}{
		{"", `{"actions":[{"type":"screenshot"}]}`},
		{action.ScreenshotOutputFile, `{"actions":[{"type":"screenshot","output":"file"}]}`},
		{action.ScreenshotOutputInline, `{"actions":[{"type":"screenshot","output":"inline"}]}`},
		{action.ScreenshotOutputBoth, `{"actions":[{"type":"screenshot","output":"both"}]}`},
	}
	for _, tc := range cases {
		act, err := parseOne(t, tc.doc)
		if err != nil {
			t.Fatalf("%s 解析失败: %v", tc.doc, err)
		}
		shot, ok := act.(*action.Screenshot)
		if !ok {
			t.Fatalf("类型不符: %T", act)
		}
		if shot.Output != tc.output {
			t.Errorf("output 期望 %q，实际 %q", tc.output, shot.Output)
		}
	}

	if _, err := parseOne(t, `{"actions":[{"type":"screenshot","output":"memory"}]}`); err == nil {
		t.Error("非法 output 应报错")
	}

	// 只有 inline / both 需要图片内容。
	if (&action.Screenshot{}).WantsImage() {
		t.Error("默认(file)不应要求图片内容")
	}
	if !(&action.Screenshot{Output: action.ScreenshotOutputInline}).WantsImage() {
		t.Error("inline 应要求图片内容")
	}
	if !(&action.Screenshot{Output: action.ScreenshotOutputBoth}).WantsImage() {
		t.Error("both 应要求图片内容")
	}
}

func TestParseUnknownTypeMentionsNewActions(t *testing.T) {
	_, err := parseOne(t, `{"actions":[{"type":"nope"}]}`)
	if err == nil {
		t.Fatal("未知动作应报错")
	}
	msg := err.Error()
	for _, want := range []string{"inspect_screen", "read_text", "click_element", "list_windows", "clipboard"} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误信息缺少动作 %q: %s", want, msg)
		}
	}
}
