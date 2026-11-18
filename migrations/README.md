# 数据库迁移约定

本目录使用 `golang-migrate`（`cmd/migrate`）执行迁移，文件通过 `embed.go` 打包进二进制。

## 目录结构：保持扁平

迁移文件**不按业务域拆子目录**，原因：

- `golang-migrate` 的版本号是全局递增的整数，跨目录无法保证执行顺序，且容易出现不同目录同版本号冲突；
- `//go:embed *.sql` 与 `iofs` 只扫描当前层，拆目录需要额外改嵌入与加载逻辑；
- 迁移之间存在顺序依赖（建表 → 引用表 → 索引），按域拆分反而增加维护成本。

分类通过**文件名前缀**完成，查找某个域的迁移：

```powershell
Get-ChildItem migrations\*_account_*          # 账号域
Get-ChildItem migrations\*_common_*           # 通用表
```

## 命名规范

```
<6 位版本号>_<域>_<对象>.up.sql
<6 位版本号>_<域>_<对象>.down.sql
```

- 版本号：从 `000001` 起严格递增，不复用、不插队、不重排；
- 域：`account`、`im`、`file`、`notification`、`admin`、`common`（`common` 表示跨域通用表）；
- 对象：表名或变更主题，使用下划线小写，如 `account_devices`、`account_client_configs`；
- `up` 与 `down` 必须成对出现且版本号、域、对象一致。

## 编写规则

1. 一个迁移只做一件事（建表、加列、加索引），同一事务内的多条 DDL 才算一个迁移。
2. 跨域变更拆成多个版本，例如先 `account` 加表，再 `common` 加索引。
3. 已合入并执行过的迁移**只增不改**：不修改内容、不重命名、不更改版本号；修正通过新增版本完成。
4. 每个迁移必须提供可执行的 `down`；确实不可逆的操作需在文件头注明原因。
5. 时间列统一 `DATETIME(3)`，应用层写 UTC；不使用数据库默认值兜底。
6. 不建数据库外键，逻辑关联在注释中说明。
7. 大表变更（加索引/加列）需评估锁表时间，必要时拆成在线变更脚本并通过发布流程单独执行。

## 当前迁移索引

| 版本 | 文件 | 域 | 内容 |
|---|---|---|---|
| 000001 | `000001_account_init` | account | `account_users`、`account_credentials`、`account_sessions` |
| 000002 | `000002_common_outbox_events` | common | `outbox_events` 通用任务表 |
| 000003 | `000003_account_devices` | account | `account_devices` 设备登记表 |
| 000004 | `000004_account_client_configs` | account | `client_configs` 客户端配置表 |
| 000005 | `000005_file_objects` | file | `file_objects` 文件对象元数据表（含 `object_deleted_at` 与回收索引） |

新增迁移后请同步更新本表。未发布阶段允许直接修正已有迁移（如调整列类型）；一旦环境已执行过某版本，只允许新增版本，禁止修改历史脚本。

## 常用命令

```powershell
go run ./cmd/migrate up       # 执行到最新
go run ./cmd/migrate down     # 回滚一个版本
go run ./cmd/migrate version  # 查看当前版本与 dirty 状态
```
