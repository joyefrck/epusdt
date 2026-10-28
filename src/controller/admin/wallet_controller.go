package admin

import (
	"strconv"
	"strings"

	"github.com/GMWalletApp/epusdt/model/dao"
	"github.com/GMWalletApp/epusdt/model/data"
	"github.com/GMWalletApp/epusdt/model/mdb"
	"github.com/GMWalletApp/epusdt/util/constant"
	"github.com/labstack/echo/v4"
)

// AdminAddWalletRequest is the payload for adding a wallet via admin.
type AdminAddWalletRequest struct {
	ApiKeyID uint64 `json:"api_key_id" validate:"required"`
	Network  string `json:"network" validate:"required" example:"tron"`
	Address  string `json:"address" validate:"required" example:"TTestTronAddress001"`
	Remark   string `json:"remark" example:"主钱包"`
}

// AdminUpdateWalletRequest patches merchant ownership and wallet remark.
type AdminUpdateWalletRequest struct {
	ApiKeyID *uint64 `json:"api_key_id"`
	Remark   *string `json:"remark" example:"已更新的备注"`
}

// AdminChangeWalletStatusRequest is the payload for toggling wallet status.
type AdminChangeWalletStatusRequest struct {
	// 状态 1=启用 2=禁用
	Status int `json:"status" validate:"required|in:1,2" enums:"1,2" example:"1"`
}

// AdminBatchImportRequest is the payload for batch importing wallets.
type AdminBatchImportRequest struct {
	ApiKeyID uint64 `json:"api_key_id" validate:"required"`
	Network  string `json:"network" validate:"required" example:"tron"`
	// Addresses listed as observation-only wallets. Private-key import
	// and API-managed modes are out of scope for v1.
	Addresses []string `json:"addresses" validate:"required" example:"TAddr001,TAddr002,TAddr003"`
}

// WalletListItem extends WalletAddress with order count.
type WalletListItem struct {
	mdb.WalletAddress
	OrderCount      int64  `json:"order_count" example:"5"`
	MerchantName    string `json:"merchant_name"`
	MerchantPID     string `json:"merchant_pid"`
	MerchantDeleted bool   `json:"merchant_deleted"`
}

// AdminListWallets extends the read model with per-wallet order counts
// so the UI can render the "订单数" column without an N+1 follow-up.
// @Summary      List wallets (admin)
// @Description  Returns wallets with per-wallet order counts
// @Tags         Admin Wallets
// @Security     AdminJWT
// @Produce      json
// @Param        network query string false "Network filter"
// @Param        api_key_id query int false "Merchant Key ID (0=unbound, omitted=all)"
// @Param        status query int false "Status filter (1=enabled, 2=disabled)"
// @Param        keyword query string false "Search by address or remark"
// @Success      200 {object} response.ApiResponse{data=[]admin.WalletListItem}
// @Failure      400 {object} response.ApiResponse
// @Router       /admin/api/v1/wallets [get]
func (c *BaseAdminController) AdminListWallets(ctx echo.Context) error {
	merchant, err := parseMerchantFilter(ctx)
	if err != nil {
		return c.FailJson(ctx, err)
	}
	network := strings.ToLower(strings.TrimSpace(ctx.QueryParam("network")))
	status := strings.TrimSpace(ctx.QueryParam("status"))
	keyword := strings.TrimSpace(ctx.QueryParam("keyword"))

	tx := dao.Mdb.Model(&mdb.WalletAddress{})
	if merchant != nil {
		tx = tx.Where("api_key_id = ?", *merchant)
	}
	if network != "" {
		tx = tx.Where("network = ?", network)
	}
	if status != "" {
		if s, err := strconv.Atoi(status); err == nil {
			tx = tx.Where("status = ?", s)
		}
	}
	if keyword != "" {
		kw := "%" + keyword + "%"
		tx = tx.Where("address LIKE ? OR remark LIKE ?", kw, kw)
	}
	var rows []mdb.WalletAddress
	if err := tx.Order("id DESC").Find(&rows).Error; err != nil {
		return c.FailJson(ctx, err)
	}

	out, err := walletListItems(rows)
	if err != nil {
		return c.FailJson(ctx, err)
	}
	return c.SucJson(ctx, out)
}

