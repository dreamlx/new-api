# Wisemodel 资源包额度门控重构设计

- 日期: 2026-06-15
- 分支: `fix/wisemodel-quota-gate-redesign`(基于 `ospreyai-dev`)
- 状态: 待用户审查

## 1. 背景与问题

### 1.1 现象
- LiuDQ(user 164)用 `minimax-m2.5-highspeed`(命中健康的准无限包,剩 ~10万亿)时,**间歇性**报 `wisemodel package quota exhausted`;同一分钟内同包请求一成一败。
- 多个用户(207/378/424)包过期后被每分钟刷屏拦截。
- 包到期后二次订阅仍报耗尽。

### 1.2 诊断结论(证据确凿)
报错那条 request `…61CpT35aB` 全生命周期:
```
[INFO] 08:34:05 | 用户 164 额度充足, 信任且不需要预扣费 (funding=wallet)
[ERR]  08:34:09 | relay error: wisemodel package quota exhausted
```
- **与额度无关**:报错瞬间命中包仅用 6.4亿/10万亿。纯误判。
- **双层双账本打架**:钱包层(`funding=wallet`)放行,wisemodel 包层(`service/wisemodel_quota.go`)拦截。
- **归因查询失控**:每请求执行 `SELECT * FROM logs ... [rows:19904]` 耗时 2–3 秒,范围被远期包(`package25` valid 2034)把 `maxTime` 撑到 8 年。按 0.67 QPS 等于 DB 常驻 ~2 个全表慢查询——既是性能灾难,也是并发误判温床。
- **时区混用**:日志输出 UTC+8,DB/request-id 用 UTC,过期判定临界抖动。

### 1.3 根因定性
门控架构本身病了:**双层(middleware DB 归因 + service Redis 计数器)双账本 + FIFO 全表追溯归因 + maxTime 失控 + 过期/reclaim 竞态 + 时区混用**。任一处瞬时不一致即误判。这不是单点 bug,需整体重构。

## 2. 目标与非目标

### 2.1 目标(对应需求 1–5,作为验收清单)
1. **额度内可正常调用**:额度充足的请求永不被误判耗尽。
2. **不同模型不同包不同额度**:模型→包覆盖按 `available_models` 精确匹配,各包独立额度。
3. **同模型多包并存**:按到期先后(FIFO)消耗,先用快过期的包。
4. **过期不可用**:过期判定准确、无时区抖动、无 TOCTOU。
5. **过期后新包接管**:交接平滑无断档;reclaim 精确退款不重复计数。

### 2.2 非目标
- 不改动通用计费逻辑(`billingexpr` / ratio / price)——门控与计费解耦(Rule 7)。
- 不改动钱包计费框架(`PreConsumeBilling`/`SettleBilling` 对 `User.quota` 的记账)。本次只重构 **wisemodel 包级门控这一层**,以及修正 reclaim 的退款口径。
- 不引入新的中间件层;不改 wisemodel 之外的 relay 流程。

## 3. 架构决策:DB 单账本 + 原子扣减(方案 B)

**单一权威账本 = `wisemodel_packages` 表上每个包的 `remain_quota` 列。**

```
请求 → 选「active + 覆盖模型 + remain >= cost」的包(按到期 FIFO 序)
        │
        ▼
  UPDATE wisemodel_packages
  SET remain_quota = remain_quota - :est
  WHERE id = :id AND remain_quota >= :est        ← 原子,行锁
        │ RowsAffected=1 成功 / 0 → 换下一个候选包
        ▼  (请求结束)
  结算: UPDATE ... SET remain_quota = remain_quota + (:est - :actual)
        WHERE id = :id                            ← 同一行,对称补差
```

**消除**:Redis `wm:pkg:remain:` 计数器、`CalculatePackageAttribution` / `AttributeLogsToPackages` 全套 FIFO 每请求归因、`maxTime` 全表扫描、middleware 与 service 各算一遍的双层判定。

**理由**:单一持久权威,无双账本背离;主键单行 UPDATE 取代 2–3 秒全表扫描(DB 负载降 2–3 个数量级,见 §10);跨三种 DB 兼容(Rule 2);不强依赖 Redis。

## 4. 数据模型变更

`model/wisemodel_package.go`,`WisemodelPackage` 新增一列:

| 列 | 类型 | 语义 |
|---|---|---|
| `RemainQuota` | `bigint` | **单一权威**:该包剩余额度。订阅时 = `QuotaGranted`;预扣 `-= est`;结算 `+= est-actual`;过期 reclaim 后清零。 |

