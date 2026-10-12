package task

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/v03413/bepusdt/app/model"
	"gorm.io/gorm"
)

func initTestDB(t *testing.T) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "task-test.db")
	db, err := gorm.Open(sqlite.Open(dbPath+"?cache=shared&mode=rwc"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	model.Db = db
	if err := db.AutoMigrate(&model.Conf{}); err != nil {
		t.Fatalf("auto migrate conf: %v", err)
	}
	db.Create(&model.Conf{K: "atom_usdt", V: "0.01"})
}

func TestAmountMatch(t *testing.T) {
	initTestDB(t)

	tests := []struct {
		name      string
		mode      model.MatchMode
		amount    string
		target    string
		tradeType string
		want      bool
	}{
		// Classic mode (exact string match)
		{"classic exact match", model.Classic, "6.01", "6.01", "usdt.bep20", true},
		{"classic mismatch zero fraction", model.Classic, "6.01", "6", "usdt.bep20", false},
		{"classic mismatch different fraction", model.Classic, "6.01", "6.02", "usdt.bep20", false},

		// HasPrefix mode
		{"has_prefix exact", model.HasPrefix, "6.01", "6.01", "usdt.bep20", true},
		{"has_prefix overpay decimal", model.HasPrefix, "6.015", "6.01", "usdt.bep20", true},
		{"has_prefix underpay", model.HasPrefix, "6.01", "6.02", "usdt.bep20", false},

		// RoundOff mode
		{"round_off integer target vs 6.01 should not match", model.RoundOff, "6.01", "6", "usdt.bep20", false},
		{"round_off integer target vs 6.00 matches", model.RoundOff, "6.00", "6", "usdt.bep20", true},
		{"round_off 6.01 vs 6.01 matches", model.RoundOff, "6.01", "6.01", "usdt.bep20", true},
		{"round_off 6.014 rounded to 2 decimals matches 6.01", model.RoundOff, "6.014", "6.01", "usdt.bep20", true},
		{"round_off 6.016 rounded to 2 decimals mismatches 6.01", model.RoundOff, "6.016", "6.01", "usdt.bep20", false},
		{"round_off 6.02 vs 6.01 mismatches", model.RoundOff, "6.02", "6.01", "usdt.bep20", false},
		{"round_off 3 decimals precision target", model.RoundOff, "6.001", "6.001", "usdt.bep20", true},
		{"round_off 3 decimals precision target mismatch", model.RoundOff, "6.002", "6.001", "usdt.bep20", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model.SetC(model.PaymentMatchMode, string(tt.mode))
			amt, err := decimal.NewFromString(tt.amount)
			if err != nil {
				t.Fatalf("invalid amount: %v", err)
			}
			got := amountMatch(amt, tt.target, tt.tradeType)
			if got != tt.want {
				t.Errorf("amountMatch(%s, %s, %s) with mode %s = %v, want %v",
					tt.amount, tt.target, tt.tradeType, tt.mode, got, tt.want)
			}
		})
	}
}

func TestAmountMatchFeeTolerance(t *testing.T) {
	initTestDB(t)

	tests := []struct {
		name      string
		amount    string
		target    string
		tradeType string
		want      bool
	}{
		{"5.99 matches 6 (0.01 withdrawal fee deducted)", "5.99", "6", "usdt.bep20", true},
		{"5.98 does not match 6 (exceeds 0.01 atom tolerance)", "5.98", "6", "usdt.bep20", false},
		{"6.00 does not match 6 in fee-tolerance pass (handled by exact pass)", "6.00", "6", "usdt.bep20", false},
		{"6.01 never matches 6", "6.01", "6", "usdt.bep20", false},
		{"5.99 does not match 6.01", "5.99", "6.01", "usdt.bep20", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			amt, err := decimal.NewFromString(tt.amount)
			if err != nil {
				t.Fatalf("invalid amount: %v", err)
			}
			got := amountMatchFeeTolerance(amt, tt.target, tt.tradeType)
			if got != tt.want {
				t.Errorf("amountMatchFeeTolerance(%s, %s, %s) = %v, want %v",
					tt.amount, tt.target, tt.tradeType, got, tt.want)
			}
		})
	}
}

func TestMatchOrderIndexTwoPassPriority(t *testing.T) {
	initTestDB(t)
	model.SetC(model.PaymentMatchMode, string(model.Classic))

	now := time.Now()
	created := model.Datetime(now.Add(-5 * time.Minute))
	expired := now.Add(15 * time.Minute)
	wallet := "0xb0a41c6f047e2719acd7fd9c3b8119ebbfa50eed"

	orders := []model.Order{
		{
			OrderId:      "ORDER_6_00",
			TradeType:    model.UsdtBep20,
			Amount:       "6",
			Address:      wallet,
			MatchAddress: wallet,
			ExpiredAt:    expired,
			AutoTimeAt:   model.AutoTimeAt{CreatedAt: &created},
		},
		{
			OrderId:      "ORDER_6_01",
			TradeType:    model.UsdtBep20,
			Amount:       "6.01",
			Address:      wallet,
			MatchAddress: wallet,
			ExpiredAt:    expired,
			AutoTimeAt:   model.AutoTimeAt{CreatedAt: &created},
		},
	}

	mkTransfer := func(amtStr string) transfer {
		amt, _ := decimal.NewFromString(amtStr)
		return transfer{
			TradeType:   model.UsdtBep20,
			RecvAddress: wallet,
			Amount:      amt,
			Timestamp:   now,
		}
	}

	// 1. 5.99 USDT (6 USDT minus 0.01 withdrawal fee) -> matches ORDER_6_00 (idx 0)
	if idx, ok := matchOrderIndex(orders, mkTransfer("5.99")); !ok || idx != 0 {
		t.Fatalf("5.99 transfer got idx=%d ok=%v, want idx=0 ok=true", idx, ok)
	}

	// 2. 6.01 USDT -> matches ORDER_6_01 (idx 1), never stolen by ORDER_6_00
	if idx, ok := matchOrderIndex(orders, mkTransfer("6.01")); !ok || idx != 1 {
		t.Fatalf("6.01 transfer got idx=%d ok=%v, want idx=1 ok=true", idx, ok)
	}

	// 3. Even if ORDER_6_01 is placed before ORDER_6_00, a 6.00 transfer MUST match ORDER_6_00 via Pass 1 (exact match)
	reversed := []model.Order{orders[1], orders[0]}
	if idx, ok := matchOrderIndex(reversed, mkTransfer("6")); !ok || reversed[idx].OrderId != "ORDER_6_00" {
		t.Fatalf("6.00 transfer on reversed list matched idx=%d ok=%v, want ORDER_6_00", idx, ok)
	}
}
