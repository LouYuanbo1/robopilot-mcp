package action

import (
	"encoding/json"
	"errors"
	"io"
)

// 本文件为 action 包预设格式前端：把声明式动作文档解析为 []Action。
// 这些前端实现 Spec 接口（见 parse.go），复用 ParseActions 的整套解析/校验规则。
//
// 当前提供 JSON 前端（纯标准库，零额外依赖）。要接入 YAML 等其它格式，只需仿照
// jsonSpec 再实现一个满足 Spec 的前端并复用 ParseActions 即可，核心无需改动。

// jsonSpec 以单条 JSON 对象承载动作描述。
type jsonSpec struct {
	raw json.RawMessage
}

func (s jsonSpec) Kind() (string, error) {
	var header struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(s.raw, &header); err != nil {
		return "", err
	}
	return header.Type, nil // 缺 type 时为空串，交由 ParseActions 统一报错
}

func (s jsonSpec) Decode(target any) error { return json.Unmarshal(s.raw, target) }

func (s jsonSpec) Line() int { return 0 }

// errNoActions 表示文档中缺少 actions 序列。
var errNoActions = errors.New("未找到 actions 序列")

// ParseJSON 从流中读取 JSON 动作文档并解析为 []Action。
// 顶层结构形如：
//
//	{"actions": [
//	  {"type": "mouse_click", "button": "left", "x": 100, "y": 200},
//	  {"type": "type_text", "text": "hello"}
//	]}
func ParseJSON(r io.Reader) ([]Action, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var root struct {
		Actions []json.RawMessage `json:"actions"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if len(root.Actions) == 0 {
		return nil, errNoActions
	}
	specs := make([]Spec, 0, len(root.Actions))
	for _, raw := range root.Actions {
		specs = append(specs, jsonSpec{raw: raw})
	}
	return ParseActions(specs)
}
