'use client';

import React, { useEffect, useState, Suspense } from 'react';
import { useParams, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import Navbar from '@/components/layout/Navbar';
import Footer from '@/components/layout/Footer';
import CartDrawer from '@/components/cart/CartDrawer';
import { api, OrderDetail, Architecture, InitiatePaymentResponse } from '@/lib/api';
import { usePolling } from '@/hooks/usePolling';
import SpotlightCard from '@/components/ui/SpotlightCard';
import { useAuth } from '@/context/AuthContext';

function OrderDetailContent() {
  const params = useParams();
  const searchParams = useSearchParams();

  const orderId = params?.id as string;
  const initialArch = (searchParams.get('arch') as Architecture) || 'optimized';
  const [arch, setArch] = useState<Architecture>(initialArch);

  const [paymentInfo, setPaymentInfo] = useState<InitiatePaymentResponse | null>(null);
  const [simulating, setSimulating] = useState<boolean>(false);
  const { user } = useAuth();

  // Poll order detail until reaching a terminal state
  const { data: order, isLoading, isStale } = usePolling<OrderDetail>({
    fetcher: () => api.getOrderDetail(orderId, arch),
    intervalMs: 1500,
    isTerminal: (ord) => ord.status !== 'pending_payment',
    enabled: Boolean(orderId),
  });

  // Fetch SePay QR details once order is loaded
  useEffect(() => {
    if (order && order.status === 'pending_payment' && !paymentInfo) {
      api.initiatePayment(order.order_id, arch)
        .then(setPaymentInfo)
        .catch(() => {});
    }
  }, [order, arch, paymentInfo]);

  // Sync order status to user history
  useEffect(() => {
    if (order && user) {
      try {
        const historyKey = `apexstock_orders_${user.id}`;
        const existing = JSON.parse(localStorage.getItem(historyKey) || '[]');
        const idx = existing.findIndex((o: any) => o.order_id === order.order_id);
        if (idx > -1) {
          existing[idx].status = order.status;
          localStorage.setItem(historyKey, JSON.stringify(existing));
        } else {
          existing.unshift({
            order_id: order.order_id,
            product_name: order.product_name,
            product_image: order.product_image,
            quantity: order.quantity,
            total_amount: order.total_amount,
            status: order.status,
            created_at: order.created_at,
            arch: arch,
          });
          localStorage.setItem(historyKey, JSON.stringify(existing.slice(0, 30)));
        }
      } catch {}
    }
  }, [order, user, arch]);

  const handleSimulate = async (outcome: 'success' | 'failure' | 'timeout' | 'cancel') => {
    if (!order || simulating) return;
    setSimulating(true);
    try {
      await api.simulatePayment(order.order_id, outcome, arch);
    } catch (err: any) {
      alert(err.message || 'Lỗi xử lý thanh toán');
    } finally {
      setSimulating(false);
    }
  };

  const isPending = order?.status === 'pending_payment';
  const isPaid = order?.status === 'paid';
  const isExpired = order?.status === 'expired';

  // Bank transfer metadata with fallback for clear display
  const bankCode = (paymentInfo?.bank_code && paymentInfo.bank_code.trim()) || 'MBBank';
  const accountNumber = (paymentInfo?.account_number && paymentInfo.account_number.trim()) || '0987654321';
  const transferContent = paymentInfo?.transfer_content || `DH ${orderId}`;
  const amount = order?.total_amount || paymentInfo?.amount || 0;

  // Real QR URL ensuring valid params
  const qrUrl =
    paymentInfo?.qr_url && !paymentInfo.qr_url.includes('acc=&')
      ? paymentInfo.qr_url
      : `https://qr.sepay.vn/img?acc=${accountNumber}&bank=${bankCode}&amount=${amount}&des=${encodeURIComponent(transferContent)}`;

  return (
    <div className="min-h-screen flex flex-col bg-[var(--background)] text-[var(--foreground)]">
      <Navbar arch={arch} setArch={setArch} />

      <main className="flex-1 max-w-4xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-8 space-y-8">
        {/* Breadcrumb */}
        <div className="flex items-center justify-between">
          <Link href="/" className="inline-flex items-center space-x-2 text-xs text-slate-500 hover:text-blue-500 transition-colors">
            <span>←</span>
            <span>Tiếp tục mua sắm / Về trang chủ</span>
          </Link>
          <div className="text-xs text-slate-500">
            Tuyến backend: <strong className="uppercase font-mono text-blue-600 dark:text-blue-400">{arch}</strong>
          </div>
        </div>

        {isLoading && !order && (
          <div className="py-24 text-center space-y-3">
            <div className="inline-block w-8 h-8 border-4 border-blue-500 border-t-transparent rounded-full animate-spin" />
            <p className="text-sm text-slate-500">Đang tải hóa đơn đơn hàng #{orderId.slice(0, 8)}...</p>
          </div>
        )}

        {!isLoading && !order && (
          <div className="py-24 text-center space-y-3">
            <span className="text-4xl block">🔍</span>
            <p className="text-sm text-red-500">Không tìm thấy đơn hàng trên tuyến {arch.toUpperCase()}.</p>
            <Link href="/" className="px-4 py-2 rounded-xl bg-slate-800 text-xs text-white">Quay lại trang chủ</Link>
          </div>
        )}

        {order && (
          <>
            {/* Order Header Card */}
            <div className="p-6 rounded-3xl bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 flex flex-col sm:flex-row sm:items-center justify-between gap-4 shadow-sm dark:shadow-xl">
              <div>
                <div className="flex items-center space-x-2">
                  <span className="text-2xl">🧾</span>
                  <h1 className="text-xl sm:text-2xl font-black text-slate-900 dark:text-white">
                    Đơn Hàng #{order.order_id.slice(0, 8)}
                  </h1>
                </div>
                <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">
                  Khởi tạo lúc: {new Date(order.created_at).toLocaleString('vi-VN')} • Mã: <span className="font-mono text-slate-400">{order.order_id}</span>
                </p>
              </div>

              <div className="flex items-center space-x-2">
                {isStale && (
                  <span className="text-xs px-2.5 py-1 rounded-lg bg-yellow-500/20 text-yellow-600 dark:text-yellow-300 border border-yellow-500/30 animate-pulse">
                    Đang kết nối lại...
                  </span>
                )}
                <span
                  className={`px-4 py-1.5 text-xs font-black rounded-xl uppercase tracking-wider ${
                    isPaid
                      ? 'bg-emerald-500/20 text-emerald-600 dark:text-emerald-400 border border-emerald-500/40'
                      : isPending
                      ? 'bg-amber-500/20 text-amber-600 dark:text-amber-400 border border-amber-500/40 animate-pulse'
                      : 'bg-red-500/20 text-red-600 dark:text-red-400 border border-red-500/40'
                  }`}
                >
                  {isPaid ? 'ĐÃ THANH TOÁN (PAID)' : isPending ? 'CHỜ THANH TOÁN' : order.status}
                </span>
              </div>
            </div>

            {/* Product Summary */}
            <SpotlightCard className="p-6 rounded-3xl bg-white dark:bg-slate-900/90 border border-slate-200 dark:border-slate-800 shadow-sm">
              <h2 className="text-sm font-bold uppercase tracking-wider text-slate-500 dark:text-slate-400 mb-4">Chi Tiết Sản Phẩm Đặt Mua</h2>
              <div className="flex items-center justify-between py-3 border-b border-slate-200 dark:border-slate-800">
                <div className="flex items-center space-x-3.5">
                  <div className="w-14 h-14 rounded-xl bg-slate-100 dark:bg-slate-800 overflow-hidden flex items-center justify-center text-xl border border-slate-200 dark:border-transparent flex-shrink-0">
                    {order.product_image ? (
                      <img src={order.product_image} alt="" className="w-full h-full object-cover" />
                    ) : (
                      '⚡'
                    )}
                  </div>
                  <div>
                    <h3 className="font-bold text-sm text-slate-900 dark:text-white">{order.product_name}</h3>
                    <p className="text-xs text-slate-500 dark:text-slate-400">Số lượng: <strong>{order.quantity} sản phẩm</strong></p>
                  </div>
                </div>
                <span className="font-extrabold text-sm text-slate-900 dark:text-slate-200">
                  {order.unit_price.toLocaleString('vi-VN')} đ
                </span>
              </div>

              <div className="pt-4 flex justify-between items-baseline">
                <span className="text-xs font-semibold text-slate-500 dark:text-slate-400">Tổng Số Tiền Cần Thanh Toán:</span>
                <span className="text-2xl font-black text-blue-600 dark:text-blue-400">
                  {order.total_amount.toLocaleString('vi-VN')} đ
                </span>
              </div>
            </SpotlightCard>

            {/* VietQR SePay Payment Section */}
            {isPending && (
              <SpotlightCard className="p-8 rounded-3xl bg-white dark:bg-slate-900 border border-blue-500/30 text-center space-y-6 shadow-md dark:shadow-xl">
                <div>
                  <div className="inline-flex items-center space-x-1.5 px-3 py-1 rounded-full bg-blue-500/15 text-blue-600 dark:text-blue-400 text-xs font-bold mb-2">
                    <span className="w-2 h-2 rounded-full bg-blue-500 animate-ping" />
                    <span>Cổng Thanh Toán VietQR Tự Động</span>
                  </div>
                  <h2 className="text-xl font-black text-slate-900 dark:text-white">Quét Mã QR Để Hoàn Tất Thanh Toán</h2>
                  <p className="text-xs text-slate-500 dark:text-slate-400 mt-1 max-w-md mx-auto">
                    Mở ứng dụng ngân hàng bất kỳ (Vietcombank, MB, Techcombank...) để quét mã chuyển khoản.
                  </p>
                </div>

                {/* QR Code Container */}
                <div className="inline-block p-4 bg-white rounded-3xl shadow-xl border-2 border-slate-200 dark:border-blue-500/30">
                  <img
                    src={qrUrl}
                    alt="VietQR SePay"
                    className="w-64 h-64 object-contain mx-auto"
                    onError={(e) => {
                      // Fallback to VietQR API if SePay image has issues
                      const fallback = `https://api.vietqr.io/image/970422-${accountNumber}-compact2.jpg?amount=${amount}&addInfo=${encodeURIComponent(transferContent)}`;
                      if (e.currentTarget.src !== fallback) {
                        e.currentTarget.src = fallback;
                      }
                    }}
                  />
                  <div className="text-[11px] font-bold text-slate-600 mt-2 font-mono">
                    VIETQR AUTO RECONCILIATION
                  </div>
                </div>

                {/* Transfer Details Card */}
                <div className="max-w-md mx-auto text-left p-5 rounded-2xl bg-slate-50 dark:bg-slate-950 border border-slate-200 dark:border-slate-800 space-y-2.5 text-xs font-mono shadow-inner">
                  <div className="flex justify-between pb-1.5 border-b border-slate-200 dark:border-slate-900">
                    <span className="text-slate-500 font-sans">Ngân hàng thụ hưởng:</span>
                    <span className="font-bold text-slate-900 dark:text-white">{bankCode}</span>
                  </div>
                  <div className="flex justify-between pb-1.5 border-b border-slate-200 dark:border-slate-900">
                    <span className="text-slate-500 font-sans">Số tài khoản:</span>
                    <span className="font-bold text-slate-900 dark:text-white">{accountNumber}</span>
                  </div>
                  <div className="flex justify-between pb-1.5 border-b border-slate-200 dark:border-slate-900">
                    <span className="text-slate-500 font-sans">Số tiền chính xác:</span>
                    <span className="font-bold text-emerald-600 dark:text-emerald-400">{amount.toLocaleString('vi-VN')} đ</span>
                  </div>
                  <div className="flex justify-between items-center pt-1">
                    <span className="text-slate-500 font-sans">Nội dung chuyển khoản:</span>
                    <span className="font-black text-amber-700 dark:text-amber-400 bg-amber-500/15 px-2 py-0.5 rounded border border-amber-500/30">
                      {transferContent}
                    </span>
                  </div>
                </div>

                {/* Fast Simulator Controls without lengthy text */}
                <div className="pt-4 border-t border-slate-200 dark:border-slate-800">
                  <div className="flex flex-wrap justify-center gap-3">
                    <button
                      onClick={() => handleSimulate('success')}
                      disabled={simulating}
                      className="px-6 py-2.5 rounded-xl text-xs font-bold bg-emerald-600 hover:bg-emerald-500 text-white shadow-lg shadow-emerald-600/25 transition-all flex items-center space-x-2 disabled:opacity-50"
                    >
                      <span>✅</span>
                      <span>Xác Nhận Đã Thanh Toán (Demo)</span>
                    </button>
                    <button
                      onClick={() => handleSimulate('cancel')}
                      disabled={simulating}
                      className="px-4 py-2.5 rounded-xl text-xs font-semibold bg-slate-100 hover:bg-slate-200 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-300 border border-slate-300 dark:border-slate-700 transition-all flex items-center space-x-1.5 disabled:opacity-50"
                    >
                      <span>✕</span>
                      <span>Hủy Đơn Hàng</span>
                    </button>
                  </div>
                </div>
              </SpotlightCard>
            )}

            {/* Success State */}
            {isPaid && (
              <div className="p-10 rounded-3xl bg-emerald-50 dark:bg-gradient-to-br dark:from-emerald-950/40 dark:via-slate-900 dark:to-slate-900 border border-emerald-500/40 text-center space-y-4 shadow-sm dark:shadow-2xl">
                <div className="text-6xl animate-bounce">🎉</div>
                <h2 className="text-2xl sm:text-3xl font-black text-emerald-600 dark:text-emerald-400">
                  Giao Dịch Đã Thanh Toán Thành Công!
                </h2>
                <p className="text-xs sm:text-sm text-slate-600 dark:text-slate-300 max-w-md mx-auto leading-relaxed">
                  Đơn hàng của bạn đã được xác nhận thanh toán tự động qua SePay Webhook.
                  Hệ thống đã chốt trừ tồn kho vĩnh viễn và tạo bản ghi hóa đơn trong cơ sở dữ liệu.
                </p>
                <div className="pt-4 flex justify-center space-x-3">
                  <Link
                    href="/"
                    className="px-5 py-2.5 rounded-xl bg-blue-600 hover:bg-blue-500 text-white font-bold text-xs shadow-lg shadow-blue-500/30 transition-all"
                  >
                    Tiếp tục mua sắm
                  </Link>
                  <Link
                    href="/orders"
                    className="px-5 py-2.5 rounded-xl bg-slate-200 hover:bg-slate-300 dark:bg-slate-800 dark:hover:bg-slate-700 text-slate-800 dark:text-slate-200 font-semibold text-xs border border-slate-300 dark:border-slate-700 transition-all"
                  >
                    Xem tất cả đơn hàng
                  </Link>
                </div>
              </div>
            )}

            {/* Expired State */}
            {isExpired && (
              <div className="p-10 rounded-3xl bg-red-50 dark:bg-red-950/30 border border-red-500/30 text-center space-y-4">
                <div className="text-6xl">⏰</div>
                <h2 className="text-2xl font-black text-red-600 dark:text-red-400">Đơn Hàng Đã Hết Hạn Giữ Chỗ</h2>
                <p className="text-xs text-slate-600 dark:text-slate-300 max-w-md mx-auto leading-relaxed">
                  Đã quá thời gian quy định mà hệ thống chưa nhận được thanh toán.
                  Số lượng sản phẩm trong đơn đã được tự động hoàn lại vào kho hàng (Auto Restock).
                </p>
                <Link
                  href="/"
                  className="inline-block px-5 py-2.5 rounded-xl bg-slate-800 hover:bg-slate-700 text-white font-bold text-xs transition-all"
                >
                  Săn lại sản phẩm khác
                </Link>
              </div>
            )}
          </>
        )}
      </main>

      <CartDrawer arch={arch} />
      <Footer />
    </div>
  );
}

export default function OrderDetailPage() {
  return (
    <Suspense fallback={<div className="min-h-screen flex items-center justify-center text-slate-400">Đang tải hóa đơn...</div>}>
      <OrderDetailContent />
    </Suspense>
  );
}
