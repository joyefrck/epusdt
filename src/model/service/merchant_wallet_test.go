package service

import (
	"fmt"
	"github.com/GMWalletApp/epusdt/internal/testutil"
	"github.com/GMWalletApp/epusdt/model/dao"
	"github.com/GMWalletApp/epusdt/model/data"
	"github.com/GMWalletApp/epusdt/model/mdb"
	"github.com/GMWalletApp/epusdt/model/request"
	"github.com/GMWalletApp/epusdt/util/constant"
	"testing"
)

func TestMerchantWalletIsolationAcrossCreateAndSwitch(t *testing.T) {
	defer testutil.SetupTestDatabases(t)()
	for _, owner := range []uint64{1, 2} {
		if _, err := data.AddMerchantWallet("tron", fmt.Sprintf("TMerchant%d", owner), "", "manual", owner); err != nil {
			t.Fatal(err)
		}
		if _, err := data.AddMerchantWallet("ethereum", fmt.Sprintf("0xMerchant%d", owner), "", "manual", owner); err != nil {
			t.Fatal(err)
		}
	}
	if err := dao.Mdb.Create(&mdb.WalletAddress{Network: "tron", Address: "TUnbound", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	for _, owner := range []uint64{1, 2} {
		key := &mdb.ApiKey{BaseModel: mdb.BaseModel{ID: owner}}
		created, err := CreateTransaction(newCreateTransactionRequest(fmt.Sprintf("merchant-%d", owner), 10), key)
		if err != nil {
			t.Fatal(err)
		}
		if created.ReceiveAddress != fmt.Sprintf("TMerchant%d", owner) || created.ActualAmount != 10 {
			t.Fatalf("wrong merchant payment %+v", created)
		}
		switched, err := SwitchNetwork(&request.SwitchNetworkRequest{TradeId: created.TradeId, Network: "ethereum", Token: "USDT"})
		if err != nil {
			t.Fatal(err)
		}
		if switched.ReceiveAddress != fmt.Sprintf("0xmerchant%d", owner) {
			t.Fatalf("switched outside merchant pool: %+v", switched)
		}
		// A placeholder must use the originating merchant, too.
		req := newCreateTransactionRequest(fmt.Sprintf("placeholder-%d", owner), 11)
		req.Token = ""
		req.Network = ""
		parent, err := CreateTransaction(req, key)
		if err != nil {
			t.Fatal(err)
		}
		selected, err := SwitchNetwork(&request.SwitchNetworkRequest{TradeId: parent.TradeId, Network: "tron", Token: "USDT"})
		if err != nil {
			t.Fatal(err)
		}
		if selected.ReceiveAddress != fmt.Sprintf("TMerchant%d", owner) {
			t.Fatalf("placeholder escaped merchant pool: %+v", selected)
		}
	}
	if err := dao.Mdb.Model(&mdb.WalletAddress{}).Where("api_key_id = ?", 1).Update("status", 2).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := CreateTransaction(newCreateTransactionRequest("no-fallback", 10), testOrderMerchant()); err != constant.NotAvailableWalletAddress {
		t.Fatalf("expected no wallet, got %v", err)
	}
	if _, err := CreateTransaction(newCreateTransactionRequest("no-identity", 10), nil); err != constant.OrderApiKeyUnavailableErr {
		t.Fatalf("nil key got %v", err)
	}
}
