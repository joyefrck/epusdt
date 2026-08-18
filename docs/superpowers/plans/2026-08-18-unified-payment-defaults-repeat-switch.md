# Unified Payment Defaults and Repeat Switch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Force all initial GMPay and EPay orders to use Epusdt global payment defaults, remove GateRank's hard-coded TRON request fields, and allow a payer to switch payment targets more than once.

**Architecture:** Epusdt will normalize create requests from persisted defaults after signature verification and before validation, while retaining explicit payer choices only in `/pay/switch-network`. Repeat switching will resolve any child order back to its root parent, reuse an active child for the same target, and permit distinct active children for distinct targets. The cashier bundle will carry `parent_trade_id`, poll the root order, and keep the switch action visible for concrete child orders.

**Tech Stack:** Go, Echo, GORM, MySQL/SQLite tests, React/Vite compiled assets, TypeScript, Node test runner.

---

### Task 1: Force Epusdt create requests to use persisted defaults

**Files:**
- Modify: `src/model/request/order_request.go`
- Modify: `src/model/service/order_service.go`
- Modify: `src/controller/comm/order_controller.go`
- Modify: `src/route/router.go`
- Modify: `src/model/service/order_service_test.go`
- Modify: `src/route/router_test.go`

- [ ] **Step 1: Add failing service tests for forced defaults**

Add tests that seed `epay.default_token=usdt`, `epay.default_network=ethereum`, and `epay.default_currency=cny`, then submit explicit `USDT/TRON/USD` values and assert the persisted order uses `USDT/ethereum/CNY`. Add cases for all-empty defaults producing status 4 and partially empty token/network returning `constant.ParamsMarshalErr`.

```go
func TestCreateTransactionForcesConfiguredPaymentDefaults(t *testing.T) {
	cleanup := testutil.SetupTestDatabases(t)
	defer cleanup()
	seedPaymentDefaults(t, "usdt", "ethereum", "cny")
	seedEthereumPaymentTarget(t)

	req := newCreateTransactionRequest("forced_defaults", 10)
	req.Token = "USDT"
	req.Network = mdb.NetworkTron
	req.Currency = "USD"
	resp, err := CreateTransaction(req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Token != "USDT" || resp.Currency != "CNY" {
		t.Fatalf("token/currency = %q/%q", resp.Token, resp.Currency)
	}
	order, _ := data.GetOrderInfoByTradeId(resp.TradeId)
	if order.Network != mdb.NetworkEthereum {
		t.Fatalf("network = %q", order.Network)
	}
}
```

- [ ] **Step 2: Run the focused tests and verify they fail**

Run:

```bash
cd src
go test ./model/service -run 'TestCreateTransaction(ForcesConfiguredPaymentDefaults|UsesPlaceholderWhenDefaultsEmpty|RejectsPartialDefaults)' -count=1 -v
```

Expected: FAIL because `CreateTransaction` still uses request asset fields.

- [ ] **Step 3: Implement one shared default normalizer**

Add an exported preparation function used by controllers before validation and defensively by `CreateTransaction`:

```go
func ApplyConfiguredPaymentDefaults(req *request.CreateTransactionRequest) {
	if req == nil {
		return
	}
	req.Token = strings.TrimSpace(data.GetSettingString(mdb.SettingKeyEpayDefaultToken, ""))
	req.Network = strings.TrimSpace(data.GetSettingString(mdb.SettingKeyEpayDefaultNetwork, ""))
	req.Currency = strings.TrimSpace(data.GetSettingString(mdb.SettingKeyEpayDefaultCurrency, ""))
	if req.Currency == "" {
		req.Currency = "cny"
	}
	if strings.EqualFold(req.PaymentType, mdb.PaymentTypeEpay) {
		req.EpayType = epayAssetType(req.Token, req.Network)
	}
}

func epayAssetType(token, network string) string {
	token = strings.ToLower(strings.TrimSpace(token))
	network = strings.ToLower(strings.TrimSpace(network))
	if token == "" || network == "" {
		return ""
	}
	return token + "." + network
}
```

Call it after `ctx.Bind(req)` and before `ValidateStruct` in both create controllers. Remove `required` validation from `Currency`, because the server now injects it. Keep signature middleware unchanged so signatures cover the original request.

- [ ] **Step 4: Update EPay route tests for forced settings**

Change selector tests so a submitted `type=usdt.tron` cannot override configured `usdt.ethereum`, and verify the stored `epay_type` is `usdt.ethereum`. Preserve tests that reject unsupported raw EPay types before order creation.

- [ ] **Step 5: Run focused service and route tests**

Run:

```bash
cd src
go test ./model/service ./route -run 'Test(CreateTransaction|EpaySubmit)' -count=1 -v
```

Expected: PASS.

- [ ] **Step 6: Commit the Epusdt default-resolution change**

