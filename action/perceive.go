package action

// 感知类动作：把屏幕上的"可交互元素"和"文字"读成结构化数据。
//
// 两个动作分工明确，都是只读的（不改动桌面状态）：
//   - InspectScreen：唯一的"看"入口，一次返回桌面几何 + 前台窗口 + 可交互元素，
//     可选 OCR 文字与截图；通过 source/strategy 覆盖纯像素视觉检测；
//   - ReadText：只要文字时用，按区域 OCR，可返回带坐标的词块。

// InspectScreen 观察屏幕：桌面状态 + 可交互元素（含中心点）+ 可选文字/截图。
//
// 元素来源默认"无障碍树优先、像素视觉兜底"；显式指定 Source 或 Strategy 时按指定方式取。
// 这就是 agent "先看一眼"的那一次调用，替代了原来分散的屏幕信息、元素感知、
// 视觉检测、综合快照四个工具。
type InspectScreen struct {
	Scan

	// Max 单次最多返回的元素数量，0 表示用感知器默认上限。
	Max int `json:"max,omitempty" jsonschema:"最多返回的元素数量,传0时用感知器自身默认上限(通常64)"`
	// MinArea 过滤面积过小的元素（误报/装饰性小点）。
	MinArea int `json:"min_area,omitempty" jsonschema:"过滤面积(像素平方)小于该值的元素,传0表示不过滤"`
	// MaxDepth 限制无障碍树遍历深度，0 表示用感知器默认值。
	MaxDepth int `json:"max_depth,omitempty" jsonschema:"限制无障碍树遍历深度,传0用默认深度;窗口层级很浅时调小可显著加快扫描"`
	// Roles 只保留的 role：可写中立 role(如 button/text_field)，
	// 也可写带平台前缀的原生 role(如 uia:Button / ax:AXButton / atspi:push button)。
	Roles []string `json:"roles,omitempty" jsonschema:"只保留的 role 列表,可写中立 role 如 button/text_field,也可写带平台前缀的原生 role 如 uia:Button;空表示不过滤"`
	// Query 文本关键字过滤：在元素名称/描述/取值里做忽略大小写的子串匹配。
	Query string `json:"query,omitempty" jsonschema:"按文本过滤元素:在名称/描述/取值里做忽略大小写的子串匹配,空表示不过滤"`
	// Region 只保留中心点落在该区域内的元素，也用作 OCR 与截图的默认区域。
	Region *Region `json:"region,omitempty" jsonschema:"只保留中心点在该矩形区域内的元素;同时作为 text 与 screenshot 的默认区域,省略表示全屏"`
	// Source 感知来源，省略表示先无障碍树、失败再回退视觉。
	Source ElementSource `json:"source,omitempty" jsonschema:"感知来源,取值 ax(仅无障碍树)/vision(仅像素视觉)/省略(先无障碍树,失败或为空时回退视觉)"`
	// Strategy 纯像素视觉的检测策略，仅在回退视觉或 Source 为 vision 时生效。
	Strategy string `json:"strategy,omitempty" jsonschema:"像素视觉检测策略,取值 vision(OCR加矩形,默认)/contour(轮廓找按钮)/segment(分割找控件)"`
	// IncludeText 是否同时 OCR 读取屏上文字。
	IncludeText bool `json:"include_text,omitempty" jsonschema:"是否同时 OCR 读取屏上文字,默认false(较慢且依赖系统语言包)"`
	// Screenshot 是否同时截屏落盘，返回图片路径供后续人工或视觉复核。
	Screenshot bool `json:"screenshot,omitempty" jsonschema:"是否同时截屏落盘并返回图片路径,默认false"`

	Artifact
}

func (*InspectScreen) isAction() {}

// ReadText 用 OCR 读取屏幕（或指定区域）上的文字。
//
// 默认返回整段文字；Words 为真时改为返回每个词/行的文本与坐标，
// 便于"按文字位置点击"或在图片界面上做文字定位。
type ReadText struct {
	// Region 只读取该矩形区域，省略表示活动显示器整屏。
	Region *Region `json:"region,omitempty" jsonschema:"只读取该矩形区域,省略表示活动显示器整屏"`
	// Languages 期望语言（BCP 47 或引擎专有名），省略用系统语言配置。
	Languages []string `json:"languages,omitempty" jsonschema:"期望的识别语言,如 en-US / zh-Hans,省略用系统语言配置"`
	// WordLevel 是否按词而非按行切分识别结果。
	WordLevel bool `json:"word_level,omitempty" jsonschema:"是否按词而非按行切分识别结果,找单个按钮文字时建议开启"`
	// MinConfidence 置信度下限，低于该值的识别结果被丢弃。
	MinConfidence float64 `json:"min_confidence,omitempty" jsonschema:"置信度下限(0到1),低于该值的识别结果被丢弃,0 表示不过滤"`
	// Words 为真时返回带坐标的词块列表，否则只返回拼接后的整段文本。
	Words bool `json:"words,omitempty" jsonschema:"为真时返回带坐标的词块列表(可用于按文字位置点击),否则只返回拼接后的整段文本"`
	// Max 词块模式下的最大返回条数。
	Max int `json:"max,omitempty" jsonschema:"词块模式下的最大返回条数,传0用默认上限(200)"`
}

func (*ReadText) isAction() {}
