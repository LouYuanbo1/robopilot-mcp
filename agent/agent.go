package agent

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/LouYuanbo1/robopilot-mcp/agent/mcp"
	"github.com/LouYuanbo1/robopilot-mcp/agent/mnemo/fs"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// RobotPilotAgent 是面向桌面操控的智能体：基于真实桌面环境，通过 MCP 服务暴露的
// 鼠标 / 键盘 / 屏幕 / 窗口 / 剪贴板工具完成用户交给的自动化任务。
type RobotPilotAgent struct {
	Agent *adk.ChatModelAgent

	// client 是底层 MCP 客户端；经回调远程持有 MCP 服务。为 nil 时无缝降级。
	client *mcp.Client

	// memory 持久化会话上下文；为 nil 时不保存记忆，退化为一次性执行。
	memory *fs.Store[*schema.Message]
}

// SetMemoryStore 开启持久化记忆并指定会话存储目录。
func (c *RobotPilotAgent) SetMemoryStore(store *fs.Store[*schema.Message]) {
	c.memory = store
}

// NewRobotPilotAgent 用一个已构造好的底层 agent 包装出 RobotPilotAgent。
func NewRobotPilotAgent(ctx context.Context, agent *adk.ChatModelAgent) (*RobotPilotAgent, error) {
	if agent == nil {
		return nil, fmt.Errorf("agent is nil")
	}
	return &RobotPilotAgent{Agent: agent}, nil
}

