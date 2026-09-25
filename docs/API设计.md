# HTTP API 设计

本地 Go 服务默认监听 127.0.0.1:18080。API 使用 UTF-8 JSON；Snake 状态通过 Server-Sent Events 推送。浏览器不直接访问 Core ML、Hugging Face 或 Bocha Jev。

## 1. Provider 标识

| provider | 用途 | model |
|---|---|---|
| laya | 本地 Laya Core ML | 使用服务端登记的模型 ID |
| bocha-jev | Bocha Jev typed-decision API | 可省略，默认 bocha-jev-v1 |

浏览器不能发送 Bocha API Key、任意 URL、任意文件系统路径或底层 Core ML 对象。

## 2. 健康和能力查询

### GET /api/v1/health

~~~json
{
  "status": "ok",
  "version": "0.1.0"
}
~~~

此接口报告 Go 服务进程健康。Provider 配置情况通过 capabilities 查询。

### GET /api/v1/capabilities

~~~json
{
  "providers": [
    {
      "id": "laya",
      "available": true,
      "models": [
        {
          "id": "aac6fef/laya-multilingual-coreml",
          "ready": true,
          "runtime": "coreml",
          "compute_units": "cpu_gpu"
        }
      ]
    },
    {
      "id": "bocha-jev",
      "available": true,
      "model": "bocha-jev-v1"
    }
  ]
}
~~~

本响应不得返回密钥、绝对用户目录或 Hugging Face 凭据。模型未下载时可返回 ready:false 和状态字段。

## 3. 通用结构化决策

### POST /api/v1/decisions

用于直接调用决策 provider。Snake session 在服务端复用同一 provider 接口。

Bocha Jev 示例：

~~~json
{
  "provider": "bocha-jev",
  "state": {
    "customer_message": "客户要求退还重复扣除的款项。"
  },
  "questions": {
    "refund": {
      "type": "noul",
      "instructions": "客户是否要求退款？"
    },
    "team": {
      "type": "choice",
      "instructions": "选择负责团队",
      "criteria": {
        "billing": "账单、付款、退款",
        "technical": "技术故障"
      }
    },
    "relevance": {
      "type": "score",
      "instructions": "与退款相关程度",
      "criteria": ["无关", "间接相关", "直接相关"]
    }
  }
}
~~~

Laya 本地模型示例：

~~~json
{
  "provider": "laya",
  "model": "aac6fef/laya-multilingual-coreml",
  "state": "订单重复扣费，需要退款。",
  "questions": {
    "refund": {
      "type": "noul",
      "instructions": "客户是否要求退款？"
    }
  }
}
~~~

响应保留 provider 的 typed answers，并增加 provider 和服务端计时：

~~~json
{
  "provider": "bocha-jev",
  "model": "bocha-jev-v1",
  "answers": {
    "refund": {
      "type": "noul",
      "noul": 0.98,
      "confidence": 0.98
    }
  },
  "usage": {
    "input_tokens": 32,
    "output_tokens": 0
  },
  "timing": {
    "provider_ms": 186,
    "request_ms": 201
  }
}
~~~

数值仅用于展示格式。不要将服务端网络耗时伪装成模型 inference_ms。choice 返回选项概率；score 是从 0 开始的概率加权等级索引；noul 是 true 的概率。

## 4. Snake session

### POST /api/v1/sessions

创建并启动游戏：

~~~json
{
  "provider": "laya",
  "model": "aac6fef/laya-multilingual-coreml",
  "fps": 12,
  "seed": 7,
  "width": 24,
  "height": 16,
  "prompt": "compact"
}
~~~

Bocha Jev session 使用 provider 为 bocha-jev，省略 model；建议在线目标速度先设为 1 fps，账号额度支持时可以设置到 2 fps。服务端最终执行速度受响应时长和全局限流约束。

成功响应：

~~~json
{
  "session_id": "sess_01example",
  "status": "running",
  "snapshot": {
    "width": 24,
    "height": 16,
    "score": 0,
    "ticks": 0,
    "alive": true,
    "won": false
  },
  "events_url": "/api/v1/sessions/sess_01example/events"
}
~~~

校验要求：

- provider 必须在 capabilities 中可用。
- Laya model 必须为服务端登记的模型 ID；Bocha 模型由服务端配置。
- width、height、seed、fps 在服务端允许范围内，棋盘尺寸符合 Snake 游戏规则约束。
- Bocha fps 不高于 BOCHA_JEV_MAX_FPS。
- 每个 session 同时至多一个决策请求；请求完成前不推进下一 tick。

### GET /api/v1/sessions/{session_id}/events

SSE event 类型：

- snapshot：board、score、tick 和游戏状态。
- decision：概率、proposed、executed、safe_directions、intervened、provider latency。
- status：running、paused、cooldown、finished。
- error：结构化 provider/session 错误，不包含密钥和 Authorization header。

客户端携带 Last-Event-ID 重连时，服务端先发当前完整快照，再发之后的事件。

### POST /api/v1/sessions/{session_id}/controls

暂停、恢复、调速、重开或停止：

~~~json
{
  "action": "set_speed",
  "fps": 2
}
~~~

action 支持 pause、resume、set_speed（同时传 fps）、reset（seed 可选）和 stop。成功响应包含新的状态和快照。

### DELETE /api/v1/sessions/{session_id}

停止并释放会话资源。会话已停止或结束时返回 204。

## 5. 错误格式

~~~json
{
  "error": {
    "code": "provider_rate_limited",
    "message": "Bocha Jev 请求受限，会话已暂停。",
    "retry_after_seconds": 12
  }
}
~~~

| HTTP | code 示例 | 含义 |
|---|---|---|
| 400 | invalid_request | JSON、question schema 或参数错误 |
| 502 | provider_auth_failed | 上游拒绝服务端配置的 Jev key |
| 502 | provider_invalid_response | provider 返回的 typed answers 不符合契约 |
| 404 | session_not_found / model_not_found | session 或已登记模型不存在 |
| 413 | request_too_large | 请求体过大 |
| 422 | model_input_invalid | 类型、候选数或 token budget 不符合要求 |
| 429 | provider_rate_limited | provider 限流，可附 Retry-After |
| 503 | provider_unavailable | Core ML 不支持、模型未就绪或远端暂不可用 |
| 504 | provider_timeout | provider 调用超时 |

Jev 返回 429 / 503 / 529 时，后端遵循 Retry-After 并限制重试次数。过长冷却时，通用决策调用返回 429；Snake session 进入 cooldown 且停止推进。错误凭据或无效输入不重试。

## 6. 请求限制与密钥保护

- 默认请求体不超过 256 KiB；questions、总候选数和 instruction 长度遵循 provider 限制。
- 出站前验证 choice、score、noul 的 criteria；Laya 额外验证 tokenizer token budget。
- 验证概率为有限数且位于 [0, 1]；choice 概率和做浮点误差容忍检查。
- 安全层在服务端验证动作合法、安全。
- 日志记录 request ID、provider、model、HTTP 状态和耗时；state 可配置不记录或只记摘要。
- Laya 调试输入输出日志默认关闭；仅在明确设置 `LAYA_LOG_IO=true` 时记录 state、tokenized input 和 typed output。
- 日志、响应和错误信息永不包含 Authorization、BOCHA_JEV_API_KEY 或 HF_TOKEN。
- 默认只绑定 loopback；Origin 限定为 Go 页面或本机 Vite dev origin。不提供任意 URL 代理。
