export type Architecture = 'baseline' | 'optimized';

export interface FlashSaleItem {
  id: string;
  event_id: string;
  product_id: string;
  product_name: string;
  product_description: string;
  product_image: string;
  original_price: number;
  sale_price: number;
  initial_quantity: number;
  available_stock: number;
  max_per_user: number;
}

export interface FlashSaleEvent {
  id: string;
  name: string;
  start_time: string;
  end_time: string;
  status: string;
  items: FlashSaleItem[];
}

export interface CreateOrderRequest {
  user_id: string;
  flash_sale_item_id: string;
  quantity: 1;
  idempotency_key: string;
}

export interface CreateOrderResponse {
  order_id: string;
  reservation_id?: string;
  user_id: string;
  flash_sale_item_id: string;
  quantity: number;
  total_amount: number;
  status: string;
  payment_status: string;
  expires_at: string;
  created_at: string;
  idempotent?: boolean;
}

export interface OrderDetail {
  order_id: string;
  user_id: string;
  status: 'pending_payment' | 'paid' | 'payment_failed' | 'expired' | 'cancelled';
  total_amount: number;
  expires_at: string;
  created_at: string;
  item_id: string;
  product_name: string;
  product_image: string;
  quantity: number;
  unit_price: number;
  payment_status: 'pending' | 'success' | 'failed' | 'refund';
  payment_method: string;
  transaction_ref?: string;
}

export interface InitiatePaymentResponse {
  order_id: string;
  qr_url: string;
  account_number: string;
  bank_code: string;
  amount: number;
  transfer_content: string;
}

export interface PaymentResultResponse {
  order_id: string;
  order_status: string;
  payment_status: string;
  idempotent_replay: boolean;
}

export function getBaseURL(arch: Architecture): string {
  if (typeof window !== 'undefined') {
    // Client-side: route through local dev ports
    return arch === 'baseline' ? 'http://localhost:8081' : 'http://localhost:8082';
  }
  return arch === 'baseline' ? 'http://api-baseline:8081' : 'http://nginx:8082';
}

// In-memory stable idempotency cache per checkout attempt
const idempotencyKeyCache = new Map<string, string>();

export function getOrCreateIdempotencyKey(itemId: string, userId: string): string {
  const cacheKey = `${userId}:${itemId}`;
  let key = idempotencyKeyCache.get(cacheKey);
  if (!key) {
    key = `checkout-${Date.now()}-${Math.random().toString(36).substring(2, 9)}`;
    idempotencyKeyCache.set(cacheKey, key);
  }
  return key;
}

export function clearIdempotencyKey(itemId: string, userId: string) {
  idempotencyKeyCache.delete(`${userId}:${itemId}`);
}

export const api = {
  async getEvents(arch: Architecture = 'optimized'): Promise<FlashSaleEvent[]> {
    const res = await fetch(`${getBaseURL(arch)}/api/v1/flash-sale/events`, { cache: 'no-store' });
    if (!res.ok) throw new Error(`Failed to fetch events: ${res.statusText}`);
    return res.json();
  },

  async getItemDetail(id: string, arch: Architecture = 'optimized'): Promise<FlashSaleItem> {
    const res = await fetch(`${getBaseURL(arch)}/api/v1/flash-sale/items/${id}`, { cache: 'no-store' });
    if (!res.ok) throw new Error(`Failed to fetch item detail: ${res.statusText}`);
    return res.json();
  },

  async createOrder(req: CreateOrderRequest, arch: Architecture): Promise<CreateOrderResponse> {
    const res = await fetch(`${getBaseURL(arch)}/api/v1/flash-sale/orders`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Idempotency-Key': req.idempotency_key,
        'X-User-ID': req.user_id,
      },
      body: JSON.stringify(req),
    });
    const data = await res.json();
    if (!res.ok) {
      const errorMsg = data?.message || data?.error || 'Tạo đơn hàng thất bại';
      throw new Error(errorMsg);
    }
    return data;
  },

  async getOrderDetail(orderId: string, arch: Architecture): Promise<OrderDetail> {
    const res = await fetch(`${getBaseURL(arch)}/api/v1/orders/${orderId}`, { cache: 'no-store' });
    if (!res.ok) throw new Error(`Failed to fetch order: ${res.statusText}`);
    return res.json();
  },

  async initiatePayment(orderId: string, arch: Architecture): Promise<InitiatePaymentResponse> {
    const res = await fetch(`${getBaseURL(arch)}/api/v1/payments/sepay/initiate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ order_id: orderId }),
    });
    if (!res.ok) throw new Error('Không thể tạo mã thanh toán SePay');
    return res.json();
  },

  async simulatePayment(orderId: string, outcome: string, arch: Architecture): Promise<PaymentResultResponse> {
    const res = await fetch(`${getBaseURL(arch)}/api/v1/payments/simulate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ order_id: orderId, outcome }),
    });
    if (!res.ok) throw new Error('Mô phỏng thanh toán thất bại');
    return res.json();
  },
};
