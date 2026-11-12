// Package neru 用 neru-core 在进程内实现 perceiver 包定义的全部感知与窗口管理能力。
//
// neru-core(https://github.com/LouYuanbo1/neru-core) 是跨平台桌面自动化核心：
// 无障碍树(Windows UIA / macOS AX / Linux AT-SPI)、屏幕捕获与 OCR、轮廓与分割检测、
// 桌面窗口枚举/激活/摆放都在进程内直连系统接口，不需要额外的守护进程。
//
// 本包把它的几项能力各翻译成 perceiver 的一个能力接口，按职责分文件：
//
//	neru.go      构造、配置与生命周期（客户端、窗口管理器与 OCR 引擎的惰性创建、预热、释放）
//	perceive.go  无障碍树扫描（Perceive / PerceiveScope 与各扫描范围的落点）
//	info.go      桌面几何、前台窗口与能力可用性
//	window.go    窗口枚举、激活与摆放（ListWindows / FrontmostWindow / ActivateWindow / SetWindowBounds）
//	text.go      OCR 读字
//	vision.go    纯像素视觉检测
//	pick.go      空间导航（nearest / toward）
//	capture.go   屏幕捕获（整屏与区域）
//
// 系统权限：Windows 无需额外授权；macOS 需在"辅助功能"与"屏幕录制"中勾选本程序。
package neru

import (
	"context"
	"fmt"
	"sync"

	"github.com/LouYuanbo1/neru-core/accessibility"
	"github.com/LouYuanbo1/neru-core/accessibility/ax"
	"github.com/LouYuanbo1/neru-core/element"
	"github.com/LouYuanbo1/neru-core/vision/ocr"
	"github.com/LouYuanbo1/neru-core/vision/ocr/engine"
	"github.com/LouYuanbo1/neru-core/window"

	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver"
)

// 编译期断言：Perceiver 满足 perceiver 包定义的每一项能力。
var (
	_ perceiver.Perceiver      = (*Perceiver)(nil)
	_ perceiver.Scoper         = (*Perceiver)(nil)
	_ perceiver.InfoProvider   = (*Perceiver)(nil)
	_ perceiver.TextReader     = (*Perceiver)(nil)
	_ perceiver.TargetDetector = (*Perceiver)(nil)
	_ perceiver.SpatialPicker  = (*Perceiver)(nil)
	_ perceiver.WindowManager  = (*Perceiver)(nil)
)

// Perceiver 通过 neru-core 感知桌面、管理窗口，并实现 perceiver 包的能力接口。
//
// 无障碍客户端、OCR 引擎与窗口管理器都在首次使用时惰性创建、之后复用，因此
// Perceiver 可安全地被多个 goroutine 并发共享（底层客户端本身允许并发调用）。
type Perceiver struct {
	mu        sync.Mutex // 保护 client / clientErr 的惰性初始化
	client    ax.Client  // neru-core 无障碍客户端；nil 表示尚未创建
	clientErr error      // 客户端创建失败的错误；非 nil 后不再重试

	ocrMu    sync.Mutex
	ocr      engine.Recognizer
	ocrErr   error
	ocrLangs []string

	windowMu  sync.Mutex     // 保护 windows / windowErr 的惰性初始化
	windows   window.Manager // neru-core 的窗口管理器；nil 表示尚未创建
	windowErr error          // 窗口管理器创建失败的错误；非 nil 后不再重试

	// roles 是构造时配置的中立 role（如 button / text_field），
	// 同时作为读取阶段的角色过滤与归一化阶段的 keepRoles；为空表示扫描平台默认可点击集。
	roles    []element.Role
	rolesSet map[string]bool
	minArea  int
	max      int
	maxDepth int
}

// Option 用于配置 Perceiver。
type Option func(*Perceiver)

// WithMinArea 过滤掉外接矩形面积小于 min 平方像素的元素（太小无法可靠点击）。
func WithMinArea(min int) Option {
	return func(p *Perceiver) { p.minArea = min }
}

