package mdb

import (
	"reflect"
	"strings"
	"testing"
)

func TestProductionIndexColumnSizes(t *testing.T) {
	tests := []struct {
		model interface{}
		field string
		want  string
	}{
		{Orders{}, "TradeId", "size:64"},
		{Orders{}, "OrderId", "size:128"},
		{WalletAddress{}, "Network", "size:32"},
		{WalletAddress{}, "Address", "size:128"},
	}
	for _, tt := range tests {
		field, ok := reflect.TypeOf(tt.model).FieldByName(tt.field)
		if !ok {
			t.Fatalf("field %s not found", tt.field)
		}
		if tag := field.Tag.Get("gorm"); !strings.Contains(tag, tt.want) {
			t.Fatalf("%T.%s gorm tag = %q, want %q", tt.model, tt.field, tag, tt.want)
		}
	}
}
