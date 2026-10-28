# Merchant Wallet Isolation Implementation Plan

> Execute inline in this chat. User approved implementation; no additional design or execution approval is needed.

**Goal:** 地址按支付 Key 隔离，地址列表展示商户名称，仪表盘与订单管理增加商户筛选，保持整体样式。

**Architecture:** 钱包增加内部 Key 外键，订单继续以现有 `api_key_id` 为归属来源。订单选址调用带商户条件的查询，管理和统计共享严格解析的筛选参数。原有扫描和回调保留按订单数据处理。

**Tech Stack:** Go / Echo / GORM；现有 React 构建资源。

## 1. 模型、迁移与选址

- [x] 在 `src/model/mdb/wallet_address_mdb.go` 增加 `ApiKeyID uint64`，数据库列 `api_key_id`，默认 0 并建索引；不修改地址唯一约束。
- [x] 新建 `src/model/dao/merchant_wallet_migration.go`，事务内以迁移标记保证只执行一次；仅在唯一商户且历史地址无其他商户订单时更新旧地址，保留历史订单。
- [x] 在 `src/model/data/wallet_address_data.go` 增加商户选址、创建与改绑入口；无效、禁用商户不能分配；已被其他商户订单使用的地址不得改绑。
- [x] 将 `src/model/service/order_service.go` 三处选址调用改为传入订单所属商户，拒绝没有所属商户的链上分配；所有旧扫描查询仍保留。
- [x] 编写迁移和选址行为测试，先确认新增测试失败，再实现并运行：`cd src && go test ./model/dao ./model/data ./model/service`。

关键查询：
```go
dao.Mdb.Model(&mdb.WalletAddress{}).
    Where("network = ? AND status = ? AND api_key_id = ?", network, mdb.TokenStatusEnable, apiKeyID).
    Find(&rows)
```

## 2. 管理 API 与商户筛选

- [x] `src/controller/admin/wallet_controller.go` 新增/编辑/批量导入支持 `api_key_id`；列表/详情批量关联商户名称与 PID，不返回 secret。
- [x] `src/model/data/order_admin_data.go` 的 `OrderListFilter` 增加可选 `*uint64` 商户 ID；0 专门表示无归属历史数据，nil 表示全部。
- [x] `src/controller/admin/order_controller.go` 解析商户筛选参数，非法数值返回参数错误；列表、父子单列表与 CSV 使用同一条件。
- [x] `src/model/data/order_data_stats.go` 所有订单聚合和最近订单支持相同商户参数；`src/controller/admin/dashboard_controller.go` 向全部订单统计端点传参；链和 RPC 状态维持全局。
- [x] 使用实际路由测试两个商户、未归属数据、非法参数、批量导入、订单 CSV 和仪表盘各端点：`cd src && go test ./route`。

## 3. 前端最小增量

- [x] 确认前端源码来源；如只能访问构建产物，按现有 `src/www/assets/addresses-JEZCoidP.js`、`orders-BnOHTHOA.js`、`dashboard-Cw_CStip.js` 的 React 导入与组件模式增加商户模块。
- [x] 新增共用商户选择组件，复用现有 select/button 的类名和组件；选项为名称与 PID，不读取 secret。
- [x] 地址列显示 `merchant_name`，弹窗和导入请求传内部 ID；筛选变更更新请求和分页。
- [x] 仪表盘各 query key 包含商户，数据请求和刷新一致；订单导出包含当前商户。
- [x] 更新 `src/www/sw.js` 中改动资源的 SHA-256 缓存 revision；检查 ES module 语法。

## 4. 验证与交付

- [x] `cd src && go test ./...`，确认已有测试适配真实商户归属而非加入共享池回退。
- [x] 本地临时数据库运行服务，用浏览器检查桌面/窄屏地址列、选择器、筛选切换和弹窗；不得写入生产。
- [x] `git diff --check`，复查本次差异及原有改动保留；记录验收结果、前端源码限制和未发布状态。
