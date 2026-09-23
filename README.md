# Laya Decision Web

本机浏览器演示：通过统一决策接口切换 **laya / mock / Bocha Jev**，用 Snake 展示候选动作概率、延迟与安全层。

![Laya Snake 演示：provider=laya，方向概率与约 40ms 延迟](docs/assets/snake-laya-demo.png)

设计文档见 [`docs/`](docs/)。

## 快速开始

需要 Go **1.24**（`toolchain go1.24.13`，与 a2a-agents 对齐）、macOS + Xcode CLT（本地 Laya）、以及 `third_party/tokenizers/libtokenizers.a`。

```bash
# 缺 libtokenizers 时（可用本机代理）
# export HTTPS_PROXY=http://127.0.0.1:7897
# make deps

# 终端 1：Go API（默认 models/snake）
make run
# 或：./scripts/run-server.sh

# 终端 2：前端
make frontend
```

打开 http://127.0.0.1:5173 。默认优先选择可用的 **laya**；也可切到 mock / bocha-jev。

本地模型在 `models/snake`（从 `laya-coreml/models/snake` 复制，无需重新下载）。首次 Core ML 编译会生成 `models/snake/model.mlmodelc` 缓存。

配置示例：`cp .env.example .env.local`

生产式单入口：

```bash
make frontend-build
STATIC_DIR=frontend/dist make run
```

## 测试

```bash
export CGO_ENABLED=1
export CGO_LDFLAGS="-L$(pwd)/third_party/tokenizers"
make test          # 全量（Laya 集成默认走 CPU，避免 ANE 编译卡住）
make test-short    # 跳过 Core ML 加载
# LAYA_TEST_ANE=1 make test-laya-ane   # 可选：ANE 路径
```

- [x] Go HTTP API、SSE、Snake、安全层
- [x] mock / Bocha Jev provider（含 429 / 401 测试）
- [x] Vite Canvas 控制台
- [x] Laya Core ML ANE（daulet/tokenizers + maruel/safetensors + ObjC bridge）
- [x] 启动脚本 / Makefile / 模型本地缓存编译
- [ ] 常规非 ANE Core ML 包路径（当前本地包为 ANE）
- [ ] 完整 golden fixture 批量 parity

移植自 [laya-coreml](https://github.com/LeeeeeeM/laya-coreml)；Apache-2.0，见 `LICENSE` / `NOTICE`。
