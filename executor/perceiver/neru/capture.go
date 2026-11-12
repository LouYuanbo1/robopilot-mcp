package neru

import (
	"context"
	"fmt"
	"image"
	"runtime"

	"github.com/LouYuanbo1/neru-core/vision/capture"
	"github.com/LouYuanbo1/neru-core/vision/pixels"

	"github.com/LouYuanbo1/robopilot-mcp/action"
)

// captureSupported 报告当前平台是否有屏幕捕获后端（与 neru-core 的支持矩阵一致）。
func captureSupported() bool {
	switch runtime.GOOS {
	case "windows", "darwin", "linux":
		return true
	default:
		return false
	}
}

// regionOf 把 image.Rectangle 转成协议层的 Region（左上角 + 宽高）。
func regionOf(r image.Rectangle) action.Region {
	return action.Region{X: r.Min.X, Y: r.Min.Y, Width: r.Dx(), Height: r.Dy()}
}

// captureFrame 捕获整屏或指定区域，返回帧与实际覆盖的屏幕区域。
func captureFrame(ctx context.Context, region *action.Region) (pixels.Frame, action.Region, error) {
	if region == nil {
		frame, err := capture.CaptureScreen(ctx)
		if err != nil {
			return pixels.Frame{}, action.Region{}, fmt.Errorf("屏幕捕获失败: %w", err)
		}
		return frame, regionOf(frame.Region), nil
	}

	rect := image.Rect(region.X, region.Y, region.X+region.Width, region.Y+region.Height)
	frame, err := capture.CaptureRect(ctx, rect)
	if err != nil {
		return pixels.Frame{}, action.Region{}, fmt.Errorf("捕获屏幕区域 %+v 失败: %w", *region, err)
	}
	return frame, regionOf(frame.Region), nil
}
