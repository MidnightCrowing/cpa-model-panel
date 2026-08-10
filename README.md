# cpa-model-panel

CPA（CLIProxyAPI）辅助面板：跨站点 **模型命名映射** + **按模型 × 站点启停**。端口 **5006**。

## 它是怎么工作的

```
CPA 三张表 ──┐
openai-compatibility │
codex-api-key        ├─► 完整模型目录（缓存在面板 SQLite）
claude-api-key       │        │
                     ┘        ├─ 白名单正则   ─► 标记 excluded=whitelist
                              ├─ 版本淘汰规则 ─► 标记 excluded=version
                              ├─ 手动删除     ─► 标记 excluded=manual
                              ├─ 协议正则     ─► protocol 标记（对清洗后的名字匹配，仅展示/筛选）
                              └─ 前后缀清洗   ─► suggested 建议名
                                      │
                     命名映射页（显示全部，含被排除的）
                     站点启停页（只显示未被排除的，按协议区分）
                                      │
                              「保存到 CPA」──► 写回三张表
```

两条铁律：

1. **模型永远写回它原本所在的那张表。** 协议正则只是一个展示标记，绝不驱动跨表搬运。只有「刷新站点模型」发现一个 CPA 里完全没有的新模型时，才用协议标记决定它落到该站点的哪张表（该表没有此站点条目则回退 openai-compatibility）。
2. **过滤只打标记，不丢数据。** 被白名单/版本/手动删除排除的模型仍留在面板缓存里，放宽规则或点「恢复」即可拿回。CPA 里被外部删掉且没有任何规则解释的模型才会真正从缓存中清除，不会被下次保存复活。

站点身份按 base-url → api-key → host 依次匹配：`codex-api-key` / `claude-api-key` 的条目没有 `name`，靠这三步归到 openai 表里对应的站点。

## 目录结构

```
cmd/server      服务入口（前端 embed 进二进制）
cmd/dryrun      只读校验：证明「零改动保存」不会动 CPA 任何一个字节
internal/
  cpa/          管理 API 客户端：client / channels / discover / codec
  catalog/      核心：types · site（站点归并）· raw（协调缓存）· pipeline（过滤管线）
                · ops（草稿操作）· writeback（写回）· fingerprint（乐观锁）
  clean/        clean（前后缀）· version（版本淘汰）· protocol（协议标记）
  store/        SQLite：settings · catalog 缓存 · refsets（排除/停用/强制保留）· snapshots · legacy 迁移
  api/          server · catalog · save · settings · snapshots
web/src/
  app/          壳层 · Header · Login
  api/          client · catalog · types
  state/        useCatalog · useDraft · useToasts
  features/     naming（命名映射）· matrix（站点启停）· settings（设置）
  components/   VirtualList · FilterMenu · Toasts · Controls
  lib/          keys · vendor · hooks
  styles/       tokens · base · naming · matrix · settings
```

## API

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/login` | `{token}` |
| GET | `/api/catalog` | 读 CPA + 跑管线，返回完整视图（含 fingerprint） |
| POST | `/api/catalog/refresh` | SSE：逐站点拉上游 `/v1/models` 合并进缓存。**不写 CPA** |
| POST | `/api/save` | `{fingerprint, ops[]}`，唯一写 CPA 的入口 |
| GET/PUT | `/api/settings` | 全量设置；PUT 后用缓存重算并返回新视图，不碰 CPA |
| GET | `/api/snapshots` · POST `/api/snapshots/{id}/rollback` | 配置快照与回滚 |

`ops` 全部携带精确的 `targets: [{site, upstream}]`，所以命名页某一行的改名只作用于该行涉及的站点与模型。

## 本地开发

```bash
export ADMIN_TOKEN=dev CPA_BASE_URL=http://127.0.0.1:5000 CPA_MANAGEMENT_SECRET=... DATA_DIR=./data
go run ./cmd/server          # 后端
cd web && npm install && npm run dev   # 前端（代理 /api 到 5006）
```

上线前务必跑一次只读校验：

```bash
CPA_BASE_URL=... CPA_MANAGEMENT_SECRET=... go run ./cmd/dryrun
# 可选：带上线上设置，看首次保存会删掉哪些模型
SETTINGS_JSON=live_settings.json ... go run ./cmd/dryrun
```

## 构建与部署

```bash
cp .env.example .env      # 填 ADMIN_TOKEN / CPA_MANAGEMENT_SECRET，按需改 PANEL_* 路径
docker compose up -d --build
```

或者本机构建、服务器只替换二进制（省掉服务器上的 Node 和 Go）：

```bash
cd web && npm run build
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/cpa-model-panel-linux ./cmd/server
# 上传 bin/ 后在服务器上 docker compose -f deploy/docker-compose.yml up -d --build
```

主机相关的位置全部走环境变量（`PANEL_ENV_FILE` / `PANEL_DATA_DIR` / `PANEL_PORT` / `CPA_BASE_URL`），仓库里不写死任何机器上的路径。

容器以 uid **10001** 运行，数据目录要 `chown -R 10001:10001`。升级会自动把旧的 `disabled_models` 表迁移成新的「手动排除 / 站点停用」两张表，配置不会丢。

## 正则

所有正则（白名单、协议标记、版本豁免）都**原样使用**：留空就是不生效，写错直接报错，没有任何内置默认值兜底。

协议标记匹配的是**清洗之后的名字**（有重映射就用重映射名，否则用清洗后的原始名），所以厂商前缀、`[free]` 标记、
`(xhigh)[1M]` 这类噪音不用写进正则。

Go 用 RE2，**不支持 `(?!…)` 前瞻**。要表达「匹配 gpt 但排除 mini/image/nano/chat/audio/oss」，正面写出想要的形状：

```
(?i)^gpt-\d+(?:[.-]\d+)*(?:-(?:luna|sol|terra|pro|compact))*$
```

设置页有「用推荐正则填充」按钮。存量配置里编译不了的正则会让 `/api/catalog` 直接报错，此时设置页仍可打开，改完即恢复。
