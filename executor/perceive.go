package executor

import (
	"context"
	"fmt"

	"github.com/LouYuanbo1/robopilot-mcp/action"
	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver"
)

// 本文件只保留"取元素"的公共管道：按来源策略调用感知器、必要时回退视觉，
// 供 inspect_screen 与 click_element 共用。感知器的能力接口在 perceiver 包里定义。

// errNoPerceiver 生成统一的"未配置感知器"错误。
func errNoPerceiver(actionName string) error {
	return fmt.Errorf(
		"未配置元素感知器(perceiver)，无法执行 %s；请通过 executor.WithPerceiver 注入(如 perceiver/neru 实现)",
		actionName,
	)
}

// scanElements 按范围扫描元素：感知器实现了 Scoper 就按范围扫，否则退化为前台窗口扫描。
func (e *Executor) scanElements(ctx context.Context, req perceiver.ScanRequest) ([]perceiver.Element, error) {
	if e.perceiver == nil {
		return nil, errNoPerceiver("元素扫描")
	}
	if scoper, ok := e.perceiver.(perceiver.Scoper); ok {
		return scoper.PerceiveScope(ctx, req)
	}
	return e.perceiver.Perceive(ctx)
}

// visionElements 用像素视觉检测可点击目标；strategy 为空时用 OCR + 矩形 + 分类。
func (e *Executor) visionElements(ctx context.Context, strategy string) ([]perceiver.Element, error) {
	if e.perceiver == nil {
		return nil, errNoPerceiver("像素视觉检测")
	}
	detector, ok := e.perceiver.(perceiver.TargetDetector)
	if !ok {
		return nil, fmt.Errorf("当前感知器不支持像素视觉检测，请把 source 留空或改为 ax")
	}
	return detector.DetectTargets(ctx, perceiver.TargetRequest{Strategy: strategy})
}

// scopeIsExplicit 报告这次扫描是否点名了具体目标（指定应用/辅助表面）。
//
// 点名了目标却没读到东西，说明目标本身有问题（应用没在跑、平台不支持该表面），
// 此时回退到前台窗口会答非所问，必须把错误如实上报。
func scopeIsExplicit(scope string) bool {
	return scope == "app" || scope == "surface"
}

// collectElements 按来源策略取回元素，返回元素与真正生效的来源。
//
// auto（默认）先扫无障碍树：树给出元素就直接用，这是最准且最快的一手信息；
// 树为空(游戏/自绘界面)或扫描失败时再回退像素视觉，并把两边的失败信息一起报出来。
func (e *Executor) collectElements(
	ctx context.Context,
	req perceiver.ScanRequest,
	source action.ElementSource,
	strategy string,
) ([]perceiver.Element, string, error) {
	if e.perceiver == nil {
		return nil, "", errNoPerceiver("元素感知")
	}

	switch source {
	case action.SourceAX:
		els, err := e.scanElements(ctx, req)
		return els, "ax", err
	case action.SourceVision:
		els, err := e.visionElements(ctx, strategy)
		return els, "vision", err
	}

	axElements, axErr := e.scanElements(ctx, req)
	if axErr == nil && len(axElements) > 0 {
		return axElements, "ax", nil
	}
	if axErr != nil && scopeIsExplicit(req.Scan.Scope) {
		return nil, "ax", axErr
	}

	visionElements, visionErr := e.visionElements(ctx, strategy)
	if visionErr == nil && len(visionElements) > 0 {
		return visionElements, "vision", nil
	}

	switch {
	case axErr != nil && visionErr != nil:
		return nil, "", fmt.Errorf("无障碍树扫描失败(%v)，像素视觉检测也失败(%v)", axErr, visionErr)
	case axErr != nil:
		return nil, "ax", axErr
	default:
		// 两种来源都成功但没有元素：这是"屏幕上确实没有可交互元素"的合法答案。
		return axElements, "ax", nil
	}
}
