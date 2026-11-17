package types

// ClientConfigView 是客户端启动配置的响应。
type ClientConfigView struct {
	ConfigVersion int               `json:"configVersion"`
	Features      map[string]bool   `json:"features"`
	IM            ClientConfigIM    `json:"im"`
	Limits        map[string]int64  `json:"limits"`
	MinAppVersion map[string]string `json:"minAppVersion"`
}

// ClientConfigIM 是配置中与 IM 连接相关的运行期信息。
type ClientConfigIM struct {
	APIBaseURL string `json:"apiBaseUrl"`
	WSURL      string `json:"wsUrl"`
	PlatformID int8   `json:"platformId"`
}

// ClientConfigSchemaView 是配置项元数据列表。
type ClientConfigSchemaView struct {
	Items []ClientConfigItemSchema `json:"items"`
}

// ClientConfigItemSchema 描述单个配置项的类型、默认值与适用范围。
type ClientConfigItemSchema struct {
	Key     string `json:"key"`
	Type    string `json:"type"`
	Default any    `json:"default"`
	Scope   string `json:"scope"`
}
