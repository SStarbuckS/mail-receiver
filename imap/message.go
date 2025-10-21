package imap

import (
	"fmt"
	"io"
	"log"
	"mime"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-message/mail"
)

// EmailMessage 邮件消息结构
type EmailMessage struct {
	UID            uint32
	SeqNum         uint32
	Subject        string
	From           []string
	To             []string
	CC             []string
	Date           time.Time
	Size           uint32
	Flags          []string
	Body           string
	HTMLBody       string
	HasAttachments bool // 是否含有附件
}

// ParseMessage 解析IMAP消息
func ParseMessage(msg *imap.Message, accountName string) (*EmailMessage, error) {
	if msg == nil {
		return nil, fmt.Errorf("消息为空")
	}

	email := &EmailMessage{
		UID:    msg.Uid,
		SeqNum: msg.SeqNum,
		Size:   msg.Size,
		Flags:  msg.Flags,
	}

	// 解析信封信息
	if msg.Envelope != nil {
		email.Subject = msg.Envelope.Subject
		email.Date = msg.Envelope.Date

		// 解析发件人
		for _, addr := range msg.Envelope.From {
			email.From = append(email.From, formatAddress(addr))
		}

		// 解析收件人
		for _, addr := range msg.Envelope.To {
			email.To = append(email.To, formatAddress(addr))
		}

		// 解析抄送
		for _, addr := range msg.Envelope.Cc {
			email.CC = append(email.CC, formatAddress(addr))
		}
	}

	// 解析邮件正文
	for _, literal := range msg.Body {
		if literal != nil {
			if err := parseBody(literal, email, accountName); err != nil {
				log.Printf("[%s] 解析邮件正文失败: %v", accountName, err)
			}
		}
	}

	return email, nil
}

// parseBody 解析邮件正文
func parseBody(r io.Reader, email *EmailMessage, accountName string) error {
	// 创建邮件阅读器
	mr, err := mail.CreateReader(r)
	if err != nil {
		return fmt.Errorf("创建邮件读取器失败: %w", err)
	}
	defer mr.Close()

	// 解析邮件头
	header := mr.Header
	if subject, err := header.Subject(); err == nil && email.Subject == "" {
		email.Subject = subject
	}
	if date, err := header.Date(); err == nil && email.Date.IsZero() {
		email.Date = date
	}

	// 遍历邮件各部分
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取邮件部分失败: %w", err)
		}

		switch h := part.Header.(type) {
		case *mail.InlineHeader:
			// 处理内联内容（正文）
			contentType, _, _ := h.ContentType()
			body, err := io.ReadAll(part.Body)
			if err != nil {
				log.Printf("[%s] 读取邮件正文失败: %v", accountName, err)
				continue
			}

			switch {
			case strings.HasPrefix(contentType, "text/plain"):
				email.Body = string(body)
			case strings.HasPrefix(contentType, "text/html"):
				email.HTMLBody = string(body)
			}

		case *mail.AttachmentHeader:
			// 标记邮件含有附件
			email.HasAttachments = true
			// 跳过附件内容
			io.Copy(io.Discard, part.Body)
		}
	}

	return nil
}

// formatAddress 格式化邮件地址
func formatAddress(addr *imap.Address) string {
	if addr == nil {
		return ""
	}

	// 构建邮箱地址
	email := fmt.Sprintf("%s@%s", addr.MailboxName, addr.HostName)

	// 如果有名字，显示为：名字 (邮箱) - 避免被当作HTML标签过滤
	if addr.PersonalName != "" {
		decoded, err := decodeRFC2047(addr.PersonalName)
		if err == nil && decoded != "" {
			return fmt.Sprintf("%s (%s)", decoded, email)
		}
		return fmt.Sprintf("%s (%s)", addr.PersonalName, email)
	}

	// 没有名字，只显示邮箱
	return email
}

// decodeRFC2047 解码RFC2047编码的字符串（用于处理中文等非ASCII字符）
func decodeRFC2047(s string) (string, error) {
	dec := new(mime.WordDecoder)
	decoded, err := dec.DecodeHeader(s)
	if err != nil {
		return s, err
	}
	return decoded, nil
}
