package action

// ActionResult 单个动作执行后的结构化结果，是动作协议的另一半
// （请求 Action / 响应 ActionResult）。
//
// Effective 统一表示"动作执行成功，且预期结果可用"：
//   - 移动/点击/按键/拖拽/滚动：无异常完成 => 生效
//   - 截图：成功落盘（产物路径见 ArtifactPath）或成功取到图片字节（ImagePNG）=> 生效
//   - 查询类（list_windows / clipboard / inspect_screen 等）：成功取回结果 => 生效
//
// Value：动作返回的"内容"，如窗口列表、剪贴板文本、元素 JSON、截图落盘时的说明；
// ArtifactPath：落盘产物的文件路径（如截图）。与 Value 分离，
// 避免把"文件位置"和"返回内容"混在一个字段里；
// ImagePNG：不落盘时随结果返回的图片字节，只在进程内传递，不进 JSON。
type ActionResult struct {
	Action       string `json:"action" jsonschema:"本次执行的动作名,例如 mouse_click / screenshot"`
	Value        string `json:"value" jsonschema:"动作返回的主要内容:窗口列表/剪贴板文本/坐标等,可能为空"`
	ArtifactPath string `json:"artifact_path" jsonschema:"落盘产物路径(如截图),为空表示无产物"`
	Effective    bool   `json:"effective" jsonschema:"动作是否执行成功且预期结果可用"`

	// ImagePNG 是不落盘时直接随结果返回的 PNG 字节。
	//
	// 它只用于进程内传递（server 把它包成 MCP 的图片内容）：json:"-" 让它既不参与
	// JSON 序列化，也不出现在 MCP 工具的输出 schema 里，避免把二进制塞进文本结果。
	ImagePNG []byte `json:"-"`
}