```bash
git add src/model/request/order_request.go src/model/service/order_service.go src/controller/comm/order_controller.go src/route/router.go src/model/service/order_service_test.go src/route/router_test.go
git commit -m "fix: enforce configured payment defaults"
```

### Task 2: Allow repeated backend switching across parent and child orders

**Files:**
- Modify: `src/model/service/order_service.go`
- Modify: `src/model/data/order_data.go`
- Modify: `src/model/response/pay_response.go`
- Modify: `src/model/service/pay_service.go`
- Modify: `src/model/service/order_service_test.go`
- Modify: `src/route/router_test.go`
- Modify: `src/controller/comm/order_controller.go`

- [ ] **Step 1: Replace the one-child expectation with failing repeat-switch tests**

Update `TestSwitchNetworkAfterWaitSelectCompletionUsesExistingSubOrderFlow` to create two distinct children and expect two active sub-orders. Add a test that passes the first child ID into the third switch and expects the backend to resolve its parent. Add same-target reuse and switch-back-to-parent cases.

```go
third, err := SwitchNetwork(&request.SwitchNetworkRequest{
	TradeId: second.TradeId,
	Token:   "USDT",
	Network: mdb.NetworkBsc,
})
if err != nil {
	t.Fatalf("third switch from child: %v", err)
}
if third.ParentTradeId != parentResp.TradeId {
	t.Fatalf("parent_trade_id = %q", third.ParentTradeId)
}
```

- [ ] **Step 2: Run the repeat-switch tests and verify they fail**

Run:

```bash
cd src
go test ./model/service ./route -run 'TestSwitchNetwork.*(Repeated|ExistingSubOrderFlow|Child|Parent)' -count=1 -v
```

Expected: FAIL with `constant.CannotSwitchSubOrder` or `constant.SubOrderLimitExceeded`.

- [ ] **Step 3: Resolve every switch request to a root parent**

Introduce a helper that accepts either parent or child trade IDs:

```go
func resolveSwitchRoot(tradeID string) (*mdb.Orders, error) {
	order, err := data.GetOrderInfoByTradeId(tradeID)
	if err != nil || order.ID == 0 {
		return order, err
	}
	if order.ParentTradeId == "" {
		return order, nil
	}
	return data.GetOrderInfoByTradeId(order.ParentTradeId)
}
```

Use the resolved root for status checks, same-target comparison, child lookup, child creation, OkPay creation, and parent refresh. Remove the `MaxSubOrders=1` all-time count gates from chain and OkPay switching. Retain active same-target lookup and global create locking to prevent duplicates.

- [ ] **Step 4: Expose root identity in checkout responses**

Add `ParentTradeId string` to `CheckoutCounterResponse` and populate it in `buildCheckoutResponse`:

```go
ParentTradeId: order.ParentTradeId,
```

Add response tests for parent and child checkout initialization.

- [ ] **Step 5: Keep EPay callback type aligned with the paid target**

When completing a status-4 EPay parent, persist `epay_type=token.network`. When a child settles an EPay parent, update the parent's `epay_type` in the same `MarkParentOrderSuccessWithStatusesWithTransaction` update using the paid child's token and network. For OkPay use `token.okpay`.

```go
updates := map[string]interface{}{
	"status": mdb.StatusPaySuccess,
	"callback_confirm": mdb.CallBackConfirmNo,
	"pay_by_sub_id": sub.ID,
}
if sub != nil && sub.Token != "" && sub.Network != "" {
	updates["epay_type"] = strings.ToLower(sub.Token) + "." + strings.ToLower(sub.Network)
}
```

- [ ] **Step 6: Verify multi-child settlement and lock cleanup**

Run:

```bash
cd src
go test ./model/service ./model/data ./route -run 'Test(SwitchNetwork|OrderProcessingSubOrder|CheckoutCounter)' -count=1 -v
```

Expected: PASS, including existing sibling expiration and runtime lock release tests.

- [ ] **Step 7: Commit repeated-switch backend support**

```bash
git add src/model/service/order_service.go src/model/data/order_data.go src/model/response/pay_response.go src/model/service/pay_service.go src/model/service/order_service_test.go src/route/router_test.go src/controller/comm/order_controller.go
git commit -m "fix: allow repeated payment target switching"
```

### Task 3: Keep the cashier switch action available after the first switch

**Files:**
- Modify: `src/www/assets/_trade_id-B2s-YYlo.js`
- Modify: `src/www/sw.js`
- Create: `src/route/cashier_repeat_switch_frontend_test.go`

- [ ] **Step 1: Add a failing compiled-asset regression test**

Create a Go test that locates the cashier trade asset from the route chunk and asserts all three policies exist:

1. root ID derives from `parent_trade_id || trade_id`;
2. `/pay/switch-network` submits the root ID;
3. switch visibility no longer depends on `!is_selected`.

