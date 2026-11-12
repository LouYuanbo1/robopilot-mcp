// server 启动一个本地 MCP(Streamable HTTP) 服务器，并预先注入基于 neru-core 的感知器：
// 元素感知、OCR 读字、视觉检测、屏幕信息与窗口管理（枚举/激活/摆放）都依赖它；
// 鼠标/键盘/截图/剪贴板等输入类工具则由 robotgo 提供，一并暴露。
//
// 用法：
//
//	go run ./cmd/server                                     # 默认监听 :8080
//	go run ./cmd/server -addr :9090 -perceive-max 32        # 自定义端口与感知元素上限
//	go run ./cmd/server -ocr-langs zh-Hans,en-US            # 指定 OCR 语言顺序
//
// 前置条件：不需要任何守护进程，neru-core 在进程内直连系统接口。
// Windows 无需额外授权；macOS 需在"辅助功能"与"屏幕录制"中勾选本程序。
// 启动后可用 agent 或任意 MCP 客户端连接 http://127.0.0.1:8080/mcp。
package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"strings"
	"syscall"

	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver/neru"
	"github.com/LouYuanbo1/robopilot-mcp/server"
)

func main() {
	var (
		addr  = flag.String("addr", ":8080", "HTTP 监听地址，如 :8080")
		max   = flag.Int("perceive-max", 64, "perceive_elements 单次最多返回的元素数量")
		minA  = flag.Int("perceive-min-area", 0, "按外接矩形面积(平方像素)过滤过小元素，0 不过滤")
		roles = flag.String("perceive-roles", "", "逗号分隔的只保留 role，如 button,text_field 或 uia:Button；空不过滤")
		depth = flag.Int("perceive-depth", 0, "无障碍树最大遍历深度，0 用后端默认值")
		langs = flag.String("ocr-langs", "", "逗号分隔的 OCR 语言顺序，如 zh-Hans,en-US；空用系统语言配置")
		hu    = flag.Bool("humanize", false, "拟人化移动/输入(平滑轨迹+随机延迟)")
		delay = flag.Int("delay-ms", 150, "拟人随机延迟(ms)，-humanize 时生效")
	)
	flag.Parse()

	opts := []neru.Option{neru.WithMaxElements(*max)}
	if *minA > 0 {
		opts = append(opts, neru.WithMinArea(*minA))
	}
	if *depth > 0 {
		opts = append(opts, neru.WithMaxDepth(*depth))
	}
	if list := splitList(*roles); len(list) > 0 {
		opts = append(opts, neru.WithRoles(list...))
	}
	if list := splitList(*langs); len(list) > 0 {
		opts = append(opts, neru.WithOCRLanguages(list...))
	}

	cfg := server.Config{
		Addr:       *addr,
		Humanize:   *hu,
		MinDelayMS: *delay,
		MaxDelayMS: *delay + 100,
		Perceiver:  neru.New(opts...),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := server.Run(ctx, cfg); err != nil {
		log.Printf("[server] 退出: %v", err)
	}
}

// splitList 把逗号分隔的 flag 值拆成去空白后的列表。
func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var list []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			list = append(list, item)
		}
	}
	return list
}
