'use client';

import React, { useEffect, useState, Suspense } from 'react';
import { useParams, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { api, OrderDetail, Architecture, InitiatePaymentResponse } from '@/lib/api';
import { usePolling } from '@/hooks/usePolling';
import SpotlightCard from '@/components/ui/SpotlightCard';

function OrderDetailContent() {
  const params = useParams();
  const searchParams = useSearchParams();

  const orderId = params?.id as string;
  const arch = (searchParams.get('arch') as Architecture) || 'optimized';

  const [paymentInfo, setPaymentInfo] = useState<InitiatePaymentResponse | null>(null);
  const [simulating, setSimulating] = useState<boolean>(false);

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

  const handleSimulate = async (outcome: 'success' | 'failure' | 'timeout' | 'cancel') => {
    if (!order || simulating) return;
    setSimulating(true);
    try {
      await api.simulatePayment(order.order_id, outcome, arch);
    } catch (err: any) {
      alert(err.message || 'Lỗi mô phỏng thanh toán');
    } finally {
      setSimulating(false);
    }
  };

  if (isLoading && !order) {
    return <div className="min-h-screen flex items-center justify-center text-slate-400">Đang tải đơn hàng...</div>;
  }

  if (!order) {
    return <div className="min-h-screen flex items-center justify-center text-red-400">Không tìm thấy đơn hàng.</div>;
  }

  const isPending = order.status === 'pending_payment';
  const isPaid = order.status === 'paid';
  const isExpired = order.status === 'expired';

  return (
    <main className="min-h-screen p-6 sm:p-12 max-w-3xl mx-auto">
      <Link href="/" className="inline-flex items-center text-sm text-slate-400 hover:text-white mb-8">
        ← Về trang chủ
      </Link>

      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-2xl font-black text-[var(--foreground)]">Đơn Hàng #{order.order_id.slice(0, 8)}</h1>
          <p className="text-xs text-slate-400">Tạo lúc: {new Date(order.created_at).toLocaleString('vi-VN')}</p>
        </div>
        <div className="flex items-center space-x-2">
          {isStale && <span className="text-xs px-2 py-1 rounded bg-yellow-500/20 text-yellow-300">Đang kết nối lại...</span>}
          <span
            className={`px-3 py-1 text-xs font-bold rounded-full ${
              isPaid
                ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30'
                : isPending
                ? 'bg-amber-500/20 text-amber-400 border border-amber-500/30 animate-pulse'
                : 'bg-red-500/20 text-red-400 border border-red-500/30'
            }`}
          >
            {order.status.toUpperCase()}
          </span>
        </div>
      </div>

      <SpotlightCard className="mb-8">
        <h2 className="text-lg font-bold mb-4">Chi Tiết Sản Phẩm</h2>
        <div className="flex justify-between items-center py-2 border-b border-[var(--card-border)]">
          <span>{order.product_name} x {order.quantity}</span>
          <span className="font-bold">{order.unit_price.toLocaleString('vi-VN')} đ</span>
        </div>
        <div className="flex justify-between items-center pt-4 text-lg font-extrabold">
          <span>Tổng thanh toán:</span>
          <span className="text-blue-400">{order.total_amount.toLocaleString('vi-VN')} đ</span>
        </div>
      </SpotlightCard>

      {/* Payment Action Section */}
      {isPending && paymentInfo && (
        <SpotlightCard className="mb-8 text-center">
          <h2 className="text-lg font-bold mb-2">Quét Mã VietQR Để Thanh Toán</h2>
          <p className="text-xs text-slate-400 mb-6">Mã QR tự động đối soát qua SePay Webhook</p>

          <div className="inline-block p-4 bg-white rounded-2xl shadow-xl mb-6">
            <img src={paymentInfo.qr_url} alt="VietQR SePay" className="w-56 h-56 object-contain" />
          </div>

          <div className="max-w-md mx-auto text-left p-4 rounded-xl bg-slate-900/60 border border-[var(--card-border)] space-y-2 text-xs mb-8">
            <div className="flex justify-between">
              <span className="text-slate-400">Ngân hàng:</span>
              <span className="font-bold">{paymentInfo.bank_code}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Số tài khoản:</span>
              <span className="font-bold">{paymentInfo.account_number}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Nội dung CK:</span>
              <span className="font-bold text-amber-400">{paymentInfo.transfer_content}</span>
            </div>
          </div>

          {/* Payment Simulator Panel for Quick Demo */}
          <div className="pt-4 border-t border-[var(--card-border)]">
            <span className="text-xs font-semibold text-slate-400 block mb-3">
              Mô Phỏng Nhanh (Simulator Không Cần Quét Ngân Hàng Thật):
            </span>
            <div className="flex flex-wrap justify-center gap-2">
              <button
                onClick={() => handleSimulate('success')}
                disabled={simulating}
                className="px-3 py-1.5 rounded-lg text-xs font-bold bg-emerald-600 hover:bg-emerald-500 text-white"
              >
                Mô Phỏng Thành Công ✅
              </button>
              <button
                onClick={() => handleSimulate('failure')}
                disabled={simulating}
                className="px-3 py-1.5 rounded-lg text-xs font-bold bg-red-600 hover:bg-red-500 text-white"
              >
                Mô Phỏng Thất Bại ❌
              </button>
              <button
                onClick={() => handleSimulate('cancel')}
                disabled={simulating}
                className="px-3 py-1.5 rounded-lg text-xs font-bold bg-slate-700 hover:bg-slate-600 text-slate-200"
              >
                Hủy Đơn 🚫
              </button>
            </div>
          </div>
        </SpotlightCard>
      )}

      {isPaid && (
        <div className="p-8 rounded-2xl bg-emerald-500/10 border border-emerald-500/30 text-center">
          <div className="text-5xl mb-4">🎉</div>
          <h2 className="text-2xl font-bold text-emerald-400 mb-2">Thanh Toán Thành Công!</h2>
          <p className="text-sm text-slate-300">Đơn hàng của bạn đã được ghi nhận và xác nhận hoàn tất.</p>
        </div>
      )}

      {isExpired && (
        <div className="p-8 rounded-2xl bg-red-500/10 border border-red-500/30 text-center">
          <div className="text-5xl mb-4">⏰</div>
          <h2 className="text-2xl font-bold text-red-400 mb-2">Đơn Hàng Đã Hết Hạn</h2>
          <p className="text-sm text-slate-300">Hết thời gian giữ hàng 5 phút. Kho đã được hoàn lại tự động.</p>
        </div>
      )}
    </main>
  );
}

export default function OrderDetailPage() {
  return (
    <Suspense fallback={<div className="min-h-screen flex items-center justify-center text-slate-400">Đang tải...</div>}>
      <OrderDetailContent />
    </Suspense>
  );
}