// NewDefaultRobotPilotAgent 以默认桌面操控工具集构造一个可直接使用的 RobotPilotAgent，
// 工具统一经 MCPClient 转发给远端 MCP 服务（cmd/server）。
func NewDefaultRobotPilotAgent(ctx context.Context, model model.ToolCallingChatModel, mcpBaseURL string) (*RobotPilotAgent, error) {

	client := mcp.NewMCPClient(mcpBaseURL)
	tools, err := newRobotPilotTools(client)
	if err != nil {
		return nil, err
	}

	instruction :=
		`
			## 角色定义
			你是一位桌面自动化操控智能体（RobotPilot），直接操纵真实电脑桌面：能"看见"屏幕上的可交互元素与文字，也能移动/点击鼠标、键入文本、发送快捷键、管理窗口与剪贴板，帮用户完成本地软件、网页、游戏等场景下的操作任务。

			## 工作方式：先看，再点，再验证
			1. **看**：任务开始时先调用 inspect_screen_tool（不带参数即可）。一次就能拿到屏幕几何、前台窗口，以及当前窗口里全部可交互元素（角色/名称/矩形/中心点）。
			2. **点**：知道控件名字就用 click_element_tool（text="保存"）；只知道坐标才用 click_mouse_at_tool（x/y 用元素返回的 center）；只想悬停/定位用 move_mouse_tool。
			3. **验证**：操作后再调用 inspect_screen_tool 或 read_text_tool 确认结果，不要凭想象继续。

			看屏幕只有这三种情形，不要再用别的工具"到处翻"：
			- 看当前窗口：inspect_screen_tool()
			- 界面没有无障碍信息（游戏、画布、远程桌面）：inspect_screen_tool(source="vision")
			- 要看不在前台的应用：inspect_screen_tool(scope="app", application="chrome.exe")。这**不会切换窗口**，能直接读到它的内容和标签页标题；需要点击时再用 set_active_window_tool 激活。
			屏幕很乱时用 query / roles / region 收窄，不要一次拉几百个元素。

			## 硬规则（违反这些会得出错误结论）
			1. **没看见不等于不存在**：说某个应用"没在运行/没有播放"之前，必须看过 list_windows_tool 的 application 字段，或用 scope="app" 扫过该应用。窗口可能最小化、在别的虚拟桌面上、或只是被别的窗口挡住。
			2. **同一个调用最多试两次**：失败一次就换参数或换工具（换 source / scope / 换工具），不要反复撞同一个错误。
			3. **你读不了图片**：截图工具保持默认 output=file（只落盘返回路径）；inline/both 只给支持看图的客户端用。
			4. **scope="surface" 仅 macOS**：Windows 上会直接报错，不要用。
			5. **坐标是屏幕绝对像素**，原点在虚拟桌面左上角；多显示器时可能为负坐标。
			6. **需求含糊时先按最可能的载体查一次，再问用户**：例如"我的哔哩哔哩在放什么"——先看浏览器标签页标题或客户端窗口；确实查不到再问清楚是哪个设备/哪个窗口，而不是直接断言"没有在播放"。

			## 输入与快捷键
			- 输入前先点击目标输入框使其获焦；长文本用 clipboard_tool 写入，再 key_tap_tool 发送 ctrl+v 粘贴，比逐字键入稳定。
			- 快捷键用 key_tap_tool，例如复制 key="c"、modifiers=["ctrl"]；Windows 键写作 "cmd"（别名 "win"）。
			- 需要"按住—移动—松开"时：move_mouse_tool → mouse_button_tool(state="down") → move_mouse_tool → mouse_button_tool(state="up")。
			- 滚动列表/面板时用 scroll_mouse_tool，可带 x/y 先把指针移过去。

			## 窗口与应用
			- list_windows_tool 返回每个可见窗口的 id / title / pid / application / 位置 / 是否最小化；application 字段就是 scope=app 要的参数，id 是 set_active_window_tool 最精确的定位方式。
			- 切换窗口：set_active_window_tool，id 或 pid 最精确，其次 application（如 chrome.exe），再次 title（真正的窗口标题，不是进程名）。
			- 摆放窗口：给 set_active_window_tool 传 rect={x,y,width,height}（屏幕绝对像素）。用 inspect_screen_tool 返回的 virtual_desktop(dx,dy,dw,dh) 计算，不要自己猜屏幕尺寸：
			  左半屏 {x:dx, y:dy, width:dw/2, height:dh}；右半屏 {x:dx+dw/2, y:dy, width:dw/2, height:dh}；整屏直接传 virtual_desktop。
			  例：virtual_desktop={0,0,2160,1440} 的左半屏是 {x:0,y:0,width:1080,height:1440}。窗口处于最大化/最小化时会先还原再摆放，返回值里的 rect 是实际矩形。
			- Windows 上贴靠/最大化可以不用 rect：先激活窗口，再 key_tap_tool 发 win+left / win+right / win+up，让系统按工作区、任务栏与多显示器规则处理；需要精确矩形时才用 rect。

			## 行为准则
			- 所有操作基于用户明确指令，不擅自执行需求之外的高风险操作（如删除文件、提交表单）。
			- 操作失败时如实报告错误并给出替代方案，不要沉默重试。
			- 涉及可能破坏用户数据的操作前，先向用户说明。

			## 输出格式规范
			完成任务后简要总结：
			【执行结果】最终达成了什么
			【操作摘要】关键工具调用及其结果
			【注意事项】（如适用）残留状态、需要用户确认的事项
		`
	robotAgent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "robotPilotAgent",
		Description:   "一个基于大模型的桌面自动化操控智能体，支持屏幕感知/元素定位与鼠标键盘操作",
		Instruction:   instruction,
		Model:         model,
		MaxIterations: 40,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("Error creating robot agent: %w", err)
	}
	return &RobotPilotAgent{Agent: robotAgent, client: client}, nil
}

