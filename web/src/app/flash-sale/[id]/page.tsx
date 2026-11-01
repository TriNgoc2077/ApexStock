'use client';

import React, { useEffect, useState, useMemo, Suspense } from 'react';
import { useParams, useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import Navbar from '@/components/layout/Navbar';
import Footer from '@/components/layout/Footer';
import CartDrawer from '@/components/cart/CartDrawer';
import SpotlightCard from '@/components/ui/SpotlightCard';
import CountdownTimer from '@/components/ui/CountdownTimer';
import { api, FlashSaleItem, Architecture, getOrCreateIdempotencyKey, clearIdempotencyKey } from '@/lib/api';
import { useAuth } from '@/context/AuthContext';
import { useCart } from '@/context/CartContext';
import { PRODUCT_EXTRA_DATA, DEFAULT_PRODUCT_INFO } from '@/data/productDetails';

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
  const [activeTab, setActiveTab] = useState<'highlights' | 'specs' | 'reviews' | 'tech'>('highlights');

  const { user, setIsModalOpen } = useAuth();
  const { addToCart } = useCart();

  useEffect(() => {
    if (!id) return;
    setLoading(true);
    api.getItemDetail(id, arch)
      .then((data) => {
        setItem(data);
        setError(null);
      })
      .catch((err) => setError(err.message || 'Không thể tải thông tin sản phẩm'))
      .finally(() => setLoading(false));
  }, [id, arch]);

  const extra = useMemo(() => {
    if (!item) return DEFAULT_PRODUCT_INFO;
    return PRODUCT_EXTRA_DATA[item.product_id] || DEFAULT_PRODUCT_INFO;
  }, [item]);

  const [selectedColor, setSelectedColor] = useState<string>('');
  const [selectedOption, setSelectedOption] = useState<string>('');

  useEffect(() => {
    if (extra.variants.colors.length > 0 && !selectedColor) {
      setSelectedColor(extra.variants.colors[0].name);
    }
    if (extra.variants.options.length > 0 && !selectedOption) {
      setSelectedOption(extra.variants.options[0]);
    }
  }, [extra, selectedColor, selectedOption]);

  const handleFlashBuy = async () => {
    if (!item || purchasing) return;
    setPurchasing(true);
    setError(null);

    // Use current authenticated user's ID (valid in database)
    const idempotencyKey = getOrCreateIdempotencyKey(item.id, user.id);

    try {
      const order = await api.createOrder({
        user_id: user.id,
        flash_sale_item_id: item.id,
        quantity: 1,
        idempotency_key: idempotencyKey,
      }, arch);

      clearIdempotencyKey(item.id, user.id);

      // Save order to user order history in localStorage
      try {
        const historyKey = `apexstock_orders_${user.id}`;
        const existing = JSON.parse(localStorage.getItem(historyKey) || '[]');
        existing.unshift({
          order_id: order.order_id,
          product_name: item.product_name,
          product_image: item.product_image,
          quantity: 1,
          total_amount: order.total_amount,
          status: order.status || 'pending_payment',
          created_at: new Date().toISOString(),
          arch: arch,
        });
        localStorage.setItem(historyKey, JSON.stringify(existing.slice(0, 30)));
      } catch {}

      router.push(`/orders/${order.order_id}?arch=${arch}`);
    } catch (err: any) {
      setError(err.message || 'Lỗi đặt hàng');
      setPurchasing(false);
    }
  };

  const isSoldOut = item ? item.available_stock <= 0 : false;
  const discount = item ? Math.round(((item.original_price - item.sale_price) / item.original_price) * 100) : 0;
  const savedAmount = item ? item.original_price - item.sale_price : 0;
  const stockPercent = item ? Math.min(100, Math.max(5, (item.available_stock / item.initial_quantity) * 100)) : 0;

  return (
    <div className="min-h-screen flex flex-col bg-[var(--background)] text-[var(--foreground)]">
      <Navbar arch={arch} setArch={setArch} />

      <main className="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-8 space-y-10">
        {/* Breadcrumb Navigation */}
        <nav className="flex items-center space-x-2 text-xs text-slate-400">
          <Link href="/" className="hover:text-white transition-colors">Trang chủ</Link>
          <span>/</span>
          <span className="text-blue-400 font-medium">Flash Sale Deals</span>
          <span>/</span>
          <span className="text-slate-200 truncate max-w-xs">{item?.product_name || 'Chi tiết'}</span>
        </nav>

        {loading && (
          <div className="py-32 text-center space-y-3">
            <div className="inline-block w-8 h-8 border-4 border-blue-500 border-t-transparent rounded-full animate-spin" />
            <p className="text-sm text-slate-400">Đang tải thông tin chi tiết sản phẩm từ Backend {arch.toUpperCase()}...</p>
          </div>
        )}

        {!loading && !item && (
          <div className="py-24 text-center space-y-4 max-w-md mx-auto">
            <span className="text-5xl block">⚠️</span>
            <h2 className="text-xl font-bold text-red-400">Không tìm thấy sản phẩm</h2>
            <p className="text-xs text-slate-400">{error || 'Sản phẩm này có thể đã kết thúc hoặc không tồn tại trong sự kiện hiện tại.'}</p>
            <Link href="/" className="inline-block px-5 py-2.5 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-bold transition-all">
              Quay lại trang chủ
            </Link>
          </div>
        )}

        {!loading && item && (
          <>
            {/* Top Product Detail Hero Section */}
            <div className="grid grid-cols-1 lg:grid-cols-12 gap-10">
              {/* Left Column: Media Gallery */}
              <div className="lg:col-span-6 space-y-4">
                <div className="relative aspect-square rounded-3xl overflow-hidden bg-slate-900 border border-slate-800 p-2 shadow-2xl">
                  <img
                    src={item.product_image || '/images/products/iphone15.jpg'}
                    alt={item.product_name}
                    className="w-full h-full object-cover rounded-2xl"
                  />
                  {discount > 0 && (
                    <div className="absolute top-5 right-5 bg-gradient-to-r from-red-600 to-rose-600 text-white text-sm font-black px-3 py-1 rounded-xl shadow-lg shadow-red-500/30">
                      GIẢM {discount}%
                    </div>
                  )}
                  <div className="absolute top-5 left-5 bg-black/60 backdrop-blur-md text-emerald-400 text-xs font-bold px-3 py-1 rounded-xl border border-emerald-500/30 flex items-center space-x-1.5">
                    <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
                    <span>CHÍNH HÃNG VN/A</span>
                  </div>
                </div>

                {/* Sub Thumbnails */}
                <div className="grid grid-cols-4 gap-3">
                  {[item.product_image, item.product_image, item.product_image, item.product_image].map((img, idx) => (
                    <div
                      key={idx}
                      className="aspect-square rounded-xl bg-slate-900 border border-slate-800 hover:border-blue-500 cursor-pointer overflow-hidden p-1 transition-all"
                    >
                      <img src={img || '/images/products/iphone15.jpg'} alt="" className="w-full h-full object-cover rounded-lg" />
                    </div>
                  ))}
                </div>

                {/* Trust Badges */}
                <div className="grid grid-cols-2 gap-3 pt-2 text-xs text-slate-300">
                  <div className="p-3 rounded-xl bg-slate-900/60 border border-slate-800/80 flex items-center space-x-2.5">
                    <span className="text-xl">🛡️</span>
                    <div>
                      <div className="font-semibold text-white">Bảo hành 12 tháng</div>
                      <div className="text-[10px] text-slate-500">Tại TTBH ủy quyền toàn quốc</div>
                    </div>
                  </div>
                  <div className="p-3 rounded-xl bg-slate-900/60 border border-slate-800/80 flex items-center space-x-2.5">
                    <span className="text-xl">⚡</span>
                    <div>
                      <div className="font-semibold text-white">Giao hỏa tốc 2 giờ</div>
                      <div className="text-[10px] text-slate-500">Miễn phí vận chuyển toàn quốc</div>
                    </div>
                  </div>
                </div>
              </div>

              {/* Right Column: Purchasing Panel */}
              <div className="lg:col-span-6 space-y-6">
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-xs font-semibold px-2.5 py-1 rounded-full bg-blue-500/15 text-blue-400 border border-blue-500/30">
                      {extra.categoryName} • Mã: {item.id.slice(0, 8)}
                    </span>
                    <div className="flex items-center space-x-2 text-xs">
                      <span className="text-slate-400">Tuyến xử lý:</span>
                      <strong className={`font-mono uppercase px-2 py-0.5 rounded ${
                        arch === 'baseline' ? 'bg-amber-500/20 text-amber-300' : 'bg-blue-500/20 text-blue-300'
                      }`}>
                        {arch} (: {arch === 'baseline' ? '8081' : '8082'})
                      </strong>
                    </div>
                  </div>

                  <h1 className="text-2xl sm:text-3xl font-black text-white tracking-tight leading-snug">
                    {item.product_name}
                  </h1>

                  <div className="flex items-center space-x-3 mt-2 text-xs">
                    <div className="flex items-center text-amber-400">
                      <span>★</span>
                      <span className="font-bold ml-1">{extra.rating}</span>
                    </div>
                    <span className="text-slate-500">•</span>
                    <span className="text-slate-400">{extra.reviewCount} đánh giá từ khách hàng</span>
                    <span className="text-slate-500">•</span>
                    <span className="text-emerald-400 font-medium">Đã bán {item.initial_quantity - item.available_stock}</span>
                  </div>
                </div>

                {/* Price Box */}
                <div className="p-5 rounded-2xl bg-gradient-to-r from-slate-900 to-slate-900/90 border border-slate-800 space-y-3">
                  <div className="flex items-baseline space-x-3">
                    <span className="text-3xl sm:text-4xl font-black text-blue-400">
                      {item.sale_price.toLocaleString('vi-VN')} đ
                    </span>
                    <span className="text-base text-slate-500 line-through">
                      {item.original_price.toLocaleString('vi-VN')} đ
                    </span>
                  </div>
                  <div className="flex items-center justify-between text-xs pt-1 border-t border-slate-800">
                    <span className="text-emerald-400 font-semibold">
                      Tiết kiệm: {savedAmount.toLocaleString('vi-VN')} đ ({discount}%)
                    </span>
                    <div className="flex items-center space-x-2">
                      <span className="text-slate-400">Kết thúc sau:</span>
                      <CountdownTimer />
                    </div>
                  </div>
                </div>

                {/* Color Variants */}
                {extra.variants.colors.length > 0 && (
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-slate-300 flex justify-between">
                      <span>Màu sắc đã chọn: <strong className="text-white">{selectedColor}</strong></span>
                    </label>
                    <div className="flex flex-wrap gap-2">
                      {extra.variants.colors.map((c) => {
                        const active = selectedColor === c.name;
                        return (
                          <button
                            key={c.name}
                            onClick={() => setSelectedColor(c.name)}
                            className={`flex items-center space-x-2 px-3 py-1.5 rounded-xl border text-xs font-semibold transition-all ${
                              active
                                ? 'bg-blue-600/20 border-blue-500 text-white shadow-sm'
                                : 'bg-slate-900 border-slate-800 text-slate-400 hover:border-slate-700'
                            }`}
                          >
                            <span
                              className="w-3.5 h-3.5 rounded-full border border-black/40"
                              style={{ backgroundColor: c.hex }}
                            />
                            <span>{c.name}</span>
                          </button>
                        );
                      })}
                    </div>
                  </div>
                )}

                {/* Storage / Version Options */}
                {extra.variants.options.length > 0 && (
                  <div className="space-y-2">
                    <label className="text-xs font-semibold text-slate-300 flex justify-between">
                      <span>Phiên bản / Bộ nhớ: <strong className="text-white">{selectedOption}</strong></span>
                    </label>
                    <div className="flex flex-wrap gap-2">
                      {extra.variants.options.map((opt) => {
                        const active = selectedOption === opt;
                        return (
                          <button
                            key={opt}
                            onClick={() => setSelectedOption(opt)}
                            className={`px-3 py-1.5 rounded-xl border text-xs font-semibold transition-all ${
                              active
                                ? 'bg-blue-600 text-white border-blue-500 shadow-sm'
                                : 'bg-slate-900 border-slate-800 text-slate-400 hover:border-slate-700'
                            }`}
                          >
                            {opt}
                          </button>
                        );
                      })}
                    </div>
                  </div>
                )}

                {/* Stock Progress & Quota Info */}
                <div className="p-4 rounded-2xl bg-slate-900/60 border border-slate-800 space-y-2">
                  <div className="flex justify-between text-xs text-slate-300">
                    <span>Tình trạng kho hàng:</span>
                    <span>
                      Còn lại <strong className="text-white">{item.available_stock}</strong> / {item.initial_quantity} cái
                    </span>
                  </div>
                  <div className="w-full bg-slate-800 rounded-full h-2 overflow-hidden">
                    <div
                      className={`h-full rounded-full transition-all duration-500 ${
                        isSoldOut ? 'bg-red-600' : 'bg-gradient-to-r from-blue-500 to-emerald-400'
                      }`}
                      style={{ width: `${stockPercent}%` }}
                    />
                  </div>
                  <div className="flex justify-between items-center text-[11px] text-slate-500 pt-1">
                    <span>Quy định: Tối đa <strong>{item.max_per_user} sản phẩm</strong> / người</span>
                    <span className="text-emerald-400">Giữ hàng 15 phút sau khi tạo đơn</span>
                  </div>
                </div>

                {/* Current Demo User & Switcher */}
                <div className="p-3 rounded-2xl bg-slate-900/80 border border-slate-800 flex items-center justify-between text-xs">
                  <div className="flex items-center space-x-2.5">
                    <span className="text-xl">{user.avatar}</span>
                    <div>
                      <div className="text-white font-semibold flex items-center space-x-1.5">
                        <span>{user.name}</span>
                        <span className="text-[10px] px-1.5 py-0.2 rounded bg-blue-500/20 text-blue-400">Người mua</span>
                      </div>
                      <p className="text-[10px] text-slate-500 font-mono truncate max-w-[200px]">ID: {user.id}</p>
                    </div>
                  </div>
                  <button
                    onClick={() => setIsModalOpen(true)}
                    className="px-2.5 py-1 text-[11px] font-semibold text-blue-400 hover:text-blue-300 hover:bg-slate-800 rounded-lg transition-colors border border-blue-500/30"
                  >
                    Đổi tài khoản ⇄
                  </button>
                </div>

                {/* Error Banner */}
                {error && (
                  <div className="p-3.5 rounded-xl bg-red-500/15 border border-red-500/40 text-red-300 text-xs flex items-center justify-between animate-shake">
                    <span>⚠️ {error}</span>
                    <button onClick={() => setError(null)} className="text-xs text-red-400 hover:text-white ml-2">✕</button>
                  </div>
                )}

                {/* Main Action Buttons */}
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2">
                  <button
                    onClick={() => addToCart(item, selectedColor, selectedOption)}
                    disabled={isSoldOut}
                    className="py-3.5 px-6 rounded-2xl bg-slate-900 hover:bg-slate-800 border border-slate-700 text-white font-bold text-xs sm:text-sm transition-all flex items-center justify-center space-x-2 shadow-lg disabled:opacity-50"
                  >
                    <span>🛒</span>
                    <span>Thêm Vào Giỏ Hàng</span>
                  </button>

                  <button
                    onClick={handleFlashBuy}
                    disabled={isSoldOut || purchasing}
                    className={`py-3.5 px-6 rounded-2xl font-black text-xs sm:text-sm transition-all flex items-center justify-center space-x-2 shadow-xl ${
                      isSoldOut
                        ? 'bg-slate-800 text-slate-500 cursor-not-allowed'
                        : 'bg-gradient-to-r from-blue-600 via-indigo-600 to-cyan-500 hover:from-blue-500 hover:to-cyan-400 text-white shadow-blue-500/30 hover:scale-[1.02] active:scale-[0.98]'
                    }`}
                  >
                    {purchasing ? (
                      <span className="flex items-center space-x-2">
                        <span className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                        <span>Đang xử lý tạo đơn...</span>
                      </span>
                    ) : (
                      <>
                        <span>⚡</span>
                        <span>{isSoldOut ? 'ĐÃ CHÁY HÀNG' : 'MUA NGAY (FLASH BUY)'}</span>
                      </>
                    )}
                  </button>
                </div>

                <p className="text-[11px] text-center text-slate-500">
                  🔒 Giao dịch demo được bảo vệ bởi Idempotency Key tự sinh chống tạo trùng lặp đơn hàng.
                </p>
              </div>
            </div>

            {/* Bottom Tabs: Highlights, Specs, Reviews, Academic Note */}
            <div className="pt-8 border-t border-slate-800 space-y-6">
              {/* Tab Navigation */}
              <div className="flex border-b border-slate-800 space-x-6 text-sm">
                <button
                  onClick={() => setActiveTab('highlights')}
                  className={`pb-3 font-bold transition-all relative ${
                    activeTab === 'highlights' ? 'text-blue-400' : 'text-slate-400 hover:text-white'
                  }`}
                >
                  Đặc Điểm Nổi Bật
                  {activeTab === 'highlights' && (
                    <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-blue-500 rounded-full" />
                  )}
                </button>
                <button
                  onClick={() => setActiveTab('specs')}
                  className={`pb-3 font-bold transition-all relative ${
                    activeTab === 'specs' ? 'text-blue-400' : 'text-slate-400 hover:text-white'
                  }`}
                >
                  Thông Số Kỹ Thuật
                  {activeTab === 'specs' && (
                    <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-blue-500 rounded-full" />
                  )}
                </button>
                <button
                  onClick={() => setActiveTab('reviews')}
                  className={`pb-3 font-bold transition-all relative ${
                    activeTab === 'reviews' ? 'text-blue-400' : 'text-slate-400 hover:text-white'
                  }`}
                >
                  Đánh Giá & Nhận Xét ({extra.reviews.length})
                  {activeTab === 'reviews' && (
                    <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-blue-500 rounded-full" />
                  )}
                </button>
                <button
                  onClick={() => setActiveTab('tech')}
                  className={`pb-3 font-bold transition-all relative ${
                    activeTab === 'tech' ? 'text-purple-400' : 'text-slate-400 hover:text-white'
                  }`}
                >
                  Ghi Chú Đồ Án Cho Hội Đồng 🎓
                  {activeTab === 'tech' && (
                    <span className="absolute bottom-0 left-0 right-0 h-0.5 bg-purple-500 rounded-full" />
                  )}
                </button>
              </div>

              {/* Tab Content */}
              <div className="p-6 rounded-2xl bg-slate-900/60 border border-slate-800">
                {activeTab === 'highlights' && (
                  <div className="space-y-4">
                    <h3 className="text-base font-bold text-white">Tính Năng & Công Nghệ Vượt Trội</h3>
                    <ul className="space-y-2 text-xs sm:text-sm text-slate-300">
                      {extra.highlights.map((h, i) => (
                        <li key={i} className="flex items-start space-x-2.5">
                          <span className="text-blue-400 mt-0.5">✓</span>
                          <span>{h}</span>
                        </li>
                      ))}
                    </ul>
                    <p className="text-xs text-slate-400 pt-2 leading-relaxed">
                      Sản phẩm được phân phối chính thức tại ApexStock. Cam kết 100% hàng mới nguyên seal,
                      đầy đủ hóa đơn điện tử VAT phục vụ doanh nghiệp và cá nhân.
                    </p>
                  </div>
                )}

                {activeTab === 'specs' && (
                  <div className="space-y-4">
                    <h3 className="text-base font-bold text-white">Bảng Thông Số Kỹ Thuật Chi Tiết</h3>
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-xs">
                      {Object.entries(extra.specs).map(([label, val]) => (
                        <div
                          key={label}
                          className="flex justify-between p-3 rounded-xl bg-slate-900 border border-slate-800/80"
                        >
                          <span className="text-slate-400 font-medium">{label}:</span>
                          <span className="text-white font-semibold text-right max-w-[60%]">{val}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {activeTab === 'reviews' && (
                  <div className="space-y-4">
                    <div className="flex items-center justify-between pb-3 border-b border-slate-800">
                      <div>
                        <h3 className="text-base font-bold text-white">Khách Hàng Đã Mua & Trải Nghiệm</h3>
                        <p className="text-xs text-slate-400">Tổng hợp đánh giá được xác thực qua đơn hàng</p>
                      </div>
                      <div className="flex items-center space-x-2 text-amber-400 font-bold">
                        <span className="text-xl">★</span>
                        <span className="text-lg">{extra.rating} / 5</span>
                      </div>
                    </div>

                    <div className="space-y-3">
                      {extra.reviews.map((r) => (
                        <div key={r.id} className="p-4 rounded-xl bg-slate-900 border border-slate-800 space-y-2">
                          <div className="flex justify-between items-center text-xs">
                            <div className="flex items-center space-x-2">
                              <span className="font-bold text-white">{r.userName}</span>
                              {r.verified && (
                                <span className="text-[10px] px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-400">
                                  ✓ Đã mua hàng
                                </span>
                              )}
                            </div>
                            <span className="text-slate-500 text-[11px]">{r.date}</span>
                          </div>
                          <div className="text-amber-400 text-xs">
                            {'★'.repeat(r.rating)}
                          </div>
                          <p className="text-xs text-slate-300 leading-relaxed">{r.comment}</p>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {activeTab === 'tech' && (
                  <div className="space-y-4">
                    <h3 className="text-base font-bold text-purple-300">
                      Cơ Chế Kỹ Thuật Đằng Sau Thao Tác Mua Hàng
                    </h3>
                    <p className="text-xs text-slate-300 leading-relaxed">
                      Khi bấm <strong>MUA NGAY (FLASH BUY)</strong>, ứng dụng sẽ thực hiện gọi API:
                      <code className="text-blue-300 font-mono ml-1">POST /api/v1/flash-sale/orders</code> trên cổng backend đang chọn:
                    </p>

                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-xs pt-1">
                      <div className="p-4 rounded-xl bg-slate-950 border border-amber-500/30 space-y-2">
                        <h4 className="font-bold text-amber-300">Nếu chọn Tuyến Baseline (:8081)</h4>
                        <p className="text-slate-400 leading-relaxed">
                          Tạo giao dịch Database <code>BEGIN TRANSACTION</code>, thực hiện khóa bi quan:
                          <code className="block mt-1 font-mono text-[11px] text-amber-200">
                            SELECT total_quantity, sold_quantity FROM flash_sale_items WHERE id = $1 FOR UPDATE;
                          </code>
                          Kiểm tra số lượng còn lại, trừ kho và insert đơn hàng. Giao dịch đồng bộ đảm bảo tính nhất quán cao nhưng dễ bị nghẽn nếu nhiều luồng cùng khóa 1 hàng.
                        </p>
                      </div>

                      <div className="p-4 rounded-xl bg-slate-950 border border-blue-500/30 space-y-2">
                        <h4 className="font-bold text-blue-300">Nếu chọn Tuyến Optimized (:8082)</h4>
                        <p className="text-slate-400 leading-relaxed">
                          Yêu cầu được chuyển qua Nginx cân bằng tải tới cụm API Go.
                          API thực thi Lua Script nguyên tử trên Redis <code>DECRBY flash_item:$id:stock</code>.
                          Nếu thành công, đẩy sự kiện đặt hàng vào NATS JetStream stream <code>ORDERS</code> và trả mã xác nhận ngay cho người dùng trong ~5-15ms.
                          Hàng đợi Worker Go phía sau sẽ lấy sự kiện ra ghi vào PostgreSQL một cách êm ái.
                        </p>
                      </div>
                    </div>
                  </div>
                )}
              </div>
            </div>
          </>
        )}
      </main>

      <CartDrawer arch={arch} />
      <Footer />
    </div>
  );
}

export default function FlashSaleDetailPage() {
  return (
    <Suspense fallback={<div className="min-h-screen flex items-center justify-center text-slate-400">Đang tải...</div>}>
      <FlashSaleDetailContent />
    </Suspense>
  );
}
