'use client';

import React, { useEffect, useState, useMemo } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import Navbar from '@/components/layout/Navbar';
import Footer from '@/components/layout/Footer';
import CartDrawer from '@/components/cart/CartDrawer';
import SpotlightCard from '@/components/ui/SpotlightCard';
import CountdownTimer from '@/components/ui/CountdownTimer';
import { api, FlashSaleEvent, FlashSaleItem, Architecture } from '@/lib/api';
import { useCart } from '@/context/CartContext';
import { useAuth } from '@/context/AuthContext';
import { PRODUCT_EXTRA_DATA, DEFAULT_PRODUCT_INFO } from '@/data/productDetails';

export default function HomePage() {
  const router = useRouter();
  const [arch, setArch] = useState<Architecture>('optimized');
  const [events, setEvents] = useState<FlashSaleEvent[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState<string>('');
  const [selectedCategory, setSelectedCategory] = useState<string>('all');
  const [copiedCoupon, setCopiedCoupon] = useState<boolean>(false);

  const { addToCart } = useCart();
  const { user } = useAuth();

  useEffect(() => {
    setLoading(true);
    api.getEvents(arch)
      .then((data) => {
        setEvents(data);
        setError(null);
      })
      .catch((err) => {
        setError(err.message || 'Không thể tải danh sách sản phẩm');
      })
      .finally(() => setLoading(false));
  }, [arch]);

  // Flatten all items across active events
  const allItems: FlashSaleItem[] = useMemo(() => {
    return events.flatMap((e) => e.items || []);
  }, [events]);

  // Filter items by category and search keyword
  const filteredItems = useMemo(() => {
    return allItems.filter((item) => {
      const extra = PRODUCT_EXTRA_DATA[item.product_id] || DEFAULT_PRODUCT_INFO;
      const matchCategory = selectedCategory === 'all' || extra.category === selectedCategory;
      const matchSearch =
        searchQuery.trim() === '' ||
        item.product_name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        item.product_description.toLowerCase().includes(searchQuery.toLowerCase());
      return matchCategory && matchSearch;
    });
  }, [allItems, selectedCategory, searchQuery]);

  // Dedicated spotlight item for benchmark presentation (iPhone 15 Pro Max)
  const heroItem = useMemo(() => {
    return (
      allItems.find((i) => i.id === '22222222-2222-2222-2222-222222222222') ||
      allItems[0] ||
      null
    );
  }, [allItems]);

  const handleCopyCoupon = () => {
    navigator.clipboard.writeText('APEX2026');
    setCopiedCoupon(true);
    setTimeout(() => setCopiedCoupon(false), 2500);
  };

  return (
    <div className="min-h-screen flex flex-col bg-[var(--background)] text-[var(--foreground)]">
      {/* Navigation Bar */}
      <Navbar
        arch={arch}
        setArch={setArch}
        searchQuery={searchQuery}
        setSearchQuery={setSearchQuery}
        selectedCategory={selectedCategory}
        setSelectedCategory={setSelectedCategory}
      />

      <main className="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-8 space-y-12">
        {/* Hero Banner Section */}
        <div className="relative rounded-3xl overflow-hidden border border-slate-800 bg-gradient-to-br from-slate-900 via-slate-900/90 to-blue-950/40 p-8 sm:p-12 shadow-2xl">
          {/* Subtle background glow */}
          <div className="absolute -top-24 -right-24 w-96 h-96 bg-blue-600/15 rounded-full blur-3xl pointer-events-none" />
          <div className="absolute -bottom-24 -left-24 w-96 h-96 bg-purple-600/15 rounded-full blur-3xl pointer-events-none" />

          <div className="relative z-10 grid grid-cols-1 lg:grid-cols-12 gap-8 items-center">
            {/* Left Content */}
            <div className="lg:col-span-7 space-y-6">
              <div className="inline-flex items-center space-x-2 px-3 py-1 rounded-full bg-blue-500/15 border border-blue-500/30 text-blue-400 text-xs font-semibold">
                <span className="w-2 h-2 rounded-full bg-blue-400 animate-ping" />
                <span>Chiến Dịch Siêu Giảm Giá Công Nghệ 2026</span>
              </div>

              <h1 className="text-3xl sm:text-5xl font-black tracking-tight text-white leading-tight">
                Săn Deal Chớp Nhoáng <br />
                <span className="bg-gradient-to-r from-blue-400 via-indigo-300 to-cyan-400 bg-clip-text text-transparent">
                  Không Sợ Sập Server
                </span>
              </h1>

              <p className="text-sm sm:text-base text-slate-300 max-w-xl leading-relaxed">
                Hệ thống thương mại điện tử với năng lực chịu tải hàng chục ngàn lượt mua đồng thời.
                Bảo đảm tuyệt đối <strong>0% oversell</strong> nhờ kết hợp hàng đợi sự kiện phân tán và bộ đệm nguyên tử.
              </p>

              {/* Countdown & Promo Voucher pill */}
              <div className="flex flex-wrap items-center gap-4 pt-2">
                <div className="flex items-center space-x-3 bg-slate-900/90 border border-slate-700/80 px-4 py-2 rounded-2xl shadow-inner">
                  <span className="text-xs font-semibold text-slate-400">Kết thúc sau:</span>
                  <CountdownTimer />
                </div>

                <button
                  onClick={handleCopyCoupon}
                  className="flex items-center space-x-2 bg-gradient-to-r from-amber-500/20 to-orange-500/20 border border-amber-500/30 px-4 py-2 rounded-2xl text-amber-300 hover:bg-amber-500/30 transition-all text-xs font-semibold"
                >
                  <span>🎟️ Mã voucher: <strong>APEX2026</strong></span>
                  <span className="text-[10px] px-1.5 py-0.5 rounded bg-amber-500/30 text-white font-mono">
                    {copiedCoupon ? 'Đã sao chép!' : 'Copy (-500k)'}
                  </span>
                </button>
              </div>

              {/* Architecture Context Bar */}
              <div className="p-4 rounded-2xl bg-slate-950/70 border border-slate-800 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                <div className="text-xs">
                  <span className="text-slate-400">Tuyến Backend Đang Hoạt Động: </span>
                  <strong className={arch === 'baseline' ? 'text-amber-400' : 'text-blue-400'}>
                    {arch === 'baseline' ? 'Baseline (:8081 - Postgres Pessimistic)' : 'Optimized (:8082 - Redis Lua + NATS)'}
                  </strong>
                  <p className="text-[11px] text-slate-500 mt-0.5">
                    Hội đồng có thể chuyển đổi linh hoạt tuyến xử lý để kiểm tra tính năng và so sánh.
                  </p>
                </div>
                <div className="flex space-x-2">
                  <button
                    onClick={() => setArch('baseline')}
                    className={`px-3 py-1.5 rounded-lg text-xs font-bold transition-all ${
                      arch === 'baseline'
                        ? 'bg-amber-600 text-white shadow-lg'
                        : 'bg-slate-800 text-slate-400 hover:text-white'
                    }`}
                  >
                    Baseline (:8081)
                  </button>
                  <button
                    onClick={() => setArch('optimized')}
                    className={`px-3 py-1.5 rounded-lg text-xs font-bold transition-all ${
                      arch === 'optimized'
                        ? 'bg-blue-600 text-white shadow-lg'
                        : 'bg-slate-800 text-slate-400 hover:text-white'
                    }`}
                  >
                    Optimized (:8082)
                  </button>
                </div>
              </div>
            </div>

            {/* Right Featured Card (Benchmark Target Star) */}
            {heroItem && (
              <div className="lg:col-span-5">
                <div
                  onClick={() => router.push(`/flash-sale/${heroItem.id}?arch=${arch}`)}
                  className="relative group p-1 rounded-3xl bg-gradient-to-b from-blue-500/30 via-indigo-500/10 to-transparent cursor-pointer transition-transform hover:-translate-y-1"
                >
                  <div className="rounded-[22px] bg-slate-900 border border-slate-700/80 p-6 space-y-5">
                    <div className="flex items-center justify-between">
                      <span className="text-[11px] font-bold px-2.5 py-1 rounded-full bg-red-500/20 text-red-400 border border-red-500/30 flex items-center space-x-1">
                        <span className="w-1.5 h-1.5 rounded-full bg-red-400 animate-pulse" />
                        <span>SẢN PHẨM BENCHMARK CHÍNH</span>
                      </span>
                      <span className="text-xs font-bold text-amber-400">GIẢM 50%</span>
                    </div>

                    <div className="aspect-[4/3] rounded-2xl overflow-hidden bg-slate-800 relative">
                      <img
                        src={heroItem.product_image || '/images/products/iphone15.jpg'}
                        alt={heroItem.product_name}
                        className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-500"
                      />
                      <div className="absolute bottom-2 left-2 px-2.5 py-1 rounded-lg bg-black/60 backdrop-blur-md text-white text-xs font-mono">
                        Kho: {heroItem.available_stock} / {heroItem.initial_quantity}
                      </div>
                    </div>

                    <div>
                      <h3 className="text-xl font-black text-white group-hover:text-blue-400 transition-colors">{heroItem.product_name}</h3>
                      <p className="text-xs text-slate-400 line-clamp-2 mt-1">{heroItem.product_description}</p>
                    </div>

                    <div className="flex items-baseline space-x-3">
                      <span className="text-2xl font-black text-blue-400">
                        {heroItem.sale_price.toLocaleString('vi-VN')} đ
                      </span>
                      <span className="text-sm text-slate-500 line-through">
                        {heroItem.original_price.toLocaleString('vi-VN')} đ
                      </span>
                    </div>

                    <div className="grid grid-cols-2 gap-3 pt-2">
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          addToCart(heroItem);
                        }}
                        className="py-2.5 px-4 rounded-xl bg-slate-800 hover:bg-slate-700 text-white font-semibold text-xs border border-slate-700 transition-colors flex items-center justify-center space-x-1.5"
                      >
                        <span>🛒</span>
                        <span>Thêm vào giỏ</span>
                      </button>
                      <Link
                        href={`/flash-sale/${heroItem.id}?arch=${arch}`}
                        onClick={(e) => e.stopPropagation()}
                        className="py-2.5 px-4 rounded-xl bg-blue-600 hover:bg-blue-500 text-white font-bold text-xs text-center shadow-lg shadow-blue-500/25 transition-all flex items-center justify-center space-x-1"
                      >
                        <span>⚡</span>
                        <span>MUA NGAY</span>
                      </Link>
                    </div>
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>

        {/* Section: Flash Sale Deals Showcase */}
        <section className="space-y-6">
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-2 border-b border-slate-800">
            <div>
              <div className="flex items-center space-x-2">
                <span className="text-2xl">⚡</span>
                <h2 className="text-2xl font-black text-white tracking-tight">
                  Flash Sale Deals Công Nghệ
                </h2>
              </div>
              <p className="text-xs text-slate-400 mt-1">
                Các sản phẩm mở bán có giới hạn số lượng và bảo vệ chống oversell qua API {arch.toUpperCase()}
              </p>
            </div>

            <div className="flex items-center space-x-2 text-xs">
              <span className="text-slate-400">Hiển thị:</span>
              <span className="px-2.5 py-1 rounded-lg bg-slate-900 border border-slate-800 font-bold text-blue-400">
                {filteredItems.length} sản phẩm
              </span>
            </div>
          </div>

          {/* Loading and Error states */}
          {loading && (
            <div className="py-20 text-center space-y-3">
              <div className="inline-block w-8 h-8 border-4 border-blue-500 border-t-transparent rounded-full animate-spin" />
              <p className="text-sm text-slate-400">Đang đồng bộ dữ liệu kho hàng từ Backend {arch.toUpperCase()}...</p>
            </div>
          )}

          {error && (
            <div className="p-4 rounded-2xl bg-red-500/10 border border-red-500/30 text-red-400 text-sm flex items-center justify-between">
              <div>
                <strong>Lỗi kết nối:</strong> {error}
                <p className="text-xs text-red-300 mt-0.5">Kiểm tra container backend {arch === 'baseline' ? ':8081' : ':8082'}</p>
              </div>
              <button
                onClick={() => {
                  setLoading(true);
                  api.getEvents(arch).then(setEvents).finally(() => setLoading(false));
                }}
                className="px-3 py-1.5 rounded-lg bg-red-600/30 hover:bg-red-600/50 text-white text-xs font-semibold"
              >
                Thử lại
              </button>
            </div>
          )}

          {/* Product Cards Grid */}
          {!loading && filteredItems.length === 0 && (
            <div className="py-16 text-center text-slate-500 space-y-2">
              <span className="text-4xl block">🔍</span>
              <p className="text-sm">Không tìm thấy sản phẩm nào khớp với từ khóa hoặc danh mục đã chọn.</p>
              <button
                onClick={() => {
                  setSearchQuery('');
                  setSelectedCategory('all');
                }}
                className="text-xs text-blue-400 hover:underline"
              >
                Đặt lại bộ lọc
              </button>
            </div>
          )}

          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-6">
            {filteredItems.map((item) => {
              const extra = PRODUCT_EXTRA_DATA[item.product_id] || DEFAULT_PRODUCT_INFO;
              const discount = Math.round(((item.original_price - item.sale_price) / item.original_price) * 100);
              const isSoldOut = item.available_stock <= 0;
              const stockPercent = Math.min(100, Math.max(5, (item.available_stock / item.initial_quantity) * 100));

              return (
                <SpotlightCard
                  key={item.id}
                  onClick={() => router.push(`/flash-sale/${item.id}?arch=${arch}`)}
                  className="flex flex-col justify-between group p-4 rounded-2xl bg-slate-900/80 border border-slate-800 hover:border-slate-700 transition-all hover:-translate-y-1 cursor-pointer"
                >
                  <div>
                    {/* Image Area */}
                    <div className="relative aspect-square rounded-xl overflow-hidden bg-slate-800 mb-4">
                      <img
                        src={item.product_image || '/images/products/iphone15.jpg'}
                        alt={item.product_name}
                        className="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
                      />
                      {discount > 0 && (
                        <span className="absolute top-2.5 right-2.5 bg-red-600 text-white text-[11px] font-black px-2 py-0.5 rounded-md shadow-md">
                          -{discount}%
                        </span>
                      )}
                      <span className="absolute top-2.5 left-2.5 bg-slate-950/70 backdrop-blur-md text-slate-300 text-[10px] font-semibold px-2 py-0.5 rounded border border-slate-700">
                        {extra.categoryName}
                      </span>
                    </div>

                    {/* Product Metadata */}
                    <div className="flex items-center space-x-1 text-amber-400 text-xs mb-1">
                      <span>★</span>
                      <span className="font-bold">{extra.rating}</span>
                      <span className="text-slate-500 text-[10px]">({extra.reviewCount})</span>
                    </div>

                    <h3 className="font-bold text-sm text-white line-clamp-1 mb-1 group-hover:text-blue-400 transition-colors">
                      {item.product_name}
                    </h3>
                    <p className="text-[11px] text-slate-400 line-clamp-2 mb-3 min-h-[32px]">
                      {item.product_description}
                    </p>

                    {/* Price */}
                    <div className="flex items-baseline space-x-2 mb-3">
                      <span className="text-base font-extrabold text-blue-400">
                        {item.sale_price.toLocaleString('vi-VN')} đ
                      </span>
                      <span className="text-xs text-slate-500 line-through">
                        {item.original_price.toLocaleString('vi-VN')} đ
                      </span>
                    </div>

                    {/* Stock status bar */}
                    <div className="space-y-1 mb-4">
                      <div className="w-full bg-slate-800 rounded-full h-1.5 overflow-hidden">
                        <div
                          className={`h-full rounded-full transition-all duration-500 ${
                            isSoldOut ? 'bg-red-600' : 'bg-gradient-to-r from-blue-500 to-emerald-400'
                          }`}
                          style={{ width: `${stockPercent}%` }}
                        />
                      </div>
                      <div className="flex justify-between text-[10px] text-slate-400">
                        <span>Kho còn: <strong className="text-white">{item.available_stock}</strong></span>
                        <span>Giới hạn: {item.max_per_user}/người</span>
                      </div>
                    </div>
                  </div>

                  {/* Actions */}
                  <div className="grid grid-cols-2 gap-2 pt-2 border-t border-slate-800/80">
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        addToCart(item);
                      }}
                      disabled={isSoldOut}
                      className="py-2 px-2 rounded-lg bg-slate-800 hover:bg-slate-700 disabled:opacity-50 text-slate-200 text-xs font-semibold transition-colors flex items-center justify-center space-x-1"
                    >
                      <span>🛒</span>
                      <span>Thêm giỏ</span>
                    </button>
                    <Link
                      href={`/flash-sale/${item.id}?arch=${arch}`}
                      onClick={(e) => e.stopPropagation()}
                      className={`py-2 px-2 rounded-lg text-center font-bold text-xs transition-all flex items-center justify-center space-x-1 ${
                        isSoldOut
                          ? 'bg-slate-800 text-slate-500 cursor-not-allowed pointer-events-none'
                          : 'bg-blue-600 hover:bg-blue-500 text-white shadow-md shadow-blue-500/20'
                      }`}
                    >
                      <span>⚡</span>
                      <span>{isSoldOut ? 'Hết Hàng' : 'Mua Ngay'}</span>
                    </Link>
                  </div>
                </SpotlightCard>
              );
            })}
          </div>
        </section>

        {/* Section: Service Commitments */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 pt-4">
          <div className="p-4 rounded-2xl bg-slate-900/60 border border-slate-800/80 flex items-center space-x-3.5 hover:border-slate-700 transition-colors">
            <div className="w-11 h-11 rounded-xl bg-blue-500/10 border border-blue-500/20 text-blue-400 flex items-center justify-center flex-shrink-0">
              <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
              </svg>
            </div>
            <div>
              <h4 className="text-xs font-bold text-white">Chính Hãng 100%</h4>
              <p className="text-[11px] text-slate-400 mt-0.5">Bảo hành 12 tháng tại TTBH ủy quyền</p>
            </div>
          </div>

          <div className="p-4 rounded-2xl bg-slate-900/60 border border-slate-800/80 flex items-center space-x-3.5 hover:border-slate-700 transition-colors">
            <div className="w-11 h-11 rounded-xl bg-amber-500/10 border border-amber-500/20 text-amber-400 flex items-center justify-center flex-shrink-0">
              <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
              </svg>
            </div>
            <div>
              <h4 className="text-xs font-bold text-white">Giao Hỏa Tốc 2H</h4>
              <p className="text-[11px] text-slate-400 mt-0.5">Nội thành Hà Nội & TP.HCM</p>
            </div>
          </div>

          <div className="p-4 rounded-2xl bg-slate-900/60 border border-slate-800/80 flex items-center space-x-3.5 hover:border-slate-700 transition-colors">
            <div className="w-11 h-11 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 flex items-center justify-center flex-shrink-0">
              <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 4v1m6 11h2m-6 0h-2v4m0-11v3m0 0h.01M12 12h4.01M16 20h4M4 12h4m12 0h.01M5 8h2a1 1 0 001-1V5a1 1 0 00-1-1H5a1 1 0 00-1 1v2a1 1 0 001 1zm12 0h2a1 1 0 001-1V5a1 1 0 00-1-1h-2a1 1 0 00-1 1v2a1 1 0 001 1zM5 20h2a1 1 0 001-1v-2a1 1 0 00-1-1H5a1 1 0 00-1 1v2a1 1 0 001 1z" />
              </svg>
            </div>
            <div>
              <h4 className="text-xs font-bold text-white">SePay VietQR Tự Động</h4>
              <p className="text-[11px] text-slate-400 mt-0.5">Xác nhận đơn tức thời qua Webhook</p>
            </div>
          </div>

          <div className="p-4 rounded-2xl bg-slate-900/60 border border-slate-800/80 flex items-center space-x-3.5 hover:border-slate-700 transition-colors">
            <div className="w-11 h-11 rounded-xl bg-purple-500/10 border border-purple-500/20 text-purple-400 flex items-center justify-center flex-shrink-0">
              <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
            </div>
            <div>
              <h4 className="text-xs font-bold text-white">Đổi Trả 30 Ngày</h4>
              <p className="text-[11px] text-slate-400 mt-0.5">1 đổi 1 nếu lỗi phần cứng nhà sản xuất</p>
            </div>
          </div>
        </div>
      </main>

      {/* Cart Drawer */}
      <CartDrawer arch={arch} />

      {/* Footer */}
      <Footer />
    </div>
  );
}
