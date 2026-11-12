package action

import (
	"fmt"
	"strings"
)

// 声明式动作的"格式无关"解析核心。
//
// 本文件把「格式无关的动作描述 -> []Action」的编译逻辑抽离出来，与任何具体格式
// （JSON/YAML/文本/...）解耦：不同前端各自实现 Spec，往同一个 ParseActions 投喂
// 动作即可复用整套解析/校验规则。故：
//   - 不依赖 runtime（不发散到 robotgo）；
//   - 不依赖具体格式库（DTO 仅携带 json/yaml 标签，核心本身不 import 任何格式库）；
//   - 将来要支持新格式，只需新增一个满足 Spec 的前端并复用 ParseActions。
//
// 旧的动作名（perceive_elements / describe_screen / detect_targets / mouse_down /
// mouse_up / get_clipboard / set_clipboard）作为别名保留，解码到合并后的新动作上，
// 这样历史脚本与配置文件不用改。

// Spec 一条声明式动作的抽象、与格式无关的描述。
// 不同前端格式各自实现该接口，往同一解析核心（ParseActions）里投喂动作。
type Spec interface {
	// Kind 抽出动作的 type 字段；缺失或不可解析时报错。
	Kind() (string, error)
	// Decode 把整条动作解到 target；target 为具体的 *Action 结构体（本包内定义）。
	Decode(target any) error
	// Line 返回该动作在源文档中的行号，用于报错定位。
	Line() int
}

// ParseActions 把一组格式无关的动作描述编译为 []Action。
func ParseActions(specs []Spec) ([]Action, error) {
	acts := make([]Action, 0, len(specs))
	for i, spec := range specs {
		act, err := parseAction(spec, i)
		if err != nil {
			return nil, err
		}
		acts = append(acts, act)
	}
	return acts, nil
}

// locate 生成带下标与行号的定位前缀，便于在长任务里精准指出出错的动作。
func locate(s Spec, i int, kind string) string {
	return fmt.Sprintf("actions[%d](type=%q, line %d)", i, kind, s.Line())
}

