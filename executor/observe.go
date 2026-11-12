package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/go-vgo/robotgo"

	"github.com/LouYuanbo1/robopilot-mcp/action"
	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver"
)

// 观察类动作的执行落点：inspect_screen（桌面状态 + 元素 + 可选文字/截图）与 read_text（OCR）。
//
// 分工：分辨率、DPI 缩放与显示器数量这些屏幕参数由 robotgo 提供；前台窗口来自感知器的
// 窗口管理能力（neru-core 的 window 包），元素、文字、视觉目标则由感知器提供。
// 两者在 screenInfoPayload 里合并成一份完整快照，模型一次调用就能建立坐标系认知。

// screenInfoPayload 是 inspect_screen 返回的屏幕状态快照。
type screenInfoPayload struct {
	// ScreenWidth/ScreenHeight 主显示器分辨率(物理像素)。
	ScreenWidth  int `json:"screen_width"`
	ScreenHeight int `json:"screen_height"`
	// DPIScale 系统 DPI 缩放(1.0 表示 100%，1.5 表示 150%)。
	DPIScale float64 `json:"dpi_scale"`
	// Displays 显示器数量。
	Displays int `json:"displays"`
	// VirtualDesktop 整个虚拟桌面区域，多显示器时左上角可能是负坐标。
	VirtualDesktop action.Region `json:"virtual_desktop"`
	// Cursor 当前光标位置。
	Cursor action.Point `json:"cursor"`
	// Frontmost 前台窗口（标题/PID/应用）。
	Frontmost perceiver.WindowInfo `json:"frontmost_window"`
	// Accessibility/Capture/OCR 三项感知能力的可用性。
	Accessibility perceiver.Capability `json:"accessibility"`
	Capture       perceiver.Capability `json:"capture"`
	OCR           perceiver.Capability `json:"ocr"`
}

// buildScreenInfo 汇总屏幕状态：先用感知器拿桌面几何、前台窗口与能力状态，
// 再用 robotgo 补齐分辨率、DPI 缩放与显示器数量。
//
// 顺序很重要：感知器初始化时会先把进程设为 per-monitor DPI aware，
// 之后 robotgo 读到的才是物理像素，两边坐标系一致。
func (e *Executor) buildScreenInfo(ctx context.Context) screenInfoPayload {
	var payload screenInfoPayload

	if provider, ok := e.perceiver.(perceiver.InfoProvider); ok {
		if info, err := provider.Info(ctx); err == nil {
			payload.VirtualDesktop = info.VirtualDesktop
			payload.Cursor = info.Cursor
			payload.Frontmost = info.Window
			payload.Accessibility = info.Accessibility
			payload.Capture = info.Capture
			payload.OCR = info.OCR
		}
	}

	width, height := robotgo.GetScreenSize()
	payload.ScreenWidth, payload.ScreenHeight = width, height
	payload.DPIScale = robotgo.ScaleF()
	payload.Displays = robotgo.DisplaysNum()

	if payload.VirtualDesktop.Empty() {
		rect := robotgo.GetScreenRect(1)
		payload.VirtualDesktop = action.Region{X: rect.X, Y: rect.Y, Width: rect.W, Height: rect.H}
	}
	if payload.Cursor == (action.Point{}) {
		x, y := robotgo.GetMousePos()
		payload.Cursor = action.Point{X: x, Y: y}
	}

	// 没注入感知器时，三项能力都标成不可用并给出同一句原因，避免留空让模型误判。
	if e.perceiver == nil {
		detail := "未配置感知器(perceiver)，无障碍/OCR/视觉检测均不可用"
		if !payload.Accessibility.Available && payload.Accessibility.Detail == "" {
			payload.Accessibility.Detail = detail
		}
		if !payload.Capture.Available && payload.Capture.Detail == "" {
			payload.Capture.Detail = detail
		}
		if !payload.OCR.Available && payload.OCR.Detail == "" {
			payload.OCR.Detail = detail
		}
	}

	return payload
}

// observe 分发观察类动作并组装 ActionResult。
func (e *Executor) observe(ctx context.Context, a action.Action) (action.ActionResult, error) {
	switch v := a.(type) {
	case *action.InspectScreen:
		return e.inspectScreen(ctx, v)
	case *action.ReadText:
		return e.readText(ctx, v)
	default:
		return action.ActionResult{}, fmt.Errorf("非观察类动作: %T", a)
	}
}

