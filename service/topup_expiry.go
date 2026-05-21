package service

import (
	"context"

	payment "github.com/QuantumNous/new-api/service/payment"
)

func CloseExpiredPendingTopUps(ctx context.Context) (int, int, error) {
	return payment.CloseExpiredPendingTopUps(ctx)
}
