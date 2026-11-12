package action

// 剪贴板类动作：一个动作同时负责读与写，用是否提供文本区分。

// Clipboard 读写剪贴板文本：Text 为空时读取当前剪贴板，非空时写入。
//
// 把读写合成一个动作，是为了避免模型在"获取内容/写入内容"两个工具之间做选择：
// 只需要记住"要不要给它一段文本"。
type Clipboard struct {
	Text string `json:"text,omitempty" jsonschema:"要写入剪贴板的文本;省略或为空时改为读取当前剪贴板文本"`
}

func (*Clipboard) isAction() {}
