package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
)

// TestToolSchemasExposeFields 验证 eino 反射出的入参 schema 确实包含模型的可见字段。
//
// 这里最容易踩的坑是"字段在 Go 结构体里存在、却没被 InferTool 暴露出来"：
// 例如嵌入了 scanParams 却没被展开、或标签写错被当成未知 token 忽略。
// 断言只看 schema 里的属性名，不关心描述文案。
func TestToolSchemasExposeFields(t *testing.T) {
	client := NewMCPClient("http://127.0.0.1:1/mcp")

	cases := []struct {
		name    string
		newTool func() (tool.InvokableTool, error)
		fields  []string
	}{
		{
			name:    "inspect_screen_tool",
			newTool: func() (tool.InvokableTool, error) { return NewInspectScreenTool(client) },
			fields: []string{
				"scope", "application", "surface", "max", "min_area", "max_depth",
				"roles", "query", "region", "source", "strategy",
				"include_text", "screenshot", "artifact_dir", "artifact_name",
			},
		},
		{
			name:    "read_text_tool",
			newTool: func() (tool.InvokableTool, error) { return NewReadTextTool(client) },
			fields:  []string{"region", "languages", "word_level", "min_confidence", "words", "max"},
		},
		{
			name:    "screenshot_tool",
			newTool: func() (tool.InvokableTool, error) { return NewScreenshotTool(client) },
			fields:  []string{"region", "output", "artifact_dir", "artifact_name"},
		},
		{
			name:    "list_windows_tool",
			newTool: func() (tool.InvokableTool, error) { return NewListWindowsTool(client) },
			fields:  []string{},
		},
		{
			name:    "set_active_window_tool",
			newTool: func() (tool.InvokableTool, error) { return NewSetActiveWindowTool(client) },
			fields:  []string{"id", "pid", "application", "title", "rect"},
		},
		{
			name:    "click_element_tool",
			newTool: func() (tool.InvokableTool, error) { return NewClickElementTool(client) },
			fields: []string{
				"text", "role", "match", "direction", "max_depth", "button",
				"double", "offset_x", "offset_y", "keep_cursor", "source",
			},
		},
		{
			name:    "click_mouse_at_tool",
			newTool: func() (tool.InvokableTool, error) { return NewClickMouseAtTool(client) },
			fields:  []string{"x", "y", "button", "double"},
		},
		{
			name:    "move_mouse_tool",
			newTool: func() (tool.InvokableTool, error) { return NewMoveMouseTool(client) },
			fields:  []string{"x", "y"},
		},
		{
			name:    "mouse_button_tool",
			newTool: func() (tool.InvokableTool, error) { return NewMouseButtonTool(client) },
			fields:  []string{"state", "button"},
		},
		{
			name:    "drag_mouse_tool",
			newTool: func() (tool.InvokableTool, error) { return NewDragMouseTool(client) },
			fields:  []string{"from_x", "from_y", "to_x", "to_y"},
		},
		{
			name:    "scroll_mouse_tool",
			newTool: func() (tool.InvokableTool, error) { return NewScrollMouseTool(client) },
			fields:  []string{"direction", "amount", "x", "y"},
		},
		{
			name:    "type_text_tool",
			newTool: func() (tool.InvokableTool, error) { return NewTypeTextTool(client) },
			fields:  []string{"text"},
		},
		{
			name:    "key_tap_tool",
			newTool: func() (tool.InvokableTool, error) { return NewKeyTapTool(client) },
			fields:  []string{"key", "modifiers", "count"},
		},
		{
			name:    "clipboard_tool",
			newTool: func() (tool.InvokableTool, error) { return NewClipboardTool(client) },
			fields:  []string{"text"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			invokable, err := tc.newTool()
			if err != nil {
				t.Fatalf("构造工具失败: %v", err)
			}
			info, err := invokable.Info(context.Background())
			if err != nil {
				t.Fatalf("读取工具信息失败: %v", err)
			}
			if info.Name != tc.name {
				t.Errorf("工具名不符: %q", info.Name)
			}
			if info.ParamsOneOf == nil {
				t.Fatal("工具没有参数 schema")
			}
			jsonSchema, err := info.ParamsOneOf.ToJSONSchema()
			if err != nil {
				t.Fatalf("生成 JSON schema 失败: %v", err)
			}
			raw, err := json.Marshal(jsonSchema)
			if err != nil {
				t.Fatalf("序列化 schema 失败: %v", err)
			}
			for _, field := range tc.fields {
				if !strings.Contains(string(raw), `"`+field+`"`) {
					t.Errorf("schema 缺少字段 %q: %s", field, raw)
				}
			}
		})
	}
}
