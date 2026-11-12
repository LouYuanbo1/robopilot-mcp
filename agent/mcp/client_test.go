package mcp

import (
	"strings"
	"testing"
)

// TestExtractSSEDataLargePayload 锁住"大结果不再被截断"的行为：
// 截图（base64 图片）或几百个元素的 JSON 会让单行 data 远超 bufio.Scanner 的 64KB 上限，
// 旧实现会报 token too long，新的按行读取必须完整返回。
func TestExtractSSEDataLargePayload(t *testing.T) {
	big := strings.Repeat("A", 512*1024) // 512KB，超过 Scanner 默认上限 8 倍
	frame := "event: message\n" +
		"data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"" + big + "\"}]}}\n\n"

	payload, err := extractSSEData([]byte(frame))
	if err != nil {
		t.Fatalf("解析大载荷失败: %v", err)
	}
	if len(payload) < len(big) {
		t.Fatalf("载荷被截断: 期望至少 %d 字节，实际 %d", len(big), len(payload))
	}
	if !strings.Contains(string(payload), `"jsonrpc"`) {
		t.Error("载荷内容不完整")
	}
}

// TestExtractSSEDataMultiLine 验证多行 data 会被拼接。
func TestExtractSSEDataMultiLine(t *testing.T) {
	frame := "event: message\r\ndata: {\"a\":1}\r\ndata: {\"b\":2}\r\n\r\n"
	payload, err := extractSSEData([]byte(frame))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if got := string(payload); got != `{"a":1}{"b":2}` {
		t.Errorf("拼接结果不符: %q", got)
	}
}

// TestExtractSSEDataMissing 验证没有 data 行时报错。
func TestExtractSSEDataMissing(t *testing.T) {
	if _, err := extractSSEData([]byte("event: ping\n\n")); err == nil {
		t.Error("缺少 data 行时应报错")
	}
}
