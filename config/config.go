package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config 应用配置
type Config struct {
	Accounts map[string]*AccountConfig `json:"accounts"`
	App      AppConfig                 `json:"app"`
}

// AccountConfig 邮箱账号配置
type AccountConfig struct {
	Server       string   `json:"server"`
	Port         int      `json:"port"`
	Username     string   `json:"username"`
	Password     string   `json:"password"`
	PollInterval int      `json:"pollinterval"`
	SendPush     string   `json:"sendpush"`
	Folders      []string `json:"folders"`
	IdleTimeout  int      `json:"idletimeout"`
}

// AppConfig 应用级配置
type AppConfig struct {
	HeartbeatURL      string `json:"heartbeat_url"`
	HeartbeatInterval int    `json:"heartbeat_interval"`
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开配置文件失败: %w", err)
	}
	defer file.Close()

	var config Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 设置默认值
	for name, acc := range config.Accounts {
		if acc.Port == 0 {
			acc.Port = 993
		}
		if acc.PollInterval == 0 {
			acc.PollInterval = 60
		}
		if acc.IdleTimeout == 0 {
			acc.IdleTimeout = 20
		}
		if len(acc.Folders) == 0 {
			acc.Folders = []string{"INBOX"}
		}
		// 验证必填字段
		if acc.Server == "" || acc.Username == "" || acc.Password == "" {
			return nil, fmt.Errorf("账号 %s 缺少必填字段 (server/username/password)", name)
		}
	}

	// 设置心跳默认值
	if config.App.HeartbeatInterval == 0 {
		config.App.HeartbeatInterval = 60
	}

	return &config, nil
}