// AdminAddWallet wraps the existing create with admin-exposed Remark +
// Source=manual so audit fields are populated.
// @Summary      Add wallet (admin)
// @Description  Create a wallet with admin-specific fields (remark, source=manual)
// @Tags         Admin Wallets
// @Security     AdminJWT
// @Accept       json
// @Produce      json
// @Param        request body admin.AdminAddWalletRequest true "Wallet payload"
// @Success      200 {object} response.ApiResponse{data=mdb.WalletAddress}
// @Failure      400 {object} response.ApiResponse
// @Router       /admin/api/v1/wallets [post]
func (c *BaseAdminController) AdminAddWallet(ctx echo.Context) error {
	req := new(AdminAddWalletRequest)
	if err := ctx.Bind(req); err != nil {
		return c.FailJson(ctx, constant.ParamsMarshalErr)
	}
	if err := c.ValidateStruct(ctx, req); err != nil {
		return c.FailJson(ctx, err)
	}
	row, err := data.AddMerchantWallet(req.Network, req.Address, req.Remark, mdb.WalletSourceManual, req.ApiKeyID)
	if err != nil {
		return c.FailJson(ctx, err)
	}
	return c.SucJson(ctx, row)
}

// AdminGetWallet returns detail including order count for one wallet.
// @Summary      Get wallet (admin)
// @Description  Returns wallet detail with order count
// @Tags         Admin Wallets
// @Security     AdminJWT
// @Produce      json
// @Param        id path int true "Wallet ID"
// @Success      200 {object} response.ApiResponse{data=admin.WalletListItem}
// @Failure      400 {object} response.ApiResponse
// @Router       /admin/api/v1/wallets/{id} [get]
func (c *BaseAdminController) AdminGetWallet(ctx echo.Context) error {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		return c.FailJson(ctx, constant.ParamsMarshalErr)
	}
	wallet, err := data.GetWalletAddressById(id)
	if err != nil {
		return c.FailJson(ctx, err)
	}
	if wallet.ID == 0 {
		return c.FailJson(ctx, constant.WalletNotFoundErr)
	}
	items, err := walletListItems([]mdb.WalletAddress{*wallet})
	if err != nil {
		return c.FailJson(ctx, err)
	}
	return c.SucJson(ctx, items[0])
}

// AdminUpdateWallet patches merchant binding and remark (status has its own endpoint).
// @Summary      Update wallet (admin)
// @Description  Patch wallet merchant binding and remark
// @Tags         Admin Wallets
// @Security     AdminJWT
// @Accept       json
// @Produce      json
// @Param        id path int true "Wallet ID"
// @Param        request body admin.AdminUpdateWalletRequest true "Fields to update"
// @Success      200 {object} response.ApiResponse
// @Failure      400 {object} response.ApiResponse
// @Router       /admin/api/v1/wallets/{id} [patch]
func (c *BaseAdminController) AdminUpdateWallet(ctx echo.Context) error {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		return c.FailJson(ctx, constant.ParamsMarshalErr)
	}
	req := new(AdminUpdateWalletRequest)
	if err := ctx.Bind(req); err != nil {
		return c.FailJson(ctx, constant.ParamsMarshalErr)
	}
	if err := data.UpdateMerchantWallet(id, req.ApiKeyID, req.Remark); err != nil {
		return c.FailJson(ctx, err)
	}
	return c.SucJson(ctx, nil)
}

// AdminChangeWalletStatus reuses existing helper.
// @Summary      Change wallet status (admin)
// @Description  Toggle enable/disable for a wallet (1=enabled, 2=disabled)
// @Tags         Admin Wallets
// @Security     AdminJWT
// @Accept       json
// @Produce      json
// @Param        id path int true "Wallet ID"
// @Param        request body admin.AdminChangeWalletStatusRequest true "Status payload"
// @Success      200 {object} response.ApiResponse
// @Failure      400 {object} response.ApiResponse
// @Router       /admin/api/v1/wallets/{id}/status [post]
func (c *BaseAdminController) AdminChangeWalletStatus(ctx echo.Context) error {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		return c.FailJson(ctx, constant.ParamsMarshalErr)
	}
	req := new(AdminChangeWalletStatusRequest)
	if err := ctx.Bind(req); err != nil {
		return c.FailJson(ctx, constant.ParamsMarshalErr)
	}
	if err := c.ValidateStruct(ctx, req); err != nil {
		return c.FailJson(ctx, err)
	}
	if err := data.ChangeWalletAddressStatus(id, req.Status); err != nil {
		return c.FailJson(ctx, err)
	}
	return c.SucJson(ctx, nil)
}

