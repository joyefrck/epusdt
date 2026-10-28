package data

import (
	"fmt"
	"github.com/GMWalletApp/epusdt/internal/testutil"
	"github.com/GMWalletApp/epusdt/model/dao"
	"github.com/GMWalletApp/epusdt/model/mdb"
	"github.com/GMWalletApp/epusdt/util/constant"
	"testing"
)

func TestMerchantWalletSelectionNeverFallsBack(t *testing.T) {
	defer testutil.SetupTestDatabases(t)()
	a := mdb.ApiKey{Name: "A", Pid: "1000", Status: 1}
	b := mdb.ApiKey{Name: "B", Pid: "2001", Status: 1}
	for _, k := range []*mdb.ApiKey{&a, &b} {
		if err := dao.Mdb.Create(k).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, w := range []mdb.WalletAddress{
		{Network: "tron", Address: "merchant-a", Status: 1, ApiKeyID: a.ID},
		{Network: "tron", Address: "merchant-b", Status: 1, ApiKeyID: b.ID},
		{Network: "tron", Address: "unbound", Status: 1},
		{Network: "tron", Address: "disabled-a", Status: 2, ApiKeyID: a.ID},
		{Network: "solana", Address: "other-chain", Status: 1, ApiKeyID: a.ID},
	} {
		if err := dao.Mdb.Create(&w).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range []mdb.ApiKey{a, b} {
		rows, err := GetMerchantWalletAddresses("tron", k.ID)
		if err != nil || len(rows) != 1 || rows[0].ApiKeyID != k.ID {
			t.Fatalf("merchant %d: rows=%v err=%v", k.ID, rows, err)
		}
	}
	if err := dao.Mdb.Model(&b).Update("status", 2).Error; err != nil {
		t.Fatal(err)
	}
	if rows, err := GetMerchantWalletAddresses("tron", b.ID); err == nil || len(rows) > 0 {
		t.Fatalf("disabled key rows=%v err=%v", rows, err)
	}
	if rows, err := GetMerchantWalletAddresses("tron", 0); err == nil || len(rows) > 0 {
		t.Fatalf("unbound key rows=%v err=%v", rows, err)
	}
}

func TestMerchantWalletCannotReuseDeletedOrPurgedHistory(t *testing.T) {
	defer testutil.SetupTestDatabases(t)()
	for _, purge := range []bool{false, true} {
		address := "retired-soft"
		if purge {
			address = "retired-purged"
		}
		wallet, err := AddMerchantWallet("ethereum", address, "", "manual", 1)
		if err != nil {
			t.Fatal(err)
		}
		order := mdb.Orders{TradeId: address, OrderId: address, ApiKeyID: 1, Network: "ethereum", ReceiveAddress: address, Status: 3}
		if err := dao.Mdb.Create(&order).Error; err != nil {
			t.Fatal(err)
		}
		if err := dao.Mdb.Delete(&order).Error; err != nil {
			t.Fatal(err)
		}
		query := dao.Mdb
		if purge {
			query = query.Unscoped()
		}
		if err := query.Delete(wallet).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := AddMerchantWallet("ethereum", address, "", "manual", 2); err != constant.WalletMerchantConflictErr {
			t.Fatalf("purge=%v err=%v", purge, err)
		}
		if !purge {
			if _, err := AddMerchantWallet("ethereum", address, "restored", "manual", 1); err != nil {
				t.Fatalf("same merchant restore: %v", err)
			}
		}
	}
}

func TestLegacySharedWalletCanBeAssignedWithoutRewritingHistory(t *testing.T) {
	defer testutil.SetupTestDatabases(t)()
	wallet := mdb.WalletAddress{Network: "tron", Address: "legacy-shared", Status: 1}
	if err := dao.Mdb.Create(&wallet).Error; err != nil {
		t.Fatal(err)
	}
	for _, owner := range []uint64{1, 2} {
		order := mdb.Orders{TradeId: fmt.Sprintf("legacy-%d", owner), OrderId: fmt.Sprintf("legacy-%d", owner), Network: "tron", ReceiveAddress: wallet.Address, ApiKeyID: owner, Status: mdb.StatusExpired}
		if owner == 2 {
			order.Status = mdb.StatusWaitPay
		}
		if err := dao.Mdb.Create(&order).Error; err != nil {
			t.Fatal(err)
		}
	}
	owner := uint64(1)
	if err := UpdateMerchantWallet(wallet.ID, &owner, nil); err != constant.WalletMerchantConflictErr {
		t.Fatalf("pending legacy payment was reassigned: %v", err)
	}
	if err := dao.Mdb.Model(&mdb.Orders{}).Where("trade_id = ?", "legacy-2").Update("status", mdb.StatusExpired).Error; err != nil {
		t.Fatal(err)
	}
	if err := UpdateMerchantWallet(wallet.ID, &owner, nil); err != nil {
		t.Fatal(err)
	}
	rows, err := GetMerchantWalletAddresses("tron", 1)
	if err != nil || len(rows) != 1 {
		t.Fatalf("assigned wallet unavailable: %v %v", rows, err)
	}
	var order mdb.Orders
	if err := dao.Mdb.Where("trade_id = ?", "legacy-2").First(&order).Error; err != nil {
		t.Fatal(err)
	}
	if order.ApiKeyID != 2 {
		t.Fatal("historical merchant was rewritten")
	}
}
