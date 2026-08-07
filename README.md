# agw

一个按配置顺序尝试上游的 Go HTTP reverse proxy。客户端不需要携带认证信息，代理会根据每个上游的 `authorization` 配置注入 `Authorization` header。

访问 `/` 可以打开基于 HTMX 的配置可视化页面；`/config` 返回可局部刷新的配置表格。认证值默认脱敏，点击“显示”后才在当前页面显示。

## 运行

```bash
go mod tidy
go run ./cmd/agw
```

`-config` 默认为当前目录的 `config.yaml`。监听端口优先读取环境变量 `PORT`，未设置时默认为 `:8080`；也可以用 `-listen` 覆盖。`-timeout` 默认是 `0`，不会设置请求总超时，适合长时间的 SSE 流；需要限制时可以显式设置，例如 `-timeout 2m`。
使用 `-debug` 可以在启动时开启 request header 日志；页面上的 `debug: true` toggle 可以在运行中切换并保存，无需重启。

配置可以是旧版 upstream 数组，也可以是带 `appSelectors` 的对象。连接失败或上游返回 `502`、`503`、`504` 时会继续尝试下一个上游；其他响应会直接返回给客户端。
上游配置中的 URL 只提供 scheme 和 host，客户端请求的原始 path 和 query 会原样传递给上游，不会做路径前缀拼接或改写。
所有上游 `4xx/5xx` 响应都会记录到实时日志和 stderr，包含响应体内容；正常响应保持流式转发。
代理向上游请求时使用 `Accept-Encoding: identity`，避免压缩错误 body 造成终端和实时日志乱码。
服务自动添加 CORS header，当前允许任意 Origin、method 和 header；浏览器的 CORS 预检请求会直接返回 `204`。

```yaml
- url: https://pai.d1v.ai/v1
  authorization:
    type: bearer
    value: sk-example
- url: https://backup.example/v1
  authorization:
    type: basic
    value: user:pass
```

认证类型暂时支持 `none`、`basic`、`bearer`。页面中使用下拉框选择类型；`none` 会原样透传客户端的 `Authorization`，`basic` 和 `bearer` 会使用配置值覆盖客户端认证。`basic` 的 `user:pass` 会自动进行 Base64 编码。

配置页面支持拖动表格行调整重试顺序，直接编辑 URL、认证类型、认证值以及 upstream 兼容的 selector；认证值可以切换显示/隐藏，点击“保存”后会写回 `config.yaml` 并自动刷新。页面还支持新增和删除 upstream，以及新增、删除、排序 AppSelector 和 header 条件。

`appSelectors` 是服务端内部的应用识别规则，不要求客户端携带任何 AGW 专用 header。每个 selector 按配置顺序匹配客户端已有的 HTTP header，支持 `exact`、`prefix`、`contains`、`regex` 和 `present`；默认不区分 header value 的大小写，可为单条规则设定 `caseSensitive: true`。首个命中的 selector 决定路由。upstream 的 `appSelectors` 是它兼容的 selector 名称列表，只有兼容的 upstream 才会进入该请求的逐级重试链。未配置任何 selector 时保持旧行为，所有 upstream 按原顺序参与重试。

```yaml
debug: false
appSelectors:
  - name: openai-client
    match:
      headers:
        - name: User-Agent
          operator: contains
          value: OpenAI
  - name: fallback
upstreams:
  - url: https://pai.d1v.ai/v1
    name: luna-primary
    appSelectors: [openai-client]
    authorization:
      type: bearer
      value: replace-with-your-token
  - url: https://backup.example/v1
    name: fallback
    appSelectors: [fallback]
    authorization:
      type: bearer
      value: replace-with-your-backup-token
```

页面下方的实时日志通过 SSE 从 `/logs` 推送，保留最近 100 条日志并自动滚动到底部。
日志下方的 Session journal 通过 `/sessions` 和 `/sessions/stream` 展示最近的 API 请求；每个入站请求由服务端分配独立 UUIDv7，不会依据客户端的 `Session-Id`、`Thread-Id` 或 `X-Client-Request-Id` 合并卡片。卡片可展开查看状态、耗时、传输量、客户端 request header 与请求时间线；`Authorization`、cookie、API key 等敏感 header 会脱敏。此 UI 使用进程内的结构化会话状态，因此不需要把终端日志改为 `slog` JSON；若后续接入外部日志平台，再额外配置 JSON `slog` handler 即可。
对于 JSON、SSE 和其他文本内容，Session journal 会截获请求体及通过 `io.MultiWriter` 实际转发给客户端的响应。正文不会塞入 Session SSE 事件：请求体完整写入进程专用的临时文件，展开卡片时才完整加载；响应也落盘，展开卡片时读取最新 64 KiB，持续刷新时等效于 `tail -f`。这避免大 payload 让 SSE 重连，同时不截断请求体。临时文件会在 gateway 退出时删除。
服务端日志统一使用接近 Gin 的格式和 `|` 分隔符；access log 包含状态码、耗时、客户端地址、method、path 和响应大小，upstream log 额外记录尝试、响应和重试事件。

```bash
curl http://localhost:8080/chat/completions \
  -H 'content-type: application/json' \
  -d '{"model":"example","messages":[]}'
```
