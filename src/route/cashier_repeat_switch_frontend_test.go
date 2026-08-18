package route

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCashierKeepsRepeatSwitchBoundToRootOrder(t *testing.T) {
	checkoutPath := filepath.Join("..", "www", "assets", "_trade_id-B2s-YYlo.js")
	checkoutBody, err := os.ReadFile(checkoutPath)
	if err != nil {
		t.Fatalf("read cashier asset: %v", err)
	}
	checkout := string(checkoutBody)
	for _, required := range []string{
		"rootTradeId=x?.parent_trade_id||x?.trade_id",
		"trade_id:rootTradeId??``",
		"body:{trade_id:rootTradeId,network:e.network.toLowerCase(),token:e.token.toLowerCase()}",
		"E=!!(x?.trade_id&&x?.network&&x?.token)",
		"showChangePaymentOption:E&&B===`payment`",
		"C?(0,G.jsxs)(I,{className:`mb-3 h-12 w-full rounded-xl text-base`,onClick:()=>{window.open(C,`okpay_checkout`,`popup,width=480,height=720`)},type:`button`,children:[(0,G.jsx)($e,{}),se()]}):null,u?",
	} {
		if !strings.Contains(checkout, required) {
			t.Fatalf("cashier asset is missing repeat-switch behavior %q", required)
		}
	}
	if strings.Contains(checkout, "E=!!(x?.trade_id&&T&&!Ge(x.is_selected))") {
		t.Fatal("cashier still hides the switch button after an option is selected")
	}

	modelPath := filepath.Join("..", "www", "assets", "checkout-model-CU56hvYm.js")
	modelBody, err := os.ReadFile(modelPath)
	if err != nil {
		t.Fatalf("read checkout model asset: %v", err)
	}
	if !strings.Contains(string(modelBody), "parent_trade_id:e.parent_trade_id") {
		t.Fatal("checkout model drops parent_trade_id")
	}

	swBody, err := os.ReadFile(filepath.Join("..", "www", "sw.js"))
	if err != nil {
		t.Fatalf("read service worker: %v", err)
	}
	for assetName, body := range map[string][]byte{
		filepath.Base(checkoutPath): checkoutBody,
		filepath.Base(modelPath):    modelBody,
	} {
		revision := fmt.Sprintf("%x", sha256.Sum256(body))
		entry := fmt.Sprintf(`{url:"assets/%s",revision:"%s"}`, assetName, revision)
		if !strings.Contains(string(swBody), entry) {
			t.Fatalf("service worker is missing revisioned cashier entry %s", entry)
		}
	}
}
