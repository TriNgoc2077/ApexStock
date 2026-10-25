package optimized

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	rediskeys "apexstock/internal/persistence/redis"
	"github.com/redis/go-redis/v9"
)

func getTestRedis(t *testing.T) *redis.Client {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		if os.Getenv("INTEGRATION_TEST") != "1" {
			t.Skipf("Skipping Redis Lua test: Redis not reachable at %s (set INTEGRATION_TEST=1)", addr)
		}
		t.Fatalf("Failed to connect to Redis at %s: %v", addr, err)
	}
	return rdb
}

func computeFP(itemID, userID string, qty int32) string {
	fp := fmt.Sprintf("%s|%s|%d", itemID, userID, qty)
	h := sha256.Sum256([]byte(fp))
	return hex.EncodeToString(h[:])
}

func TestLuaReserve_StockDecrement(t *testing.T) {
	rdb := getTestRedis(t)
	ctx := context.Background()
	itemID := newID()
	userID := newID()
	orderID := newID()
	resID := newID()
	idempKey := "key-" + newID()
	fp := computeFP(itemID, userID, 1)

	// Seed stock = 10
	rdb.Set(ctx, rediskeys.StockKey(itemID), 10, 0)
	defer rdb.Del(ctx, rediskeys.StockKey(itemID), rediskeys.ReservedKey(itemID), rediskeys.UserKey(itemID, userID))

	keys := []string{
		rediskeys.StockKey(itemID),
		rediskeys.ReservedKey(itemID),
		rediskeys.UserKey(itemID, userID),
		rediskeys.IdempotencyUserKey(itemID, userID, idempKey),
		rediskeys.ReservationKey(itemID, resID),
	}
	expires := time.Now().Add(5 * time.Minute).Unix()

	res, err := rdb.Eval(ctx, reserveLua, keys, 1, 2, resID, orderID, itemID, userID, expires, fp, "100000", time.Now().Format(time.RFC3339Nano)).Result()
	if err != nil {
		t.Fatalf("Lua eval failed: %v", err)
	}

	vals := res.([]interface{})
	if fmt.Sprint(vals[0]) != "0" {
		t.Fatalf("expected code 0, got %v", vals[0])
	}

	// Verify stock is 9
	stock, _ := rdb.Get(ctx, rediskeys.StockKey(itemID)).Int()
	if stock != 9 {
		t.Fatalf("expected stock 9, got %d", stock)
	}
}

func TestLuaReserve_OutOfStock(t *testing.T) {
	rdb := getTestRedis(t)
	ctx := context.Background()
	itemID := newID()
	userID := newID()
	resID := newID()
	idempKey := "key-" + newID()
	fp := computeFP(itemID, userID, 1)

	rdb.Set(ctx, rediskeys.StockKey(itemID), 0, 0)
	defer rdb.Del(ctx, rediskeys.StockKey(itemID))

	keys := []string{
		rediskeys.StockKey(itemID),
		rediskeys.ReservedKey(itemID),
		rediskeys.UserKey(itemID, userID),
		rediskeys.IdempotencyUserKey(itemID, userID, idempKey),
		rediskeys.ReservationKey(itemID, resID),
	}

	res, err := rdb.Eval(ctx, reserveLua, keys, 1, 2, resID, newID(), itemID, userID, time.Now().Add(5*time.Minute).Unix(), fp, "100000", time.Now().Format(time.RFC3339Nano)).Result()
	if err != nil {
		t.Fatalf("Lua eval failed: %v", err)
	}
	vals := res.([]interface{})
	if fmt.Sprint(vals[0]) != "-2" {
		t.Fatalf("expected code -2 (OUT_OF_STOCK), got %v", vals[0])
	}
}

