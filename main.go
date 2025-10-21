package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"mail-receiver/config"
	"mail-receiver/receiver"
)

func main() {
	// 初始化日志
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 加载配置
	cfg, err := config.LoadConfig("config.json")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	log.Printf("正在启动邮件接收器")

	// 创建接收器
	recv := receiver.NewReceiver(cfg)

	// 启动接收器
	if err := recv.Start(); err != nil {
		log.Fatalf("启动接收器失败: %v", err)
	}

	// 启动心跳检测
	recv.StartHeartbeat()

	// 设置信号处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// 等待退出信号
	sig := <-sigCh
	log.Printf("收到信号: %v，立即退出", sig)
	os.Exit(0)
}
