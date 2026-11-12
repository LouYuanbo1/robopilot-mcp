package executor

import (
	"math/rand/v2"
	"time"
)

// humanDelay 在 [min,max] 毫秒之间随机休眠，模拟人类操作间隔。
func humanDelay(minMs, maxMs int) {
	if maxMs <= minMs {
		time.Sleep(time.Duration(minMs) * time.Millisecond)
		return
	}
	ms := rand.IntN(maxMs-minMs) + minMs
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

// pause 是动作间的拟人停顿：只有显式开启 humanize 时才会真正等待。
// 关闭拟人时完全不睡，保证脚本化场景的最快响应。
func (e *Executor) pause() {
	if !e.humanize {
		return
	}
	humanDelay(e.minDelayMS, e.maxDelayMS)
}
