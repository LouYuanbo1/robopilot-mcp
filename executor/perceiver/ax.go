package perceiver

// 本文件收敛"无障碍角色(role)"的公共约定：把调用方写的 role 列表翻译成本平台的原生
// role，并推导出结果过滤集合。role 属于感知契约的一部分（工具入参就允许写中立 role
// 或带平台前缀的原生 role），因此解析规则放在契约层，由各平台实现复用。
//
// 角色有两种写法，均由 neru-core 的 element.ResolveRoleEntries 统一解析：
//   - 中立 role：button / link / text_field / menu_item ...，跨平台同一套词汇；
//   - 原生 role：uia:Button(Windows)、ax:AXButton(macOS)、atspi:push button(Linux)，
//     用于中立词汇表达不了控件类型的场景。
//
// 拼错的中立 role 会明确报错（附建议），属于其它平台的条目会被忽略而不是静默失配。
//
// 具体感知能力（无障碍树扫描、OCR、视觉检测）由子包实现：
// executor/perceiver/neru 基于 neru-core；本包只定义契约与中立模型，便于替换后端。

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/LouYuanbo1/neru-core/accessibility"
	"github.com/LouYuanbo1/neru-core/element"
)

// ResolveRoles 把 role 条目解析成本平台的原生 role，并推导中立 role 过滤集合。
//
// 返回的 native 直接交给无障碍后端做扫描过滤；keep 用于对扫描结果做二次过滤
// （nil 表示不过滤）。当某个条目无法映射回中立 role 时，keep 返回 nil，
// 完全信任后端的原生过滤，避免误删合法结果。
//
// entries 为空时返回 nil, nil, nil，表示使用感知器自身的 role 配置。
func ResolveRoles(entries []string) (native []string, keep map[string]bool, err error) {
	if len(entries) == 0 {
		return nil, nil, nil
	}

	resolution := element.ResolveRoleEntries(entries, runtime.GOOS)
	if resolution.HasFatal() {
		return nil, nil, fmt.Errorf("role 无法解析: %s", strings.Join(resolution.FatalMessages(), "; "))
	}
	if len(resolution.Native) == 0 {
		detail := strings.Join(resolution.IgnoredMessages(), "; ")
		if detail == "" {
			detail = "这些 role 在当前平台没有对应控件类型"
		}
		return nil, nil, fmt.Errorf("当前平台(%s)上指定的 role 均不可用: %s", runtime.GOOS, detail)
	}

	mapper, ok := accessibility.Mapper()
	if !ok {
		return resolution.Native, nil, nil
	}

	keep = make(map[string]bool, len(entries))
	for _, entry := range resolution.Entries {
		if entry.Role != "" {
			keep[string(entry.Role)] = true
			continue
		}
		for _, nativeRole := range entry.Native {
			mapped, mappedOK := element.MapNativeRole(mapper, nativeRole, "")
			if !mappedOK {
				return resolution.Native, nil, nil
			}
			keep[string(mapped)] = true
		}
	}
	return resolution.Native, keep, nil
}
