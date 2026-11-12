package action

// 键盘类动作：整段文本输入 / 单键(含组合键) / 长按与释放。
// 名称对应用户键盘布局，与 robotgo 的 keycode 映射关系在 runtime 层实现。

// TypeText 把一段文本按输入法逐字敲入当前焦点控件。
type TypeText struct {
	Text string `json:"text" jsonschema:"要输入的文本内容(必填)"`
}

func (*TypeText) isAction() {}

// KeyTap 发送一次按键；带 Modifiers 表示组合键（如按键为 c 且修饰符含 ctrl 即 Ctrl+C）。
type KeyTap struct {
	Key       string   `json:"key" jsonschema:"要发送的按键名,如 a/enter/F5/space 等"`
	Modifiers []string `json:"modifiers,omitempty" jsonschema:"组合修饰键,如 ctrl/alt/shift/win"`
	Count     int      `json:"count,omitempty" jsonschema:"连按次数,默认1"`
}

func (*KeyTap) isAction() {}