// AdminDeleteWallet soft-deletes the wallet.
// @Summary      Delete wallet (admin)
// @Description  Soft-delete a wallet
// @Tags         Admin Wallets
// @Security     AdminJWT
// @Produce      json
// @Param        id path int true "Wallet ID"
// @Success      200 {object} response.ApiResponse
// @Failure      400 {object} response.ApiResponse
// @Router       /admin/api/v1/wallets/{id} [delete]
func (c *BaseAdminController) AdminDeleteWallet(ctx echo.Context) error {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err != nil {
		return c.FailJson(ctx, constant.ParamsMarshalErr)
	}
	if err := data.DeleteWalletAddressById(id); err != nil {
		return c.FailJson(ctx, err)
	}
	return c.SucJson(ctx, nil)
}

// AdminBatchImportWallets accepts a list of addresses and creates
// them all as observation wallets. Per-row failures are collected and
// returned so the UI can surface which addresses already existed.
// @Summary      Batch import wallets
// @Description  Import multiple wallet addresses at once. Per-row status is returned; failed rows include error_code for frontend i18n.
// @Tags         Admin Wallets
// @Security     AdminJWT
// @Accept       json
// @Produce      json
// @Param        request body admin.AdminBatchImportRequest true "Batch import payload"
// @Success      200 {object} response.ApiResponse{data=[]admin.BatchImportResult}
// @Failure      400 {object} response.ApiResponse
// @Router       /admin/api/v1/wallets/batch-import [post]
func (c *BaseAdminController) AdminBatchImportWallets(ctx echo.Context) error {
	req := new(AdminBatchImportRequest)
	if err := ctx.Bind(req); err != nil {
		return c.FailJson(ctx, constant.ParamsMarshalErr)
	}
	if err := c.ValidateStruct(ctx, req); err != nil {
		return c.FailJson(ctx, err)
	}
	type result struct {
		Address   string `json:"address"`
		OK        bool   `json:"ok"`
		ErrorCode int    `json:"error_code,omitempty"`
		Error     string `json:"error,omitempty"`
	}
	out := make([]result, 0, len(req.Addresses))
	for _, addr := range req.Addresses {
		_, err := data.AddMerchantWallet(req.Network, addr, "", mdb.WalletSourceImport, req.ApiKeyID)
		if err != nil {
			code, msg := constant.ResolveErrno(err)
			out = append(out, result{Address: addr, OK: false, ErrorCode: code, Error: msg})
			continue
		}
		out = append(out, result{Address: addr, OK: true})
	}
	return c.SucJson(ctx, out)
}

// Fetch names in one query, including deleted keys for historical ownership.
func walletListItems(rows []mdb.WalletAddress) ([]WalletListItem, error) {
	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		if row.ApiKeyID > 0 {
			ids = append(ids, row.ApiKeyID)
		}
	}
	keys := map[uint64]mdb.ApiKey{}
	if len(ids) > 0 {
		var merchants []mdb.ApiKey
		if err := dao.Mdb.Unscoped().Select("id, name, pid, deleted_at").Where("id IN ?", ids).Find(&merchants).Error; err != nil {
			return nil, err
		}
		for _, key := range merchants {
			keys[key.ID] = key
		}
	}
	counts, err := data.CountOrdersByAddress()
	if err != nil {
		return nil, err
	}
	out := make([]WalletListItem, 0, len(rows))
	for _, row := range rows {
		key := keys[row.ApiKeyID]
		out = append(out, WalletListItem{WalletAddress: row, OrderCount: counts[row.Address], MerchantName: key.Name, MerchantPID: key.Pid, MerchantDeleted: key.DeletedAt.Valid})
	}
	return out, nil
}
