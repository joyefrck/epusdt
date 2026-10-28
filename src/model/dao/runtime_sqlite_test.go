package dao

import (
	"testing"
	"time"

	"github.com/GMWalletApp/epusdt/model/mdb"
)

func TestMigrateLegacyBSCRuntimeNetwork(t *testing.T) {
	db := setupSeedTableTestDB(t, &mdb.TransactionLock{}, &mdb.EvmScanCursor{})
	RuntimeDB = db
	if err := db.Create(&mdb.TransactionLock{
		Network: "bsc", Address: "0xlegacy", Token: "USDT", AmountScaled: 100,
		AmountPrecision: 2, TradeId: "legacy", ExpiresAt: time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatalf("create legacy lock: %v", err)
	}
	if err := db.Create(&mdb.EvmScanCursor{Network: "bsc", LastBlock: 123}).Error; err != nil {
		t.Fatalf("create legacy cursor: %v", err)
	}

	if err := migrateLegacyBSCRuntimeNetwork(); err != nil {
		t.Fatalf("migrate runtime BSC: %v", err)
	}
	for _, model := range []interface{}{&mdb.TransactionLock{}, &mdb.EvmScanCursor{}} {
		var legacy int64
		if err := db.Model(model).Where("network = ?", "bsc").Count(&legacy).Error; err != nil || legacy != 0 {
			t.Fatalf("legacy runtime count = %d, err=%v", legacy, err)
		}
	}
}
