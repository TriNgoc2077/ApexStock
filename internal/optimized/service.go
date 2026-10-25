package optimized

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"apexstock/internal/domain"
	"apexstock/internal/messaging"
	rediskeys "apexstock/internal/persistence/redis"
	"github.com/redis/go-redis/v9"
)

var (
	ErrItemNotFound        = errors.New("item not found")
	ErrOutOfStock          = errors.New("out of stock")
	ErrMaxPerUser          = errors.New("max per user")
	ErrIdempotencyConflict = errors.New("idempotency conflict")
)

const reserveLua = `-- atomic reserve; see internal/persistence/redis/lua/reserve_order.lua
local e=redis.call('HGETALL',KEYS[4]); if #e>0 then local f={}; for i=1,#e,2 do f[e[i]]=e[i+1] end; if f['fingerprint']~=ARGV[8] then return {-4,'IDEMPOTENCY_CONFLICT'} end; return {1,f['reservation_id'],f['order_id'],f['item_id'],f['user_id'],f['qty'],f['expires_at'],f['state'],f['total_amount'],f['created_at']} end
local s=tonumber(redis.call('GET',KEYS[1]) or '-1'); local q=tonumber(ARGV[1]); if s<0 then return {-1,'ITEM_NOT_FOUND'} end; if s<q then return {-2,'OUT_OF_STOCK',s} end
local u=tonumber(redis.call('GET',KEYS[3]) or '0'); if u+q>tonumber(ARGV[2]) then return {-3,'MAX_PER_USER_REACHED',u,ARGV[2]} end
redis.call('DECRBY',KEYS[1],q); redis.call('INCRBY',KEYS[3],q); redis.call('ZADD',KEYS[2],tonumber(ARGV[7]),ARGV[3]); redis.call('HSET',KEYS[5],'reservation_id',ARGV[3],'order_id',ARGV[4],'item_id',ARGV[5],'user_id',ARGV[6],'qty',ARGV[1],'state','reserved','expires_at',ARGV[7]); redis.call('HSET',KEYS[4],'fingerprint',ARGV[8],'reservation_id',ARGV[3],'order_id',ARGV[4],'item_id',ARGV[5],'user_id',ARGV[6],'qty',ARGV[1],'expires_at',ARGV[7],'state','reserved','total_amount',ARGV[9],'created_at',ARGV[10]); redis.call('EXPIRE',KEYS[4],86400); redis.call('EXPIRE',KEYS[5],86400); return {0,ARGV[3],ARGV[4],ARGV[5],ARGV[6],ARGV[1],ARGV[7],'reserved',ARGV[9],ARGV[10]}`
const releaseLua = `local s=redis.call('HGET',KEYS[4],'state'); if s==false or s=='released' or s=='confirmed' then return {1,s or 'missing'} end; local q=tonumber(redis.call('HGET',KEYS[4],'qty') or '0'); redis.call('INCRBY',KEYS[1],q); redis.call('DECRBY',KEYS[3],q); redis.call('ZREM',KEYS[2],ARGV[1]); redis.call('HSET',KEYS[4],'state','released'); return {0,'released',q}`
const confirmLua = `local s=redis.call('HGET',KEYS[2],'state'); if s=='confirmed' then return {1,'confirmed'} end; if s~='reserved' then return {1,s or 'missing'} end; redis.call('ZREM',KEYS[1],ARGV[1]); redis.call('HSET',KEYS[2],'state','confirmed'); return {0,'confirmed'}`

type item struct {
	ProductID  string
	Price      int64
	Max        int32
	Total      int32
	Sold       int32
	Start, End time.Time
	Status     string
}
type Service struct {
	db   *sql.DB
	rdb  *redis.Client
	nats *messaging.Client
	ttl  time.Duration
}

