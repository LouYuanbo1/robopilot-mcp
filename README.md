# robopilot-mcp

[![English](https://img.shields.io/badge/Language-English-blue.svg)](./README.md) [![中文](https://img.shields.io/badge/Language-%E4%B8%AD%E6%96%87-red.svg)](./README_zh.md)

A **desktop automation MCP server** written in Go. It can both **see** the screen
(accessibility tree / OCR / pixel vision) and **act** on it (mouse / keyboard / window /
clipboard), and exposes all of that to any MCP client over the
[Model Context Protocol](https://modelcontextprotocol.io) — Claude Desktop, the built-in
RobotPilot agent, or your own agent.

The perception and window-management parts are built on
[`neru-core`](https://github.com/LouYuanbo1/neru-core): pure Go, no CGo, talking directly to
system interfaces in-process (Windows UIA / macOS AX / Linux AT-SPI + screen capture + OCR +
contour/segmentation detection + window enumeration/activation/placement). No extra daemon
is required.

## Tools (14 — one intent, one tool)

### Perception (read-only)

| Tool | Description |
| --- | --- |
| `inspect_screen_tool` | **Call this first.** Returns it all at once: desktop geometry (resolution / DPI / virtual desktop / cursor), the foreground window (title / PID / app), and the current window's interactive elements (role / name / bounds / center point); optionally OCR text and a screenshot written to disk |
| `read_text_tool` | OCR the screen or a region; `words=true` returns word blocks with coordinates so you can click by text position |
| `screenshot_tool` | Take a screenshot. `output` controls the form: `file` (default, written to `~/robopilot-artifacts`) / `inline` (**no disk write**, image bytes returned directly with the result) / `both` |

> `inline` / `both` only make sense for clients that can render images; text-only models
> (like the current built-in agent) can stay on the default `file`, hand the path straight to
> the user, and avoid a few MB of base64 crossing the wire for nothing.

The three key parameters of `inspect_screen_tool`:

| Parameter | Purpose |
| --- | --- |
| `scope` | `window` (default) current window / `application` the whole foreground app / `app` an app specified by `application` / `surface` auxiliary surface (macOS only) |
| `query` / `roles` / `region` | Narrow down the result on element-heavy UIs |
| `source` / `strategy` | `source=vision` switches to pixel vision (`strategy=vision/contour/segment`) for games, canvases, remote desktops, and other UIs with no readable accessibility data |

`scope=app` is the key to "reading another app": pass the `application` returned by
`list_windows_tool` (e.g. `chrome.exe`) and read that app's content **without switching
windows** — for example, browser tab titles.

### Window

| Tool | Description |
| --- | --- |
| `list_windows_tool` | List all visible top-level windows: title, PID, app identifier (executable name on Windows/Linux, bundle id on macOS), window id, on-screen bounds, minimized flag, foreground flag |
| `set_active_window_tool` | Activate by `id` (from list_windows, most precise) / `pid` (precise) / `application` (e.g. chrome.exe) / `title` (the actual window title); optionally pass `rect={x,y,width,height}` to move the window into that rectangle at the same time, handy for putting a target app and a log side by side |

Don't guess screen sizes for left/right halves: use the `virtual_desktop` (`dx,dy,dw,dh`)
returned by `inspect_screen_tool`. The left half is `{x:dx, y:dy, width:dw/2, height:dh}`,
the right half is `{x:dx+dw/2, y:dy, width:dw/2, height:dh}`. On Windows you can also activate a
window first and use `key_tap_tool` to send `win+left` / `win+right` / `win+up` and let the OS
snap/maximize per workspace and multi-monitor rules; use `rect` only when you need an exact
rectangle.

### Action

| Tool | Description |
| --- | --- |
| `click_element_tool` | **Prefer this one:** locate an element by name/role/direction and click; by default it moves the cursor back to where it was after clicking |
| `click_mouse_at_tool` | Click at coordinates (an element's returned `center` works directly) |
| `move_mouse_tool` | Move / hover the pointer |
| `mouse_button_tool` | Hold or release with `state=down/up`, for staged drags and marquee selection when combined with movement |
| `drag_mouse_tool` / `scroll_mouse_tool` | Drag / scroll wheel (can optionally move the pointer to a target first, then scroll) |
| `type_text_tool` / `key_tap_tool` | Type text / single keys and key combos |
| `clipboard_tool` | Write with `text`, read without it |

### Recommended workflow

```
see:      inspect_screen_tool()                               ← everything at once (incl. virtual_desktop)
arrange:  set_active_window_tool(application="chrome.exe",
              rect={x:0, y:0, width:1080, height:1440})       ← left half, beside the log
click:    click_element_tool(text="Save")                     ← click by name when known
              or click_mouse_at_tool(x=…, y=…center…)         ← click by coordinates otherwise
verify:   inspect_screen_tool() / read_text_tool()
```

## Quick start

```bash
# Start only the MCP server (defaults to listening on :8080)
go run ./cmd/server

# Custom port and perception parameters
go run ./cmd/server -addr :9090 -perceive-max 32 -perceive-depth 12 -ocr-langs zh-Hans,en-US

# Terminal REPL agent (spins up the MCP server in-process; needs LLM_API_KEY)
set LLM_API_KEY=sk-xxxx && go run ./cmd/agent

# Try an action directly without a model
go run ./cmd/demo -act '{"actions":[{"type":"inspect_screen","max":10}]}'
go run ./cmd/demo -act '{"actions":[{"type":"inspect_screen","scope":"app","application":"chrome.exe"}]}'
go run ./cmd/demo -act '{"actions":[{"type":"click_element","text":"Save"}]}'
go run ./cmd/demo -act '{"actions":[{"type":"read_text","region":{"x":0,"y":0,"width":600,"height":200},"words":true}]}'
```

MCP endpoint: `http://127.0.0.1:8080/mcp` (Streamable HTTP); health check: `GET /healthz`.

Legacy action names (`perceive_elements` / `describe_screen` / `detect_targets` / `mouse_down` /
`mouse_up` / `get_clipboard` / `set_clipboard`) still parse and map onto the merged actions.

### Connect an MCP client

For example, Claude Desktop:

```json
{
  "mcpServers": {
    "robopilot": {
      "url": "http://127.0.0.1:8080/mcp"
    }
  }
}
```

### Platform permissions

| Platform | What's needed |
| --- | --- |
| Windows | No extra authorization (UIA and GDI capture don't prompt) |
| macOS | System Settings → Privacy & Security → **Accessibility** and **Screen Recording**: tick this program |
| Linux | No special permissions; under Wayland some compositors may prompt once on first input injection |

## Architecture

```
action/                      Pure protocol layer: Action / ActionResult / declarative JSON parsing
                                   and validation (zero platform dependencies)
executor/                    Execution layer: action routing + robotgo driver (mouse / keyboard /
                                   screen / clipboard)
executor/perceiver/          Perception contract layer (pure logic, no platform code):
                                   perceiver.go capability interfaces / element.go element model /
                                   request.go requests & readings
                                   window.go window capability contract / filter.go filter-dedup-sort /
                                   select.go selection & spatial navigation
                                   render.go serialization / ax.go accessibility-role vocabulary parsing
executor/perceiver/neru/     Perception & window implementation layer (neru-core adapter):
                                   neru.go construction & lifecycle / perceive.go accessibility-tree scan /
                                   info.go desktop state
                                   window.go enumeration, activation & placement / text.go OCR /
                                   vision.go visual detection
                                   pick.go spatial navigation / capture.go screen capture
server/                      MCP service layer: tool registration, timeouts, logging, image content
agent/                       Built-in agent: eino tools + prompt, calls the server via the MCP client
cmd/server                   MCP server entrypoint
cmd/agent                    REPL agent entrypoint
cmd/demo                     Action-protocol debugging entrypoint
```

Design highlights:

- **Protocol separated from implementation**: `action` only describes "what to do" and depends on
  no platform library; perception- and window-type actions are carried by the `perceiver`
  implementation, input actions by `executor` via robotgo.
- **Capabilities split by interface**: `perceiver` describes capabilities with several small
  interfaces (`Scoper` / `InfoProvider` / `TextReader` / `TargetDetector` / `SpatialPicker` /
  `WindowManager`). An implementation gains an extra capability automatically for every interface
  it implements, and fails loudly for whichever one it's missing.
- **Window complexity lives in neru-core**: enumeration, activation and placement aren't written
  platform-file by platform-file; they call the `window` package of neru-core. Win32
  `EnumWindows`/`SetForegroundWindow`/`SetWindowPos`, macOS AX windows and `NSRunningApplication`,
  and X11 EWMH client messages are all handled there; `executor` only depends on
  `perceiver.WindowManager`.
- **`perceiver` covers "perception + window"**: the contract, the readings (elements / text /
  desktop info / windows), and window actions like activate/place; mouse/keyboard/screenshot/
  clipboard stay in `executor`. Specific backends plug in via the capability interfaces above.
- **Eyes and hands separated**: perception and window management go through neru-core
  (accessibility tree + pixels + windows); input goes through robotgo (SendInput / XTest /
  CGEvent). Both paths converge inside `executor`, exposing only a unified coordinate system
  upward.
- **Accessibility first, vision as fallback**: reads the accessibility tree by default; falls
  back to OCR + visual detection automatically when the tree is empty or fails. But if a target
  is explicitly specified (`source=ax`, `scope=app`, `scope=surface`), it errors out on failure
  rather than silently dropping back to the foreground window.

## Coordinate system

All coordinates are **screen-absolute**, with the origin at the top-left of the virtual desktop.
On Windows/Linux they are physical pixels; on macOS both the accessibility coordinates and the
mouse-injection coordinates are system logical points. With multiple monitors, screens to the
left of or above the primary show negative coordinates; the `virtual_desktop` returned by
`inspect_screen_tool` is the complete usable range.

On Windows the process sets per-monitor DPI aware on first perception, so the rectangles returned
by the accessibility tree/windows and the mouse-injection coordinates use the same set of physical
pixels.

## Development & testing

```bash
go build ./...
go vet ./...
go test ./...              # pure-logic unit tests, no desktop needed

# Real-desktop verification: read-only perception by default (no clicks, no input);
# TestLiveSetWindowBounds creates and places a winver window
$env:ROBOPILOT_LIVE=1      # PowerShell
go test ./executor/perceiver/neru -run TestLive -v
```

Window enumeration, activation and placement depend on neru-core's `window` package. When
robopilot-mcp is checked out alongside a sibling `neru-core/` at the repo root, the `go.work`
at the project root makes the build use that local neru-core directly, so you can iterate on the
source without publishing a version first. When building robopilot-mcp standalone, the neru-core
version required by `go.mod` must include the `window` package; otherwise keep this `go.work`, or
point the `go.mod` dependency at a neru-core version that includes it.

## License

MIT.