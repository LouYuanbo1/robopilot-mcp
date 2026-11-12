package action

// 屏幕类动作：截图。
//
// 桌面几何、光标与前台窗口这类状态信息由 InspectScreen 一并返回，
// 这里只保留"把画面取出来"这一件事——按 Output 决定落盘、直接返回，或两者都要。

// 截图的输出方式。
const (
	// ScreenshotOutputFile 只落盘并返回路径（默认）。产物可用作审计留档或后续复用。
	ScreenshotOutputFile = "file"
	// ScreenshotOutputInline 不落盘，PNG 字节直接随结果返回给支持看图的客户端。
	ScreenshotOutputInline = "inline"
	// ScreenshotOutputBoth 既落盘又返回图片内容，兼顾留档与直读。
	ScreenshotOutputBoth = "both"
)

// Screenshot 截取全屏或指定区域。
type Screenshot struct {
	// Region 为 nil 时截取全屏；否则只截取该矩形区域。
	Region *Region `json:"region,omitempty" jsonschema:"要截取的屏幕区域,省略则截全屏"`
	// Output 决定产出形态，省略等同 file。
	Output string `json:"output,omitempty" jsonschema:"输出方式,取值 file(默认,只落盘返回路径)/inline(不落盘,图片内容直接随结果返回)/both(既落盘又返回图片)"`
	Artifact
}

func (*Screenshot) isAction() {}

// WantsImage 报告本动作的结果是否需要携带图片内容。
func (s *Screenshot) WantsImage() bool {
	return s.Output == ScreenshotOutputInline || s.Output == ScreenshotOutputBoth
}
