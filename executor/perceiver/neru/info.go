package neru

import (
	"context"
	"image"

	"github.com/LouYuanbo1/neru-core/vision/ocr/engine"

	"github.com/LouYuanbo1/robopilot-mcp/action"
	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver"
)

// Info 报告桌面几何、光标、前台窗口与三项感知能力的可用性。
func (p *Perceiver) Info(ctx context.Context) (perceiver.Info, error) {
	info := perceiver.Info{}

	// 前台窗口优先走窗口管理器：它给出跨平台一致的标题、PID 与应用标识，
	// 不依赖无障碍树是否可读。窗口管理不可用的平台再退回无障碍树上的窗口对象。
	if front, err := p.FrontmostWindow(ctx); err == nil {
		info.Window = perceiver.WindowInfo{
			Title:       front.Title,
			PID:         front.PID,
			Application: front.Application,
		}
	}

	if cl, err := p.clientOnce(); err != nil {
		info.Accessibility = perceiver.Capability{Detail: err.Error()}
	} else {
		info.Accessibility = perceiver.Capability{Available: cl.CheckPermissions()}
		if !info.Accessibility.Available {
			info.Accessibility.Detail = "系统未授予无障碍/辅助功能权限，请在系统设置的隐私与安全性中勾选本程序"
		}

		info.VirtualDesktop = regionOf(cl.ActiveScreenBounds())
		if pt := cl.CursorPosition(); pt != (image.Point{}) {
			info.Cursor = action.Point{X: pt.X, Y: pt.Y}
		}

		if info.Window.Title == "" && info.Window.PID == 0 {
			// 窗口管理器答不出时的兜底：标题来自平台窗口对象，应用标识来自
			// 前台应用（部分平台的窗口对象本身不带应用标识）。
			if window, winErr := cl.FrontmostWindow(ctx); winErr == nil {
				if titled, ok := window.(interface{ Title() string }); ok {
					info.Window.Title = titled.Title()
				}
				if owned, ok := window.(interface{ BundleIdentifier() string }); ok {
					info.Window.Application = owned.BundleIdentifier()
				}
				window.Release()
			}
			if app, appErr := cl.FocusedApplication(ctx); appErr == nil {
				if info.Window.Application == "" {
					info.Window.Application = app.BundleIdentifier()
				}
				app.Release()
			}
		}
	}

	info.Capture = perceiver.Capability{Available: captureSupported()}
	if !info.Capture.Available {
		info.Capture.Detail = "当前平台没有屏幕捕获后端"
	}

	recognizer, ocrErr := p.ocrOnce()
	if ocrErr != nil {
		info.OCR = perceiver.Capability{Detail: ocrErr.Error()}
	} else if healthErr := engine.Health(ctx, recognizer); healthErr != nil {
		info.OCR = perceiver.Capability{Detail: healthErr.Error()}
	} else {
		info.OCR = perceiver.Capability{Available: true}
	}

	return info, nil
}
