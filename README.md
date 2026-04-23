[中文](README.md) | [English](README.en.md)

# doppel

![doppel hero image](assets/doppel.png)

一个 macOS 专用的命令行工具，把 `.app` bundle 克隆成一个独立可运行的第二个实例，具备新的 bundle identifier 和本地 ad-hoc 重签名。

一份二进制，两种模式：

- **TUI**（默认）：`doppel` — 全屏交互式 app 选择器 → 配置表单 → 实时进度 → 结果页
- **CLI**：`doppel <inspect|clone|verify|doctor> …` — 可脚本化，支持 `--json` 输出

## 安装

### Homebrew（推荐）

```bash
brew install tt-a1i/tap/doppel
```

macOS 首次启动时可能会拦截未签名二进制。若被拦：

```bash
xattr -dr com.apple.quarantine "$(brew --prefix)/bin/doppel"
```

### Go

```bash
go install github.com/tt-a1i/doppel/cmd/doppel@latest
```

### 源码构建

```bash
git clone https://github.com/tt-a1i/doppel
cd doppel
make build         # 产出 ./doppel
make install       # 拷到 $GOPATH/bin（可选）
```

仅支持 macOS（Intel 或 Apple Silicon）。源码 / `go install` 路径需要 Go 1.26+。运行时 `doppel` 通过系统自带的 `/usr/bin/ditto`、`/usr/bin/codesign`、`/usr/sbin/spctl` 完成实际操作。

## 使用

### 交互式（TUI）

```bash
doppel
```

启动全屏 picker，扫描 `/Applications`、`/Applications/Utilities`、`~/Applications`。挑一个 app，填入新名字和 bundle ID，实时观察克隆流水线。

### CLI

```bash
# 列出可克隆的 app（agent-friendly）
doppel list
doppel list --json

# 检查指定 bundle
doppel inspect /Applications/cmux.app
doppel inspect /Applications/cmux.app --json

# 试跑（只推演计划，不动任何文件）
doppel clone /Applications/cmux.app --name cmux2 --bundle-id com.example.cmux2 --dry-run

# 真实克隆
doppel clone /Applications/cmux.app \
  --name cmux2 \
  --bundle-id com.example.cmux2 \
  --target /Applications/cmux2.app

# 覆盖已存在的 .app 目标（先删后建；目标必须以 .app 结尾）
doppel clone /Applications/cmux.app --name cmux2 --bundle-id com.example.cmux2 --force

doppel verify /Applications/cmux2.app
doppel doctor /Applications/cmux.app
```

全局 flag：`--json`（结构化输出）、`--verbose`。
运行 `doppel <cmd> --help` 查看每个子命令的完整 flag 列表。

### 退出码（便于脚本化）

| 码值 | 含义 |
|---|---|
| 0 | OK |
| 1 | 一般错误 |
| 2 | 无效输入（路径非法、bundle id 非法、目标已存在但未加 `--force` 等） |
| 3 | 不支持的环境（非 macOS） |
| 4–9 | 分阶段失败：copy / plist / sign / verify / launch-test / inspect |

## 工作原理

克隆流水线分 6 个阶段，每个阶段通过 stage event 推送给 TUI / CLI 渲染：

1. **copy** — `/usr/bin/ditto` 保留 `cp` 无法处理的 xattrs、ACLs、resource fork
2. **plist** — 改写 `CFBundleIdentifier`、`CFBundleName`、`CFBundleDisplayName`；同时改写那些嵌入父 ID 的 helper bundle ID（Electron 模式）
3. **entitlements** — 从源 app 提取，剥离和身份绑定的 key（`application-identifier`、`keychain-access-groups`、team-identifier 等），否则 ad-hoc 重签后会挂
4. **discover** — 走遍 `Contents/Frameworks`、`XPCServices`、`PlugIns`、`Helpers`、`LoginItems` 找嵌套可签名项，按深度倒排
5. **resign** — ad-hoc（`codesign --sign -`）按深度倒序：最深的先、外层 bundle 最后
6. **verify** — `codesign --verify --deep --strict` + 可选的 `spctl --assess`

已知权衡：

- **Ad-hoc 签名本机可运行，但不具备厂商信任。** `spctl` 会拒绝克隆；Gatekeeper 首次启动会弹窗。这是正常的——本地 ad-hoc 签名就是 Apple 对本地开发构建的预期，不是分发场景。
- **永不修改源 bundle。** 源 app 全程只读。
- **`--force` 支持已有 `.app` 目标。** doppel 可以删掉已有克隆目标再建，但仍然拒绝危险的非 `.app` 路径。

## 支持情况

具体每个 app 的结果见 [`docs/support-matrix.md`](docs/support-matrix.md)，doctor 规则目录见 [`docs/failure-modes.md`](docs/failure-modes.md)。

速览：Swift/Rust/原生 app 克隆得非常干净。Electron 支持因应用而异：doppel 会改写 `Contents/Frameworks/*.app` 下的 helper bundle ID、保留 helper 依赖的 bundle name、必要时改写 Electron `app.asar` 的包身份。Cherry Studio 已验证可作为独立第二实例与源 app 同时运行；Claude 仍会在启动时卡在自己的 `app.asar` 完整性校验。Sparkle 自更新 app 可以克隆但更新器会坏。沙盒 app 可以克隆，但容器目录是空的。源盘上带有 `codesign --strict` 问题的 app（例如 Chrome 的 FinderInfo xattrs）会在克隆开始前被标记。

## 注意

- 仅 macOS（启动时强制检查）
- 实验性质——不是所有 app 都能干净克隆；`doctor` 会点出常见失败模式
- App Store 应用不在 v1 范围
- 不保留厂商公证、更新渠道或与原 team 绑定的 entitlement 身份
