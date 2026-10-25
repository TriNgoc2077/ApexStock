'use client';

import React, { useEffect, useState, Suspense } from 'react';
import { useParams, useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { api, FlashSaleItem, Architecture, getOrCreateIdempotencyKey, clearIdempotencyKey } from '@/lib/api';
import SpotlightCard from '@/components/ui/SpotlightCard';

function FlashSaleDetailContent() {
  const params = useParams();
  const searchParams = useSearchParams();
  const router = useRouter();

  const id = params?.id as string;
  const initialArch = (searchParams.get('arch') as Architecture) || 'optimized';

  const [arch, setArch] = useState<Architecture>(initialArch);
  const [item, setItem] = useState<FlashSaleItem | null>(null);
  const [loading, setLoading] = useState<boolean>(true);
  const [purchasing, setPurchasing] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);

  // Mock demo user
  const userId = '00000000-0000-0000-0000-000000000001';

  useEffect(() => {
    if (!id) return;
    setLoading(true);
    api.getItemDetail(id, arch)
      .then((data) => {
        setItem(data);
        setError(null);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [id, arch]);

  const handleFlashBuy = async () => {
    if (!item || purchasing) return;
    setPurchasing(true);
    setError(null);

    const idempotencyKey = getOrCreateIdempotencyKey(item.id, userId);

    try {
      const order = await api.createOrder({
        user_id: userId,
        flash_sale_item_id: item.id,
        quantity: 1,
        idempotency_key: idempotencyKey,
      }, arch);

      clearIdempotencyKey(item.id, userId);
      router.push(`/orders/${order.order_id}?arch=${arch}`);
    } catch (err: any) {
      setError(err.message || 'Lỗi đặt hàng');
      setPurchasing(false);
    }
  };

  if (loading) {
    return <div className="min-h-screen flex items-center justify-center text-slate-400">Đang tải sản phẩm...</div>;
  }

  if (!item) {
    return (
      <div className="min-h-screen flex flex-col items-center justify-center text-center p-6">
        <p className="text-red-400 mb-4">{error || 'Không tìm thấy sản phẩm'}</p>
        <Link href="/" className="px-4 py-2 rounded-lg bg-slate-800 text-white">Quay lại trang chủ</Link>
      </div>
    );
  }

  const isSoldOut = item.available_stock <= 0;

  return (
    <main className="min-h-screen p-6 sm:p-12 max-w-4xl mx-auto">
      <Link href="/" className="inline-flex items-center text-sm text-slate-400 hover:text-white mb-8">
        ← Quay lại danh sách
      </Link>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
        {/* Product Media */}
        <div className="aspect-square rounded-2xl bg-slate-800/80 border border-[var(--card-border)] flex items-center justify-center text-7xl overflow-hidden">
          {item.product_image ? (
            <img src={item.product_image} alt={item.product_name} className="object-cover w-full h-full" />
          ) : (
            '⚡'
          )}
        </div>

        {/* Product Actions & Checkout */}
        <SpotlightCard className="flex flex-col justify-between">
          <div>
            <div className="flex items-center justify-between mb-4">
              <span className="text-xs px-2.5 py-1 rounded-full bg-blue-500/10 text-blue-400 border border-blue-500/20">
                Flash Sale Item
              </span>
              <div className="flex items-center space-x-1.5 text-xs">
                <span className="text-slate-400">Tuyến xử lý:</span>
                <span className="font-bold text-white uppercase">{arch}</span>
              </div>
            </div>

            <h1 className="text-3xl font-extrabold text-[var(--foreground)] mb-2">{item.product_name}</h1>
            <p className="text-sm text-slate-400 mb-6">{item.product_description}</p>

            <div className="flex items-baseline space-x-3 mb-6">
              <span className="text-3xl font-black text-blue-400">
                {item.sale_price.toLocaleString('vi-VN')} đ
              </span>
              <span className="text-sm text-slate-500 line-through">
                {item.original_price.toLocaleString('vi-VN')} đ
              </span>
            </div>

            <div className="p-4 rounded-xl bg-slate-900/60 border border-[var(--card-border)] mb-6 space-y-2 text-sm">
              <div className="flex justify-between text-slate-300">
                <span>Số lượng mua:</span>
                <span className="font-bold">1 sản phẩm (giới hạn tối đa)</span>
              </div>
              <div className="flex justify-between text-slate-300">
                <span>Kho khả dụng:</span>
                <span className={`font-bold ${isSoldOut ? 'text-red-400' : 'text-emerald-400'}`}>
                  {item.available_stock} sản phẩm
                </span>
              </div>
            </div>

            {error && (
              <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/30 text-red-400 text-xs mb-6">
                {error}
              </div>
            )}
          </div>

          <button
            onClick={handleFlashBuy}
            disabled={isSoldOut || purchasing}
            className={`w-full py-4 rounded-xl font-extrabold text-base transition-all flex items-center justify-center space-x-2 ${
              isSoldOut
                ? 'bg-slate-800 text-slate-500 cursor-not-allowed'
                : purchasing
                ? 'bg-blue-600/50 text-white cursor-wait'
                : 'bg-blue-600 hover:bg-blue-500 text-white shadow-xl shadow-blue-500/25 active:scale-95'
            }`}
          >
            {purchasing ? (
              <span>Đang tạo đơn hàng...</span>
            ) : isSoldOut ? (
              <span>Đã Hết Hàng</span>
            ) : (
              <span>MUA NGAY (FLASH BUY) ⚡</span>
            )}
          </button>
        </SpotlightCard>
      </div>
    </main>
  );
}

export default function FlashSaleDetailPage() {
  return (
    <Suspense fallback={<div className="min-h-screen flex items-center justify-center text-slate-400">Đang tải...</div>}>
      <FlashSaleDetailContent />
    </Suspense>
  );
}