// newRobotPilotTools 集中注册 RobotPilot 智能体所需的全部桌面工具（14 个）。
// 每个工具都是指向远端 MCP 服务的轻客户端：由 MCPClient 统一构造。
// 这里用一张注册表描述“工具名 → 创建函数”，统一依次创建并聚合到工具列表，
// 避免在 NewDefaultRobotPilotAgent 中堆叠十余段重复的错误处理代码。
//
// 工具集刻意保持"一个意图一个工具"，减少模型的选择负担：
// 看屏 = inspect_screen，读字 = read_text，点元素 = click_element，点坐标 = click_mouse_at。
func newRobotPilotTools(c *mcp.Client) ([]tool.BaseTool, error) {
	constructors := []struct {
		name string
		new  func() (tool.InvokableTool, error)
	}{
		{name: "inspect_screen_tool", new: func() (tool.InvokableTool, error) { return mcp.NewInspectScreenTool(c) }},
		{name: "read_text_tool", new: func() (tool.InvokableTool, error) { return mcp.NewReadTextTool(c) }},
		{name: "screenshot_tool", new: func() (tool.InvokableTool, error) { return mcp.NewScreenshotTool(c) }},
		{name: "list_windows_tool", new: func() (tool.InvokableTool, error) { return mcp.NewListWindowsTool(c) }},
		{name: "set_active_window_tool", new: func() (tool.InvokableTool, error) { return mcp.NewSetActiveWindowTool(c) }},
		{name: "click_element_tool", new: func() (tool.InvokableTool, error) { return mcp.NewClickElementTool(c) }},
		{name: "click_mouse_at_tool", new: func() (tool.InvokableTool, error) { return mcp.NewClickMouseAtTool(c) }},
		{name: "move_mouse_tool", new: func() (tool.InvokableTool, error) { return mcp.NewMoveMouseTool(c) }},
		{name: "mouse_button_tool", new: func() (tool.InvokableTool, error) { return mcp.NewMouseButtonTool(c) }},
		{name: "drag_mouse_tool", new: func() (tool.InvokableTool, error) { return mcp.NewDragMouseTool(c) }},
		{name: "scroll_mouse_tool", new: func() (tool.InvokableTool, error) { return mcp.NewScrollMouseTool(c) }},
		{name: "type_text_tool", new: func() (tool.InvokableTool, error) { return mcp.NewTypeTextTool(c) }},
		{name: "key_tap_tool", new: func() (tool.InvokableTool, error) { return mcp.NewKeyTapTool(c) }},
		{name: "clipboard_tool", new: func() (tool.InvokableTool, error) { return mcp.NewClipboardTool(c) }},
	}

	tools := make([]tool.BaseTool, 0, len(constructors))
	for _, ctor := range constructors {
		t, err := ctor.new()
		if err != nil {
			return nil, fmt.Errorf("创建工具 %s 失败: %w", ctor.name, err)
		}
		tools = append(tools, t)
	}
	return tools, nil
}

func (c *RobotPilotAgent) Run(ctx context.Context, input *adk.AgentInput, options ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	return c.Agent.Run(ctx, input, options...)
}

// OutputMessage 执行一轮对话：把流式输出写到 streamFunc，
// 并在启用了持久化记忆时，将本轮消息按会话 sessionID 追加保存，
// 同时把历史消息作为上下文重新注入本轮。
func (c *RobotPilotAgent) OutputMessage(ctx context.Context, sessionID, input string, streamFunc func(string), options ...adk.AgentRunOption) error {
	var sess *fs.Session[*schema.Message]
	var history []*schema.Message
	if c.memory != nil {
		s, err := c.memory.GetOrCreate(sessionID)
		if err != nil {
			return fmt.Errorf("get session %q: %w", sessionID, err)
		}
		sess = s
		history = s.GetMessages()
	}

	// 历史 + 本轮用户消息，作为完整输入提交给 agent
	msgs := make([]*schema.Message, 0, len(history)+1)
	msgs = append(msgs, history...)
	msgs = append(msgs, schema.UserMessage(input))

	runCtx := ctx

	runner := adk.NewRunner(runCtx, adk.RunnerConfig{Agent: c.Agent, EnableStreaming: true})
	iter := runner.Run(runCtx, msgs, options...)

	produced, err := stream(iter, streamFunc)

	// 无论 stream 是否出错，只要已有会话且产生了内容，都应尽量持久化，
	// 避免一次消息错误就导致整段对话丢失。
	if sess != nil {
		sess.Append(schema.UserMessage(input))
		for _, m := range produced {
			// 只持久化输入/回答/工具调用/工具结果，不保存思考过程。
			if m.Role == schema.Assistant {
				m.ReasoningContent = ""
			}
			sess.Append(m)
		}
	}
	return err
}

