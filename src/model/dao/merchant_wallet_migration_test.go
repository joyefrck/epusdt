package dao

import (
	"github.com/GMWalletApp/epusdt/model/mdb"
	"testing"
)

func TestMerchantWalletMigrationSingleKeyAndRestart(t *testing.T) {
	db := setupSeedTableTestDB(t, &mdb.ApiKey{}, &mdb.WalletAddress{}, &mdb.Orders{})
	key := mdb.ApiKey{Pid: "1000", Name: "A", Status: 1}
	db.Create(&key)
	wallet := mdb.WalletAddress{Network: "tron", Address: "old-wallet", Status: 1}
	db.Create(&wallet)
	if err := migrateMerchantWallets(db); err != nil {
		t.Fatal(err)
	}
	db.First(&wallet, wallet.ID)
	if wallet.ApiKeyID != key.ID {
		t.Fatalf("wallet not migrated: %+v", wallet)
	}
	db.Model(&wallet).Update("api_key_id", 0)
	if err := migrateMerchantWallets(db); err != nil {
		t.Fatal(err)
	}
	wallet = mdb.WalletAddress{BaseModel: mdb.BaseModel{ID: wallet.ID}}
	db.First(&wallet)
	if wallet.ApiKeyID != 0 {
		t.Fatal("restart overwrote manual unbinding")
	}
}

func TestMerchantWalletMigrationLeavesAmbiguousOwnersUnbound(t *testing.T) {
	for _, multiple := range []bool{false, true} {
		t.Run(map[bool]string{false: "historical-conflict", true: "multiple-keys"}[multiple], func(t *testing.T) {
			db := setupSeedTableTestDB(t, &mdb.ApiKey{}, &mdb.WalletAddress{}, &mdb.Orders{})
			key := mdb.ApiKey{Pid: "1000", Status: 1}
			db.Create(&key)
			if multiple {
				db.Create(&mdb.ApiKey{Pid: "1001", Status: 1})
			} else {
				db.Create(&mdb.Orders{TradeId: "old", OrderId: "old", Network: "tron", ReceiveAddress: "old-wallet", ApiKeyID: key.ID + 1})
			}
			wallet := mdb.WalletAddress{Network: "tron", Address: "old-wallet", Status: 1}
			db.Create(&wallet)
			if err := migrateMerchantWallets(db); err != nil {
				t.Fatal(err)
			}
			db.First(&wallet, wallet.ID)
			if wallet.ApiKeyID != 0 {
				t.Fatal("ambiguous address assigned")
			}
		})
	}
}

func TestMerchantWalletStartupMigratesOldSchemaWithoutChangingOrders(t *testing.T) {
	db := setupSeedTableTestDB(t, &mdb.ApiKey{}, &mdb.Orders{})
	Mdb = db
	ResetMdbTableInitForTest()
	t.Cleanup(ResetMdbTableInitForTest)
	if err := db.Exec("CREATE TABLE wallet_address (id INTEGER PRIMARY KEY, network TEXT, address TEXT, status INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO wallet_address (id, network, address, status) VALUES (1, 'tron', 'TLegacyStartup', 1)").Error; err != nil {
		t.Fatal(err)
	}
	key := mdb.ApiKey{Pid: "1000", Name: "A", Status: 1}
	if err := db.Create(&key).Error; err != nil {
		t.Fatal(err)
	}
	order := mdb.Orders{TradeId: "legacy-startup", OrderId: "legacy-startup", ApiKeyID: 0, Network: "tron", ReceiveAddress: "TLegacyStartup", Status: 1}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	if err := MdbTableInit(); err != nil {
		t.Fatal(err)
	}
	var wallet mdb.WalletAddress
	if err := db.First(&wallet, 1).Error; err != nil {
		t.Fatal(err)
	}
	if wallet.ApiKeyID != key.ID {
		t.Fatalf("old wallet not bound: %+v", wallet)
	}
	if err := db.First(&order, order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if order.ApiKeyID != 0 || order.ReceiveAddress != "TLegacyStartup" {
		t.Fatalf("historical order rewritten: %+v", order)
	}
}
