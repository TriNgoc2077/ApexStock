'use client';

import React, { useState, useEffect } from 'react';
import Link from 'next/link';
import Navbar from '@/components/layout/Navbar';
import Footer from '@/components/layout/Footer';
import CartDrawer from '@/components/cart/CartDrawer';
import { useAuth } from '@/context/AuthContext';
import { Architecture } from '@/lib/api';

interface SavedOrder {
  order_id: string;
  product_name: string;
  product_image?: string;
  quantity: number;
  total_amount: number;
  status: string;
  created_at: string;
  arch?: Architecture;
}

export default function OrdersListPage() {
  const { user } = useAuth();
  const [arch, setArch] = useState<Architecture>('optimized');
  const [orders, setOrders] = useState<SavedOrder[]>([]);
  const [filterStatus, setFilterStatus] = useState<string>('all');
  const [loading, setLoading] = useState<boolean>(true);

  useEffect(() => {
    try {
      const historyKey = `apexstock_orders_${user.id}`;
      const saved = localStorage.getItem(historyKey);
      if (saved) {
        setOrders(JSON.parse(saved));
      } else {
        // Fallback demo orders for realistic presentation
        const demoOrders: SavedOrder[] = [
          {
            order_id: '636bee21-4d70-4411-9056-0bb7729a3224',
            product_name: 'Tai Nghe Sony WH-1000XM5 Hi-Res Audio',
            product_image: '/images/products/sony-xm5.jpg',
            quantity: 1,
            total_amount: 6490000,
            status: 'paid',
            created_at: new Date(Date.now() - 3600 * 1000 * 2).toISOString(),
            arch: 'optimized',
          },
        ];
        setOrders(demoOrders);
      }
    } catch {
      // Ignore json parse error
    } finally {
      setLoading(false);
    }
  }, [user.id]);

  const filteredOrders = orders.filter((o) => {
    if (filterStatus === 'all') return true;
    return o.status === filterStatus;
  });

  return (
    <div className="min-h-screen flex flex-col bg-[var(--background)] text-[var(--foreground)]">
      <Navbar arch={arch} setArch={setArch} />

      <main className="flex-1 max-w-5xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-8 space-y-6">
        {/* Header */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-4 border-b border-slate-200 dark:border-slate-800">
          <div>
            <div className="flex items-center space-x-2.5">
              <span className="text-2xl">📦</span>
              <h1 className="text-2xl font-black text-slate-900 dark:text-white">
                Đơn Hàng Của Tôi
              </h1>
            </div>
            <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
              Khách hàng: <strong>{user.name}</strong> • Email: {user.email}
            </p>
          </div>

          <Link
            href="/"
            className="inline-flex items-center space-x-1.5 px-4 py-2 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-bold transition-all shadow-md shadow-blue-500/20 self-start sm:self-auto"
          >
            <span>⚡</span>
            <span>Khám phá Deal Hot</span>
          </Link>
        </div>

        {/* Filter status tabs */}
        <div className="flex space-x-2 overflow-x-auto pb-1 text-xs font-semibold">
          {[
            { id: 'all', label: 'Tất cả đơn' },
            { id: 'pending_payment', label: 'Chờ thanh toán' },
            { id: 'paid', label: 'Đã thanh toán' },
            { id: 'expired', label: 'Đã hết hạn' },
          ].map((tab) => {
            const active = filterStatus === tab.id;
            return (
              <button
                key={tab.id}
                onClick={() => setFilterStatus(tab.id)}
                className={`px-3.5 py-1.5 rounded-xl border transition-all whitespace-nowrap ${
                  active
                    ? 'bg-blue-600 text-white border-blue-600 shadow-sm'
                    : 'bg-white dark:bg-slate-900 border-slate-200 dark:border-slate-800 text-slate-600 dark:text-slate-400 hover:border-slate-300'
                }`}
              >
                {tab.label}
              </button>
            );
          })}
        </div>

        {/* Orders List */}
        {loading ? (
          <div className="py-20 text-center text-slate-400">Đang tải lịch sử đơn hàng...</div>
        ) : filteredOrders.length === 0 ? (
          <div className="py-20 text-center space-y-3 bg-white dark:bg-slate-900 rounded-3xl border border-slate-200 dark:border-slate-800 p-8 shadow-sm">
            <span className="text-5xl block">🛍️</span>
            <h3 className="text-base font-bold text-slate-900 dark:text-white">Chưa có đơn hàng nào</h3>
            <p className="text-xs text-slate-500 dark:text-slate-400 max-w-sm mx-auto">
              Bạn chưa có đơn hàng nào trong trạng thái này. Hãy chọn sản phẩm yêu thích và tiến hành đặt hàng nhé!
            </p>
            <Link
              href="/"
              className="inline-block mt-2 px-5 py-2.5 rounded-xl bg-blue-600 text-white text-xs font-bold shadow-md shadow-blue-500/20"
            >
              Xem Sản Phẩm Flash Sale
            </Link>
          </div>
        ) : (
          <div className="space-y-4">
            {filteredOrders.map((ord) => {
              const isPaid = ord.status === 'paid';
              const isPending = ord.status === 'pending_payment';
              const orderArch = ord.arch || arch;

              return (
                <div
                  key={ord.order_id}
                  className="p-5 rounded-2xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 shadow-sm space-y-4 hover:border-slate-300 dark:hover:border-slate-700 transition-all"
                >
                  {/* Order meta bar */}
                  <div className="flex flex-col sm:flex-row sm:items-center justify-between pb-3 border-b border-slate-100 dark:border-slate-800 gap-2">
                    <div className="flex items-center space-x-2 text-xs">
                      <span className="font-mono font-bold text-slate-900 dark:text-white">
                        #{ord.order_id.slice(0, 8)}
                      </span>
                      <span className="text-slate-400">•</span>
                      <span className="text-slate-500 dark:text-slate-400">
                        {new Date(ord.created_at).toLocaleString('vi-VN')}
                      </span>
                      <span className="text-slate-400">•</span>
                      <span className="font-mono uppercase text-[10px] px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-400">
                        {orderArch}
                      </span>
                    </div>

                    <span
                      className={`self-start sm:self-auto text-[11px] font-black uppercase px-2.5 py-1 rounded-lg ${
                        isPaid
                          ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30'
                          : isPending
                          ? 'bg-amber-500/15 text-amber-600 dark:text-amber-400 border border-amber-500/30 animate-pulse'
                          : 'bg-red-500/15 text-red-600 dark:text-red-400 border border-red-500/30'
                      }`}
                    >
                      {isPaid ? 'ĐÃ THANH TOÁN (PAID)' : isPending ? 'CHỜ THANH TOÁN' : ord.status}
                    </span>
                  </div>

                  {/* Order Content */}
                  <div className="flex items-center justify-between">
                    <div className="flex items-center space-x-3.5">
                      <div className="w-14 h-14 rounded-xl bg-slate-100 dark:bg-slate-800 overflow-hidden flex items-center justify-center border border-slate-200 dark:border-transparent flex-shrink-0">
                        {ord.product_image ? (
                          <img src={ord.product_image} alt="" className="w-full h-full object-cover" />
                        ) : (
                          '⚡'
                        )}
                      </div>
                      <div>
                        <h4 className="text-sm font-bold text-slate-900 dark:text-white line-clamp-1">
                          {ord.product_name}
                        </h4>
                        <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
                          Số lượng: <strong>{ord.quantity}</strong>
                        </p>
                      </div>
                    </div>

                    <div className="text-right">
                      <div className="text-xs text-slate-500 dark:text-slate-400">Tổng thanh toán</div>
                      <div className="text-base font-black text-blue-600 dark:text-blue-400">
                        {ord.total_amount.toLocaleString('vi-VN')} đ
                      </div>
                    </div>
                  </div>

                  {/* Actions */}
                  <div className="pt-2 flex justify-end space-x-2 border-t border-slate-100 dark:border-slate-800/80">
                    <Link
                      href={`/orders/${ord.order_id}?arch=${orderArch}`}
                      className="px-4 py-2 rounded-xl bg-slate-100 dark:bg-slate-800 hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-800 dark:text-white text-xs font-semibold transition-colors flex items-center space-x-1"
                    >
                      <span>{isPaid ? 'Xem Hóa Đơn' : 'Chi Tiết & Quét QR'}</span>
                      <span>→</span>
                    </Link>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </main>

      <CartDrawer arch={arch} />
      <Footer />
    </div>
  );
}