// stream 消费 agent 事件流，把文本输出交给 streamFunc，
// 并收集本轮产生的完整消息（含助手消息、工具调用与工具结果），用于持久化。
func stream(iter *adk.AsyncIterator[*adk.AgentEvent], streamFunc func(string)) ([]*schema.Message, error) {
	var produced []*schema.Message
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}

		if event.Err != nil {
			log.Printf("Runner 错误: %v", event.Err)
			return produced, event.Err
		}

		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		mo := event.Output.MessageOutput

		// 流式消息：边输出文本边累积 chunk，结束后拼接为完整消息
		if mo.MessageStream != nil {
			msg, err := streamMessage(mo.MessageStream, streamFunc)
			if err != nil {
				return produced, err
			}
			if msg != nil {
				produced = append(produced, msg)
			}
			continue
		}

		// 非流式消息（通常是工具结果）：直接使用完整消息
		if mo.Message != nil {
			produced = append(produced, mo.Message)
		}
	}
	return produced, nil
}

func streamMessage(s adk.MessageStream, streamFunc func(string)) (*schema.Message, error) {
	// 【关键修复 2】：防御性判空，防止外部意外传入 nil 导致 panic
	if s == nil {
		return nil, nil
	}
	defer s.Close()

	chunks := make([]*schema.Message, 0, 8)

	// 工具调用信息缓存：在流式模式下，工具调用参数会跨多个 chunk 到达，需要累积
	accToolCalls := make(map[int]*schema.ToolCall)

	for {
		chunk, err := s.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Printf("Stream Recv 错误: %v", err)
			return nil, err
		}

		chunks = append(chunks, chunk)

		// 处理思考过程 (如果开启了 DeepSeek-R1 等支持 Reasoning 的模型)
		if chunk.ReasoningContent != "" {
			flushToolCalls(accToolCalls, streamFunc)
			streamFunc(chunk.ReasoningContent)
			continue
		}

		// 处理工具调用：累积各 chunk 中的参数片段
		if len(chunk.ToolCalls) > 0 {
			accumulateToolCalls(accToolCalls, chunk.ToolCalls)
			continue
		}

		// 处理文本内容：先刷新累积的工具调用，再输出文本
		if chunk.Content != "" {
			flushToolCalls(accToolCalls, streamFunc)
			streamFunc(chunk.Content)
		}
	}

	// 流结束时刷新残留的工具调用
	flushToolCalls(accToolCalls, streamFunc)

	if len(chunks) == 0 {
		return nil, nil
	}
	return schema.ConcatMessages(chunks)
}

// accumulateToolCalls 将流式 chunk 中的工具调用信息累积到 acc 中
func accumulateToolCalls(acc map[int]*schema.ToolCall, toolCalls []schema.ToolCall) {
	for _, tc := range toolCalls {
		idx := 0
		if tc.Index != nil {
			idx = *tc.Index
		}

		if existing, ok := acc[idx]; ok {
			if tc.ID != "" {
				existing.ID = tc.ID
			}
			if tc.Type != "" {
				existing.Type = tc.Type
			}
			if tc.Function.Name != "" {
				existing.Function.Name = tc.Function.Name
			}
			existing.Function.Arguments += tc.Function.Arguments
		} else {
			cp := tc
			acc[idx] = &cp
		}
	}
}

// flushToolCalls 输出所有累积的工具调用信息并清空缓存
func flushToolCalls(acc map[int]*schema.ToolCall, streamFunc func(string)) {
	for idx := 0; idx < len(acc); idx++ {
		tc, ok := acc[idx]
		if !ok {
			continue
		}
		line := "\n[工具调用] " + tc.Function.Name
		if len(tc.Function.Arguments) > 0 {
			line += ", 参数: " + tc.Function.Arguments
		}
		streamFunc(line + "\n")
		delete(acc, idx)
	}
}
