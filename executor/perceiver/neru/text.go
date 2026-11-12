package neru

import (
	"context"
	"fmt"
	"strings"

	"github.com/LouYuanbo1/neru-core/vision"
	"github.com/LouYuanbo1/neru-core/vision/ocr/engine"

	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver"
)

// ReadText 用 OCR 读取屏幕或指定区域上的文字。
//
// 返回的坐标都落在屏幕绝对坐标系里（neru-core 负责把图像像素空间换算回桌面），
// 因此识别出的词块可以直接拿去做坐标点击。
func (p *Perceiver) ReadText(ctx context.Context, req perceiver.TextRequest) (perceiver.TextResult, error) {
	var result perceiver.TextResult

	recognizer, err := p.ocrOnce()
	if err != nil {
		return result, fmt.Errorf("OCR 引擎不可用: %w", err)
	}

	frame, region, err := captureFrame(ctx, req.Region)
	if err != nil {
		return result, err
	}
	result.Region = region

	langs := req.Languages
	if len(langs) == 0 {
		langs = p.ocrLangs
	}

	words, err := recognizer.Recognize(ctx, frame.Img, engine.Options{
		Languages:     langs,
		WordLevel:     req.WordLevel,
		MinConfidence: req.MinConfidence,
	})
	if err != nil {
		return result, fmt.Errorf("OCR 识别失败: %w", err)
	}

	regions := vision.RegionsFromWords(words, frame.Region, frame.Img.Rect, req.MinConfidence)
	result.Words = make([]perceiver.Word, 0, len(regions))
	parts := make([]string, 0, len(regions))
	for _, region := range regions {
		parts = append(parts, region.Label)
		result.Words = append(result.Words, perceiver.Word{
			Text:       region.Label,
			X:          region.Bounds.Min.X,
			Y:          region.Bounds.Min.Y,
			Width:      region.Bounds.Dx(),
			Height:     region.Bounds.Dy(),
			Confidence: region.Score,
		})
	}
	result.Text = strings.Join(parts, " ")
	return result, nil
}
