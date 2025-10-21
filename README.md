# Mail Receiver

一个轻量级的 Go IMAP 邮件接收器，支持 IDLE 实时推送和多账号监控。

## 功能特性

- 支持多邮箱账号并发监控
- IDLE 实时推送（自动降级到轮询）
- 自动重连和错误重试
- 邮件推送到 Webhook
- 全局心跳检测

## 安装

```bash
# 克隆项目
git clone https://github.com/SStarbuckS/mail-receiver.git
cd mail-receiver

# 安装依赖
go mod tidy

# 编译
go build -o mail-receiver
```

## 配置

创建 `config.json` 文件：

```json
{
    "app": {
        "heartbeat_url": "https://your-heartbeat-url",
        "heartbeat_interval": 30
    },
    "accounts": {
        "my-account1": {
            "server": "imap.example.com",
            "port": 993,
            "username": "your-email@example.com",
            "password": "your-password",
            "pollinterval": 60,
            "sendpush": "https://your-webhook-url",
            "folders": ["INBOX"],
            "idletimeout": 20
        },
        "my-account2": {
            "server": "imap.example.com",
            "port": 993,
            "username": "your-email@example.com",
            "password": "your-password",
            "pollinterval": 60,
            "sendpush": "https://your-webhook-url",
            "folders": ["INBOX"],
            "idletimeout": 20
        }
    }
}
```

### 配置说明

**账号配置** (`accounts`)：
- `server`: IMAP 服务器地址
- `port`: IMAP 端口（默认 993）
- `username`: 邮箱账号
- `password`: 邮箱密码或授权码
- `pollinterval`: 轮询间隔（秒，默认 60）
- `sendpush`: 推送 Webhook URL（可选）
- `folders`: 监控的文件夹（默认 ["INBOX"]）
- `idletimeout`: IDLE 超时时间（分钟，默认 20）

**应用配置** (`app`)：
- `heartbeat_url`: 心跳检测 URL（可选，留空不启用）
- `heartbeat_interval`: 心跳间隔（秒，默认 60）

### 常见邮箱配置

| 邮箱 | 服务器 | 端口 | 说明 |
|------|--------|------|------|
| Gmail | imap.gmail.com | 993 | 需要应用专用密码 |
| Outlook | outlook.office365.com | 993 | - |
| QQ邮箱 | imap.qq.com | 993 | 使用授权码 |
| 163邮箱 | imap.163.com | 993 | 使用授权码 |
| Yandex | imap.yandex.com | 993 | - |
| 阿里企业邮 | imap.qiye.aliyun.com | 993 | - |

## 运行

```bash
# 直接运行（需要在配置文件所在目录）
./mail-receiver
```

## Docker 部署

```bash

# 运行容器
docker run -d \
  --name mail-receiver \
  -v /path/to/config.json:/app/config.json \
  sstarbucks/mail-receiver:latest
```

或使用 `docker-compose.yml`文件

## 工作原理

1. 程序启动后为每个账号创建独立的监控协程
2. 登录 IMAP 服务器并列出可用文件夹
3. 优先使用 IDLE 模式实时监听，不支持时自动降级到轮询模式
4. 收到新邮件后解析内容并推送到配置的 Webhook
5. 推送成功后标记邮件为已读

## 注意事项

1. 部分邮箱需要生成应用专用密码或授权码
2. 确保邮箱已开启 IMAP 服务
3. IDLE 模式会保持长连接，超时后自动重连
4. 推送失败的邮件不会标记为已读

