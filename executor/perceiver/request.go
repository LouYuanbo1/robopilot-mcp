package perceiver

import "github.com/LouYuanbo1/robopilot-mcp/action"

// Info 是桌面状态的快照：几何、光标、前台窗口与感知能力可用性。
type Info struct {
	// VirtualDesktop 整个虚拟桌面（多显示器并集）的屏幕绝对坐标区域，可能有负坐标。
	VirtualDesktop action.Region `json:"virtual_desktop"`
	// Cursor 当前光标位置。
	Cursor action.Point `json:"cursor"`
	// Window 前台窗口信息。
	Window WindowInfo `json:"frontmost_window"`
	// Accessibility 无障碍树读取能力。
	Accessibility Capability `json:"accessibility"`
	// Capture 屏幕捕获能力。
	Capture Capability `json:"capture"`
	// OCR 文字识别能力。
	OCR Capability `json:"ocr"`
}

// WindowInfo 前台窗口的可读信息。
type WindowInfo struct {
	Title       string `json:"title,omitempty"`
	PID         int    `json:"pid,omitempty"`
	Application string `json:"application,omitempty"`
}

// Capability 描述一项感知能力是否可用；不可用时 Detail 说明原因与补救办法。
type Capability struct {
	Available bool   `json:"available"`
	Detail    string `json:"detail,omitempty"`
}

// TextRequest 一次 OCR 读取的参数。
type TextRequest struct {
	// Region 只读取该屏幕区域；nil 表示活动显示器整屏。
	Region *action.Region
	// Languages 期望的识别语言，空表示引擎默认。
	Languages []string
	// WordLevel 是否按词切分。
	WordLevel bool
	// MinConfidence 置信度下限。
	MinConfidence float64
}

// TextResult 一次 OCR 读取的结果。
type TextResult struct {
	// Text 识别出的整段文字（按引擎阅读顺序用空格拼接）。
	Text string `json:"text"`
	// Region 实际读取的屏幕区域。
	Region action.Region `json:"region"`
	// Words 词块列表（含屏幕坐标），供按文字位置点击。
	Words []Word `json:"words,omitempty"`
}

// Word 一个识别出的词/行及其屏幕矩形。
type Word struct {
	Text       string  `json:"text"`
	X          int     `json:"x"`
	Y          int     `json:"y"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Confidence float64 `json:"confidence,omitempty"`
}

// Center 返回词块中心点坐标。
func (w Word) Center() (x, y int) { return w.X + w.Width/2, w.Y + w.Height/2 }

// TargetRequest 一次视觉检测的参数。
type TargetRequest struct {
	// Strategy 检测策略：vision（OCR + 矩形，带语义标签）、contour（轮廓）、segment（分割）。
	Strategy string
	// Targets segment 策略的检测目标。
	Targets string
	// Region 只检测该屏幕区域；nil 表示活动显示器整屏。
	Region *action.Region
}
