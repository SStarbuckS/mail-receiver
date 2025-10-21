package receiver

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"mail-receiver/config"
	"mail-receiver/heartbeat"
	"mail-receiver/imap"
	"mail-receiver/push"
)

// Receiver 邮件接收器
type Receiver struct {
	config    *config.Config
	accounts  map[string]*AccountReceiver
	heartbeat *heartbeat.Heartbeat
	wg        sync.WaitGroup
}

var (
	// HTML标签正则表达式
	htmlTagRegex = regexp.MustCompile(`<[^>]*>`)
	// HTML实体映射
	htmlEntities = map[string]string{
		"&nbsp;":   " ",
		"&lt;":     "<",
		"&gt;":     ">",
		"&amp;":    "&",
		"&quot;":   "\"",
		"&apos;":   "'",
		"&hellip;": "...",
		"&mdash;":  "—",
		"&ndash;":  "–",
	}
)

// stripHTML 去除HTML标签并清理文本
func stripHTML(html string) string {
	if html == "" {
		return ""
	}

	// 去除 <style> 标签及其内容
	text := regexp.MustCompile(`(?i)<style[^>]*>[\s\S]*?</style>`).ReplaceAllString(html, "")

	// 去除 <script> 标签及其内容
	text = regexp.MustCompile(`(?i)<script[^>]*>[\s\S]*?</script>`).ReplaceAllString(text, "")

	// 将块级标签替换为换行（<p> <div> <br> 等）
	text = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>|</tr>|</li>`).ReplaceAllString(text, "\n")

	// 去除所有HTML标签
	text = htmlTagRegex.ReplaceAllString(text, "")

	// 解码HTML实体
	for entity, replacement := range htmlEntities {
		text = strings.ReplaceAll(text, entity, replacement)
	}

	// 去除每行首尾空白并过滤空行
	lines := strings.Split(text, "\n")
	var cleanedLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanedLines = append(cleanedLines, line)
		}
	}
	text = strings.Join(cleanedLines, "\n")

	// 去除首尾空白
	text = strings.TrimSpace(text)

	return text
}

// AccountReceiver 单个账号的接收器
type AccountReceiver struct {
	name         string
	config       *config.AccountConfig
	client       *imap.Client
	retries      int
	maxRetries   int
	retryDelay   time.Duration
	pusher       *push.Pusher
	firstConnect bool // 是否是首次连接
}

// NewReceiver 创建新的接收器
func NewReceiver(cfg *config.Config) *Receiver {
	return &Receiver{
		config:    cfg,
		accounts:  make(map[string]*AccountReceiver),
		heartbeat: heartbeat.New(cfg.App.HeartbeatURL, cfg.App.HeartbeatInterval, "system"),
	}
}

// Start 启动接收器
func (r *Receiver) Start() error {
	// 遍历所有账号配置
	for name, accCfg := range r.config.Accounts {
		log.Printf("[%s] 启动邮件监控", name)

		accReceiver := &AccountReceiver{
			name:         name,
			config:       accCfg,
			client:       imap.NewClient(accCfg.Server, accCfg.Port, accCfg.Username, accCfg.Password, name, accCfg.IdleTimeout),
			maxRetries:   3,                // 最多重试3次
			retryDelay:   30 * time.Second, // 重试间隔30秒
			pusher:       push.NewPusher(accCfg.SendPush, name),
			firstConnect: true, // 首次连接标志
		}

		r.accounts[name] = accReceiver

		r.wg.Add(1)
		go r.runAccountReceiver(accReceiver)
	}

	if len(r.accounts) == 0 {
		return fmt.Errorf("没有找到任何账号配置")
	}

	return nil
}

// StartHeartbeat 启动全局心跳检测
func (r *Receiver) StartHeartbeat() {
	r.heartbeat.Start()
}

// runAccountReceiver 运行单个账号的接收器
func (r *Receiver) runAccountReceiver(ar *AccountReceiver) {
	defer r.wg.Done()

	for {
		if err := ar.run(); err != nil {
			ar.handleError(err)
		}
	}
}

// run 运行账号接收器的主逻辑
func (ar *AccountReceiver) run() error {
	// 连接并登录IMAP服务器
	if err := ar.client.Connect(); err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}
	defer ar.client.Logout()

	if err := ar.client.Login(); err != nil {
		return fmt.Errorf("登录失败: %w", err)
	}
	log.Printf("[%s] 登录成功", ar.name)

	// 登录成功，重置重试计数器
	ar.retries = 0

	// 首次连接时列出所有可用的文件夹
	if ar.firstConnect {
		ar.firstConnect = false
		if folders, err := ar.client.ListFolders(); err == nil {
			log.Printf("[%s] 可用文件夹列表:", ar.name)
			for _, folder := range folders {
				log.Printf("[%s]   - %s", ar.name, folder)
			}
		} else {
			log.Printf("[%s] 获取文件夹列表失败: %v", ar.name, err)
		}
	}

	// 获取要监控的文件夹（只使用第一个）
	if len(ar.config.Folders) == 0 {
		return fmt.Errorf("未配置监控文件夹")
	}
	folder := ar.config.Folders[0]

	// 首先处理现有邮件
	ar.fetchAndProcessMessages(folder)

	// 开始监控新邮件
	pollInterval := time.Duration(ar.config.PollInterval) * time.Second
	monitor := ar.client.IdleWithFallback(folder, pollInterval)

	// 等待监控结果
	err := <-monitor.UpdateCh
	if err != nil {
		return err
	}

	// 有新邮件，处理后重新连接
	ar.fetchAndProcessMessages(folder)
	return nil
}

// fetchAndProcessMessages 获取并处理邮件
func (ar *AccountReceiver) fetchAndProcessMessages(folder string) {
	messages, err := ar.client.FetchMessages(
		folder,
		50,    // 每次最多获取50封
		false, // 不自动标记已读（推送成功后会手动标记）
	)

	if err != nil {
		log.Printf("[%s] 获取邮件失败: %v", ar.name, err)
		return
	}

	if len(messages) == 0 {
		return
	}

	log.Printf("[%s] 收到 %d 封新邮件", ar.name, len(messages))

	// 处理每条消息
	for _, msg := range messages {
		email, err := imap.ParseMessage(msg, ar.name)
		if err != nil {
			log.Printf("[%s] 解析邮件失败: %v", ar.name, err)
			continue
		}

		// 推送邮件信息
		if ar.pusher != nil {
			// 获取邮件正文（优先使用纯文本，否则清理HTML后使用）
			body := email.Body
			if body == "" && email.HTMLBody != "" {
				// 清理HTML标签
				body = stripHTML(email.HTMLBody)
			}

			// 构建推送消息内容
			from := ""
			if len(email.From) > 0 {
				from = email.From[0]
			}
			receiveTime := email.Date.Format("2006-01-02 15:04:05")

			msgContent := push.BuildMessageContent(body, receiveTime, from, email.To, email.HasAttachments)

			// 发送推送
			success, err := ar.pusher.Push(email.Subject, msgContent)
			if err != nil {
				log.Printf("[%s] 推送失败: %v", ar.name, err)
			} else if success {
				// 推送成功，标记邮件为已读
				ar.client.MarkAsRead(email.UID)
				log.Printf("[%s] 已推送: %s", ar.name, email.Subject)
			}
		}

		// 这里可以添加更多的处理逻辑，如：
		// - 保存到数据库
		// - 转发到其他服务
		// - 触发webhook
		// - 保存附件到本地
	}
}

// handleError 处理错误和重试
func (ar *AccountReceiver) handleError(err error) {
	ar.retries++

	if ar.retries >= ar.maxRetries {
		log.Printf("[%s] 已达到最大重试次数 (%d)，程序退出", ar.name, ar.maxRetries)

		// 发送告警推送
		if ar.pusher != nil {
			title := "请检查 Mail 服务"
			msg := fmt.Sprintf("账号 [%s] 已达最大重试次数 (%d)，程序已退出\n最后错误: %v",
				ar.name, ar.maxRetries, err)
			ar.pusher.Push(title, msg) // Push 方法会阻塞直到完成或超时
		}

		os.Exit(1)
	}

	log.Printf("[%s] %v, 将在 %v 后重试 (第 %d/%d 次尝试)",
		ar.name, err, ar.retryDelay, ar.retries, ar.maxRetries)

	time.Sleep(ar.retryDelay)
}
