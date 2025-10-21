package imap

import (
	"crypto/tls"
	"fmt"
	"log"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

// Client IMAP客户端封装
type Client struct {
	server       string
	port         int
	username     string
	password     string
	client       *client.Client
	idleClient   *IdleClient
	accountName  string
	idleTimeout  int
	supportsIDLE bool
}

// MonitorResult 监控结果
type MonitorResult struct {
	UpdateCh <-chan error // 更新通知通道，接收错误或nil（有新邮件）
}

// logWriter 自定义日志写入器，将 go-imap 的错误日志转发到标准日志
type logWriter struct {
	accountName string
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	// 去掉末尾的换行符
	msg := string(p)
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	log.Printf("[%s] %s", w.accountName, msg)
	return len(p), nil
}

// NewClient 创建新的IMAP客户端
func NewClient(server string, port int, username, password, accountName string, idleTimeout int) *Client {
	return &Client{
		server:      server,
		port:        port,
		username:    username,
		password:    password,
		accountName: accountName,
		idleTimeout: idleTimeout,
	}
}

// Connect 连接到IMAP服务器
func (c *Client) Connect() error {
	addr := fmt.Sprintf("%s:%d", c.server, c.port)

	var err error
	// 默认使用TLS连接
	tlsConfig := &tls.Config{
		ServerName: c.server,
	}
	c.client, err = client.DialTLS(addr, tlsConfig)

	if err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}

	// 设置自定义错误日志写入器，使错误日志格式与其他日志一致
	c.client.ErrorLog = log.New(&logWriter{accountName: c.accountName}, "", 0)

	// 创建IDLE客户端并检查支持
	c.idleClient = NewIdleClient(c.client, c.accountName, c.idleTimeout)
	c.supportsIDLE = c.idleClient.CheckIDLESupport()

	return nil
}

// Login 登录到IMAP服务器
func (c *Client) Login() error {
	if err := c.client.Login(c.username, c.password); err != nil {
		return fmt.Errorf("登录失败: %w", err)
	}
	return nil
}

// ListFolders 列出所有可用的邮箱文件夹
func (c *Client) ListFolders() ([]string, error) {
	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)

	go func() {
		done <- c.client.List("", "*", mailboxes)
	}()

	var folders []string
	for m := range mailboxes {
		folders = append(folders, m.Name)
	}

	if err := <-done; err != nil {
		return nil, fmt.Errorf("列出文件夹失败: %w", err)
	}

	return folders, nil
}

// SelectFolder 选择文件夹
func (c *Client) SelectFolder(folder string) (*imap.MailboxStatus, error) {
	mbox, err := c.client.Select(folder, false)
	if err != nil {
		return nil, fmt.Errorf("选择文件夹 %s 失败: %w", folder, err)
	}
	return mbox, nil
}

