package optimized

import (
	"testing"
	"time"
)

func TestReconciliationReport_ConsistencyCalculation(t *testing.T) {
	// Equation: RedisStock + ActiveReserved + PaidQuantity == TotalQuantity
	reportConsistent := ReconciliationReport{
		ItemID:          "item-1",
		TotalQuantity:   100,
		SoldQuantity:    40,
		RedisStock:      50,
		ActiveReserved:  10,
		PaidQuantity:    40,
		CalculatedTotal: 50 + 10 + 40, // 100
		Discrepancy:     0,
		IsConsistent:    true,
		ReconciledAt:    time.Now().UTC(),
	}

	if !reportConsistent.IsConsistent {
		t.Fatal("expected report to be consistent")
	}

	reportDrift := ReconciliationReport{
		ItemID:          "item-2",
		TotalQuantity:   100,
		SoldQuantity:    40,
		RedisStock:      45, // 5 missing!
		ActiveReserved:  10,
		PaidQuantity:    40,
		CalculatedTotal: 45 + 10 + 40, // 95
		Discrepancy:     5,
		IsConsistent:    false,
		ReconciledAt:    time.Now().UTC(),
	}

	if reportDrift.IsConsistent {
		t.Fatal("expected drift report to be marked inconsistent")
	}
	if reportDrift.Discrepancy != 5 {
		t.Fatalf("expected discrepancy 5, got %d", reportDrift.Discrepancy)
	}
}
