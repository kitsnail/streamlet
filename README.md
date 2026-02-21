# Streamlet

> 极简个人视频库，支持登录认证和视频流播放

## 功能特性

- 🔐 JWT 认证登录
- 📁 自动扫描 MP4 视频文件
- 🎬 支持 Range 请求（视频拖动）
- 🔍 前端搜索过滤
- 🎨 极简深色主题 UI

## 快速开始

### 编译

```bash
go build -o streamlet
```

### 运行

```bash
# 设置环境变量
export VIDEO_DIR=/path/to/your/videos
export AUTH_USER=admin
export AUTH_PASS=your-password
export JWT_SECRET=your-secret-key
export PORT=8080

# 运行
./streamlet
```

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `VIDEO_DIR` | 视频目录路径 | `./videos` |
| `AUTH_USER` | 登录用户名 | `admin` |
| `AUTH_PASS` | 登录密码 | `admin123` |
| `JWT_SECRET` | JWT 密钥 | `streamlet-secret-change-me` |
| `PORT` | 服务端口 | `8080` |
| `ENV` | 环境 | `development` |

## 技术栈

- **后端**: Go + Gin
- **前端**: HTML + Tailwind CSS
- **认证**: JWT

## 项目结构

```
streamlet/
├── main.go              # 入口文件
├── config/
│   └── config.go        # 配置管理
├── handlers/
│   ├── auth.go          # 认证处理
│   └── video.go         # 视频处理
├── static/
│   ├── login.html       # 登录页
│   ├── index.html       # 视频列表页
│   └── player.html      # 播放页
└── README.md
```

## License

MIT
