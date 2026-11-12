package executor

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/go-vgo/robotgo"

	"github.com/LouYuanbo1/robopilot-mcp/action"
)

// defaultArtifactDirName 是未指定产物目录时，用户主目录下的默认子目录名。
const defaultArtifactDirName = "robopilot-artifacts"

// artifactPath 计算产物落盘路径：目录为空时先用执行层默认目录，仍为空则回落到
// 用户主目录下的默认产物目录；文件名为空时按时间戳自动生成（并统一补 .png 后缀）。
//
// 回落到用户主目录而不是当前工作目录，避免把截图散落在项目目录里。
func (e *Executor) artifactPath(dir, name string) string {
	if dir == "" {
		dir = e.artifactDir
	}
	if dir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, defaultArtifactDirName)
		} else {
			dir = os.TempDir()
		}
	}
	if name == "" {
		name = fmt.Sprintf("screenshot_%d", time.Now().UnixNano())
	}
	return filepath.Join(dir, name+".png")
}

// captureRGBA 抓取屏幕区域并转成标准 RGBA；这是落盘与内存直出两条路径的共同起点。
func captureRGBA(x, y, w, h int) (*image.RGBA, error) {
	bit := robotgo.CaptureScreen(x, y, w, h)
	defer robotgo.FreeBitmap(bit)
	img := robotgo.ToRGBA(bit)
	if img == nil || img.Bounds().Dx() == 0 {
		return nil, fmt.Errorf("截屏失败: 请检查屏幕录制/权限")
	}
	return img, nil
}

// saveShot 用 robotgo 官方转换链把截图帧转成标准 RGBA 并落盘为 PNG。
func saveShot(x, y, w, h int, path string) error {
	img, err := captureRGBA(x, y, w, h)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// capturePNG 把截图帧编码成内存中的 PNG，全程不碰磁盘。
func capturePNG(x, y, w, h int) ([]byte, error) {
	img, err := captureRGBA(x, y, w, h)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	// PNG 压缩后通常远小于原始像素，按上限的 1/8 预估，减少扩容拷贝。
	buf.Grow(w * h / 8)
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("图片编码失败: %w", err)
	}
	return buf.Bytes(), nil
}

// 屏幕域动作的执行落点：只保留截图。
// 按 Output 分三种产出：只落盘(默认)、只回图片、两者都要。

// screen 执行截图动作并组装 ActionResult。
func (e *Executor) screen(a action.Action) (action.ActionResult, error) {
	res := action.ActionResult{Action: nameOf(a)}

	v, ok := a.(*action.Screenshot)
	if !ok {
		return res, fmt.Errorf("非截图动作: %T", a)
	}

	// 未限定区域则截全屏。
	sw, sh := robotgo.GetScreenSize()
	x, y, w, h := 0, 0, sw, sh
	if v.Region != nil {
		x, y, w, h = v.Region.X, v.Region.Y, v.Region.Width, v.Region.Height
	}

	switch v.Output {
	case action.ScreenshotOutputInline:
		// 不落盘：直接在内存里编码，字节随结果回到 server，由它包成图片内容。
		data, err := capturePNG(x, y, w, h)
		if err != nil {
			return res, err
		}
		res.ImagePNG = data
		res.Value = "图片已随结果直接返回（未落盘）"
		res.Effective = true
		return res, nil

	case action.ScreenshotOutputBoth, action.ScreenshotOutputFile, "":
		path := e.artifactPath(v.Artifact.Dir, v.Artifact.Name)
		if err := saveShot(x, y, w, h, path); err != nil {
			return res, err
		}
		res.ArtifactPath, res.Effective = path, true
		if v.Output == action.ScreenshotOutputBoth {
			// 直接回读刚写出的文件，避免同一帧编码两次。
			data, err := os.ReadFile(path)
			if err != nil {
				return res, err
			}
			res.ImagePNG = data
		}
		return res, nil

	default:
		return res, fmt.Errorf("非法 output=%q，可选 file|inline|both", v.Output)
	}
}