// parseAction 先抽取 type 字段，再按类型把 Spec 解到具体动作做必填校验。
func parseAction(s Spec, i int) (Action, error) {
	kind, err := s.Kind()
	if err != nil {
		return nil, fmt.Errorf("%s: 无法识别动作头: %w", locate(s, i, ""), err)
	}
	at := locate(s, i, kind)
	if kind == "" {
		return nil, fmt.Errorf("%s: 缺少必填的 type 字段", at)
	}

	switch kind {
	// ---- 鼠标 ----
	case "mouse_move":
		var a MouseMove
		return &a, s.Decode(&a)
	case "mouse_click":
		var a MouseClick
		if err := s.Decode(&a); err != nil {
			return nil, err
		}
		if a.Button == "" {
			a.Button = MouseButtonLeft
		}
		return &a, nil
	case "mouse_click_at":
		var a MouseClickAt
		if err := s.Decode(&a); err != nil {
			return nil, err
		}
		if a.Button == "" {
			a.Button = MouseButtonLeft
		}
		return &a, nil
	case "mouse_button", "mouse_down", "mouse_up":
		var a MouseButtonAction
		if err := s.Decode(&a); err != nil {
			return nil, err
		}
		if kind == "mouse_down" {
			a.State = "down"
		}
		if kind == "mouse_up" {
			a.State = "up"
		}
		switch a.State {
		case "down", "up":
		default:
			return nil, fmt.Errorf("%s: 非法 state=%q，可选 down|up", at, a.State)
		}
		if a.Button == "" {
			a.Button = MouseButtonLeft
		}
		return &a, nil
	case "mouse_drag":
		var a MouseDrag
		return &a, s.Decode(&a)
	case "mouse_scroll":
		var a MouseScroll
		if err := s.Decode(&a); err != nil {
			return nil, err
		}
		switch a.Direction {
		case ScrollUp, ScrollDown:
		default:
			return nil, fmt.Errorf("%s: 非法 direction=%q，可选 up|down", at, a.Direction)
		}
		if a.Amount <= 0 {
			return nil, fmt.Errorf("%s: amount 必须为正整数", at)
		}
		if (a.X == nil) != (a.Y == nil) {
			return nil, fmt.Errorf("%s: x 与 y 需同时提供或同时省略", at)
		}
		return &a, nil

	// ---- 键盘 ----
	case "type_text", "type":
		var a TypeText
		if err := s.Decode(&a); err != nil {
			return nil, err
		}
		if a.Text == "" {
			return nil, fmt.Errorf("%s: text 必填", at)
		}
		return &a, nil
	case "key_tap", "key":
		var a KeyTap
		if err := s.Decode(&a); err != nil {
			return nil, err
		}
		if a.Key == "" {
			return nil, fmt.Errorf("%s: key 必填", at)
		}
		return &a, nil

	// ---- 屏幕 ----
	case "screenshot", "shot":
		var a Screenshot
		if err := s.Decode(&a); err != nil {
			return nil, err
		}
		if a.Region != nil && a.Region.Empty() {
			return nil, fmt.Errorf("%s: region 宽高必须为正数", at)
		}
		switch a.Output {
		case "", ScreenshotOutputFile, ScreenshotOutputInline, ScreenshotOutputBoth:
		default:
			return nil, fmt.Errorf("%s: 非法 output=%q，可选 file|inline|both", at, a.Output)
		}
		return &a, nil

	// ---- 窗口 ----
	case "list_windows":
		return &ListWindows{}, nil
	case "set_active_window":
		var a SetActiveWindow
		if err := s.Decode(&a); err != nil {
			return nil, err
		}
		if a.ID == "" && a.PID == 0 && a.Application == "" && a.Title == "" {
			return nil, fmt.Errorf("%s: id、pid、application、title 至少提供其一", at)
		}
		if a.Rect != nil && a.Rect.Empty() {
			return nil, fmt.Errorf("%s: rect 的 width 与 height 必须为正数", at)
		}
		return &a, nil

	// ---- 剪贴板 ----
	case "clipboard", "get_clipboard", "set_clipboard":
		var a Clipboard
		if err := s.Decode(&a); err != nil {
			return nil, err
		}
		return &a, nil

	// ---- 感知 ----
	case "inspect_screen", "perceive_elements", "describe_screen", "detect_targets":
		var a InspectScreen
		if err := s.Decode(&a); err != nil {
			return nil, err
		}
		if err := validateScan(at, a.Scan); err != nil {
			return nil, err
		}
		if err := validateSource(at, a.Source); err != nil {
			return nil, err
		}
		switch a.Strategy {
		case "", "vision", "contour", "segment":
		default:
			return nil, fmt.Errorf("%s: 非法 strategy=%q，可选 vision|contour|segment", at, a.Strategy)
		}
		if a.Region != nil && a.Region.Empty() {
			return nil, fmt.Errorf("%s: region 宽高必须为正数", at)
		}
		return &a, nil
	case "read_text":
		var a ReadText
		if err := s.Decode(&a); err != nil {
			return nil, err
		}
		if a.Region != nil && a.Region.Empty() {
			return nil, fmt.Errorf("%s: region 宽高必须为正数", at)
		}
		if a.MinConfidence < 0 || a.MinConfidence > 1 {
			return nil, fmt.Errorf("%s: min_confidence 取值需在 0 到 1 之间", at)
		}
		return &a, nil

	// ---- 元素级操作 ----
	case "click_element":
		var a ClickElement
		if err := s.Decode(&a); err != nil {
			return nil, err
		}
		if err := validateScan(at, a.Scan); err != nil {
			return nil, err
		}
		if err := validateSource(at, a.Source); err != nil {
			return nil, err
		}
		if a.Text == "" && a.Direction == "" {
			return nil, fmt.Errorf("%s: text 与 direction 至少提供其一", at)
		}
		switch a.Direction {
		case "", "up", "down", "left", "right":
		default:
			return nil, fmt.Errorf("%s: 非法 direction=%q，可选 up|down|left|right", at, a.Direction)
		}
		if _, err := normalizeMatch(a.Match); err != nil {
			return nil, fmt.Errorf("%s: %w", at, err)
		}
		if a.Button == "" {
			a.Button = MouseButtonLeft
		}
		return &a, nil

	default:
		return nil, fmt.Errorf("%s: 未知动作类型，支持 inspect_screen/read_text/screenshot"+
			"/list_windows/set_active_window/click_element"+
			"/mouse_move/mouse_click/mouse_click_at/mouse_button/mouse_drag/mouse_scroll"+
			"/type_text/key_tap/clipboard", at)
	}
}

// validateScan 校验扫描范围参数：scope 取值合法，且所需的目标标识齐全。
func validateScan(at string, scan Scan) error {
	switch scan.Scope {
	case "", "window", "application":
		return nil
	case "app":
		if strings.TrimSpace(scan.Application) == "" {
			return fmt.Errorf("%s: scope=app 时必须提供 application", at)
		}
		return nil
	case "surface":
		if strings.TrimSpace(scan.Surface) == "" {
			return fmt.Errorf("%s: scope=surface 时必须提供 surface", at)
		}
		return nil
	default:
		return fmt.Errorf("%s: 非法 scope=%q，可选 window|application|app|surface", at, scan.Scope)
	}
}

// validateSource 校验感知来源取值。
func validateSource(at string, src ElementSource) error {
	switch src {
	case SourceAuto, SourceAX, SourceVision:
		return nil
	default:
		return fmt.Errorf("%s: 非法 source=%q，可选 ax|vision(留空表示先无障碍树再回退视觉)", at, src)
	}
}

// normalizeMatch 把空值归一为 contains，并拒绝未知匹配方式。
func normalizeMatch(m MatchMode) (MatchMode, error) {
	switch m {
	case "":
		return MatchContains, nil
	case MatchContains, MatchExact, MatchPrefix:
		return m, nil
	default:
		return "", fmt.Errorf("非法 match=%q，可选 contains|exact|prefix", m)
	}
}
