package baseline

import (
	"context"
	"database/sql"
	"time"

	"apexstock/internal/domain"
	"apexstock/internal/persistence/postgres"
)

type OrderService struct {
	repo *postgres.OrderRepository
	ttl  time.Duration
}

func NewOrderService(db *sql.DB) *OrderService {
	return &OrderService{repo: postgres.NewOrderRepository(db), ttl: 5 * time.Minute}
}

func (s *OrderService) Create(ctx context.Context, req domain.CreateOrderRequest) (domain.OrderResponse, error) {
	return s.repo.Create(ctx, req, s.ttl)
}
