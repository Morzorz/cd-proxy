# 让 Claude Desktop 也能用上 DeepSeek：cd-proxy 介绍

Claude Desktop 提供了开发者模式，允许用户配置第三方 API 后端——这本该是好事。但有一个让人头疼的限制：不管你配的是哪个后端，**模型名称必须使用 Anthropic 官方模型名**（比如 `claude-sonnet-4-20250514`）。而第三方后端的模型名往往是另一套命名（比如 DeepSeek 的是 `deepseek-v4-pro`），两者对不上，请求直接报错。

![Claude Desktop 模型名限制报错](images/claude_err.png)

于是就出现了一个尴尬的局面：手里有第三方的 API Key，Claude Desktop 也允许你填 API 地址，但模型名对不上，客户端就是不认。

cd-proxy 就是为这个问题而生的。

## 它做了什么

cd-proxy 是一个本地反向代理，跑在你的本机上。你把客户端的 Base URL 指向它，它负责把 `/v1/messages` 请求转发给你配置的任意上游——可以是 DeepSeek、可以在其他 Anthropic 兼容后端，也可以是 Anthropic 官方 API 本身。

一句话概括：**一条本地代理，打通所有 Anthropic 协议的后端。**

它不是给生产环境高并发用的，也不是一个通用网关。它的定位很明确：让桌面端工具（比如 Claude Desktop）能透明地使用多种后端，同时提供可视化管理，避免改配置文件 + 重启的折腾。

## 核心能力

**灵活的模型路由。** 你可以为每个模型指定不同的上游。比如 `claude-sonnet-4-20250514` 走 DeepSeek，`claude-opus-4-20250514` 走另一个提供商，没匹配到的模型走默认上游。模型名称映射也是自动的——客户端说 "我要 sonnet"，代理帮你转成上游实际认识的模型名。

**配置热加载。** 改完配置不用重启服务。cd-proxy 用 atomic pointer 管理运行时状态，配置文件变更后下一次请求自动生效。写配置文件时用原子写入（先写 `.tmp` 再 rename），基本不会出现读到半截配置的情况。

**内置 Web 管理面板。** 一个嵌入在二进制里的单页应用，可以查看运行状态、管理模型映射、修改默认上游、调整监听端口和超时时间、实时查看请求日志。配置还支持 YAML 导入导出，方便备份和迁移。

![cd-proxy Web 管理面板](images/企业微信截图_6db3e3c6-e2be-4b0d-99de-3ca6e67bb2e1.png)

**跨平台 + 多形态。** macOS 上有菜单栏图标（systray），可以一键打开面板或退出；Windows 和 Linux 上自动开浏览器。也可以用 `--headless` 模式纯命令行跑，适合服务器或 CI 环境。三种平台都可以编译成单个二进制文件，没有运行时依赖。

**API Key 安全处理。** 管理面板展示配置时会自动遮蔽 Key（只留前 4 位和后 5 位），更新配置时如果前端传来的是遮蔽后的值，后端会保留现有 Key 而不是用星号覆盖。

**SSE 实时日志。** 内置环形缓冲区（最多 2000 条）加上发布/订阅机制，Web 面板通过 SSE 拿到历史快照后再持续接收增量日志，排查问题时比翻文件方便不少。

## 技术栈

Go 1.24，依赖极简——yaml 解析用了 `gopkg.in/yaml.v3`，macOS 菜单栏用了 `getlantern/systray`。web 前端是纯 HTML/CSS/JS，用 `//go:embed` 嵌进二进制，没有 Node 工具链，没有 CDN 依赖。

整体结构也很直白：两个 HTTP server（代理服务 + 管理后台），一份热加载的代理状态，一个环形日志缓冲。代码量不大，十几源个文件，每个职责清晰，读起来不费劲。

## 适用场景与局限

**适合的场景：**
- 用 Claude Desktop 但想走第三方模型后端
- 开发 Anthropic API 应用时需要在不同后端之间切换测试
- 个人或小团队需要一个小巧的本地代理，不想搭复杂网关

**不太适合的场景：**
- 生产环境的高并发代理——它是本地单实例设计，没有横向扩展、限流、熔断这些能力
- 需要鉴权或租户隔离的多用户场景——代理本身不做认证
- 协议转换需求——它只转发 Anthropic 协议的请求，不支持 OpenAI 等其他 API 格式

这些不是缺陷，是取舍。cd-proxy 选择了简单、透明、零运维成本，代价是放弃了大规模网关的功能清单。

## 快速上手

```bash
git clone https://github.com/your-repo/cd-proxy.git
cd cd-proxy

# 复制一份配置，填上你的 API Key
cp config.example.yaml config.yaml

# 编译运行
go build -o cd-proxy .
./cd-proxy

# 或者直接跑（go run 也行）
go run . --config config.yaml
```

然后把 Claude Desktop 的 Anthropic Base URL 改成 `http://localhost:48271`（具体改法因客户端而异），打开 `http://localhost:59183` 就能看到管理面板了。

![Claude Desktop 开发者模式配置](images/企业微信截图_02da6e4e-fc60-446d-b94b-582eb0b2128a.png)

## 结语

cd-proxy 做了一件很具体的事——在 Anthropic 协议的客户端和多样的后端之间架一座本地桥梁。它不企图成为一个平台，也不想替代任何网关产品。它就是一段不到两千行的 Go 代码，帮你省掉切换模型时改配置、重启、踩坑的重复劳动。

如果你正在用 Claude Desktop 又恰好有第三方模型的 API Key，不妨试试。如果它有帮到你，欢迎提 issue 反馈；如果你想补上它目前缺的功能，也欢迎提 PR。