func TestLuaReserve_MaxPerUserReached(t *testing.T) {
	rdb := getTestRedis(t)
	ctx := context.Background()
	itemID := newID()
	userID := newID()
	resID := newID()
	idempKey := "key-" + newID()
	fp := computeFP(itemID, userID, 1)

	rdb.Set(ctx, rediskeys.StockKey(itemID), 10, 0)
	rdb.Set(ctx, rediskeys.UserKey(itemID, userID), 1, 0) // Max is 1
	defer rdb.Del(ctx, rediskeys.StockKey(itemID), rediskeys.UserKey(itemID, userID))

	keys := []string{
		rediskeys.StockKey(itemID),
		rediskeys.ReservedKey(itemID),
		rediskeys.UserKey(itemID, userID),
		rediskeys.IdempotencyUserKey(itemID, userID, idempKey),
		rediskeys.ReservationKey(itemID, resID),
	}

	res, err := rdb.Eval(ctx, reserveLua, keys, 1, 1, resID, newID(), itemID, userID, time.Now().Add(5*time.Minute).Unix(), fp, "100000", time.Now().Format(time.RFC3339Nano)).Result()
	if err != nil {
		t.Fatalf("Lua eval failed: %v", err)
	}
	vals := res.([]interface{})
	if fmt.Sprint(vals[0]) != "-3" {
		t.Fatalf("expected code -3 (MAX_PER_USER_REACHED), got %v", vals[0])
	}
}

func TestLuaReserve_Idempotency_SamePayload(t *testing.T) {
	rdb := getTestRedis(t)
	ctx := context.Background()
	itemID := newID()
	userID := newID()
	orderID := newID()
	resID := newID()
	idempKey := "key-" + newID()
	fp := computeFP(itemID, userID, 1)

	// Seed stock = 10
	rdb.Set(ctx, rediskeys.StockKey(itemID), 10, 0)
	defer rdb.Del(ctx, rediskeys.StockKey(itemID), rediskeys.ReservedKey(itemID), rediskeys.UserKey(itemID, userID))

	keys := []string{
		rediskeys.StockKey(itemID),
		rediskeys.ReservedKey(itemID),
		rediskeys.UserKey(itemID, userID),
		rediskeys.IdempotencyUserKey(itemID, userID, idempKey),
		rediskeys.ReservationKey(itemID, resID),
	}
	expires := time.Now().Add(5 * time.Minute).Unix()

	// First reserve: success
	res1, err := rdb.Eval(ctx, reserveLua, keys, 1, 2, resID, orderID, itemID, userID, expires, fp, "100000", time.Now().Format(time.RFC3339Nano)).Result()
	if err != nil {
		t.Fatalf("first Lua eval failed: %v", err)
	}
	vals1 := res1.([]interface{})
	if fmt.Sprint(vals1[0]) != "0" {
		t.Fatalf("expected code 0, got %v", vals1[0])
	}

	// Second reserve with same idempotency key and same payload (fingerprint)
	res2, err := rdb.Eval(ctx, reserveLua, keys, 1, 2, newID(), newID(), itemID, userID, expires, fp, "100000", time.Now().Format(time.RFC3339Nano)).Result()
	if err != nil {
		t.Fatalf("second Lua eval failed: %v", err)
	}
	vals2 := res2.([]interface{})
	if fmt.Sprint(vals2[0]) != "1" {
		t.Fatalf("expected code 1 (idempotent replay), got %v", vals2[0])
	}
	if fmt.Sprint(vals2[1]) != resID {
		t.Fatalf("expected reservation ID %s, got %v", resID, vals2[1])
	}
	if fmt.Sprint(vals2[2]) != orderID {
		t.Fatalf("expected order ID %s, got %v", orderID, vals2[2])
	}

	// Stock must NOT decrement twice (should be 9, not 8)
	stock, _ := rdb.Get(ctx, rediskeys.StockKey(itemID)).Int()
	if stock != 9 {
		t.Fatalf("expected stock 9, got %d", stock)
	}
}