func NewService(db *sql.DB, rdb *redis.Client, nc *messaging.Client) *Service {
	return &Service{db: db, rdb: rdb, nats: nc, ttl: 5 * time.Minute}
}
func (s *Service) Create(ctx context.Context, req domain.CreateOrderRequest) (domain.OrderResponse, error) {
	var it item
	err := s.db.QueryRowContext(ctx, `SELECT fsi.product_id,fsi.sale_price,fsi.max_per_user,fsi.total_quantity,fsi.sold_quantity,fse.start_time,fse.end_time,fse.status::text FROM flash_sale_items fsi JOIN flash_sale_events fse ON fse.id=fsi.event_id WHERE fsi.id=$1`, req.FlashSaleItemID).Scan(&it.ProductID, &it.Price, &it.Max, &it.Total, &it.Sold, &it.Start, &it.End, &it.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.OrderResponse{}, ErrItemNotFound
	}
	if err != nil {
		return domain.OrderResponse{}, err
	}
	now := time.Now().UTC()
	if it.Status != "active" || now.Before(it.Start) || !now.Before(it.End) {
		return domain.OrderResponse{}, ErrItemNotFound
	}
	orderID := newID()
	reservationID := newID()
	expires := now.Add(s.ttl)
	fp := fmt.Sprintf("%s|%s|%d", req.FlashSaleItemID, req.UserID, req.Quantity)
	h := sha256.Sum256([]byte(fp))
	fingerprint := hex.EncodeToString(h[:])
	total := it.Price * int64(req.Quantity)
	created := now
	vals, err := s.rdb.Eval(ctx, reserveLua, []string{rediskeys.StockKey(req.FlashSaleItemID), rediskeys.ReservedKey(req.FlashSaleItemID), rediskeys.UserKey(req.FlashSaleItemID, req.UserID), rediskeys.IdempotencyUserKey(req.FlashSaleItemID, req.UserID, req.IdempotencyKey), rediskeys.ReservationKey(req.FlashSaleItemID, reservationID)}, req.Quantity, it.Max, reservationID, orderID, req.FlashSaleItemID, req.UserID, expires.Unix(), fingerprint, strconv.FormatInt(total, 10), created.Format(time.RFC3339Nano)).Result()
	if err != nil {
		return domain.OrderResponse{}, err
	}
	a, ok := vals.([]interface{})
	if !ok || len(a) < 2 {
		return domain.OrderResponse{}, fmt.Errorf("invalid reserve result")
	}
	code := fmt.Sprint(a[0])
	if code == "-1" {
		return domain.OrderResponse{}, ErrItemNotFound
	}
	if code == "-2" {
		return domain.OrderResponse{}, ErrOutOfStock
	}
	if code == "-3" {
		return domain.OrderResponse{}, ErrMaxPerUser
	}
	if code == "-4" {
		return domain.OrderResponse{}, ErrIdempotencyConflict
	}
	response := domain.OrderResponse{OrderID: fmt.Sprint(a[2]), ReservationID: fmt.Sprint(a[1]), UserID: fmt.Sprint(a[4]), FlashSaleItemID: fmt.Sprint(a[3]), Quantity: atoi32(a[5]), TotalAmount: atoi64(a[8]), Status: "pending_payment", PaymentStatus: "pending", ExpiresAt: time.Unix(atoi64(a[6]), 0).UTC(), CreatedAt: parseTime(fmt.Sprint(a[9])), Idempotent: code == "1"}
	if !response.Idempotent {
		event := messaging.OrderReserved{EventID: response.OrderID, OrderID: response.OrderID, ReservationID: response.ReservationID, UserID: response.UserID, ItemID: response.FlashSaleItemID, ProductID: it.ProductID, Quantity: response.Quantity, TotalAmount: response.TotalAmount, ExpiresAt: response.ExpiresAt, CreatedAt: response.CreatedAt, IdempotencyKey: req.IdempotencyKey}
		if err = s.nats.Publish(ctx, event); err != nil {
			_, _ = s.rdb.Eval(ctx, releaseLua, []string{rediskeys.StockKey(req.FlashSaleItemID), rediskeys.ReservedKey(req.FlashSaleItemID), rediskeys.UserKey(req.FlashSaleItemID, req.UserID), rediskeys.ReservationKey(req.FlashSaleItemID, response.ReservationID)}, response.ReservationID).Result()
			return domain.OrderResponse{}, err
		}
	}
	return response, nil
}
func (s *Service) Release(ctx context.Context, itemID, userID, reservationID string) error {
	_, err := s.rdb.Eval(ctx, releaseLua, []string{rediskeys.StockKey(itemID), rediskeys.ReservedKey(itemID), rediskeys.UserKey(itemID, userID), rediskeys.ReservationKey(itemID, reservationID)}, reservationID).Result()
	return err
}
func (s *Service) Confirm(ctx context.Context, itemID, reservationID string) error {
	_, err := s.rdb.Eval(ctx, confirmLua, []string{rediskeys.ReservedKey(itemID), rediskeys.ReservationKey(itemID, reservationID)}, reservationID).Result()
	return err
}
func atoi64(v interface{}) int64   { n, _ := strconv.ParseInt(fmt.Sprint(v), 10, 64); return n }
func atoi32(v interface{}) int32   { return int32(atoi64(v)) }
func parseTime(v string) time.Time { t, _ := time.Parse(time.RFC3339Nano, v); return t }
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fallback safe formatting
		return fmt.Sprintf("%08x-%04x-4000-8000-%012x", time.Now().UnixNano()>>32, time.Now().UnixNano()&65535, time.Now().UnixNano()&0xffffffffffff)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant RFC 4122
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