- 保留不变:`QuotaGranted`(授予总额,不可变)、`ValidUntil`、`ReclaimedAt`、`AvailableModels`、`OriginalPoints/Tokens`。
- **不新增** `consumed_quota` 审计列(YAGNI):已消费 = `QuotaGranted - RemainQuota` 可推算;`logs.wisemodel_package_id` 仍逐条记录消费,审计/展示从 logs 取。
- 迁移:GORM `AutoMigrate` 加列;SQLite 走 `ALTER TABLE ADD COLUMN`(Rule 2,见 `model/main.go` 模式)。新列默认 0,由 §9 一次性回填初始化。

## 5. 门控流程(取代 middleware + service 双层)

**额度门控收敛为单点:relay 预扣钩子**(`controller/relay.go` 调用处)。原因:原子扣减需要 `est = priceData.QuotaToPreConsume`,该值在 relay 计费后才可用,middleware 阶段拿不到。

- `middleware/wisemodel_package_check.go` **降级为存在性粗检**:仅判断"是否 wisemodel token + 是否存在 active 且覆盖该模型的包"(不读取/比较任何额度数字),用于尽早拒绝无包/不支持模型的请求。它**不构成账本**,因此不会与门控点形成"双层背离"。
- 删除 `service/wisemodel_quota.go` 中基于 Redis 的 `PrepareWisemodelPackageForPreConsume` / `PreConsumeWisemodelPkg`,以及 `model` 层每请求的 `CalculatePackageAttribution` 调用;`PreConsumeWisemodelPkg` 重写为下述 DB 原子扣减。

### 5.1 选包 + 原子预扣
```
1. 仅对 wisemodel token 生效(token_name=="wisemodel-token" || key 前缀 wisemodel-)。
2. packages = active 包(valid_until 未过期, reclaimed_at IS NULL), 按 ValidUntil ASC(nil 最后)。
3. eligible = FilterPackagesByModel(packages, requestedModel)。
   - 空 → 403 model_not_allowed_by_wisemodel_package。
4. 依 FIFO 序遍历 eligible:
     UPDATE ... SET remain_quota = remain_quota - est
       WHERE id = pkg.id AND remain_quota >= est
     RowsAffected==1 → 选中该包, 记 ctx{wisemodel_package_id, wisemodel_pre_consumed=est}, 放行。
     RowsAffected==0 → 试下一个。
5. 全部候选都不足 → 403 wisemodel_quota_exhausted。
```
- `est <= 0`(免费模型):跳过扣减,但仍需选中一个 active+覆盖的包写入 ctx(供结算与日志归属);无则按无包/不支持模型处理。
- **原子性即门控**:`WHERE remain_quota >= est` 由 DB 行锁保证并发不超卖,取代 Redis DecrBy + 双层判定。一次判定,无 TOCTOU。

### 5.2 结算(成功/失败路径对称)
- 成功:`SettleWisemodelPkg(actual)` → `UPDATE SET remain_quota = remain_quota + (pre_consumed - actual) WHERE id=?`。
- 失败(上游报错):`SettleWisemodelPkg(0)` → 全额退还 `+= pre_consumed`。
- 幂等保护:结算后清除 ctx 的 `wisemodel_pre_consumed`,防重复补差。
- `est==0` 时,结算改为直接 `-= actual`(免费/按次模型预扣为 0 的场景),保证按量门控不被旁路。

### 5.3 与钱包计费的关系
钱包层(`PreConsumeBilling`/`SettleBilling` 扣 `User.quota`)**保持不变**,与包 `remain_quota` 并存:钱包=用户总额度池,包 remain=单包硬上限。两者职责分离,互不依赖。

## 6. 模型覆盖(需求 2)
- 保留 `FilterPackagesByModel`(按 `available_models` 逗号列表、大小写不敏感精确匹配)。
- 单账本天然消除"FIFO 灌爆遗留包"问题:`remain_quota` 是独立列,只按精确扣减增减,不再被每请求的历史无主日志重算。
- 模型名变体(如 `minimax-m2` vs `minimax-m2.5-highspeed`)由**包配置的 `available_models` 决定覆盖**,属业务数据范畴;若某模型无对应有效包,按"不支持/无额度"正确拒绝(非 bug)。

## 7. 过期判定与时区(需求 4)
- 过期判定统一:`active = valid_until IS NULL OR valid_until > now`,`now` 与存储口径统一为 **UTC**。
- `CreateOrder` 解析上游 `valid_until`(RFC3339)后,统一以 UTC 存储与比较,消除临界 ±8h 抖动。
- 时区修复同步进行:核对 `SQL_DSN`(`loc`/`parseTime`)与应用 `TZ`,确保 `time.Now()`、GORM 存取、过期比较三者一致。**实现期需 192.168.4.4 的 new-api compose 配置(TZ/SQL_DSN)以验证**。
- 取消双层 TOCTOU:门控收敛为单次原子 UPDATE,选包与扣减在同一步完成,不存在"选包(t)→预扣(t+δ)"窗口。

