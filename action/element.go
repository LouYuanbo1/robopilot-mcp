package action

// 元素级动作：先"找到"再"操作"，把坐标计算从模型手里收回执行层。
//
// 与坐标类动作(mouse_click_at)的区别：调用方只说"点名哪个控件"或"光标往右第一个"，
// 定位交给无障碍树/视觉检测，再由执行层换算成点击点。名称、层级、窗口缩放变化时
// 不需要重新猜坐标。

// ClickElement 按名称、角色或空间方向定位一个可交互元素，并点击它。
//
// 定位优先级：
//  1. Text 非空时按文本匹配（可叠加 Role 收窄）；
//  2. Direction 非空时按空间方向从当前光标出发取最近元素；
//  3. Source 允许回退时，无障碍树没找到会再用像素视觉(OCR)找一次。
//
// 匹配到多个元素时，优先精确匹配、可点击、无障碍来源，再取面积较小者
// （容器通常比真正的控件大，取小的更接近用户意图）。
type ClickElement struct {
	Scan

	// Text 目标元素的名称/描述/取值关键字。
	Text string `json:"text,omitempty" jsonschema:"目标元素的名称关键字(在名称/描述/取值里匹配);与 direction 至少提供其一"`
	// Role 限定中立 role，用于同名元素过多时收窄。
	Role string `json:"role,omitempty" jsonschema:"限定元素的 role 用于收窄,如 button/link/menu_item/text_field,空不限定"`
	// Match 文本匹配方式，默认 contains。
	Match MatchMode `json:"match,omitempty" jsonschema:"文本匹配方式,取值 contains(默认)/exact/prefix"`
	// Direction 空间方向：从当前光标出发选该方向上最近的元素。
	Direction string `json:"direction,omitempty" jsonschema:"从当前光标出发的空间方向,取值 up/down/left/right;提供时优先按方向选最近元素"`
	// MaxDepth 限制无障碍树遍历深度，0 用默认值。
	MaxDepth int `json:"max_depth,omitempty" jsonschema:"限制无障碍树遍历深度,传0用默认深度"`

	// Button 要按下的鼠标键，默认 left。
	Button MouseButton `json:"button,omitempty" jsonschema:"要按下的鼠标键,默认 left"`
	// Double 是否双击。
	Double bool `json:"double,omitempty" jsonschema:"是否双击,默认 false"`
	// OffsetX 点击点相对元素中心的横向偏移(像素)。
	OffsetX int `json:"offset_x,omitempty" jsonschema:"点击点相对元素中心的横向偏移(像素),默认0即正中心"`
	// OffsetY 点击点相对元素中心的纵向偏移(像素)。
	OffsetY int `json:"offset_y,omitempty" jsonschema:"点击点相对元素中心的纵向偏移(像素),默认0即正中心"`
	// KeepCursor 是否把光标留在目标处；默认 false，即点击后把指针放回原处。
	KeepCursor bool `json:"keep_cursor,omitempty" jsonschema:"是否把光标留在目标元素处,默认 false 即点击后把指针放回原处,不惊扰用户"`
	// Source 感知来源，省略表示先无障碍树、没找到再回退视觉。
	Source ElementSource `json:"source,omitempty" jsonschema:"感知来源,取值 ax(仅无障碍树)/vision(仅像素视觉)/省略(先无障碍树,没找到再回退视觉)"`
}

func (*ClickElement) isAction() {}
