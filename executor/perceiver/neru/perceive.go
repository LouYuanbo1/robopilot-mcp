package neru

import (
	"context"
	"fmt"
	"runtime"
	"strconv"

	"github.com/LouYuanbo1/neru-core/accessibility"
	"github.com/LouYuanbo1/neru-core/accessibility/ax"

	"github.com/LouYuanbo1/robopilot-mcp/action"
	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver"
)

// Perceive 扫描前台窗口的可交互元素，是 Perceiver 的基础能力。
func (p *Perceiver) Perceive(ctx context.Context) ([]perceiver.Element, error) {
	return p.PerceiveScope(ctx, perceiver.ScanRequest{})
}

// PerceiveScope 按指定范围扫描可交互元素，并做统一的过滤/去重/排序/截断。
func (p *Perceiver) PerceiveScope(ctx context.Context, req perceiver.ScanRequest) ([]perceiver.Element, error) {
	cl, err := p.clientOnce()
	if err != nil {
		return nil, fmt.Errorf("初始化无障碍感知失败: %w", err)
	}

	native, keep, err := perceiver.ResolveRoles(req.Roles)
	if err != nil {
		return nil, err
	}
	if len(req.Roles) == 0 {
		keep = p.rolesSet
	}

	nodes, err := p.scan(ctx, cl, req.Scan, native, req.MaxDepth)
	if err != nil {
		return nil, err
	}

	els := accessibility.FromNodes(nodes)
	accessibility.ReleaseNodes(nodes)

	raw := make([]perceiver.Element, 0, len(els))
	for _, e := range els {
		if e == nil {
			continue
		}
		b := e.Bounds()
		raw = append(raw, perceiver.Element{
			Role:        string(e.Role()),
			Title:       e.Title(),
			Description: e.Description(),
			Value:       e.Value(),
			Identifier:  e.Identifier(),
			Source:      "ax",
			Focused:     e.IsFocused(),
			Clickable:   e.IsClickable(),
			X:           b.Min.X,
			Y:           b.Min.Y,
			Width:       b.Dx(),
			Height:      b.Dy(),
		})
	}

	out := perceiver.Clean(raw, perceiver.CleanOptions{
		MinArea:   p.minArea,
		KeepRoles: keep,
		Max:       p.max,
	})
	// neru-core 不提供稳定标签，这里按屏幕左上→右下顺序编号，供 agent 引用。
	for i := range out {
		out[i].Label = strconv.Itoa(i + 1)
	}
	return out, nil
}

// scan 按扫描范围取回无障碍节点，是各 scope 的统一落点。
func (p *Perceiver) scan(
	ctx context.Context,
	cl ax.Client,
	scan action.Scan,
	nativeRoles []string,
	maxDepth int,
) ([]ax.Node, error) {
	switch scan.Scope {
	case "", "window":
		window, err := cl.FrontmostWindow(ctx)
		if err != nil {
			return nil, fmt.Errorf("获取前台窗口失败: %w", err)
		}
		defer window.Release()
		return cl.ClickableNodes(ctx, window, nativeRoles, maxDepth, true)

	case "application":
		app, err := cl.FocusedApplication(ctx)
		if err != nil {
			return nil, fmt.Errorf("获取前台应用失败: %w", err)
		}
		defer app.Release()
		return cl.ClickableNodes(ctx, app, nativeRoles, maxDepth, true)

	case "app":
		app, err := cl.ApplicationByBundleID(ctx, scan.Application)
		if err != nil {
			return nil, fmt.Errorf("定位应用 %q 失败: %w", scan.Application, err)
		}
		defer app.Release()
		return cl.ClickableNodes(ctx, app, nativeRoles, maxDepth, true)

	case "surface":
		if !cl.SupportsSupplementaryElements() {
			return nil, fmt.Errorf("当前平台(%s)不支持 scope=surface 的辅助表面 %q", runtime.GOOS, scan.Surface)
		}
		nodes, err := accessibility.SurfaceNodes(ctx, cl, ax.Surface(scan.Surface), nativeRoles, maxDepth)
		if err != nil {
			return nil, fmt.Errorf("扫描辅助表面 %q 失败: %w", scan.Surface, err)
		}
		return nodes, nil

	default:
		return nil, fmt.Errorf("未知扫描范围 %q", scan.Scope)
	}
}
