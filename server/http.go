package server

import (
	"context"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/LouYuanbo1/robopilot-mcp/executor"
	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver"
)

// ---- HTTP 服务层 ----
// 把 MCP 服务端点暴露为外层 HTTP 服务器（监听端口，路由到 /mcp），并提供健康检查。
// MCP 端点使用官方 go-sdk 的 Streamable HTTP 处理器，SDK 负责 Streamable HTTP 协议的
// 完整编解码与状态管理；这里只负责路由与优雅启停。

const (
	// defaultAddr 默认监听地址。
	defaultAddr = ":8080"
	// mcpPath MCP 端点路径。
	mcpPath = "/mcp"
	// healthPath 健康检查路径。
	healthPath = "/healthz"
)

// Config HTTP 服务器的配置项；各字段均有"零值为默认"的语义。
type Config struct {
	Addr           string              // 监听地址，如 ":8080"；空则用 defaultAddr
	Implementation *mcp.Implementation // MCP 服务器信息；空则使用默认
	ServerOptions  *mcp.ServerOptions  // MCP 服务器选项；可空
	Humanize       bool                // 是否拟人化移动/输入(平滑轨迹)；默认 false
	MinDelayMS     int                 // 拟人随机延迟下限(ms)
	MaxDelayMS     int                 // 拟人随机延迟上限(ms)
	// Perceiver 屏幕感知与窗口管理实现(如 executor/perceiver/neru.New())。设置后即可使用
	// 元素感知、OCR、视觉检测、桌面信息与窗口管理（枚举/激活/摆放）动作；为空时这些动作
	// 会明确返回"未配置感知器"。
	Perceiver perceiver.Perceiver
}

// NewHTTPHandler 构造路由：/mcp → MCP(Streamable HTTP)端点；/healthz → 健康检查。
func NewHTTPHandler(cfg Config) http.Handler {
	opts := []executor.Option{executor.WithHumanize(cfg.Humanize)}
	if cfg.MaxDelayMS > 0 {
		opts = append(opts, executor.WithDelayMS(cfg.MinDelayMS, cfg.MaxDelayMS))
	}
	if cfg.Perceiver != nil {
		opts = append(opts, executor.WithPerceiver(cfg.Perceiver))
	}
	ex := executor.New(opts...)

	server := NewMCPServer(ex, cfg.Implementation, cfg.ServerOptions)

	mux := http.NewServeMux()
	mux.Handle("GET "+healthPath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	mux.Handle(mcpPath, mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{Stateless: true}))
	return mux
}

// New 依据配置构造一个可直接 ListenAndServe 的 *http.Server。
func New(cfg Config) *http.Server {
	if cfg.Addr == "" {
		cfg.Addr = defaultAddr
	}
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           NewHTTPHandler(cfg),
		ReadHeaderTimeout: 10 * time.Second,
	}
}

// Run 启动 HTTP 服务并阻塞至被 ctx 取消（收到信号后优雅关闭），适合作为命令行入口调用。
func Run(ctx context.Context, cfg Config) error {
	srv := New(cfg)

	// 预热感知后端：Linux 上提前打开 Wayland 输入会话，其它平台让进程尽早进入
	// per-monitor DPI aware 状态。失败只记日志，不影响服务启动。
	if warmer, ok := cfg.Perceiver.(interface{ WarmUp(context.Context) error }); ok {
		warmCtx, warmCancel := context.WithTimeout(ctx, 10*time.Second)
		if err := warmer.WarmUp(warmCtx); err != nil {
			log.Printf("[server] 感知后端预热失败: %v", err)
		}
		warmCancel()
	}

	// 退出时释放感知器持有的原生资源（无障碍客户端、OCR 引擎、WinRT 线程等）。
	defer func() {
		if closer, ok := cfg.Perceiver.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				log.Printf("[server] 释放感知器失败: %v", err)
			}
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("[server] 监听 %s (MCP 端点: %s)", srv.Addr, mcpPath)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
