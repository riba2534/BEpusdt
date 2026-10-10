package task

import (
	"path/filepath"
	"testing"

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
		// USDT bep20 standard precision is 2 decimals (0.01).
		// Target "6" has precision 0, but standard precision is 2. Max(2, 0) = 2.
		// Amount 6.01 rounded to 2 decimals is 6.01, target 6 rounded to 2 decimals is 6.00.
		// They must NOT match!
		{"round_off integer target vs 6.01 should not match", model.RoundOff, "6.01", "6", "usdt.bep20", false},
		{"round_off integer target vs 6.00 matches", model.RoundOff, "6.00", "6", "usdt.bep20", true},
		{"round_off 6.01 vs 6.01 matches", model.RoundOff, "6.01", "6.01", "usdt.bep20", true},
		{"round_off 6.014 rounded to 2 decimals matches 6.01", model.RoundOff, "6.014", "6.01", "usdt.bep20", true},
		{"round_off 6.016 rounded to 2 decimals mismatches 6.01", model.RoundOff, "6.016", "6.01", "usdt.bep20", false},
		{"round_off 6.02 vs 6.01 mismatches", model.RoundOff, "6.02", "6.01", "usdt.bep20", false},
		// When target has higher precision than standard precision (3 decimals > 2 decimals)
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