// WithRoles 只保留给定的中立 role（如 "button"、"text_field"）；这些 role 会同步
// 交给 neru-core 解析成本平台的原生角色用于扫描，并在归一化阶段再次过滤。
// 空集表示扫描平台默认可点击角色集。
func WithRoles(roles ...string) Option {
	return func(p *Perceiver) {
		p.roles = p.roles[:0]
		if len(roles) == 0 {
			p.rolesSet = nil
			return
		}
		set := make(map[string]bool, len(roles))
		for _, r := range roles {
			set[r] = true
			p.roles = append(p.roles, element.Role(r))
		}
		p.rolesSet = set
	}
}

// WithMaxElements 限制单次最多返回的元素个数（默认 64），避免一次性刷屏模型。
// 截断前会先排序（左上→右下），因此保留的是屏幕上前 n 个。
func WithMaxElements(n int) Option {
	return func(p *Perceiver) { p.max = n }
}

// WithMaxDepth 限制无障碍树的遍历深度（0 表示后端默认）。窗口层级很浅时调小可显著加快扫描。
func WithMaxDepth(depth int) Option {
	return func(p *Perceiver) { p.maxDepth = depth }
}

// WithOCRLanguages 指定 OCR 的默认语言顺序（如 []string{"zh-Hans", "en-US"}）。
// 单次调用仍可用 TextRequest.Languages 覆盖。
func WithOCRLanguages(langs ...string) Option {
	return func(p *Perceiver) { p.ocrLangs = append([]string(nil), langs...) }
}

// New 构造一个 neru-core 感知器；opts 为空时使用默认配置。
func New(opts ...Option) *Perceiver {
	p := &Perceiver{
		minArea: 0,
		max:     64,
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// clientOnce 惰性创建并复用底层 neru-core 无障碍客户端；创建失败会缓存错误并返回。
func (p *Perceiver) clientOnce() (ax.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client != nil || p.clientErr != nil {
		return p.client, p.clientErr
	}
	c, err := accessibility.New(accessibility.Options{Roles: p.roles, MaxDepth: p.maxDepth})
	if err != nil {
		p.clientErr = err
		return nil, err
	}
	p.client = c
	return p.client, nil
}

// ocrOnce 惰性创建并复用 OCR 引擎；创建失败会缓存错误并返回。
func (p *Perceiver) ocrOnce() (engine.Recognizer, error) {
	p.ocrMu.Lock()
	defer p.ocrMu.Unlock()
	if p.ocr != nil || p.ocrErr != nil {
		return p.ocr, p.ocrErr
	}
	r, err := ocr.New()
	if err != nil {
		p.ocrErr = err
		return nil, err
	}
	p.ocr = r
	return p.ocr, nil
}

// WarmUp 预热底层后端：Linux 上会提前打开 Wayland 输入会话（可能弹一次授权框）
// 与 uinput 滚轮，避免第一次操作时才付出这个代价；其它平台是 no-op。
//
// 同时会提前创建无障碍客户端，把进程设为 per-monitor DPI aware，
// 使后续读取的屏幕几何与注入坐标从一开始就处在同一套物理像素坐标系里。
func (p *Perceiver) WarmUp(ctx context.Context) error {
	client, err := p.clientOnce()
	if err != nil {
		return fmt.Errorf("初始化无障碍感知失败: %w", err)
	}
	return accessibility.WarmUp(ctx, client)
}

// Close 释放底层无障碍客户端与 OCR 引擎；未创建时对应项为 no-op。
func (p *Perceiver) Close() error {
	var firstErr error

	p.mu.Lock()
	if p.client != nil {
		if err := p.client.Close(); err != nil {
			firstErr = err
		}
		p.client = nil
	}
	p.mu.Unlock()

	p.ocrMu.Lock()
	if closer, ok := p.ocr.(interface{ Close() error }); ok && p.ocr != nil {
		if err := closer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	p.ocr = nil
	p.ocrMu.Unlock()

	p.windowMu.Lock()
	if p.windows != nil {
		if err := p.windows.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		p.windows = nil
	}
	p.windowMu.Unlock()

	return firstErr
}
