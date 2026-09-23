# Laya Decision Web

本机浏览器演示：通过统一决策接口切换 **laya / mock / Bocha Jev**，用 Snake 展示候选动作概率、延迟与安全层。

![Laya Snake 演示：provider=laya，方向概率与约 40ms 延迟](docs/assets/snake-laya-demo.png)

设计文档见 [`docs/`](docs/)。

## 快速开始

需要：

- Go **1.24**（`toolchain go1.24.13`）
- macOS + Xcode Command Line Tools（本地 Laya / Core ML）
- `third_party/tokenizers/libtokenizers.a`（缺省时执行 `make deps`；如需代理可设置 `HTTPS_PROXY`）

```bash
# 终端 1：Go API（默认 models/snake）
make run
# 或：./scripts/run-server.sh

# 终端 2：前端开发服务器
make frontend
```

打开 http://127.0.0.1:5173 。默认优先选择可用的 **laya**；也可切到 mock / bocha-jev。

本地模型目录为 `models/snake`（也可从 [laya-coreml](https://github.com/LeeeeeeM/laya-coreml) 的同名目录复制）。若目录不完整，启动时会按 `LAYA_MODEL_ID` **阻塞下载** Hugging Face 快照到 `LAYA_MODEL_DIR`（默认 `models/snake`），完成后再监听端口。可选 `HF_TOKEN`（私有仓库）、`LAYA_MODEL_REVISION`、`HTTPS_PROXY`。

首次 Core ML 编译会生成 `models/snake/model.mlmodelc` 缓存。

配置示例：`cp .env.example .env`

生产式单入口（API + 静态前端）：

```bash
make frontend-build
STATIC_DIR=frontend/dist make run
```

## 测试

```bash
export CGO_ENABLED=1
export CGO_LDFLAGS="-L$(pwd)/third_party/tokenizers"
make test          # 全量（Laya 集成默认走 CPU）
make test-short    # 跳过 Core ML 加载
# LAYA_TEST_ANE=1 make test-laya-ane   # 可选：ANE 路径
```

## 能力概览

- Go HTTP API、SSE 会话、Snake 引擎与安全层
- Provider：本地 Laya（Core ML ANE）、mock、Bocha Jev
- Vite + Canvas 控制台；模型缺失时自动从 Hugging Face 拉取

移植自 [laya-coreml](https://github.com/LeeeeeeM/laya-coreml)；Apache-2.0，见 `LICENSE` / `NOTICE`。
