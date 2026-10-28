package admin

import (
	"github.com/GMWalletApp/epusdt/util/constant"
	"github.com/labstack/echo/v4"
	"strconv"
	"strings"
)

func parseMerchantFilter(ctx echo.Context) (*uint64, error) {
	value, exists := ctx.QueryParams()["api_key_id"]
	if !exists {
		return nil, nil
	}
	if len(value) != 1 || strings.TrimSpace(value[0]) == "" {
		return nil, constant.ParamsMarshalErr
	}
	id, err := strconv.ParseUint(value[0], 10, 64)
	if err != nil {
		return nil, constant.ParamsMarshalErr
	}
	return &id, nil
}
