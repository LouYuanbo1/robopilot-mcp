// agent 以终端 REPL 方式运行 RobotPilot 桌面操控智能体：启动时会在进程内自动
// 拉起一个本地 MCP 服务器(等价于 cmd/server)，通过其暴露的鼠标/键盘/屏幕/窗口/
// 剪贴板/元素感知/OCR/视觉检测工具，在真实桌面上执行用户自然语言指令。
// 无需再单独运行 ./cmd/server。
//
// 用法：
//
//	set LLM_API_KEY=sk-xxxx && go run ./cmd/agent
//	go run ./cmd/agent -addr :9090 -perceive-max 32     # 自定义端口与感知元素上限
//
// 可选环境变量：
//
//	LLM_API_KEY  模型密钥(必填)
//
// 交互：输入指令回车执行；exit/quit/q 退出。
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/schema"

	"github.com/LouYuanbo1/robopilot-mcp/agent"
	"github.com/LouYuanbo1/robopilot-mcp/agent/mnemo/fs"
	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver/neru"
	"github.com/LouYuanbo1/robopilot-mcp/server"
)

func main() {
	ctx := context.Background()

	var (
		addr  = flag.String("addr", ":8080", "进程内 MCP 服务器监听地址，如 :8080")
		max   = flag.Int("perceive-max", 64, "perceive_elements 单次最多返回的元素数量")
		minA  = flag.Int("perceive-min-area", 0, "按外接矩形面积(平方像素)过滤过小元素，0 不过滤")
		roles = flag.String("perceive-roles", "", "逗号分隔的只保留 role；空不过滤")
		depth = flag.Int("perceive-depth", 0, "无障碍树最大遍历深度，0 用后端默认值")
		langs = flag.String("ocr-langs", "", "逗号分隔的 OCR 语言顺序，如 zh-Hans,en-US；空用系统语言配置")
		hu    = flag.Bool("humanize", false, "拟人化移动/输入(平滑轨迹+随机延迟)")
		delay = flag.Int("delay-ms", 150, "拟人随机延迟(ms)，-humanize 时生效")
	)
	flag.Parse()

	mcpAddr := *addr
	if !strings.HasPrefix(mcpAddr, ":") && !strings.Contains(mcpAddr, ":") {
		mcpAddr = ":" + mcpAddr
	}
	mcpBaseURL := "http://127.0.0.1" + mcpAddr + "/mcp"
	srv := startMCPServer(mcpAddr, *max, *minA, *roles, *depth, *langs, *hu, *delay)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("[MCP 服务器] 关闭失败: %v", err)
		}
	}()

	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" {
		log.Fatal("未设置 LLM_API_KEY 环境变量，无法构造对话模型")
	}
	chatModel, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey: apiKey,
		Model:  "deepseek-flash",
	})
	if err != nil {
		log.Fatalf("构造对话模型失败: %v", err)
	}

	modelAgent, err := agent.NewDefaultRobotPilotAgent(ctx, chatModel, mcpBaseURL)
	if err != nil {
		log.Fatalf("构造 RobotPilot 智能体失败: %v", err)
	}

	// 持久化会话记忆到本地 sessions 目录（可多轮延续上下文）。
	memStore, err := fs.NewStore[*schema.Message](filepath.Join("user_data", "sessions"))
	if err != nil {
		log.Fatalf("创建记忆存储失败: %v", err)
	}
	modelAgent.SetMemoryStore(memStore)

	sessionID := time.Now().Format("20060102150405")
	fmt.Printf("[MCP 端点] %s (进程内自动拉起)\n[会话] %s\n输入指令执行；exit/quit/q 退出\n", mcpBaseURL, sessionID)

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("\n>>> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("\n[输入结束，退出]")
			return
		}
		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}
		switch strings.ToLower(input) {
		case "exit", "quit", "q":
			fmt.Println("[退出]")
			return
		}

		if err := modelAgent.OutputMessage(ctx, sessionID, input, func(s string) {
			fmt.Print(s)
		}); err != nil {
			fmt.Printf("\n[执行出错: %v]\n", err)
		}
		fmt.Println()
	}
}

// startMCPServer 在进程内启动一个 MCP HTTP 服务器并返回，供 defer 关闭。
// 免去单独运行 cmd/server 的步骤。
func startMCPServer(
	addr string,
	max, minArea int,
	roles string,
	depth int,
	langs string,
	humanize bool,
	delay int,
) *http.Server {
	opts := []neru.Option{neru.WithMaxElements(max)}
	if minArea > 0 {
		opts = append(opts, neru.WithMinArea(minArea))
	}
	if depth > 0 {
		opts = append(opts, neru.WithMaxDepth(depth))
	}
	if list := splitList(roles); len(list) > 0 {
		opts = append(opts, neru.WithRoles(list...))
	}
	if list := splitList(langs); len(list) > 0 {
		opts = append(opts, neru.WithOCRLanguages(list...))
	}

	cfg := server.Config{
		Addr:       addr,
		Humanize:   humanize,
		MinDelayMS: delay,
		MaxDelayMS: delay + 100,
		Perceiver:  neru.New(opts...),
	}

	srv := server.New(cfg)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[MCP 服务器] 启动失败: %v", err)
		}
	}()
	// 留少许时间让监听就绪，避免 agent 首条工具调用恰好落后于服务器绑定。
	time.Sleep(200 * time.Millisecond)
	return srv
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
