package redis

import "fmt"

const (
	StockKeyPattern        = "flash_item:%s:stock"
	ReservedKeyPattern     = "flash_item:%s:reserved"
	UserKeyPattern         = "flash_item:%s:user:%s"
	IdempotencyUserPattern = "flash_item:%s:idempotency:%s:%s"
	ReservationPattern     = "flash_item:%s:reservation:%s"
)

func StockKey(item string) string      { return fmt.Sprintf(StockKeyPattern, item) }
func ReservedKey(item string) string   { return fmt.Sprintf(ReservedKeyPattern, item) }
func UserKey(item, user string) string { return fmt.Sprintf(UserKeyPattern, item, user) }
func IdempotencyUserKey(item, user, key string) string {
	return fmt.Sprintf(IdempotencyUserPattern, item, user, key)
}
func ReservationKey(item, reservation string) string {
	return fmt.Sprintf(ReservationPattern, item, reservation)
}
