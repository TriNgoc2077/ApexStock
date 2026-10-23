package redis

import "fmt"

const (
	StockKeyPattern    = "flash_item:%s:stock"
	ReservedKeyPattern = "flash_item:%s:reserved"
	UserKeyPattern     = "flash_item:%s:user:%s"
	EventMetaPattern   = "flash_event:%s:meta"
	IdempotencyPattern = "idempotency:%s"
	RateLimitPattern   = "rate_limit:%s:%s"
	SessionPattern     = "session:%s"
)

func StockKey(itemID string) string        { return fmt.Sprintf(StockKeyPattern, itemID) }
func ReservedKey(itemID string) string     { return fmt.Sprintf(ReservedKeyPattern, itemID) }
func UserKey(itemID, userID string) string { return fmt.Sprintf(UserKeyPattern, itemID, userID) }
func EventMetaKey(eventID string) string   { return fmt.Sprintf(EventMetaPattern, eventID) }
func IdempotencyKey(key string) string     { return fmt.Sprintf(IdempotencyPattern, key) }
