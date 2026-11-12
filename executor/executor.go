package executor

import (
	"context"

	"github.com/LouYuanbo1/robopilot-mcp/action"
	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver"
)

// Executor 是桌面操控的运行时门面：持有一份执行配置，并把动作批量/单个执行。
// server 等上层只与 Executor 打交道；robotgo 与 perceiver 的调用收敛在 exec.go 及各领域文件。
type Executor struct {
	// Humanize 开启拟人：鼠标平滑移动 + 动作间随机延迟。
	humanize   bool
	minDelayMS int
	maxDelayMS int
	// SafeRegion 光标允许操作的屏幕区域(虚拟桌面坐标)；nil 表示不限制。
	// 用于防止 LLM/脚本把鼠标发到危险区域，Move/Click 前会做拦截。
	safeRegion *action.Region
	// ArtifactDir 落盘产物(截图)的默认目录；动作自己指定了 Dir 时优先用动作的。
	artifactDir string
	// perceiver 感知与窗口管理的统一接入点；nil 时执行感知类/窗口类动作会报错。
	// 任何实现 perceiver.Perceiver 的感知器(如 perceiver/neru)都可通过 WithPerceiver 注入；
	// 额外实现了 Scoper / InfoProvider / TextReader / TargetDetector / SpatialPicker / WindowManager 的，
	// 会自动解锁对应的能力（多范围扫描、屏幕信息、OCR、视觉检测、空间导航、窗口枚举/激活/摆放）。
	perceiver perceiver.Perceiver
}

type Option func(*Executor)

func WithHumanize(humanize bool) Option {
	return func(e *Executor) {
		e.humanize = humanize
	}
}

func WithDelayMS(min int, max int) Option {
	return func(e *Executor) {
		e.minDelayMS = min
		e.maxDelayMS = max
	}
}

func WithSafeRegion(r *action.Region) Option {
	return func(e *Executor) {
		e.safeRegion = r
	}
}

func WithArtifactDir(dir string) Option {
	return func(e *Executor) {
		e.artifactDir = dir
	}
}

// WithPerceiver 注入"屏幕感知器"。任意感知库只要实现 perceiver.Perceiver
// (如 perceiver/neru)即可，之后 executor 就能执行感知类动作。
// 不注入时执行这些动作会返回明确错误。
func WithPerceiver(p perceiver.Perceiver) Option {
	return func(e *Executor) {
		e.perceiver = p
	}
}

func New(opts ...Option) *Executor {
	e := &Executor{
		humanize:    true,
		minDelayMS:  100,
		maxDelayMS:  200,
		safeRegion:  nil,
		artifactDir: "",
		perceiver:   nil,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Execute 按顺序执行一批动作，返回每步结果；任一步出错即中断并返回已执行的结果与错误。
// ctx 会传给需要长时间运行的感知动作（无障碍树扫描、OCR、视觉检测），
// 让上层超时与取消真正生效。
func (e *Executor) Execute(ctx context.Context, acts []action.Action) ([]action.ActionResult, error) {
	results := make([]action.ActionResult, 0, len(acts))
	for _, a := range acts {
		res, err := e.exec(ctx, a)
		if err != nil {
			return results, err
		}
		results = append(results, res)
	}
	return results, nil
}

// nameOf 返回动作的规范动作名，供 ActionResult.Action 回填。
// 不用类型名反射拼接（会残留 '.'），而是逐个穷举，编译期可保证覆盖完整。
func nameOf(a action.Action) string {
	switch a.(type) {
	case *action.MouseMove:
		return "mouse_move"
	case *action.MouseClick:
		return "mouse_click"
	case *action.MouseClickAt:
		return "mouse_click_at"
	case *action.MouseButtonAction:
		return "mouse_button"
	case *action.MouseDrag:
		return "mouse_drag"
	case *action.MouseScroll:
		return "mouse_scroll"
	case *action.ClickElement:
		return "click_element"
	case *action.TypeText:
		return "type_text"
	case *action.KeyTap:
		return "key_tap"
	case *action.Screenshot:
		return "screenshot"
	case *action.ListWindows:
		return "list_windows"
	case *action.SetActiveWindow:
		return "set_active_window"
	case *action.Clipboard:
		return "clipboard"
	case *action.InspectScreen:
		return "inspect_screen"
	case *action.ReadText:
		return "read_text"
	}
	return "unknown"
}
