package neru

// 真实桌面验证：默认跳过，设置 ROBOPILOT_LIVE=1 后运行。
//
//	go test ./executor/perceiver/neru -run TestLive -v
//	$env:ROBOPILOT_LIVE=1; go test ./executor/perceiver/neru -run TestLive -v
//
// 这些用例用于确认 neru-core 在目标机器上能正常读到无障碍树、截屏、OCR 与视觉目标。
// 除 TestLiveSetWindowBounds 会创建一个 winver 窗口并移动/缩放它之外，其余用例只读
// 感知（不点击、不输入）。

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/LouYuanbo1/neru-core/accessibility"

	"github.com/LouYuanbo1/robopilot-mcp/action"
	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver"
)

func requireLive(t *testing.T) {
	t.Helper()
	if os.Getenv("ROBOPILOT_LIVE") == "" {
		t.Skip("设置 ROBOPILOT_LIVE=1 才会访问真实桌面")
	}
}

// TestLiveInfo 验证桌面几何、前台窗口与能力探测。
func TestLiveInfo(t *testing.T) {
	requireLive(t)

	p := New()
	defer p.Close()

	info, err := p.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	t.Logf("virtual_desktop=%+v cursor=%+v window=%+v", info.VirtualDesktop, info.Cursor, info.Window)
	t.Logf("accessibility=%+v capture=%+v ocr=%+v", info.Accessibility, info.Capture, info.OCR)

	if info.VirtualDesktop.Empty() {
		t.Fatalf("虚拟桌面区域为空")
	}
}

// TestLivePerceive 验证无障碍树扫描；同时打印原始节点数，便于区分
// "后端没扫到" 与 "上层过滤掉了"。
func TestLivePerceive(t *testing.T) {
	requireLive(t)

	ctx := context.Background()
	p := New(WithMaxElements(8))
	defer p.Close()

	elements, err := p.Perceive(ctx)
	if err != nil {
		t.Fatalf("Perceive: %v", err)
	}
	t.Logf("perceive elements=%d", len(elements))
	for _, e := range elements {
		t.Logf("  %s role=%s title=%q bounds=(%d,%d %dx%d)", e.Label, e.Role, e.Title, e.X, e.Y, e.Width, e.Height)
	}

	// 直接调用底层客户端，观察未经本包过滤的原始节点。
	client, err := accessibility.New(accessibility.Options{})
	if err != nil {
		t.Fatalf("accessibility.New: %v", err)
	}
	defer client.Close()

	t.Logf("clickable roles=%v", client.ClickableRoles())

	window, err := client.FrontmostWindow(ctx)
	if err != nil {
		t.Fatalf("FrontmostWindow: %v", err)
	}
	nodes, err := client.ClickableNodes(ctx, window, nil, 0, false)
	window.Release()
	if err != nil {
		t.Fatalf("ClickableNodes: %v", err)
	}
	t.Logf("raw nodes=%d", len(nodes))
	for i, node := range nodes {
		if i >= 10 {
			break
		}
		t.Logf("  raw role=%s title=%q bounds=%v", node.Role(), node.Title(), node.Bounds())
	}
	accessibility.ReleaseNodes(nodes)
}

// TestLiveReadText 验证 OCR：只读屏幕左上角一小块，避免把整屏内容带进日志。
func TestLiveReadText(t *testing.T) {
	requireLive(t)

	p := New()
	defer p.Close()

	region := &action.Region{X: 0, Y: 0, Width: 240, Height: 48}
	result, err := p.ReadText(context.Background(), perceiver.TextRequest{
		Region:    region,
		WordLevel: true,
	})
	if err != nil {
		t.Fatalf("ReadText: %v", err)
	}
	t.Logf("region=%+v words=%d", result.Region, len(result.Words))
}

// TestLiveDetectTargets 验证纯视觉检测（不依赖 OCR 的轮廓策略最快）。
func TestLiveDetectTargets(t *testing.T) {
	requireLive(t)

	p := New()
	defer p.Close()

	elements, err := p.DetectTargets(context.Background(), perceiver.TargetRequest{
		Strategy: "contour",
		Region:   &action.Region{X: 0, Y: 0, Width: 800, Height: 400},
	})
	if err != nil {
		t.Fatalf("DetectTargets: %v", err)
	}
	t.Logf("contour targets=%d", len(elements))
}

// TestLiveWindows 验证窗口枚举与前台窗口读取（只读，不激活、不改动桌面）。
func TestLiveWindows(t *testing.T) {
	requireLive(t)

	ctx := context.Background()
	p := New()
	defer p.Close()

	windows, err := p.ListWindows(ctx)
	if err != nil {
		t.Fatalf("ListWindows: %v", err)
	}
	t.Logf("visible windows=%d", len(windows))

	focused := 0
	for i, w := range windows {
		if w.Focused {
			focused++
		}
		if i >= 10 {
			continue
		}
		t.Logf("  id=%s pid=%d app=%q title=%q rect=%+v minimized=%v focused=%v",
			w.ID, w.PID, w.Application, w.Title, w.Region, w.Minimized, w.Focused)
	}

	if len(windows) > 0 && focused != 1 {
		t.Fatalf("期望恰好一个前台窗口, got %d", focused)
	}

	frontmost, err := p.FrontmostWindow(ctx)
	if err != nil {
		t.Fatalf("FrontmostWindow: %v", err)
	}
	t.Logf("frontmost id=%s pid=%d app=%q title=%q",
		frontmost.ID, frontmost.PID, frontmost.Application, frontmost.Title)

	info, err := p.Info(ctx)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Window.PID == 0 {
		t.Errorf("Info 没有带回前台窗口 PID: %+v", info.Window)
	}
}

// TestLiveSetWindowBounds 端到端验证"激活并摆放"：启动一个普通 Win32 窗口，
// 通过 perceiver 把它移动到指定矩形，确认返回的矩形就是实际矩形。
//
// 只读取与窗口几何，不点击、不输入；结束后关闭测试窗口并恢复原前台窗口。
func TestLiveSetWindowBounds(t *testing.T) {
	requireLive(t)

	ctx := context.Background()
	p := New()
	defer p.Close()

	original, originalErr := p.FrontmostWindow(ctx)
	if originalErr != nil {
		t.Logf("没有可恢复的前台窗口: %v", originalErr)
	}

	command := exec.Command("winver.exe")
	if err := command.Start(); err != nil {
		t.Fatalf("启动 winver.exe: %v", err)
	}

	defer func() {
		if original.ID != "" {
			_, _ = p.ActivateWindow(context.Background(), perceiver.WindowRequest{ID: original.ID})
		}
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
	}()

	var target perceiver.Window

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		windows, err := p.ListWindows(ctx)
		if err != nil {
			t.Fatalf("ListWindows: %v", err)
		}

		for _, w := range windows {
			if w.PID == command.Process.Pid && w.Title != "" {
				target = w
				break
			}
		}

		if target.ID != "" {
			break
		}

		time.Sleep(100 * time.Millisecond)
	}

	if target.ID == "" {
		t.Fatalf("进程 %d 一直没有出现带标题的窗口", command.Process.Pid)
	}

	want := action.Region{X: 200, Y: 150, Width: 500, Height: 380}

	placed, err := p.SetWindowBounds(ctx, perceiver.WindowRequest{ID: target.ID}, want)
	if err != nil {
		t.Fatalf("SetWindowBounds: %v", err)
	}

	if placed.Region != want {
		t.Fatalf("窗口落在 %+v, 期望 %+v", placed.Region, want)
	}

	t.Logf("placed id=%s pid=%d rect=%+v", placed.ID, placed.PID, placed.Region)
}
