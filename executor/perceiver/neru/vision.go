package neru

import (
	"context"
	"fmt"
	"strconv"

	"github.com/LouYuanbo1/neru-core/element"
	"github.com/LouYuanbo1/neru-core/vision"
	"github.com/LouYuanbo1/neru-core/vision/contour"

	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver"
)

// DetectTargets 用纯像素视觉检测可点击目标。
//
// 三种策略：
//   - vision：OCR 识别文字 + 轮廓补矩形，合并后用几何启发式分类出 button/link/text_field 等；
//   - contour：只找按钮形状的矩形框，速度最快，适合图标工具栏；
//   - segment：按行/短语/控件做像素分割，适合密集文字界面。
func (p *Perceiver) DetectTargets(ctx context.Context, req perceiver.TargetRequest) ([]perceiver.Element, error) {
	frame, _, err := captureFrame(ctx, req.Region)
	if err != nil {
		return nil, err
	}

	var found []*element.Element
	switch req.Strategy {
	case "contour":
		found, err = vision.DetectContours(ctx, frame, contour.DefaultParams())
	case "segment":
		targets := req.Targets
		if targets == "" {
			targets = "controls"
		}
		found, err = vision.DetectSegments(ctx, frame, targets)
	default:
		recognizer, ocrErr := p.ocrOnce()
		if ocrErr != nil {
			return nil, fmt.Errorf("视觉检测(vision 策略)需要 OCR 引擎: %w", ocrErr)
		}
		pipeline := &vision.Pipeline{
			Text:             recognizer,
			DetectText:       true,
			DetectRectangles: true,
			WordLevel:        true,
		}
		found, err = pipeline.Detect(ctx, frame)
	}
	if err != nil {
		return nil, fmt.Errorf("视觉检测失败: %w", err)
	}

	out := make([]perceiver.Element, 0, len(found))
	for _, e := range found {
		if e == nil {
			continue
		}
		b := e.Bounds()
		source := "ax"
		if e.IsVisionOnly() {
			source = "vision"
		}
		out = append(out, perceiver.Element{
			Role:        string(e.Role()),
			Title:       e.Title(),
			Description: e.Description(),
			Value:       e.Value(),
			Identifier:  string(e.ID()),
			Source:      source,
			Focused:     e.IsFocused(),
			Clickable:   e.IsClickable(),
			X:           b.Min.X,
			Y:           b.Min.Y,
			Width:       b.Dx(),
			Height:      b.Dy(),
		})
	}

	out = perceiver.Clean(out, perceiver.CleanOptions{
		MinArea: p.minArea,
		Max:     p.max,
	})
	for i := range out {
		out[i].Label = strconv.Itoa(i + 1)
	}
	return out, nil
}
