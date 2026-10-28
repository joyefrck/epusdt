package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/GMWalletApp/epusdt/config"
	"github.com/GMWalletApp/epusdt/model/dao"
	"github.com/GMWalletApp/epusdt/model/data"
	"github.com/GMWalletApp/epusdt/model/mdb"
	"github.com/GMWalletApp/epusdt/model/request"
	"github.com/GMWalletApp/epusdt/model/service"
	"github.com/GMWalletApp/epusdt/util/constant"
	appLog "github.com/GMWalletApp/epusdt/util/log"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"os"
	"strings"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func digest(v interface{}) string {
	b, e := json.Marshal(v)
	must(e)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
func audit() map[string]interface{} {
	var keys []mdb.ApiKey
	must(dao.Mdb.Select("id,name,pid,status").Where("pid = ? AND name = ?", "1000", "大象网络").Find(&keys).Error)
	if len(keys) != 1 || keys[0].Status != 1 {
		panic("expected unique enabled 大象网络 / PID 1000")
	}
	var wallets []mdb.WalletAddress
	must(dao.Mdb.Unscoped().Order("id ASC").Find(&wallets).Error)
	type stableWallet struct {
		ID               uint64
		Network, Address string
		Status           int64
		Remark, Source   string
		Deleted          bool
	}
	stable := []stableWallet{}
	live, bound := 0, 0
	for _, w := range wallets {
		if !w.DeletedAt.Valid {
			live++
		}
		if w.ApiKeyID == keys[0].ID {
			bound++
		}
		stable = append(stable, stableWallet{w.ID, w.Network, w.Address, w.Status, w.Remark, w.Source, w.DeletedAt.Valid})
	}
	var count int64
	must(dao.Mdb.Unscoped().Model(&mdb.Orders{}).Count(&count).Error)
	return map[string]interface{}{"merchant_id": keys[0].ID, "merchant_pid": keys[0].Pid, "merchant_name": keys[0].Name, "wallet_total": len(wallets), "wallet_live": live, "wallet_bound_to_merchant": bound, "wallet_metadata_sha256": digest(stable), "orders_total": count}
}

func main() {
	if len(os.Args) < 4 {
		panic("usage: dbcheck ENV audit|preflight REPORT")
	}
	config.SetConfigPath(os.Args[1])
	config.Init()
	appLog.Sugar = zap.NewNop().Sugar()
	mode := os.Args[2]
	if mode == "preflight" && !strings.Contains(viper.GetString("mysql_database"), "epusdt_merchant_preflight_") {
		panic("preflight must use isolated clone database")
	}
	must(dao.MysqlInit())
	before := audit()
	if mode == "preflight" {
		must(dao.MdbTableInit())
		must(dao.RuntimeInit())
		migrated := audit()
		if migrated["wallet_metadata_sha256"] != before["wallet_metadata_sha256"] || migrated["orders_total"] != before["orders_total"] {
			panic("migration changed historical data")
		}
		if migrated["wallet_bound_to_merchant"] != before["wallet_live"] {
			panic("single merchant migration did not bind all live wallets")
		}
		for _, index := range []string{"orders_trade_id_uindex", "orders_order_id_uindex"} {
			if !dao.Mdb.Migrator().HasIndex(&mdb.Orders{}, index) {
				panic("missing order unique index")
			}
		}
		if !dao.Mdb.Migrator().HasIndex(&mdb.WalletAddress{}, "wallet_address_network_address_uindex") {
			panic("missing wallet unique index")
		}

		config.SettingsGetString = func(k string) string { return data.GetSettingString(k, "") }
		config.RateCacheLoad = data.LoadRateCacheSnapshot
		config.RateCacheLoadAll = data.ListRateCacheSnapshots
		config.RateCacheSave = data.SaveRateCacheSnapshot
		must(data.SetSetting("epay", mdb.SettingKeyEpayDefaultToken, "usdt", "string"))
		must(data.SetSetting("epay", mdb.SettingKeyEpayDefaultNetwork, "tron", "string"))
		must(data.SetSetting("epay", mdb.SettingKeyEpayDefaultCurrency, "cny", "string"))
		must(data.SetSetting("rate", "rate.forced_rate_list", `{"cny":{"usdt":1}}`, "json"))
		keys := []mdb.ApiKey{{Name: "clone-only-A", Pid: "clone-only-A", SecretKey: "synthetic-only", Status: 1}, {Name: "clone-only-B", Pid: "clone-only-B", SecretKey: "synthetic-only", Status: 1}}
		for i := range keys {
			must(dao.Mdb.Create(&keys[i]).Error)
			_, err := data.AddMerchantWallet("tron", fmt.Sprintf("TCloneOnlyMerchant%d", i), "preflight only", "manual", keys[i].ID)
			must(err)
		}
		for i := range keys {
			req := &request.CreateTransactionRequest{OrderId: fmt.Sprintf("clone-only-%d", i), Amount: 1, Currency: "cny", Token: "usdt", Network: "tron", NotifyUrl: "https://example.com/epusdt-preflight"}
			response, err := service.CreateTransaction(req, &keys[i])
			must(err)
			if response.ReceiveAddress != fmt.Sprintf("TCloneOnlyMerchant%d", i) {
				panic("cross-merchant allocation")
			}
			var owner mdb.Orders
			must(dao.Mdb.Where("trade_id = ?", response.TradeId).First(&owner).Error)
			if owner.ApiKeyID != keys[i].ID {
				panic("order ownership changed")
			}
		}
		if _, err := service.CreateTransaction(&request.CreateTransactionRequest{OrderId: "clone-no-identity", Amount: 1, Currency: "cny", Token: "usdt", Network: "tron", NotifyUrl: "https://example.com/epusdt-preflight"}, nil); err != constant.OrderApiKeyUnavailableErr {
			panic("missing merchant accepted")
		}
		after := audit()
		after["clone_payment_isolation_verified"] = true
		after["migration_verified"] = true
		after["before"] = before
		output(after, os.Args[3])
		return
	}
	if mode != "audit" {
		panic("invalid mode")
	}
	output(before, os.Args[3])
}
func output(v interface{}, path string) {
	b, e := json.MarshalIndent(v, "", "  ")
	must(e)
	must(os.WriteFile(path, append(b, '\n'), 0600))
	fmt.Println(string(b))
}
