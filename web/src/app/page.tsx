'use client';

import React, { useEffect, useState } from 'react';
import Link from 'next/link';
import AuroraHero from '@/components/ui/AuroraHero';
import SpotlightCard from '@/components/ui/SpotlightCard';
import ThemeToggle from '@/components/ui/ThemeToggle';
import { api, FlashSaleEvent, Architecture } from '@/lib/api';

export default function HomePage() {
  const [arch, setArch] = useState<Architecture>('optimized');
  const [events, setEvents] = useState<FlashSaleEvent[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setLoading(true);
    api.getEvents(arch)
      .then((data) => {
        setEvents(data);
        setError(null);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [arch]);

  return (
    <main className="min-h-screen p-6 sm:p-12 max-w-6xl mx-auto">
      {/* Top Header */}
      <header className="flex justify-between items-center mb-8">
        <div className="flex items-center space-x-3">
          <div className="w-8 h-8 rounded-lg bg-blue-600 flex items-center justify-center font-bold text-white">⚡</div>
          <span className="font-extrabold text-xl tracking-tight">ApexStock Flash Sale</span>
        </div>
        <div className="flex items-center space-x-4">
          <Link
            href="/war-room"
            className="px-3 py-1.5 text-xs font-semibold rounded-lg bg-purple-500/20 text-purple-300 border border-purple-500/30 hover:bg-purple-500/30 transition-colors"
          >
            War Room Dashboard 📊
          </Link>
          <ThemeToggle />
        </div>
      </header>

      {/* Aurora Banner */}
      <AuroraHero
        badge="Flash Sale Săn Deal Chớp Nhoáng"
        title="Siêu Giảm Giá Công Nghệ 2026"
        subtitle="Hệ thống bán hàng chớp nhoáng với kiến trúc chịu tải phân tán, đảm bảo tuyệt đối không oversell."
      />

      {/* Architecture Toggle Bar */}
      <div className="flex items-center justify-between mb-8 p-4 rounded-xl border border-[var(--card-border)] bg-[var(--card-bg)]">
        <div>
          <span className="text-sm font-semibold text-slate-300">Cổng kết nối kiểm thử:</span>
          <p className="text-xs text-slate-400">Chọn kiến trúc backend phục vụ request</p>
        </div>
        <div className="flex space-x-2">
          <button
            onClick={() => setArch('baseline')}
            className={`px-3 py-1.5 rounded-lg text-xs font-bold transition-all ${
              arch === 'baseline'
                ? 'bg-amber-600 text-white shadow-md'
                : 'bg-slate-800 text-slate-400 hover:text-white'
            }`}
          >
            Baseline (:8081)
          </button>
          <button
            onClick={() => setArch('optimized')}
            className={`px-3 py-1.5 rounded-lg text-xs font-bold transition-all ${
              arch === 'optimized'
                ? 'bg-blue-600 text-white shadow-md'
                : 'bg-slate-800 text-slate-400 hover:text-white'
            }`}
          >
            Optimized (:8082)
          </button>
        </div>
      </div>

      {/* Products Grid */}
      {loading && (
        <div className="text-center py-16 text-slate-400">Đang tải danh sách sự kiện flash sale...</div>
      )}

      {error && (
        <div className="p-4 rounded-xl bg-red-500/10 border border-red-500/30 text-red-400 text-sm mb-6">
          {error}
        </div>
      )}

      {!loading && events.length === 0 && !error && (
        <div className="text-center py-16 text-slate-500">Chưa có sự kiện flash sale nào đang mở.</div>
      )}

      <div className="space-y-12">
        {events.map((event) => (
          <section key={event.id}>
            <div className="flex justify-between items-center mb-6">
              <h2 className="text-2xl font-bold tracking-tight text-[var(--foreground)]">{event.name}</h2>
              <span className="text-xs px-2.5 py-1 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                {event.status === 'active' ? 'Đang Diễn Ra' : 'Sắp Diễn Ra'}
              </span>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6">
              {event.items.map((item) => {
                const discount = Math.round(((item.original_price - item.sale_price) / item.original_price) * 100);
                const isSoldOut = item.available_stock <= 0;

                return (
                  <SpotlightCard key={item.id} className="flex flex-col justify-between">
                    <div>
                      <div className="relative aspect-video rounded-lg overflow-hidden bg-slate-800 mb-4 flex items-center justify-center text-4xl">
                        {item.product_image ? (
                          <img src={item.product_image} alt={item.product_name} className="object-cover w-full h-full" />
                        ) : (
                          '📱'
                        )}
                        {discount > 0 && (
                          <span className="absolute top-2 right-2 bg-red-600 text-white text-xs font-black px-2 py-0.5 rounded">
                            -{discount}%
                          </span>
                        )}
                      </div>

                      <h3 className="font-bold text-lg text-[var(--foreground)] mb-1">{item.product_name}</h3>
                      <p className="text-xs text-slate-400 line-clamp-2 mb-4">{item.product_description}</p>

                      <div className="flex items-baseline space-x-2 mb-4">
                        <span className="text-xl font-extrabold text-blue-400">
                          {item.sale_price.toLocaleString('vi-VN')} đ
                        </span>
                        <span className="text-xs text-slate-500 line-through">
                          {item.original_price.toLocaleString('vi-VN')} đ
                        </span>
                      </div>

                      {/* Stock Bar */}
                      <div className="w-full bg-slate-800 rounded-full h-2 mb-2 overflow-hidden">
                        <div
                          className={`h-2 rounded-full transition-all duration-500 ${
                            isSoldOut ? 'bg-red-600' : 'bg-emerald-500'
                          }`}
                          style={{
                            width: `${Math.min(100, (item.available_stock / item.initial_quantity) * 100)}%`,
                          }}
                        />
                      </div>
                      <div className="flex justify-between text-xs text-slate-400 mb-6">
                        <span>Kho còn: <strong className="text-white">{item.available_stock}</strong></span>
                        <span>Tổng mở bán: {item.initial_quantity}</span>
                      </div>
                    </div>

                    <Link
                      href={`/flash-sale/${item.id}?arch=${arch}`}
                      className={`w-full py-2.5 rounded-lg text-center font-bold text-sm transition-all block ${
                        isSoldOut
                          ? 'bg-slate-800 text-slate-500 cursor-not-allowed pointer-events-none'
                          : 'bg-blue-600 hover:bg-blue-500 text-white shadow-lg shadow-blue-500/25'
                      }`}
                    >
                      {isSoldOut ? 'Đã Hết Hàng' : 'Săn Ngay ⚡'}
                    </Link>
                  </SpotlightCard>
                );
              })}
            </div>
          </section>
        ))}
      </div>
    </main>
  );
}