// FetchMessages 获取邮件
func (c *Client) FetchMessages(folder string, limit uint32, markAsRead bool) ([]*imap.Message, error) {
	mbox, err := c.SelectFolder(folder)
	if err != nil {
		return nil, err
	}

	// 如果邮箱为空，直接返回
	if mbox.Messages == 0 {
		return nil, nil
	}

	// 尝试搜索未读邮件
	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.SeenFlag}

	var seqset *imap.SeqSet
	var useFallback bool

	ids, err := c.client.Search(criteria)
	if err != nil {
		// 某些服务器（如阿里云）不支持 WithoutFlags，改用序列号范围获取
		// 静默处理，稍后会在需要时输出日志
		useFallback = true

		// 获取最新的邮件（按limit或全部）
		seqset = new(imap.SeqSet)
		if limit > 0 && mbox.Messages > limit {
			seqset.AddRange(mbox.Messages-limit+1, mbox.Messages)
		} else {
			seqset.AddRange(1, mbox.Messages)
		}
	} else {
		if len(ids) == 0 {
			return nil, nil
		}

		if limit > 0 && uint32(len(ids)) > limit {
			ids = ids[len(ids)-int(limit):]
		}

		seqset = new(imap.SeqSet)
		for _, id := range ids {
			seqset.AddNum(id)
		}
	}

	// 设置要获取的邮件部分
	items := []imap.FetchItem{
		imap.FetchEnvelope,
		imap.FetchFlags,
		imap.FetchInternalDate,
		imap.FetchRFC822Size,
		imap.FetchUid,
		"BODY.PEEK[]", // 使用PEEK避免自动标记为已读
	}

	if markAsRead {
		items[5] = "BODY[]" // 不使用PEEK，会自动标记为已读
	}

	// 创建消息通道（使用合适的缓冲大小）
	channelSize := len(ids)
	if channelSize == 0 {
		channelSize = int(limit)
		if channelSize == 0 {
			channelSize = 10 // 默认缓冲大小
		}
	}
	messages := make(chan *imap.Message, channelSize)
	done := make(chan error, 1)

	go func() {
		done <- c.client.Fetch(seqset, items, messages)
	}()

	var result []*imap.Message
	totalCount := 0

	for msg := range messages {
		totalCount++
		// 如果使用了备用方法（ids == nil），需要过滤已读邮件
		if ids == nil {
			// 检查是否为未读邮件
			isUnread := true
			for _, flag := range msg.Flags {
				if flag == imap.SeenFlag {
					isUnread = false
					break
				}
			}
			if isUnread {
				result = append(result, msg)
			}
		} else {
			result = append(result, msg)
		}
	}

	if err := <-done; err != nil {
		// 如果备用方法也失败了，才输出错误日志
		if useFallback {
			return nil, fmt.Errorf("获取邮件失败（标准搜索和备用方法均失败）: %w", err)
		}
		return nil, fmt.Errorf("获取邮件失败: %w", err)
	}

	// 如果使用了备用方法，静默处理并应用limit
	if ids == nil {
		// 应用limit限制
		if limit > 0 && uint32(len(result)) > limit {
			result = result[len(result)-int(limit):]
		}
	}

	return result, nil
}

// IdleWithFallback 使用IDLE或轮询监听新邮件
func (c *Client) IdleWithFallback(folder string, pollInterval time.Duration) *MonitorResult {
	updateCh := make(chan error)

	go func() {
		defer close(updateCh)

		// 根据服务器支持情况选择IDLE或轮询
		if c.supportsIDLE && c.idleClient != nil {
			c.idleMode(folder, updateCh)
		} else {
			c.pollMode(folder, pollInterval, updateCh)
		}
	}()

	return &MonitorResult{
		UpdateCh: updateCh,
	}
}

// idleMode IDLE模式监听
func (c *Client) idleMode(folder string, updateCh chan<- error) {
	log.Printf("[%s] 使用 IDLE 模式监控文件夹: %s", c.accountName, folder)

	// 使用IDLE客户端监控
	idleUpdateCh := c.idleClient.MonitorWithIDLE(folder)

	hasUpdate, ok := <-idleUpdateCh
	if !ok {
		// IDLE监控结束（idle.go中已输出详细日志）
		updateCh <- fmt.Errorf("IDLE 已结束")
		return
	}
	if hasUpdate {
		// 有新邮件更新
		log.Printf("[%s] IDLE 收到新邮件通知", c.accountName)
		updateCh <- nil
		return
	}
}

// pollMode 轮询模式
func (c *Client) pollMode(folder string, interval time.Duration, updateCh chan<- error) {
	log.Printf("[%s] 使用轮询模式监控文件夹: %s (间隔: %v)", c.accountName, folder, interval)

	var lastMessageCount uint32

	// 获取初始消息数
	if mbox, err := c.SelectFolder(folder); err == nil {
		lastMessageCount = mbox.Messages
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		mbox, err := c.SelectFolder(folder)
		if err != nil {
			updateCh <- err
			return
		}

		if mbox.Messages != lastMessageCount {
			log.Printf("[%s] 检测到新邮件 (数量: %d → %d)", c.accountName, lastMessageCount, mbox.Messages)
			updateCh <- nil // 通知有更新
			return
		}
	}
}

// Logout 登出并关闭连接
func (c *Client) Logout() error {
	if c.client != nil {
		return c.client.Logout()
	}
	return nil
}

// MarkAsRead 标记邮件为已读
func (c *Client) MarkAsRead(uid uint32) error {
	if c.client == nil {
		return fmt.Errorf("客户端未连接")
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.SeenFlag}

	if err := c.client.UidStore(seqSet, item, flags, nil); err != nil {
		return fmt.Errorf("标记邮件为已读失败: %w", err)
	}

	return nil
}

// IsConnected 检查是否已连接
func (c *Client) IsConnected() bool {
	return c.client != nil && c.client.State() != imap.LogoutState
}