func TestLuaReserve_IdempotencyConflict_FingerprintMismatch(t *testing.T) {
	rdb := getTestRedis(t)
	ctx := context.Background()
	itemID := newID()
	userID := newID()
	resID1 := newID()
	idempKey := "key-" + newID()
	fp1 := computeFP(itemID, userID, 1)

	rdb.Set(ctx, rediskeys.StockKey(itemID), 10, 0)
	defer rdb.Del(ctx, rediskeys.StockKey(itemID))

	keys := []string{
		rediskeys.StockKey(itemID),
		rediskeys.ReservedKey(itemID),
		rediskeys.UserKey(itemID, userID),
		rediskeys.IdempotencyUserKey(itemID, userID, idempKey),
		rediskeys.ReservationKey(itemID, resID1),
	}

	// First request succeeds
	_, err := rdb.Eval(ctx, reserveLua, keys, 1, 2, resID1, newID(), itemID, userID, time.Now().Add(5*time.Minute).Unix(), fp1, "100000", time.Now().Format(time.RFC3339Nano)).Result()
	if err != nil {
		t.Fatalf("First reserve failed: %v", err)
	}

	// Second request with SAME idempotency key but DIFFERENT fingerprint (e.g. qty=2)
	fp2 := computeFP(itemID, userID, 2)
	res2, err := rdb.Eval(ctx, reserveLua, keys, 2, 2, newID(), newID(), itemID, userID, time.Now().Add(5*time.Minute).Unix(), fp2, "200000", time.Now().Format(time.RFC3339Nano)).Result()
	if err != nil {
		t.Fatalf("Second reserve failed: %v", err)
	}
	vals := res2.([]interface{})
	if fmt.Sprint(vals[0]) != "-4" {
		t.Fatalf("expected code -4 (IDEMPOTENCY_CONFLICT), got %v", vals[0])
	}
}

func TestLuaReserve_MissingStockKey_ReturnsItemNotFound(t *testing.T) {
	rdb := getTestRedis(t)
	ctx := context.Background()
	itemID := newID()
	userID := newID()
	resID := newID()
	idempKey := "key-" + newID()
	fp := computeFP(itemID, userID, 1)

	// Ensure stock key does NOT exist
	rdb.Del(ctx, rediskeys.StockKey(itemID))

	keys := []string{
		rediskeys.StockKey(itemID),
		rediskeys.ReservedKey(itemID),
		rediskeys.UserKey(itemID, userID),
		rediskeys.IdempotencyUserKey(itemID, userID, idempKey),
		rediskeys.ReservationKey(itemID, resID),
	}

	res, err := rdb.Eval(ctx, reserveLua, keys, 1, 2, resID, newID(), itemID, userID, time.Now().Add(5*time.Minute).Unix(), fp, "100000", time.Now().Format(time.RFC3339Nano)).Result()
	if err != nil {
		t.Fatalf("Lua eval failed: %v", err)
	}
	vals := res.([]interface{})
	if fmt.Sprint(vals[0]) != "-1" {
		t.Fatalf("expected code -1 (ITEM_NOT_FOUND), got %v", vals[0])
	}
}

