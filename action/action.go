// Package action 是 robopilot-mcp 的桌面操控动作协议层。
//
// 它只描述"做什么"（Action）与"做完返回什么"（ActionResult），不涉及任何执行细节，
// 是各模块之间传递任务与结果的统一中立契约：
//   - executor 依赖本包，把输入类动作翻译成 robotgo 操作，把窗口类动作交给 perceiver 的窗口能力；
//   - server   把动作结构体映射为 MCP 工具入参；
//   - 解析核心（action.Spec + action.ParseActions，见 parse.go）对任意格式/配置类型无感知。
//
// 依赖方向始终指向本包，本包不反向依赖任何上层模块，且刻意不与 robotgo/neru-core 耦合：
// 这里的动作是"纯协议"，可在编译期隔离那串沉重的 robotgo 传递依赖（win API/screenshot/OCR...）。
package action

// Action 是所有动作的密封接口：各具体动作都实现私有方法 isAction
type Action interface {
	isAction()
}

// ---- 动作间共享的公共值类型 ----

// MouseButton 鼠标按键。
type MouseButton string

const (
	MouseButtonLeft   MouseButton = "left"
	MouseButtonRight  MouseButton = "right"
	MouseButtonMiddle MouseButton = "middle"
)

// ScrollDirection 滚轮滚动方向。
type ScrollDirection string

const (
	ScrollUp   ScrollDirection = "up"
	ScrollDown ScrollDirection = "down"
	//在Windows系统中，水平滚动不一定有用，且用处不大
	//ScrollLeft  ScrollDirection = "left"
	//ScrollRight ScrollDirection = "right"
)

// Region 屏幕上的矩形区域，用于截屏等限定范围的操作。
// 坐标以屏幕左上角为原点。
type Region struct {
	X      int `json:"x" jsonschema:"区域左边界横坐标"`
	Y      int `json:"y" jsonschema:"区域上边界纵坐标"`
	Width  int `json:"width" jsonschema:"区域宽度"`
	Height int `json:"height" jsonschema:"区域高度"`
}

// Empty 判断区域是否不含任何像素，供执行层做入参校验。
func (r Region) Empty() bool { return r.Width <= 0 || r.Height <= 0 }

// Contains 判断点是否落在区域内（左闭右开，与屏幕像素栅格一致）。
func (r Region) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.Width && y >= r.Y && y < r.Y+r.Height
}

// Point 屏幕绝对坐标点，单位像素，原点在屏幕左上角。
type Point struct {
	X int `json:"x" jsonschema:"横坐标(屏幕绝对坐标,像素)"`
	Y int `json:"y" jsonschema:"纵坐标(屏幕绝对坐标,像素)"`
}

// ElementSource 选择用哪种感知来源找出元素。
//
// 无障碍(AX)树给出的元素自带角色、名称与焦点状态，是最可靠的一手信息；
// 像素视觉(截图 + OCR / 轮廓检测)在 AX 树为空时(游戏、自绘界面、远程桌面画面)兜底。
type ElementSource string

const (
	// SourceAuto 先扫无障碍树，树为空或失败时自动回退像素视觉。
	SourceAuto ElementSource = ""
	// SourceAX 只用无障碍树，不触发截图与 OCR，速度最快。
	SourceAX ElementSource = "ax"
	// SourceVision 只用像素视觉检测，适合 AX 树读不到内容的界面。
	SourceVision ElementSource = "vision"
)

// MatchMode 文本匹配方式，用于按名称定位元素。
type MatchMode string

const (
	// MatchContains 子串匹配(忽略大小写)，默认值。
	MatchContains MatchMode = "contains"
	// MatchExact 完全相等(忽略大小写)，适合名称短且易混淆的控件。
	MatchExact MatchMode = "exact"
	// MatchPrefix 前缀匹配(忽略大小写)。
	MatchPrefix MatchMode = "prefix"
)

// Scan 描述一次感知扫描的范围，是元素类动作共享的"看哪里"参数。
//
// 各平台能力不同：Windows 的 surface 一律返回不支持；scope 为 app 时
// Windows 用可执行文件名(如 notepad.exe)，macOS 用 bundle id。
type Scan struct {
	// Scope 扫描范围，省略等同 window。
	Scope string `json:"scope,omitempty" jsonschema:"扫描范围,取值 window 前台窗口(默认)/application 前台应用全部窗口/app 由 application 指定的应用/surface 平台辅助表面"`
	// Application 目标应用标识，仅在 scope 为 app 时使用。
	Application string `json:"application,omitempty" jsonschema:"scope 为 app 时的目标应用:Windows 传可执行文件名如 notepad.exe,macOS 传 bundle id"`
	// Surface 平台辅助表面名，仅在 scope 为 surface 时使用。
	Surface string `json:"surface,omitempty" jsonschema:"scope 为 surface 时的表面名:dock/notification_center/stage_manager/picture_in_picture/screen_capture_ui,仅 macOS 支持"`
}

// Artifact 描述一份落盘产物的保存位置与命名，供产物型动作复用。
// Dir 为空时使用默认保存目录；Name 为空时由生成方按内容自动生成。
type Artifact struct {
	// Dir 指定产物落盘目录；为空时使用默认目录。
	Dir string `json:"artifact_dir,omitempty" jsonschema:"产物要保存的目录,为空时用默认目录"`
	// Name 指定产物文件名（不含扩展名）；为空时由生成方自动生成。
	Name string `json:"artifact_name,omitempty" jsonschema:"产物文件名(不含扩展名),为空时自动生成"`
}
