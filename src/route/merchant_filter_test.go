package route

import (
	"encoding/json"
	"fmt"
	"github.com/GMWalletApp/epusdt/model/dao"
	"github.com/GMWalletApp/epusdt/model/mdb"
	"github.com/dromara/carbon/v2"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestMerchantFiltersOrdersAndDashboard(t *testing.T) {
	e, token := setupAdminTestEnv(t)
	now := time.Now().Add(-time.Minute)
	for i, owner := range []uint64{1, 2, 0} {
		row := mdb.Orders{TradeId: fmt.Sprintf("filter-%d", owner), OrderId: fmt.Sprintf("filter-%d", owner), ApiKeyID: owner, Network: "tron", Token: "USDT", ReceiveAddress: fmt.Sprintf("wallet-%d", owner), ActualAmount: float64((i + 1) * 10), Amount: float64((i + 1) * 70), Status: 2, BaseModel: mdb.BaseModel{CreatedAt: carbon.Time{Carbon: carbon.CreateFromStdTime(now)}, UpdatedAt: carbon.Time{Carbon: carbon.CreateFromStdTime(now.Add(time.Second * 10))}}}
		if err := dao.Mdb.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"/orders", "/orders/list-with-sub"} {
		for _, id := range []uint64{1, 2, 0} {
			body := assertOK(t, doGetAdmin(e, fmt.Sprintf("/admin/api/v1%s?api_key_id=%d", path, id), token))["data"].(map[string]interface{})
			if body["total"].(float64) != 1 {
				t.Fatalf("%s filter %d: %v", path, id, body)
			}
			rows := body["list"].([]interface{})
			if rows[0].(map[string]interface{})["api_key_id"].(float64) != float64(id) {
				t.Fatal("wrong merchant returned")
			}
		}
	}
	overview := assertOK(t, doGetAdmin(e, "/admin/api/v1/dashboard/overview?api_key_id=1", token))["data"].(map[string]interface{})
	for key, want := range map[string]float64{"total_asset": 10, "volume": 10, "order_count": 1, "seven_day_volume": 10, "active_addresses": 1, "success_rate": 1} {
		if overview[key] != want {
			t.Fatalf("%s=%v want %v", key, overview[key], want)
		}
	}
	stats := assertOK(t, doGetAdmin(e, "/admin/api/v1/dashboard/order-stats?api_key_id=1", token))["data"].(map[string]interface{})
	if stats["order_count"] != float64(1) || stats["success_count"] != float64(1) || stats["avg_payment_seconds"] != float64(10) {
		t.Fatalf("wrong stats: %v", stats)
	}
	for _, endpoint := range []string{"asset-trend", "revenue-trend", "asset-trend?group_by=address"} {
		sep := "?"
		if strings.Contains(endpoint, "?") {
			sep = "&"
		}
		body := assertOK(t, doGetAdmin(e, "/admin/api/v1/dashboard/"+endpoint+sep+"api_key_id=1", token))
		var sum float64
		for _, raw := range body["data"].([]interface{}) {
			sum += raw.(map[string]interface{})["actual_amount"].(float64)
		}
		if sum != 10 {
			t.Fatalf("%s summed other merchants: %v", endpoint, sum)
		}
	}
	recent := assertOK(t, doGetAdmin(e, "/admin/api/v1/dashboard/recent-orders?api_key_id=1", token))["data"].([]interface{})
	if len(recent) != 1 || recent[0].(map[string]interface{})["api_key_id"] != float64(1) {
		t.Fatalf("recent orders: %v", recent)
	}
	csv := doGetAdmin(e, "/admin/api/v1/orders/export?api_key_id=1", token)
	if csv.Code != 200 || !strings.Contains(csv.Body.String(), "filter-1") || strings.Contains(csv.Body.String(), "filter-2") {
		t.Fatalf("CSV escaped merchant: %s", csv.Body.String())
	}
	for _, path := range []string{"orders", "orders/list-with-sub", "orders/export", "wallets", "dashboard/overview", "dashboard/asset-trend", "dashboard/revenue-trend", "dashboard/order-stats", "dashboard/recent-orders"} {
		for _, value := range []string{"bad", "-1", "", "1&api_key_id=2"} {
			rec := doGetAdmin(e, "/admin/api/v1/"+path+"?api_key_id="+value, token)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("%s invalid merchant %q returned %d", path, value, rec.Code)
			}
		}
	}
}

