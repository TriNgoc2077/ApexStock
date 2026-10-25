package payments

import (
	"context"
	"database/sql"
	"log"
	"time"
)

type ExpirySweeper struct {
	db        *sql.DB
	processor PaymentProcessor
}

func NewExpirySweeper(db *sql.DB, processor PaymentProcessor) *ExpirySweeper {
	return &ExpirySweeper{
		db:        db,
		processor: processor,
	}
}

// SweepOnce queries expired pending orders using FOR UPDATE SKIP LOCKED and cancels them.
func (s *ExpirySweeper) SweepOnce(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM orders
		WHERE status = 'pending_payment' AND expires_at <= now()
		ORDER BY expires_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, batchSize)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var orderIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			orderIDs = append(orderIDs, id)
		}
	}
	if err = rows.Err(); err != nil {
		return 0, err
	}

	sweptCount := 0
	for _, id := range orderIDs {
		_, err := s.processor.Process(ctx, ProcessPaymentRequest{
			OrderID:  id,
			Outcome:  OutcomeTimeout,
			Provider: "scheduler",
		})
		if err == nil {
			sweptCount++
		}
	}

	return sweptCount, nil
}

// Start runs the sweeper on a periodic interval until ctx is cancelled.
func (s *ExpirySweeper) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			count, err := s.SweepOnce(ctx, 100)
			if err != nil {
				log.Printf("[scheduler] sweep error: %v", err)
			} else if count > 0 {
				log.Printf("[scheduler] swept %d expired orders", count)
			}
		}
	}
}