func TestLuaRelease_RestoresStockAndIsIdempotent(t *testing.T) {
	rdb := getTestRedis(t)
	ctx := context.Background()
	itemID := newID()
	userID := newID()
	resID := newID()

	rdb.Set(ctx, rediskeys.StockKey(itemID), 9, 0)
	rdb.Set(ctx, rediskeys.UserKey(itemID, userID), 1, 0)
	rdb.HSet(ctx, rediskeys.ReservationKey(itemID, resID), "qty", 1, "state", "reserved")
	rdb.ZAdd(ctx, rediskeys.ReservedKey(itemID), redis.Z{Score: float64(time.Now().Unix()), Member: resID})

	keys := []string{
		rediskeys.StockKey(itemID),
		rediskeys.ReservedKey(itemID),
		rediskeys.UserKey(itemID, userID),
		rediskeys.ReservationKey(itemID, resID),
	}

	// First release restores stock
	res1, err := rdb.Eval(ctx, releaseLua, keys, resID).Result()
	if err != nil {
		t.Fatalf("First release failed: %v", err)
	}
	vals1 := res1.([]interface{})
	if fmt.Sprint(vals1[0]) != "0" {
		t.Fatalf("expected code 0, got %v", vals1[0])
	}
	stock, _ := rdb.Get(ctx, rediskeys.StockKey(itemID)).Int()
	if stock != 10 {
		t.Fatalf("expected restored stock 10, got %d", stock)
	}

	// Second release is idempotent no-op
	res2, err := rdb.Eval(ctx, releaseLua, keys, resID).Result()
	if err != nil {
		t.Fatalf("Second release failed: %v", err)
	}
	vals2 := res2.([]interface{})
	if fmt.Sprint(vals2[0]) != "1" {
		t.Fatalf("expected code 1 (idempotent no-op), got %v", vals2[0])
	}
	stockAfter, _ := rdb.Get(ctx, rediskeys.StockKey(itemID)).Int()
	if stockAfter != 10 {
		t.Fatalf("stock drift detected: expected 10, got %d", stockAfter)
	}
}

func TestLuaConfirm_MarksConfirmedAndIsIdempotent(t *testing.T) {
	rdb := getTestRedis(t)
	ctx := context.Background()
	itemID := newID()
	resID := newID()

	rdb.HSet(ctx, rediskeys.ReservationKey(itemID, resID), "qty", 1, "state", "reserved")
	rdb.ZAdd(ctx, rediskeys.ReservedKey(itemID), redis.Z{Score: float64(time.Now().Unix()), Member: resID})

	keys := []string{
		rediskeys.ReservedKey(itemID),
		rediskeys.ReservationKey(itemID, resID),
	}

	// First confirm
	res1, err := rdb.Eval(ctx, confirmLua, keys, resID).Result()
	if err != nil {
		t.Fatalf("First confirm failed: %v", err)
	}
	vals1 := res1.([]interface{})
	if fmt.Sprint(vals1[0]) != "0" {
		t.Fatalf("expected code 0, got %v", vals1[0])
	}

	// Second confirm: idempotent
	res2, err := rdb.Eval(ctx, confirmLua, keys, resID).Result()
	if err != nil {
		t.Fatalf("Second confirm failed: %v", err)
	}
	vals2 := res2.([]interface{})
	if fmt.Sprint(vals2[0]) != "1" {
		t.Fatalf("expected code 1 (idempotent no-op), got %v", vals2[0])
	}
}

func TestLuaConfirm_OnReleasedOrder(t *testing.T) {
	rdb := getTestRedis(t)
	ctx := context.Background()
	itemID := newID()
	resID := newID()

	rdb.HSet(ctx, rediskeys.ReservationKey(itemID, resID), "qty", 1, "state", "released")
	defer rdb.Del(ctx, rediskeys.ReservationKey(itemID, resID))

	keys := []string{
		rediskeys.ReservedKey(itemID),
		rediskeys.ReservationKey(itemID, resID),
	}

	res, err := rdb.Eval(ctx, confirmLua, keys, resID).Result()
	if err != nil {
		t.Fatalf("Confirm on released order failed: %v", err)
	}
	vals := res.([]interface{})
	if fmt.Sprint(vals[0]) != "1" {
		t.Fatalf("expected code 1 (noop), got %v", vals[0])
	}
	if fmt.Sprint(vals[1]) != "released" {
		t.Fatalf("expected state 'released', got %v", vals[1])
	}

	// State in Redis remains 'released'
	state, _ := rdb.HGet(ctx, rediskeys.ReservationKey(itemID, resID), "state").Result()
	if state != "released" {
		t.Fatalf("expected reservation state to remain 'released', got %s", state)
	}
}