Also verify the Service Worker revision equals the modified asset SHA-256.

- [ ] **Step 2: Run the frontend regression test and verify it fails**

Run:

```bash
cd src
go test ./route -run TestCashierRepeatSwitchFrontendPolicy -count=1 -v
```

Expected: FAIL because the compiled checkout asset still hides switching on selected children and submits the child ID.

- [ ] **Step 3: Patch the compiled cashier asset**

Update its normalized checkout model to preserve `parent_trade_id`. Derive a root trade ID from the loaded response, use it for status polling and switch requests, and simplify the switch-button predicate to concrete waiting on-chain orders with available alternatives. Keep display fields sourced from the currently selected concrete order.

The semantic behavior must be equivalent to:

```ts
const rootTradeId = order.parent_trade_id || order.trade_id;
const canSwitch = Boolean(order.trade_id && hasChainTarget && supportedOptions.length);
await switchNetwork({ body: { trade_id: rootTradeId, network, token } });
pollStatus(rootTradeId);
```

- [ ] **Step 4: Update Service Worker revision and verify JavaScript syntax**

Set the cashier asset revision to its SHA-256 content hash, then run:

```bash
node --check src/www/assets/_trade_id-B2s-YYlo.js
node --check src/www/sw.js
cd src && go test ./route -run TestCashierRepeatSwitchFrontendPolicy -count=1 -v
```

Expected: all commands PASS.

- [ ] **Step 5: Commit the cashier update**

```bash
git add src/www/assets/_trade_id-B2s-YYlo.js src/www/sw.js src/route/cashier_repeat_switch_frontend_test.go
git commit -m "fix: keep cashier payment switching available"
```

### Task 4: Remove GateRank's hard-coded initial payment asset

**Files:**
- Modify: `/Users/joyefrack/Documents/GitHub/GateRank/backend/src/services/paymentGatewayService.ts`
- Modify: `/Users/joyefrack/Documents/GitHub/GateRank/backend/tests/paymentGatewayService.test.ts`

- [ ] **Step 1: Change the GateRank request test first**

Update the HMAC creation test to assert that `currency`, `token`, and `network` are absent from the outgoing JSON and that the signature verifies over the remaining fields.

```ts
assert.equal('currency' in requestedBodies[0], false);
assert.equal('token' in requestedBodies[0], false);
assert.equal('network' in requestedBodies[0], false);
assert.equal(
  requestedBodies[0].signature,
  signWithHmacSha256Secret(requestedBodies[0], 'epay-secret'),
);
```

- [ ] **Step 2: Run the GateRank focused test and verify it fails**

Run:

```bash
cd /Users/joyefrack/Documents/GitHub/GateRank
npx tsx --test backend/tests/paymentGatewayService.test.ts
```

Expected: FAIL because the request still includes the three hard-coded fields.

- [ ] **Step 3: Remove the fields from `createUsdtOrder`**

Keep `pid`, `order_id`, `amount`, `notify_url`, `redirect_url`, and `name`; calculate HMAC after constructing that reduced request object. Do not add a second configuration source in GateRank.

- [ ] **Step 4: Verify flexible callback signatures**

Extend the current notification signature test with an Ethereum payload and verify it succeeds without a TRON assumption.

- [ ] **Step 5: Run GateRank tests and type checks**

Run:

```bash
cd /Users/joyefrack/Documents/GitHub/GateRank
npx tsx --test backend/tests/paymentGatewayService.test.ts
npm run server:typecheck
```

Expected: PASS.

- [ ] **Step 6: Commit only the intended GateRank source and test files**

```bash
git add backend/src/services/paymentGatewayService.ts backend/tests/paymentGatewayService.test.ts
git commit -m "fix: use gateway payment defaults"
```

Do not stage pre-existing `dist`, `.vite`, or Python cache changes.

### Task 5: Full verification and handoff

**Files:**
- Verify all files changed by Tasks 1-4

- [ ] **Step 1: Run Epusdt full verification**

```bash
cd /Users/joyefrack/Documents/GitHub/epusdt/src
go test ./...
cd ..
node --check src/www/assets/_trade_id-B2s-YYlo.js
node --check src/www/sw.js
git diff --check
```

Expected: PASS with zero test failures or syntax errors.

- [ ] **Step 2: Run GateRank full scoped verification**

Run the focused payment gateway tests, backend typecheck, and `git diff --check`. Expected: PASS.

- [ ] **Step 3: Verify repository scope**

Confirm Epusdt and GateRank commits contain only intended files. Confirm all pre-existing dirty files remain unstaged and unchanged.

- [ ] **Step 4: Report release readiness**

Report commit SHAs, test commands, and the production deployment scope. Do not push or deploy unless the user has explicitly authorized those actions for this change.
