package executor

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/LouYuanbo1/robopilot-mcp/action"
	"github.com/LouYuanbo1/robopilot-mcp/executor/perceiver"
)

// fakeWindowManager is a perceiver that also implements the window capability,
// so the executor's routing can be tested without a desktop.
type fakeWindowManager struct {
	list   []perceiver.Window
	result perceiver.Window

	activated  []perceiver.WindowRequest
	placements []boundsCall
}

type boundsCall struct {
	request perceiver.WindowRequest
	bounds  action.Region
}

func (f *fakeWindowManager) Perceive(context.Context) ([]perceiver.Element, error) {
	return nil, nil
}

func (f *fakeWindowManager) ListWindows(context.Context) ([]perceiver.Window, error) {
	return f.list, nil
}

func (f *fakeWindowManager) FrontmostWindow(context.Context) (perceiver.Window, error) {
	return perceiver.Window{}, nil
}

func (f *fakeWindowManager) ActivateWindow(
	_ context.Context,
	req perceiver.WindowRequest,
) (perceiver.Window, error) {
	f.activated = append(f.activated, req)

	return f.result, nil
}

func (f *fakeWindowManager) SetWindowBounds(
	_ context.Context,
	req perceiver.WindowRequest,
	bounds action.Region,
) (perceiver.Window, error) {
	f.placements = append(f.placements, boundsCall{request: req, bounds: bounds})

	return f.result, nil
}

func TestWindowAction_ListWindowsReturnsWindows(t *testing.T) {
	fake := &fakeWindowManager{list: []perceiver.Window{
		{
			ID:      "0x1",
			PID:     7,
			Title:   "editor",
			Region:  action.Region{X: 1, Y: 2, Width: 300, Height: 200},
			Focused: true,
		},
	}}

	executor := New(WithPerceiver(fake))

	results, err := executor.Execute(context.Background(), []action.Action{&action.ListWindows{}})
	if err != nil {
		t.Fatalf("Execute(ListWindows): %v", err)
	}

	var windows []perceiver.Window
	if err := json.Unmarshal([]byte(results[0].Value), &windows); err != nil {
		t.Fatalf("ListWindows value is not a window list: %v", err)
	}

	if len(windows) != 1 || windows[0].ID != "0x1" || !windows[0].Focused {
		t.Fatalf("ListWindows returned %+v", windows)
	}

	if !results[0].Effective {
		t.Fatal("ListWindows result is not effective")
	}
}

func TestWindowAction_ActivatesWithoutRect(t *testing.T) {
	fake := &fakeWindowManager{result: perceiver.Window{ID: "0x1", Title: "editor"}}
	executor := New(WithPerceiver(fake))

	_, err := executor.Execute(context.Background(), []action.Action{
		&action.SetActiveWindow{Application: "chrome.exe"},
	})
	if err != nil {
		t.Fatalf("Execute(SetActiveWindow): %v", err)
	}

	if len(fake.activated) != 1 {
		t.Fatalf("ActivateWindow calls = %d, want 1", len(fake.activated))
	}

	if len(fake.placements) != 0 {
		t.Fatalf("SetWindowBounds calls = %d, want 0", len(fake.placements))
	}

	if fake.activated[0].Application != "chrome.exe" {
		t.Fatalf("ActivateWindow request = %+v", fake.activated[0])
	}
}

func TestWindowAction_PlacesWithRect(t *testing.T) {
	fake := &fakeWindowManager{result: perceiver.Window{ID: "0x1", Title: "editor"}}
	executor := New(WithPerceiver(fake))

	rect := action.Region{X: 0, Y: 0, Width: 960, Height: 1440}

	results, err := executor.Execute(context.Background(), []action.Action{
		&action.SetActiveWindow{Title: "editor", Rect: &rect},
	})
	if err != nil {
		t.Fatalf("Execute(SetActiveWindow with rect): %v", err)
	}

	if len(fake.placements) != 1 {
		t.Fatalf("SetWindowBounds calls = %d, want 1", len(fake.placements))
	}

	if len(fake.activated) != 0 {
		t.Fatalf("ActivateWindow calls = %d, want 0: placement must be one resolution pass", len(fake.activated))
	}

	if fake.placements[0].bounds != rect {
		t.Fatalf("placement bounds = %+v, want %+v", fake.placements[0].bounds, rect)
	}

	if fake.placements[0].request.Title != "editor" {
		t.Fatalf("placement request = %+v", fake.placements[0].request)
	}

	if !results[0].Effective {
		t.Fatal("placement result is not effective")
	}
}

func TestWindowAction_RejectsAnEmptyRect(t *testing.T) {
	fake := &fakeWindowManager{}
	executor := New(WithPerceiver(fake))

	rect := action.Region{Width: 0, Height: 100}

	if _, err := executor.Execute(context.Background(), []action.Action{
		&action.SetActiveWindow{Title: "editor", Rect: &rect},
	}); err == nil {
		t.Fatal("an empty rect should be rejected")
	}

	if len(fake.placements) != 0 {
		t.Fatalf("SetWindowBounds calls = %d, want 0", len(fake.placements))
	}
}

func TestWindowAction_WithoutAPerceiverFailsLoudly(t *testing.T) {
	executor := New()

	_, err := executor.Execute(context.Background(), []action.Action{&action.ListWindows{}})
	if err == nil {
		t.Fatal("window actions without a perceiver should fail")
	}

	if !strings.Contains(err.Error(), "未配置元素感知器") {
		t.Fatalf("error = %v, want the missing-perceiver explanation", err)
	}
}

func TestWindowAction_APerceiverWithoutTheCapabilityFailsLoudly(t *testing.T) {
	executor := New(WithPerceiver(plainPerceiver{}))

	_, err := executor.Execute(context.Background(), []action.Action{&action.ListWindows{}})
	if err == nil {
		t.Fatal("window actions without the window capability should fail")
	}

	if !strings.Contains(err.Error(), "WindowManager") {
		t.Fatalf("error = %v, want it to name the missing interface", err)
	}
}

// plainPerceiver is a perceiver that does not implement the window capability.
type plainPerceiver struct{}

func (plainPerceiver) Perceive(context.Context) ([]perceiver.Element, error) {
	return nil, nil
}
