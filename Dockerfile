# 多阶段构建 - 构建阶段
FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS builder

# 声明构建参数
ARG TARGETOS
ARG TARGETARCH

# 安装必要的构建工具
RUN apk add --no-cache git ca-certificates tzdata

# 设置工作目录
WORKDIR /build

# 复制 go.mod 和 go.sum 并下载依赖（利用Docker缓存）
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 交叉编译程序（静态编译，禁用 CGO）
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -a -installsuffix cgo -ldflags="-w -s" -o mail-receiver .

# 第二阶段：运行阶段
FROM alpine:latest

# 安装必要的运行时依赖
RUN apk add --no-cache ca-certificates tzdata

# 设置时区为上海
ENV TZ=Asia/Shanghai

# 创建非特权用户
RUN addgroup -g 1000 mailapp && \
    adduser -D -u 1000 -G mailapp mailapp

# 设置工作目录
WORKDIR /app

# 从构建阶段复制编译好的程序
COPY --from=builder /build/mail-receiver .

# 修改程序权限（让所有用户可执行）
RUN chown -R mailapp:mailapp /app

# 切换到非特权用户
USER mailapp

# 启动程序
CMD ["./mail-receiver"] 