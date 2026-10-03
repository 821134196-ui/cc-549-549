# 墨衡 · 出版社稿酬结算与授权区域系统

面向出版社**编辑、财务、作者**三类角色的稿酬结算系统：编辑维护作品合同（作者分成、授权语种、销售区域、有效期限、纸书/电子书费率）；财务按期导入销量、按销售时有效的合同版本计酬、确认结算、处理异常、补差与模拟付款；作者登录后自助查看每一笔稿酬的计算明细，减少邮件对账。

- **后端**：Go + Gin + GORM，SQLite（纯 Go 驱动 `glebarez/sqlite`，无需 CGO）
- **前端**：Vue 3 + Vue Router + Vite，构建产物由 Go `embed` 打进单个二进制
- **金额**：全部以整数「分」记账，分成拆分采用最大余数法，零头一分钱不丢
- **启动**：`scripts/run.sh` 一条命令完成前端构建 → 后端构建 → 示例数据自检 → 启动

## 一条命令启动

```bash
scripts/run.sh            # 构建 + 自检 + 启动 http://localhost:8080
PORT=9000 scripts/run.sh  # 指定端口
scripts/run.sh verify     # 只跑示例数据自检验证（51 项断言）
```

启动后打开 http://localhost:8080 ，示例账号：

| 角色 | 邮箱 | 密码 |
| --- | --- | --- |
| 编辑 | `editor@pub.com` | `editor123` |
| 财务 | `finance@pub.com` | `finance123` |
| 作者（林岚） | `lin@author.com` | `author123` |
| 作者（陈墨） | `chen@author.com` | `author123` |
| 作者（周禾） | `zhou@author.com` | `author123` |

> 需要本机有 Go 1.22+ 与 Node 18+。若 Go 未在 PATH，脚本会自动使用 `~/.local/go`。
> 首次启动自动写入示例数据；重置数据直接删除 `data/royalty.db` 后重启。

## 核心业务规则

### 合同版本（只追加，不改历史）
- 每份合同记录：作者及分成比例（bps，合计必须 100%）、授权语种、销售区域、有效期、纸书与电子书各自费率（百分比 bps 或按册 分/册）。
- 合同按版本管理，区间为半开区间 `[effective_from, effective_to)`。新版本生效日落在现行版本区间内时，旧版本**自动截断**；历史版本永久保留。
- 计酬时严格按**每笔销售发生当日**有效的合同版本匹配费率、区域、语种与作者名单（6 月的销售按 6 月的合同，哪怕 8 月才导入/结算）。

### 销量导入与幂等
- 一次导入为一个批次（`batch_ref` 唯一）。同一批次号再次导入：整批拒绝、不写入任何行、不重复计酬，批次上累计拦截次数。
- 一条销量对一位作者只生成一条计酬记录，数据库层 `(sales_line_id, author_id)` 联合唯一索引兜底。

### 异常销量隔离（不混入正常收入）
导入时逐行判定并落库，异常行不进入任何结算单：

| 异常类型 | 判定 |
| --- | --- |
| `cross_region` 越区销售 | 销售区域不在销售时合同版本的授权区域内（如授权 CN,US,GB，卖到 JP） |
| `language_not_licensed` | 语种不在授权语种内 |
| `cross_period_return` 跨期退货 | 负数销量（退货）的发生月与导入归属期不同 |
| `post_expiry` 到期后补传 | 销售日晚于该作品最后一份合同的到期日 |
| `settlement_locked` | 归属期已有作者的结算单确认，补传销量无法再进正常收入 |

异常经核实后可「特批计酬」：系统按销售时合同版本给各位作者生成**独立补差单**（退货为负），异常收入始终与正常收入分开列示。

### 结算、冻结与补差
- 每个结算期为每位作者生成一张草稿结算单，可反复重算；明细同时关联**原始销量行**与**抵扣依据（合同版本 + 分成比例）**。
- 草稿**确认后金额冻结**，任何接口都不能再改其金额；之后发现差错或补传销量，一律开具**补差单**（可正可负）并关联原结算单。补差单同样走「确认 → 付款」。
- 付款在本地模拟：确认后的单据生成付款回执（单号、通道、金额、收款账户），重复付款幂等。

### 多作者分成
- 版税总额先按费率对整数分四舍五入，再按各作者 bps 拆分；零头按最大余数法（比例高者优先）逐分分配，**分配合计恒等于版税总额**。
- 例：版税 101 分按 50/30/20 拆为 51/30/20。

## 示例数据与自检验证

`scripts/run.sh verify` 重建临时库并执行 51 项端到端断言，覆盖：

- **多作者分成**：《山海图注》v1（林岚 60%/陈墨 40%）→ v2（林岚 50%/陈墨 30%/周禾 20%，新增英国授权、费率上调）；6 月销售命中 v1、8 月命中 v2；含按册计酬作品与 101 分零头拆分。
- **重复导入**：同一批次导入两次，行数与作者稿酬不变，每销量每作者仅 1 条计酬记录。
- **越区销售**：JP 行被隔离，不进结算单；特批后生成补差单。
- **跨期退货**：6 月退货 8 月到账，按 v1 的 60/40 生成 −15.00 / −10.00 两张负补差，周禾（v2 才加入）不涉及。
- **到期后补传**：《旧译丛》合同 3 月到期，8 月销量列为 `post_expiry`。
- **确认冻结**：确认后补传销量被拦为 `settlement_locked`，原单金额分文不动，差错经补差单更正并关联原单。
- **角色权限**：作者 403 拒绝导入/财务接口，只能查看本人账单；作者端明细可追溯到原始销量行与合同版本。

预期 8 月正常稿酬：林岚 ¥984.51、陈墨 ¥590.70、周禾 ¥1,353.80（含其独立作品《星岛夜航》¥960.00）。

## 常用 API（均需 `Authorization: Bearer <token>`）

```
POST /api/auth/login                 登录（editor/finance/author）
GET  /api/works                      作品列表
GET  /api/works/:id                  作品 + 全部合同版本
GET  /api/works/:id/license?date=&region=&language=   作品页授权范围直接查询
POST /api/works/:id/contracts        编辑：新建合同版本（自动截断旧版本）
POST /api/sales/import               财务：按批次导入销量（幂等）
GET  /api/sales/lines?status=exception               异常销量
POST /api/exceptions/:lineId/allow   异常特批 → 生成补差单
POST /api/settlements/generate       按期计酬（草稿可重复执行）
POST /api/settlements/:id/confirm    确认冻结
POST /api/adjustments                开补差单（original_settlement_id 关联原单）
POST /api/adjustments/:id/confirm    确认补差
POST /api/payments/pay               模拟付款（type=settlement|adjustment）
GET  /api/my/settlements(/:id)       作者：本人结算单与计算明细
GET  /api/my/summary                 作者：累计/到账汇总
```

## 目录结构

```
scripts/run.sh          一条命令：构建 → 自检 → 启动
backend/
  main.go               CLI（serve / verify / seed）+ 静态资源 embed
  models.go             GORM 模型（合同版本/分成/批次/销量/结算/补差/回执/用户）
  engine.go             计酬引擎：版本匹配、异常分类、幂等导入、按期计酬
  money.go              整数分的 bps 计算与最大余数法拆分
  handlers*.go          Gin API 与角色权限
  auth.go               HMAC token 登录
  seed.go               示例数据
  verify.go             51 项端到端自检
frontend/               Vue 3 前端（vite build 输出到 backend/web/dist）
data/                   SQLite 数据库（运行时生成，已 gitignore）
```
