package dao

import (
	"github.com/GMWalletApp/epusdt/model/mdb"
	"gorm.io/gorm"
)

// Separate from editable settings so administrators cannot accidentally rerun it.
type merchantWalletMigration struct {
	ID uint `gorm:"primaryKey;autoIncrement:false"`
}

func migrateMerchantWallets(db *gorm.DB) error {
	if err := db.AutoMigrate(&merchantWalletMigration{}); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var done int64
		if err := tx.Model(&merchantWalletMigration{}).Where("id = ?", 1).Count(&done).Error; err != nil {
			return err
		}
		if done > 0 {
			return nil
		}
		var keys []mdb.ApiKey
		if err := tx.Find(&keys).Error; err != nil {
			return err
		}
		if len(keys) == 1 {
			// Include deleted orders: their ownership must not be reassigned either.
			var conflicts int64
			if err := tx.Unscoped().Model(&mdb.Orders{}).Where("api_key_id <> ? AND api_key_id <> 0", keys[0].ID).Count(&conflicts).Error; err != nil {
				return err
			}
			if conflicts == 0 {
				if err := tx.Model(&mdb.WalletAddress{}).Where("api_key_id = 0 OR api_key_id IS NULL").Update("api_key_id", keys[0].ID).Error; err != nil {
					return err
				}
			}
		}
		return tx.Create(&merchantWalletMigration{ID: 1}).Error
	})
}
