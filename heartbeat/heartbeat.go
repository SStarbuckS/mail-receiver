package heartbeat

import (
	"io"
	"log"
	"net/http"
	"time"
)

// Heartbeat 心跳检测器
type Heartbeat struct {
	url         string
	interval    time.Duration
	accountName string
	client      *http.Client
}

// New 创建心跳检测器
func New(url string, intervalSeconds int, accountName string) *Heartbeat {
	return &Heartbeat{
		url:         url,
		interval:    time.Duration(intervalSeconds) * time.Second,
		accountName: accountName,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Start 启动心跳检测
func (h *Heartbeat) Start() {
	if h.url == "" {
		return
	}

	log.Printf("[%s] 启动心跳检测 (间隔: %v, URL: %s)", h.accountName, h.interval, h.url)

	go func() {
		// 立即发送一次心跳
		h.sendHeartbeat()

		for {
			// 等待间隔后发送下一次心跳
			time.Sleep(h.interval)
			h.sendHeartbeat()
		}
	}()
}

// sendHeartbeat 发送心跳请求
func (h *Heartbeat) sendHeartbeat() {
	resp, err := h.client.Get(h.url)
	if err != nil {
		log.Printf("[%s] 心跳请求失败: %v", h.accountName, err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[%s] 心跳响应 [%d]: %s", h.accountName, resp.StatusCode, string(body))
}
