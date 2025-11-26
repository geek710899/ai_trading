# 策略说明

## 策略模式（增强版）
- 风格：稳健型，风险优先，增加多因子过滤与波动自适应。
- 类型：单交易所衍生品“基差”均值回归 + 资金费率 carry 策略。
- 信号定义：
  - 基差：`basis = (markPrice - indexPrice) / indexPrice`
  - 使用过去 N=120 个 `basis` 构成滚动序列，计算均值与标准差，形成 `z = (basis - mean) / std`。
  - 触发阈值：`|z| >= 2.0`（自适应于波动）；且盘口价差占比 `< 0.2%`；且资金费率绝对值 `<= 0.2%`。
  - 方向选择与资金费率对齐：
    - `basis > 0` → 做空；仅当资金费率 `> 0`（空单收取资金费率）时触发
    - `basis < 0` → 做多；仅当资金费率 `< 0`（多单收取资金费率）时触发
- 仓位大小：与 `|z|` 成比例，设 `size = base_size * min(3, |z|)`，其中 `base_size=0.001`；上限控制避免过度暴露。
- 冷却时间：每个交易对触发后 5 分钟冷却，避免重复进出。
- 持有与结算：持有 10 分钟后使用最新 `last` 做标的估值，记录 `mock_pnl_close` 用于离线评估。
- 执行方式：Mock 下单，记录创建与成交日志，不调用真实下单接口。

## 使用接口
- 公共行情：
  - `GET /capi/v2/market/ticker` 权重(IP): 1，用于获取`last/best_bid/best_ask/markPrice/indexPrice`
  - `GET /capi/v2/market/index` 权重(IP): 1，用于获取指数价
  - `GET /capi/v2/market/depth` 权重(IP): 1，用于估算盘口价差与滑点
  - `GET /capi/v2/market/currentFundRate` 权重(IP): 1，用于获取资金费率
  - `GET /capi/v2/market/time` 权重(IP): 1，用于时间漂移校准
- 私有查询：
  - `GET /capi/v2/account/accounts` 权重(IP): 5, 权重(UID): 5，用于启动时验证私有接口与鉴权。

## 请求频率与限流策略
- 平台限流：按IP与按UID各自`500权重/10秒`，互不影响。
- 本Bot限流：双桶滑窗权重控制，任何请求都必须先占用对应桶的权重；超过窗口容量自动等待，避免`429`。
- 默认查询频率：`WEEX_QUERY_INTERVAL=5s`，每轮对16个交易对执行下述查询：
  - `ticker`×16（权重总计16）
  - `index`×16（权重总计16）
  - `depth`×16（权重总计16）
  - `currentFundRate`×16（权重总计16）
  - 合计约64权重/5秒，远低于`500/10秒`阈值。

## 日志
- 目录：`ai_trading/log`
- 分类：
  - `info/YYYY-MM-DD.log`：正常触发、行情查询、订单状态、私有接口可达性等。
  - `error/YYYY-MM-DD.log`：HTTP错误、JSON解析失败、鉴权失败、限流等待过长等。
- 格式：`ISO8601 level tag k=v ...`。

## 环境变量
- `WEEX_BASE_URL` 默认`https://api-contract.weex.com`
- `WEEX_API_KEY`/`WEEX_API_SECRET`/`WEEX_API_PASSPHRASE`：私有接口鉴权；不在代码库内明文存储。
- `WEEX_SYMBOLS` 可选，自定义逗号分隔交易对列表。
- `WEEX_QUERY_INTERVAL` 默认`5s`。

## 风险控制
- 波动自适应阈值：使用滚动 `basis` 的 `z` 值减少在高波动期的误触发。
- 资金费率对齐：仅在资金费率对策略方向有正 carry 时触发，提升收益期望。
- 盘口价差过滤：`(ask1 - bid1)/index <= 0.2%`才执行，降低滑点风险。
- 冷却与持有：5 分钟冷却、10 分钟持有；均可调，以平衡信号质量与交易频率。
- Mock 执行：真实接口不下单，防止任何实盘风险；日志完整用于评估。