// inspectScreen 是唯一的"看"入口：桌面状态 + 可交互元素 + 可选 OCR/截图。
//
// 各部分都是尽力而为：元素、文字、截图某一项失败只记进 notes，仍返回其余信息；
// 但显式指定的目标（source=ax / scope=app / scope=surface）失败时不降级，直接报错，
// 避免"答非所问"地回退到前台窗口。
func (e *Executor) inspectScreen(ctx context.Context, v *action.InspectScreen) (action.ActionResult, error) {
	res := action.ActionResult{Action: "inspect_screen"}

	if v.Scope == "surface" && runtime.GOOS != "darwin" {
		return res, fmt.Errorf(
			"scope=surface 仅 macOS 支持，当前平台是 %s；请改用 scope=window/application/app",
			runtime.GOOS,
		)
	}

	payload := struct {
		screenInfoPayload
		Elements       []perceiver.RenderElement `json:"elements"`
		ElementsSource string                    `json:"elements_source,omitempty"`
		Text           string                    `json:"text,omitempty"`
		Screenshot     string                    `json:"screenshot,omitempty"`
		Notes          []string                  `json:"notes,omitempty"`
	}{screenInfoPayload: e.buildScreenInfo(ctx)}

	elements, used, err := e.collectElements(ctx, perceiver.ScanRequest{
		Scan:     v.Scan,
		Roles:    v.Roles,
		MaxDepth: v.MaxDepth,
	}, v.Source, v.Strategy)

	explicitTarget := v.Source != action.SourceAuto || v.Scope == "app" || v.Scope == "surface"
	if err != nil {
		if explicitTarget {
			return res, err
		}
		payload.Notes = append(payload.Notes, "元素感知失败: "+err.Error())
	} else {
		payload.Elements = perceiver.RenderElements(perceiver.Clean(elements, perceiver.CleanOptions{
			MinArea: v.MinArea,
			Max:     v.Max,
			Query:   v.Query,
			Region:  v.Region,
		}))
		payload.ElementsSource = used
	}

	if v.IncludeText {
		if reader, ok := e.perceiver.(perceiver.TextReader); ok {
			text, textErr := reader.ReadText(ctx, perceiver.TextRequest{Region: v.Region})
			if textErr != nil {
				payload.Notes = append(payload.Notes, "OCR 读取失败: "+textErr.Error())
			} else {
				payload.Text = text.Text
			}
		} else {
			payload.Notes = append(payload.Notes, "当前感知器不支持 OCR")
		}
	}

	if v.Screenshot {
		path := e.artifactPath(v.Artifact.Dir, v.Artifact.Name)
		sw, sh := robotgo.GetScreenSize()
		x, y, width, height := 0, 0, sw, sh
		if v.Region != nil {
			x, y, width, height = v.Region.X, v.Region.Y, v.Region.Width, v.Region.Height
		}
		if shotErr := saveShot(x, y, width, height, path); shotErr != nil {
			payload.Notes = append(payload.Notes, "截图失败: "+shotErr.Error())
		} else {
			payload.Screenshot = path
			res.ArtifactPath = path
		}
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return res, err
	}
	res.Value, res.Effective = string(encoded), true
	return res, nil
}

// readText 执行 OCR：默认把整段文字回填到 Value，Words 为真时改回填带坐标的词块 JSON。
func (e *Executor) readText(ctx context.Context, v *action.ReadText) (action.ActionResult, error) {
	res := action.ActionResult{Action: "read_text"}

	if e.perceiver == nil {
		return res, errNoPerceiver(res.Action)
	}
	reader, ok := e.perceiver.(perceiver.TextReader)
	if !ok {
		return res, fmt.Errorf("当前感知器不支持 OCR 读字，请改用 screenshot 后自行识图，或换用支持 OCR 的感知器")
	}

	result, err := reader.ReadText(ctx, perceiver.TextRequest{
		Region:        v.Region,
		Languages:     v.Languages,
		WordLevel:     v.WordLevel,
		MinConfidence: v.MinConfidence,
	})
	if err != nil {
		return res, err
	}

	if !v.Words {
		res.Value, res.Effective = result.Text, true
		return res, nil
	}

	limit := v.Max
	if limit <= 0 {
		limit = 200
	}
	words := result.Words
	if len(words) > limit {
		words = words[:limit]
	}
	rendered := make([]wordPayload, 0, len(words))
	for _, w := range words {
		cx, cy := w.Center()
		rendered = append(rendered, wordPayload{Word: w, Center: action.Point{X: cx, Y: cy}})
	}
	encoded, err := json.Marshal(rendered)
	if err != nil {
		return res, err
	}
	res.Value, res.Effective = string(encoded), true
	return res, nil
}

// wordPayload 是词块序列化的结构：内嵌词块并附上中心点，便于直接按坐标点击。
type wordPayload struct {
	perceiver.Word
	Center action.Point `json:"center"`
}
