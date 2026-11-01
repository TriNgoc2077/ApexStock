'use client';

import React, { useState } from 'react';
import { useRouter } from 'next/navigation';
import { useCart } from '@/context/CartContext';
import { useAuth } from '@/context/AuthContext';
import { api, Architecture, getOrCreateIdempotencyKey, clearIdempotencyKey } from '@/lib/api';

interface CartDrawerProps {
  arch: Architecture;
}

export default function CartDrawer({ arch }: CartDrawerProps) {
  const router = useRouter();
  const { items, isCartOpen, setIsCartOpen, updateQuantity, removeFromCart, totalAmount, totalItems } = useCart();
  const { user } = useAuth();

  const [couponCode, setCouponCode] = useState('');
  const [discountAmount, setDiscountAmount] = useState(0);
  const [couponMsg, setCouponMsg] = useState<string | null>(null);
  const [checkingOut, setCheckingOut] = useState(false);
  const [checkoutError, setCheckoutError] = useState<string | null>(null);

  if (!isCartOpen) return null;

  const handleApplyCoupon = (e: React.FormEvent) => {
    e.preventDefault();
    if (couponCode.toUpperCase() === 'APEX2026' || couponCode.toUpperCase() === 'FLASH500') {
      setDiscountAmount(500000);
      setCouponMsg('✓ Áp dụng mã giảm 500.000 đ thành công!');
    } else {
      setCouponMsg('✕ Mã voucher không hợp lệ hoặc đã hết lượt');
      setDiscountAmount(0);
    }
  };

  const finalTotal = Math.max(0, totalAmount - discountAmount);

  const handleCheckout = async () => {
    if (items.length === 0 || checkingOut) return;
    setCheckingOut(true);
    setCheckoutError(null);

    const checkoutItem = items[0];
    const idempotencyKey = getOrCreateIdempotencyKey(checkoutItem.itemId, user.id);

    try {
      const order = await api.createOrder({
        user_id: user.id,
        flash_sale_item_id: checkoutItem.itemId,
        quantity: 1,
        idempotency_key: idempotencyKey,
      }, arch);

      clearIdempotencyKey(checkoutItem.itemId, user.id);
      removeFromCart(checkoutItem.id);

      // Save order to user order history in localStorage
      try {
        const historyKey = `apexstock_orders_${user.id}`;
        const existing = JSON.parse(localStorage.getItem(historyKey) || '[]');
        existing.unshift({
          order_id: order.order_id,
          product_name: checkoutItem.name,
          product_image: checkoutItem.image,
          quantity: 1,
          total_amount: order.total_amount,
          status: order.status || 'pending_payment',
          created_at: new Date().toISOString(),
          arch: arch,
        });
        localStorage.setItem(historyKey, JSON.stringify(existing.slice(0, 30)));
      } catch {}

      setIsCartOpen(false);
      router.push(`/orders/${order.order_id}?arch=${arch}`);
    } catch (err: any) {
      setCheckoutError(err.message || 'Lỗi thanh toán. Vui lòng thử lại.');
      setCheckingOut(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 overflow-hidden">
      {/* Backdrop */}
      <div
        onClick={() => setIsCartOpen(false)}
        className="absolute inset-0 bg-black/60 backdrop-blur-sm transition-opacity"
      />

      <div className="fixed inset-y-0 right-0 max-w-full flex pl-10">
        <div className="w-screen max-w-md bg-white dark:bg-slate-900 border-l border-slate-200 dark:border-slate-800 text-slate-900 dark:text-slate-100 flex flex-col shadow-2xl">
          {/* Header */}
          <div className="p-6 border-b border-slate-200 dark:border-slate-800 flex items-center justify-between bg-white dark:bg-slate-900">
            <div className="flex items-center space-x-2.5">
              <span className="text-2xl">🛒</span>
              <div>
                <h2 className="text-lg font-bold text-slate-900 dark:text-white">Giỏ Hàng Của Bạn</h2>
                <p className="text-xs text-slate-500 dark:text-slate-400">{totalItems} món hàng đã chọn</p>
              </div>
            </div>
            <button
              onClick={() => setIsCartOpen(false)}
              className="p-2 rounded-xl text-slate-400 hover:text-slate-800 dark:hover:text-white hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
            >
              ✕
            </button>
          </div>

          {/* Cart Items List */}
          <div className="flex-1 overflow-y-auto p-6 space-y-4 bg-slate-50/50 dark:bg-slate-950/20">
            {checkoutError && (
              <div className="p-3 rounded-xl bg-red-500/10 border border-red-500/30 text-red-600 dark:text-red-400 text-xs">
                {checkoutError}
              </div>
            )}

            {items.length === 0 ? (
              <div className="py-20 text-center">
                <span className="text-5xl block mb-4">🛍️</span>
                <p className="text-slate-500 dark:text-slate-400 text-sm mb-4">Giỏ hàng của bạn đang trống.</p>
                <button
                  onClick={() => setIsCartOpen(false)}
                  className="px-4 py-2 rounded-xl text-xs font-semibold bg-blue-600 hover:bg-blue-500 text-white transition-colors"
                >
                  Khám phá Deal Hot ngay
                </button>
              </div>
            ) : (
              items.map((item) => (
                <div
                  key={item.id}
                  className="p-3.5 rounded-2xl bg-white dark:bg-slate-800/60 border border-slate-200 dark:border-slate-700/60 flex space-x-3.5 items-center shadow-sm"
                >
                  <div className="w-16 h-16 rounded-xl bg-slate-100 dark:bg-slate-700 overflow-hidden flex-shrink-0 border border-slate-200 dark:border-transparent">
                    <img
                      src={item.image}
                      alt={item.name}
                      className="w-full h-full object-cover"
                    />
                  </div>

                  <div className="flex-1 min-w-0">
                    <h4 className="text-sm font-semibold text-slate-900 dark:text-white truncate">{item.name}</h4>
                    {(item.selectedColor || item.selectedOption) && (
                      <p className="text-[11px] text-slate-500 dark:text-slate-400">
                        {item.selectedColor} {item.selectedOption ? `• ${item.selectedOption}` : ''}
                      </p>
                    )}
                    <div className="flex items-baseline space-x-2 mt-1">
                      <span className="text-sm font-bold text-blue-600 dark:text-blue-400">
                        {item.salePrice.toLocaleString('vi-VN')} đ
                      </span>
                      <span className="text-[10px] text-slate-400 dark:text-slate-500 line-through">
                        {item.originalPrice.toLocaleString('vi-VN')} đ
                      </span>
                    </div>
                  </div>

                  {/* Quantity & Delete */}
                  <div className="flex flex-col items-end space-y-2">
                    <button
                      onClick={() => removeFromCart(item.id)}
                      className="text-xs text-slate-400 hover:text-red-500 transition-colors"
                      title="Xóa khỏi giỏ"
                    >
                      🗑️
                    </button>
                    <div className="flex items-center space-x-1.5 bg-slate-100 dark:bg-slate-900 rounded-lg p-0.5 border border-slate-200 dark:border-slate-700">
                      <button
                        onClick={() => updateQuantity(item.id, -1)}
                        className="w-5 h-5 flex items-center justify-center text-xs text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white font-bold"
                      >
                        -
                      </button>
                      <span className="text-xs font-semibold w-4 text-center text-slate-800 dark:text-white">{item.quantity}</span>
                      <button
                        onClick={() => updateQuantity(item.id, 1)}
                        disabled={item.quantity >= item.maxPerUser}
                        className={`w-5 h-5 flex items-center justify-center text-xs font-bold ${
                          item.quantity >= item.maxPerUser
                            ? 'text-slate-300 dark:text-slate-600 cursor-not-allowed'
                            : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white'
                        }`}
                      >
                        +
                      </button>
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>

          {/* Footer & Checkout */}
          {items.length > 0 && (
            <div className="p-6 border-t border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 space-y-4 shadow-lg">
              {/* Voucher Input */}
              <form onSubmit={handleApplyCoupon} className="flex space-x-2">
                <input
                  type="text"
                  placeholder="Nhập mã APEX2026..."
                  value={couponCode}
                  onChange={(e) => setCouponCode(e.target.value)}
                  className="flex-1 px-3 py-2 text-xs rounded-xl bg-slate-100 dark:bg-slate-800 border border-slate-300 dark:border-slate-700 text-slate-900 dark:text-white uppercase focus:outline-none focus:border-blue-500"
                />
                <button
                  type="submit"
                  className="px-3.5 py-2 text-xs font-semibold rounded-xl bg-slate-200 dark:bg-slate-800 hover:bg-slate-300 dark:hover:bg-slate-700 text-slate-800 dark:text-slate-200 border border-slate-300 dark:border-slate-700 transition-colors"
                >
                  Áp dụng
                </button>
              </form>
              {couponMsg && (
                <p className={`text-[11px] ${discountAmount > 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-500'}`}>
                  {couponMsg}
                </p>
              )}

              {/* Bill Breakdown */}
              <div className="space-y-1.5 text-xs text-slate-600 dark:text-slate-400">
                <div className="flex justify-between">
                  <span>Tạm tính ({totalItems} sản phẩm):</span>
                  <span className="font-medium text-slate-800 dark:text-slate-200">{totalAmount.toLocaleString('vi-VN')} đ</span>
                </div>
                <div className="flex justify-between">
                  <span>Phí vận chuyển:</span>
                  <span className="text-emerald-600 dark:text-emerald-400 font-semibold">Miễn phí (Freeship)</span>
                </div>
                {discountAmount > 0 && (
                  <div className="flex justify-between text-emerald-600 dark:text-emerald-400">
                    <span>Voucher giảm giá:</span>
                    <span>-{discountAmount.toLocaleString('vi-VN')} đ</span>
                  </div>
                )}
                <div className="pt-2 border-t border-slate-200 dark:border-slate-800 flex justify-between items-baseline text-sm font-bold text-slate-900 dark:text-white">
                  <span>Tổng thanh toán:</span>
                  <span className="text-lg text-blue-600 dark:text-blue-400 font-black">{finalTotal.toLocaleString('vi-VN')} đ</span>
                </div>
              </div>

              {/* Checkout info */}
              <div className="flex items-center justify-between text-[11px] px-3 py-2 rounded-xl bg-blue-50 dark:bg-blue-950/30 border border-blue-200 dark:border-blue-500/20 text-slate-600 dark:text-slate-400">
                <span>Tuyến xử lý: <strong className="text-blue-700 dark:text-white uppercase">{arch}</strong></span>
                <span>Người mua: <strong className="text-slate-900 dark:text-white">{user.name.split(' ')[0]}</strong></span>
              </div>

              {/* Checkout Button */}
              <button
                onClick={handleCheckout}
                disabled={checkingOut}
                className="w-full py-3.5 rounded-xl bg-gradient-to-r from-blue-600 to-indigo-600 hover:from-blue-500 hover:to-indigo-500 text-white font-bold text-sm shadow-lg shadow-blue-500/25 transition-all flex items-center justify-center space-x-2 disabled:opacity-50"
              >
                {checkingOut ? (
                  <span>Đang xử lý tạo đơn hàng...</span>
                ) : (
                  <>
                    <span>Tiến Hành Đặt Hàng & Thanh Toán</span>
                    <span>→</span>
                  </>
                )}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
