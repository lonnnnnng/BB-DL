package bbdown

import "sync"

// Config 保存运行期的全局认证与解析参数。
// 这里保持和原版同样的“进程级状态”模型，方便后续对齐命令行行为和请求头拼装。
type Config struct {
	mu sync.RWMutex

	Cookie  string
	Token   string
	Debug   bool
	Host    string
	EpHost  string
	TvHost  string
	Area    string
	WBISalt string
}

func NewConfig() *Config {
	return &Config{
		Host:   "api.bilibili.com",
		EpHost: "api.bilibili.com",
		TvHost: "api.snm0516.aisee.tv",
	}
}

