package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ---- MCP HTTP 客户端 ----
// agent 的工具不直接操纵桌面，而是把每次调用作为 JSON-RPC 的 tools/call 请求发给
// 远端 MCP 服务（cmd/server 已把全部桌面操控工具通过 Streamable HTTP 暴露出来）。这样 agent
// 与桌面操控彻底解耦，工具的入参 schema 也统一由 MCP 服务端维护。

const (
	// defaultMCPBaseURL 内置的默认 MCP 端点；可用 NewMCPClient 传入其它值覆盖。
	defaultMCPBaseURL = "http://127.0.0.1:8080/mcp"
	// defaultCallTimeout 单次工具调用的总超时上限。
	defaultCallTimeout = time.Minute
)

// jsonRPCRequest / jsonRPCResponse 是 MCP 传输所需的最小 JSON-RPC 2.0 结构。
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCRawResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      int              `json:"id"`
	Result  *toolsCallResult `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// toolsCallResult 对应 MCP tools/call 的标准 result 结构。
type toolsCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError,omitempty"`
}

// Client 封装到远端 MCP 服务的 Streamable HTTP 调用。
// 本项目的 MCP 服务是"一人份"（单实例），会话之间不共享任何浏览器/状态句柄：
// 每次工具调用直接作用于当前桌面，无需 web_id、借网/归还等会话级资源管理。
type Client struct {
	baseURL string
	http    *http.Client

	idMu sync.Mutex
	id   int
}

// NewMCPClient 创建一个指向 baseURL（JSON-RPC 端点，如 "http://127.0.0.1:8080/mcp"）的客户端。
// baseURL 为空时回退到默认端点 defaultMCPBaseURL。
func NewMCPClient(baseURL string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultMCPBaseURL
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: defaultCallTimeout},
	}
}

// CallTool 以 JSON 参数调用远端名为 toolName 的 MCP 工具，返回其文本结果
// （即 ActionResult 的 JSON 串）。本地工具封装（utils.InferTool）拿到该串后原样回传给模型。
func (c *Client) CallTool(ctx context.Context, toolName string, arguments map[string]any) (string, error) {
	return c.callToolRaw(ctx, toolName, arguments)
}

func truncate(s string) string {
	const n = 400
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(截断)"
}

// extractSSEData 从 text/event-stream 帧里提取并拼接所有 data: 载荷。
// SSE 用行分隔：event/message 的 JSON-RPC 载荷放在一行或多行连续的 data: 里，
// 空行表示一个事件结束。这里拼出全部 data 行（各事件通常就一条 JSON）返回。
//
// 用 bufio.Reader 按行读取而不是 bufio.Scanner：Scanner 默认有 64KB 的 token 上限，
// 截图等工具的结果（base64 图片或几百个元素的 JSON）会直接超出，报 "token too long"。
func extractSSEData(b []byte) ([]byte, error) {
	var out bytes.Buffer
	reader := bufio.NewReader(bytes.NewReader(b))

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(line, "data:") {
				payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				out.WriteString(payload)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}

	if out.Len() == 0 {
		return nil, fmt.Errorf("SSE 帧中未找到 data: 载荷")
	}
	return out.Bytes(), nil
}

func (c *Client) callToolRaw(ctx context.Context, toolName string, arguments map[string]any) (string, error) {
	params, err := json.Marshal(map[string]any{
		"name":      toolName,
		"arguments": arguments,
	})
	if err != nil {
		return "", err
	}

	c.idMu.Lock()
	c.id++
	reqID := c.id
	c.idMu.Unlock()

	reqBody, err := json.Marshal(jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  "tools/call",
		Params:  params,
	})
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Streamable 传输要求 Accept 同时声明 application/json 与 text/event-stream。
	httpReq.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("调用 MCP 工具 %s 失败: %w", toolName, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("MCP 服务返回 HTTP %d: %s", resp.StatusCode, truncate(string(raw)))
	}

	// 服务端可能用 Streamable HTTP 的 SSE 帧(text/event-stream)分发 JSON-RPC 消息：
	//   event: message
	//   data: { "jsonrpc":"2.0", ... }
	// 因此按 Content-Type 决定是直接按 JSON 解析，还是先取出其中的 data 载荷再解析。
	body := raw
	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "text/event-stream") {
		payload, err := extractSSEData(raw)
		if err != nil {
			return "", fmt.Errorf("解析 MCP 响应(SSE)失败: %v\n%s", err, truncate(string(raw)))
		}
		body = payload
	}

	var rpc jsonRPCRawResponse
	if err := json.Unmarshal(body, &rpc); err != nil {
		return "", fmt.Errorf("解析 MCP 响应失败: %v\n%s", err, truncate(string(raw)))
	}
	if rpc.Error != nil {
		return "", fmt.Errorf("MCP 工具 %s 调用错误(%d): %s", toolName, rpc.Error.Code, rpc.Error.Message)
	}
	if rpc.Result == nil || len(rpc.Result.Content) == 0 {
		return "", fmt.Errorf("MCP 工具 %s 未返回结果内容", toolName)
	}
	return rpc.Result.Content[0].Text, nil
}
