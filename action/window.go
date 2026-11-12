package action

// 窗口类动作：枚举、激活与摆放。
//
// 枚举返回每个可见顶层窗口的标题、PID、应用可执行文件名与屏幕位置；
// 拿到应用名后，可以配合 InspectScreen 的 scope=app 在不激活窗口的情况下读取该应用的内容。
// 激活支持按窗口 ID / PID / 应用名 / 标题定位，并可选把窗口摆放到指定矩形，
// 用于测试时把目标应用与日志窗口并排摆放。

// ListWindows 枚举当前所有可见顶层窗口。
type ListWindows struct{}

func (*ListWindows) isAction() {}

// SetActiveWindow 将指定窗口带到前台并聚焦。
type SetActiveWindow struct {
	// ID 目标窗口 id，来自 list_windows 的返回，最精确，优先使用。
	ID string `json:"id,omitempty" jsonschema:"目标窗口ID(来自list_windows返回,最精确,优先使用)"`
	// PID 目标进程 PID，最精确，优先使用。
	PID int `json:"pid,omitempty" jsonschema:"目标窗口进程PID(最精确,优先使用)"`
	// Application 目标应用的可执行文件名，如 chrome.exe，用于激活该应用的第一个可见窗口。
	Application string `json:"application,omitempty" jsonschema:"目标应用的可执行文件名,如 chrome.exe;激活该应用的第一个可见窗口"`
	// Title 目标窗口标题关键字，按真正的窗口标题做忽略大小写的子串匹配。
	Title string `json:"title,omitempty" jsonschema:"目标窗口标题关键字,匹配可见窗口的标题(不是可执行文件名)"`
	// Rect 可选：把窗口移动到该屏幕矩形并调整大小(物理像素)。
	// 给定时会先还原最大化/最小化再摆放，返回的 rect 是平台实际接受的矩形。
	// 左右半屏用 inspect_screen 的 virtual_desktop 计算，例如左半屏
	// {x:dx, y:dy, width:dw/2, height:dh}。
	Rect *Region `json:"rect,omitempty" jsonschema:"可选:把窗口摆放到该屏幕矩形(物理像素);左右半屏用 inspect_screen 的 virtual_desktop 计算,左半屏 {x:dx,y:dy,width:dw/2,height:dh};给定时会先还原最大化/最小化"`
}

func (*SetActiveWindow) isAction() {}