func TestMerchantWalletAdminBindingAndHistory(t *testing.T) {
	e, token := setupAdminTestEnv(t)
	created := assertOK(t, doPostAdmin(e, "/admin/api/v1/wallets", map[string]interface{}{"api_key_id": 1, "network": "tron", "address": "TNewMerchantWallet"}, token))["data"].(map[string]interface{})
	id := uint64(created["id"].(float64))
	list := assertOK(t, doGetAdmin(e, "/admin/api/v1/wallets?api_key_id=1", token))["data"].([]interface{})
	for _, raw := range list {
		row := raw.(map[string]interface{})
		if row["api_key_id"] != float64(1) || row["merchant_name"] != "test-universal" {
			t.Fatalf("wallet missing merchant name: %v", row)
		}
	}
	if rec := doPostAdmin(e, "/admin/api/v1/wallets", map[string]interface{}{"api_key_id": 999, "network": "tron", "address": "TInvalidOwnerWallet"}, token); rec.Code != 400 {
		t.Fatal("invalid merchant accepted")
	}
	if rec := doPostAdmin(e, "/admin/api/v1/wallets", map[string]interface{}{"network": "tron", "address": "TMissingOwnerWallet"}, token); rec.Code != 400 {
		t.Fatal("missing merchant accepted")
	}
	path := fmt.Sprintf("/admin/api/v1/wallets/%d", id)
	assertOK(t, doPatchAdmin(e, path, map[string]interface{}{"api_key_id": 2}, token))
	if err := dao.Mdb.Create(&mdb.Orders{TradeId: "used-wallet", OrderId: "used-wallet", ApiKeyID: 2, Network: "tron", ReceiveAddress: "TNewMerchantWallet", Status: 3}).Error; err != nil {
		t.Fatal(err)
	}
	rec := doPatchAdmin(e, path, map[string]interface{}{"api_key_id": 1}, token)
	if rec.Code != 400 {
		t.Fatal("used address moved across merchants")
	}
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["status_code"] != float64(10047) {
		t.Fatalf("wrong conflict error: %s", rec.Body.String())
	}
	assertOK(t, doPostAdmin(e, "/admin/api/v1/wallets/batch-import", map[string]interface{}{"api_key_id": 2, "network": "tron", "addresses": []string{"TBatchMerchant001", "TBatchMerchant002"}}, token))
	var rows []mdb.WalletAddress
	dao.Mdb.Where("address LIKE ?", "TBatchMerchant%").Find(&rows)
	if len(rows) != 2 || rows[0].ApiKeyID != 2 || rows[1].ApiKeyID != 2 {
		t.Fatalf("batch owner lost: %v", rows)
	}
	// Deleted keys retain their historical label but no secret is exposed.
	assertOK(t, doDeleteAdmin(e, "/admin/api/v1/api-keys/2", token))
	detail := assertOK(t, doGetAdmin(e, path, token))["data"].(map[string]interface{})
	if detail["merchant_name"] != "test-epay-pid-1" || detail["merchant_deleted"] != true {
		t.Fatalf("deleted merchant label lost: %v", detail)
	}
	keys := assertOK(t, doGetAdmin(e, "/admin/api/v1/api-keys?include_deleted=true", token))["data"].([]interface{})
	if len(keys) != 2 {
		t.Fatalf("deleted key omitted from history filter: %v", keys)
	}
	for _, raw := range keys {
		if _, exposed := raw.(map[string]interface{})["secret_key"]; exposed {
			t.Fatal("secret exposed in merchant choices")
		}
	}
}