## 8. 多包并存(需求 3)
- `SortPackagesByValidUntil` 保留:ValidUntil ASC(nil 永久包最后),同到期按 CreatedAt、PackageId 稳定排序。
- 遍历候选时优先扣最早到期的包;某包 `remain < est` 自动滚到下一个。先用快过期的,减少浪费。

## 9. 历史数据一次性迁移
上线迁移脚本(一次性,非每请求):
```
对每个包(含已过期未来仍需展示的):
  used = 精确归因(sum logs where wisemodel_package_id = pkg.PackageId)
       + 一次性 FIFO 回填(历史 wisemodel_package_id 为空的 consume 日志,按现有
         AttributeLogsToPackagesWithBaseline 规则归属一次)
  remain_quota = max(0, QuotaGranted - used)
```
- 复用现有归因算法跑**一次**,固化结果到 `remain_quota`,之后永不重算。
- 迁移幂等:仅当 `remain_quota` 为初始 0 且包从未被新逻辑写过时回填(用标记或迁移版本号控制)。
- Redis 旧计数器 `wm:pkg:remain:*` 迁移后清理。

## 10. 性能与并发(定量)
- 实测峰值:wisemodel **0.67 QPS**(40 req/min),全平台 0.58 QPS。
- 方案 B 增量:每请求 2 次主键单行 UPDATE(预扣+结算)= 峰值 ~1.3 UPDATE/s。MySQL 8.2(buffer pool 512M)单行主键写能力数千 QPS,余量充足(涨 100×仍轻松)。
- 行锁:各用户扣各自包行,无热点;单用户 0.1–0.2 QPS,同包并发≈0。
- **净减负**:取代每请求 2–3 秒、19904 行的全表扫描,DB 负载下降 2–3 个数量级。
- 可选优化(后续,YAGNI):对 `remain >> est` 的充裕包走"快路径"——调用前不预扣、仅结算扣实际量(1 次写/请求),仅在 `remain < 安全阈值` 时启用严格两段式防超卖。首版不做。

## 11. reclaim 重写(需求 5)
- `ReclaimExpiredPackages` 改为:过期包的未消费额度 **= `remain_quota`**(精确,非"整窗口消费"估算)。
- `refund = pkg.RemainQuota`;事务内乐观锁抢占 `reclaimed_at`,再 `User.quota -= refund`,然后 `remain_quota = 0`。
- 消除多包时间窗重叠导致的重复计数(每包 remain 独立精确)。
- 全局定时任务 `ReclaimAllExpiredPackages` 逻辑不变,仅退款口径修正。

## 12. 三 DB 兼容(Rule 2)
- 仅用 GORM:`UpdateColumn("remain_quota", gorm.Expr("remain_quota - ?", est))` + `Where("remain_quota >= ?", est)`,无原生 SQL、无 DB 特有函数。
- 原子性依赖单语句 `UPDATE ... WHERE`,三库一致。
- 迁移加列走 AutoMigrate / SQLite ADD COLUMN。

## 13. 测试策略(TDD)
- **单元**:选包遍历(首包不足滚动到次包)、原子预扣并发不超卖、结算补差对称、est=0 按量扣减、过期/模型过滤、reclaim 精确退款、迁移回填初始化。
- **并发**:模拟同包并发预扣,断言不超卖、`remain` 不为负。
- **回归**:保留对旧"预扣=0 旁路""双账本背离""maxTime 全表扫描"的回归断言(确保不复现)。
- **跨 DB**:关键路径在 SQLite + MySQL 跑(PostgreSQL 至少编译/迁移验证)。

## 14. 验收对照(需求 1–5)
| 需求 | 重构后保证 | 关键机制 |
|---|---|---|
| 1 额度内可调用 | 充足额度永不误判 | 单账本 remain,单次原子判定,无双层背离 |
| 2 不同模型不同包额度 | 精确覆盖+独立额度 | FilterPackagesByModel + 独立 remain 列 |
| 3 同模型多包并存 | 先用快过期包 | FIFO 序遍历原子扣减 |
| 4 过期不可用 | 准确无抖动 | UTC 统一过期判定,选扣合一无 TOCTOU |
| 5 过期后新包接管 | 平滑+精确退款 | active 过滤即时切换,reclaim 按 remain 退 |

## 15. 上线与回滚
- 迁移前备份 `wisemodel_packages`。
- 灰度:先回填 `remain_quota` 并与现有 Redis/归因结果对账(差异告警),确认一致后切换门控读源。
- 回滚:保留旧门控代码路径开关(env flag)一个版本,异常可切回。

## 16. 待确认事项(实现前)
1. 192.168.4.4 上 new-api 服务的 compose 配置(`TZ` / `SQL_DSN` 的 `loc`/`parseTime`)——确认时区修复范围。
2. 灰度对账期长度与差异容忍阈值。
