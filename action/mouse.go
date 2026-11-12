package action

// 鼠标类动作：移动 / 点击 / 按键相位 / 拖拽 / 滚轮。
// 坐标均为屏幕绝对坐标(左上角为原点)，单位像素。

// MouseMove 把指针移动到指定屏幕坐标。
type MouseMove struct {
	X int `json:"x" jsonschema:"目标横坐标"`
	Y int `json:"y" jsonschema:"目标纵坐标"`
}

func (*MouseMove) isAction() {}

// MouseClick 在当前指针位置按下并抬起鼠标键；可双击。
// 定位不做：需要点击指定坐标时，先用 MouseMove 移动指针。
type MouseClick struct {
	Button MouseButton `json:"button,omitempty" jsonschema:"要点击的鼠标键,默认left"`
	Double bool        `json:"double,omitempty" jsonschema:"是否双击,默认false"`
}

func (*MouseClick) isAction() {}

// MouseClickAt 复合动作：先 Move 到指定坐标，再在该处触发一次 Click。
// 是 MouseMove + MouseClick 的便捷门面，供调用方一步"点到某处"。
// X/Y 为指针且可省略：不提供（或只提供一个）时，改为在当前光标位置直接点击。
type MouseClickAt struct {
	X *int `json:"x,omitempty" jsonschema:"目标横坐标(屏幕绝对坐标),省略则在当前光标处点击"`
	Y *int `json:"y,omitempty" jsonschema:"目标纵坐标(屏幕绝对坐标),省略则在当前光标处点击"`
	MouseClick
}

func (*MouseClickAt) isAction() {}

// MouseButtonAction 在当前位置按住或松开某个鼠标键。
//
// 它把"按住—移动—松开"拆成多步，用来实现分段拖拽、框选、按住拖过多个落点等
// 单步 Drag 表达不了的操作；两种相位共用一个动作，避免调用方在
// "按下工具/松开工具"之间做选择。
type MouseButtonAction struct {
	// State 相位：down 按住，up 松开。
	State string `json:"state" jsonschema:"相位,取值 down 按住/up 松开"`
	// Button 要操作的鼠标键，默认 left。
	Button MouseButton `json:"button,omitempty" jsonschema:"要操作的鼠标键,默认left"`
}

func (*MouseButtonAction) isAction() {}

// MouseDrag 按住某键从左点拖拽到右点，用于选区/拖动元素。
type MouseDrag struct {
	FromX int `json:"from_x" jsonschema:"拖拽起点横坐标"`
	FromY int `json:"from_y" jsonschema:"拖拽起点纵坐标"`
	ToX   int `json:"to_x" jsonschema:"拖拽终点横坐标"`
	ToY   int `json:"to_y" jsonschema:"拖拽终点纵坐标"`
}

func (*MouseDrag) isAction() {}

// MouseScroll 在指针当前位置滚动滚轮。
//
// X/Y 可选：提供时先把指针移到该点再滚动，省去一次 move_mouse 调用，
// 用于"滚动这个列表/这个面板"而光标还停在别处的情况。
type MouseScroll struct {
	Direction ScrollDirection `json:"direction" jsonschema:"滚动方向,取值up/down"`
	Amount    int             `json:"amount" jsonschema:"滚动量(格数,正整数)"`
	X         *int            `json:"x,omitempty" jsonschema:"可选:滚动前把指针移到该横坐标(屏幕绝对坐标)"`
	Y         *int            `json:"y,omitempty" jsonschema:"可选:滚动前把指针移到该纵坐标(屏幕绝对坐标)"`
}

func (*MouseScroll) isAction() {}
