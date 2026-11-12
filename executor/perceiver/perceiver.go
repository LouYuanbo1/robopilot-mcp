// Package perceiver 定义"感知屏幕与窗口"的统一契约与中立数据模型。
//
// 无论底层用哪种 UI 感知或窗口管理库（neru-core 的无障碍树、OCR、图像视觉、窗口管理），
// 都可以把各自扫描到的元素翻译成本包的 Element（含屏幕坐标）、把窗口读数翻译成 Window，
// 并各自实现本包定义的能力接口。上层（executor / server / agent）只依赖本包，即可拿到
// "屏幕上哪里有可点的元素、点哪里、写了什么"，以及"有哪些窗口、怎么切过去、摆到哪里"。
//
// 能力按接口切分而不是塞进一个胖接口：一种实现可以选择只提供无障碍树扫描，
// 也可以同时提供 OCR、视觉检测与窗口管理，调用方用类型断言探测能力即可，缺哪一项就明确报错。
//
// 本包只放契约、读数与"解释读数"的纯函数，按职责分文件：
//
//	perceiver.go  能力接口与扫描请求（本文件）
//	element.go    元素模型与其上的匹配
//	request.go    各种请求/结果类型（信息、OCR、视觉检测）
//	window.go     窗口能力契约（枚举、前台窗口、激活、摆放）
//	filter.go     结果过滤、去重、排序、截断
//	select.go     从候选里挑一个（文本匹配 / 空间导航）
//	render.go     把结果序列化成给模型看的 JSON
//	ax.go         无障碍角色词汇解析（中立 role <-> 平台原生 role）
//
// 具体后端实现放在子包：executor/perceiver/neru 基于 neru-core。
package perceiver

import (
	"context"

	"github.com/LouYuanbo1/robopilot-mcp/action"
)

// Perceiver 是屏幕元素感知器的基础能力：扫描当前前台窗口的可交互元素。
type Perceiver interface {
	// Perceive 感知当前屏幕上所有可交互元素（含坐标）。
	Perceive(ctx context.Context) ([]Element, error)
}

// Scoper 是可选能力：按调用方指定的范围（前台窗口/整个应用/指定应用/平台辅助表面）扫描。
// 未实现时上层退化为 Perceiver 的前台窗口扫描。
type Scoper interface {
	PerceiveScope(ctx context.Context, req ScanRequest) ([]Element, error)
}

// ScanRequest 是一次元素扫描的完整参数。
type ScanRequest struct {
	// Scan 扫描范围（前台窗口/应用/指定应用/平台表面）。
	Scan action.Scan
	// Roles 角色过滤：中立 role 或带平台前缀的原生 role；空表示用感知器自身配置。
	Roles []string
	// MaxDepth 限制无障碍树遍历深度；0 表示用感知器默认值。
	MaxDepth int
}

// InfoProvider 是可选能力：报告桌面几何、前台窗口与三项感知能力(无障碍/捕获/OCR)的可用性。
type InfoProvider interface {
	Info(ctx context.Context) (Info, error)
}

// TextReader 是可选能力：用 OCR 读取屏幕或指定区域的文字。
type TextReader interface {
	ReadText(ctx context.Context, req TextRequest) (TextResult, error)
}

// TargetDetector 是可选能力：用像素视觉检测可点击目标（不依赖无障碍树）。
type TargetDetector interface {
	DetectTargets(ctx context.Context, req TargetRequest) ([]Element, error)
}

// SpatialPicker 是可选能力：按"从某点出发、朝某方向"的空间关系挑元素。
// 由底层感知库用自己成熟的几何算法实现（如 neru-core 的 Nearest / Toward），
// 未实现时上层退化为按中心点距离排序。
type SpatialPicker interface {
	Pick(ctx context.Context, elements []Element, from action.Point, direction string) (Element, bool)
}
