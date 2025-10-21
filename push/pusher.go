package push

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Pusher 推送器
type Pusher struct {
	url         string
	accountName string
	client      *http.Client
}

// NewPusher 创建新的推送器
func NewPusher(url string, accountName string) *Pusher {
	return &Pusher{
		url:         url,
		accountName: accountName,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Push 推送邮件信息
func (p *Pusher) Push(title, msg string) (bool, error) {
	if p.url == "" {
		return false, nil
	}

	// 构建表单数据
	formData := url.Values{}
	formData.Set("title", title)
	formData.Set("msg", msg)

	// 发送POST请求（表单格式）
	resp, err := p.client.PostForm(p.url, formData)
	if err != nil {
		return false, fmt.Errorf("推送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode == 200 {
		return true, nil
	}

	return false, nil
}

// BuildMessageContent 构建推送消息内容
func BuildMessageContent(body, receiveTime, from string, to []string, hasAttachments bool) string {
	var content bytes.Buffer

	content.WriteString(body)

	// 如果有附件，在正文后追加提示
	if hasAttachments {
		content.WriteString("\n\n该邮件含有附件，请手动查看")
	}

	content.WriteString("\n\n")
	content.WriteString("----------------------------\n")
	content.WriteString(fmt.Sprintf("收件时间: %s\n", receiveTime))
	content.WriteString(fmt.Sprintf("发件人: %s\n", from))

	if len(to) > 0 {
		content.WriteString(fmt.Sprintf("收件人: %s\n", to[0]))
		for i := 1; i < len(to); i++ {
			content.WriteString(fmt.Sprintf("        %s\n", to[i]))
		}
	}

	return content.String()
}
