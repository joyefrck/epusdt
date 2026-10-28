package data

import (
	"strings"
	"sync"

	"github.com/GMWalletApp/epusdt/model/dao"
	"github.com/GMWalletApp/epusdt/model/mdb"
	"github.com/GMWalletApp/epusdt/util/constant"
	"gorm.io/gorm"
)

// Serializes address ownership edits in the supported single-instance deployment.
var merchantWalletMu sync.RWMutex

// ProtectWalletAllocation holds ownership stable until an order and reservation are saved.
func ProtectWalletAllocation() func() {
	merchantWalletMu.RLock()
	return merchantWalletMu.RUnlock
}

func enabledMerchant(tx *gorm.DB, id uint64) error {
	if id == 0 {
		return constant.OrderApiKeyUnavailableErr
	}
	var count int64
	if err := tx.Model(&mdb.ApiKey{}).Where("id = ? AND status = ?", id, mdb.ApiKeyStatusEnable).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return constant.OrderApiKeyUnavailableErr
	}
	return nil
}

// GetMerchantWalletAddresses deliberately never falls back to the global pool.
// Global wallet queries remain available to scanners for previously issued orders.
func GetMerchantWalletAddresses(network string, apiKeyID uint64) ([]mdb.WalletAddress, error) {
	if err := enabledMerchant(dao.Mdb, apiKeyID); err != nil {
		return nil, err
	}
	var rows []mdb.WalletAddress
	err := dao.Mdb.Where("network = ? AND status = ? AND api_key_id = ?", normalizeWalletNetwork(network), mdb.TokenStatusEnable, apiKeyID).Order("id ASC").Find(&rows).Error
	return rows, err
}

// merchantAddressHistoryAllows prevents used, bound wallets from moving across
// merchants. Legacy unbound wallets may be assigned manually after other
// merchants' pending payments finish; historical order ownership is untouched.
func merchantAddressHistoryAllows(tx *gorm.DB, w *mdb.WalletAddress, owner uint64) error {
	if w.ID > 0 && owner == w.ApiKeyID {
		return nil
	}
	var used int64
	query := tx.Unscoped().Model(&mdb.Orders{}).Where("network = ?", w.Network)
	query = applyOrderReceiveAddressFilter(query, w.Network, w.Address)
	if w.ID > 0 && w.ApiKeyID == 0 && owner > 0 {
		query = query.Where("status = ?", mdb.StatusWaitPay)
	}
	if err := query.Where("api_key_id <> ?", owner).Count(&used).Error; err != nil {
		return err
	}
	if used > 0 {
		return constant.WalletMerchantConflictErr
	}
	return nil
}

func AddMerchantWallet(network, address, remark, source string, owner uint64) (*mdb.WalletAddress, error) {
	merchantWalletMu.Lock()
	defer merchantWalletMu.Unlock()
	network = normalizeWalletNetwork(network)
	address, err := normalizeWalletAddressByNetworkE(network, address)
	if err != nil {
		return nil, err
	}
	var row mdb.WalletAddress
	err = dao.Mdb.Transaction(func(tx *gorm.DB) error {
		if err := enabledMerchant(tx, owner); err != nil {
			return err
		}
		query := applyLockAddressFilter(tx.Unscoped().Where("network = ?", network), network, address)
		if err := query.Limit(1).Find(&row).Error; err != nil {
			return err
		}
		if row.ID > 0 {
			if !row.DeletedAt.Valid {
				return constant.WalletAddressAlreadyExists
			}
			if err := merchantAddressHistoryAllows(tx, &row, owner); err != nil {
				return err
			}
			return tx.Unscoped().Model(&row).Updates(map[string]interface{}{"deleted_at": nil, "api_key_id": owner, "status": mdb.TokenStatusEnable, "remark": remark, "source": source}).Error
		}
		row = mdb.WalletAddress{Network: network, Address: address, ApiKeyID: owner, Status: mdb.TokenStatusEnable, Remark: remark, Source: source}
		// Even a purged wallet may still be referenced by historical orders.
		if err := merchantAddressHistoryAllows(tx, &row, owner); err != nil {
			return err
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return nil, err
	}
	row.ApiKeyID = owner
	row.Remark = remark
	row.Source = source
	row.Status = mdb.TokenStatusEnable
	row.DeletedAt = gorm.DeletedAt{}
	return &row, nil
}

func UpdateMerchantWallet(id uint64, owner *uint64, remark *string) error {
	merchantWalletMu.Lock()
	defer merchantWalletMu.Unlock()
	return dao.Mdb.Transaction(func(tx *gorm.DB) error {
		var row mdb.WalletAddress
		if err := tx.Where("id = ?", id).Limit(1).Find(&row).Error; err != nil {
			return err
		}
		if row.ID == 0 {
			return constant.WalletNotFoundErr
		}
		fields := map[string]interface{}{}
		if owner != nil {
			if *owner != 0 && *owner != row.ApiKeyID {
				if err := enabledMerchant(tx, *owner); err != nil {
					return err
				}
			}
			if err := merchantAddressHistoryAllows(tx, &row, *owner); err != nil {
				return err
			}
			fields["api_key_id"] = *owner
		}
		if remark != nil {
			fields["remark"] = strings.TrimSpace(*remark)
		}
		if len(fields) == 0 {
			return nil
		}
		return tx.Model(&row).Updates(fields).Error
	})
}
