'use client';

import React from 'react';
import Link from 'next/link';
import ThemeToggle from '@/components/ui/ThemeToggle';
import { useAuth } from '@/context/AuthContext';
import { useCart } from '@/context/CartContext';
import { Architecture } from '@/lib/api';

interface NavbarProps {
  arch: Architecture;
  setArch?: (arch: Architecture) => void;
  searchQuery?: string;
  setSearchQuery?: (q: string) => void;
  selectedCategory?: string;
  setSelectedCategory?: (cat: string) => void;
}

export default function Navbar({
  arch,
  setArch,
  searchQuery = '',
  setSearchQuery,
  selectedCategory = 'all',
  setSelectedCategory,
}: NavbarProps) {
  const { user, setIsModalOpen } = useAuth();
  const { totalItems, setIsCartOpen } = useCart();

  const categories = [
    { id: 'all', label: 'Tất Cả' },
    { id: 'phone', label: 'Điện Thoại' },
    { id: 'laptop', label: 'Laptop & Mac' },
    { id: 'audio', label: 'Tai Nghe & Âm Thanh' },
    { id: 'watch', label: 'Đồng Hồ' },
    { id: 'keyboard', label: 'Bàn Phím & Phụ Kiện' },
  ];

  return (
    <header className="sticky top-0 z-40 w-full backdrop-blur-xl bg-slate-950/85 border-b border-slate-800/80 transition-all">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex items-center justify-between h-16 gap-4">
          {/* Logo & Brand */}
          <Link href="/" className="flex items-center space-x-3 group flex-shrink-0">
            <div className="w-10 h-10 rounded-xl bg-gradient-to-tr from-blue-600 via-indigo-600 to-cyan-400 p-0.5 shadow-lg shadow-blue-500/20 group-hover:scale-105 transition-transform">
              <div className="w-full h-full bg-slate-950 rounded-[10px] flex items-center justify-center text-blue-400 font-black text-xl">
                ⚡
              </div>
            </div>
            <div>
              <span className="font-black text-xl tracking-tight text-white group-hover:text-blue-400 transition-colors">
                ApexStock
              </span>
            </div>
          </Link>

          {/* Search bar */}
          {setSearchQuery && (
            <div className="flex-1 max-w-md hidden md:block">
              <div className="relative">
                <input
                  type="text"
                  placeholder="Tìm kiếm iPhone 15 Pro Max, MacBook M3, Sony XM5..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="w-full pl-9 pr-4 py-2 text-xs rounded-xl bg-slate-900/90 border border-slate-700/80 text-white placeholder-slate-500 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-all"
                />
                <span className="absolute left-3 top-2.5 text-xs text-slate-500">🔍</span>
                {searchQuery && (
                  <button
                    onClick={() => setSearchQuery('')}
                    className="absolute right-3 top-2.5 text-xs text-slate-400 hover:text-white"
                  >
                    ✕
                  </button>
                )}
              </div>
            </div>
          )}

          {/* Right Action Icons & Controls */}
          <div className="flex items-center space-x-2 sm:space-x-3 flex-shrink-0">
            {/* Architecture Selector Pill */}
            {setArch && (
              <div className="hidden lg:flex items-center bg-slate-900 p-1 rounded-xl border border-slate-700/80">
                <span className="text-[11px] font-semibold text-slate-400 px-2">Backend:</span>
                <button
                  onClick={() => setArch('baseline')}
                  className={`px-2.5 py-1 rounded-lg text-[11px] font-bold transition-all ${
                    arch === 'baseline'
                      ? 'bg-amber-600 text-white shadow-md'
                      : 'text-slate-400 hover:text-slate-200'
                  }`}
                  title="Postgres Pessimistic Lock (:8081)"
                >
                  Baseline (:8081)
                </button>
                <button
                  onClick={() => setArch('optimized')}
                  className={`px-2.5 py-1 rounded-lg text-[11px] font-bold transition-all ${
                    arch === 'optimized'
                      ? 'bg-blue-600 text-white shadow-md'
                      : 'text-slate-400 hover:text-slate-200'
                  }`}
                  title="Redis In-Memory + NATS JetStream (:8082)"
                >
                  Optimized (:8082)
                </button>
              </div>
            )}

            {/* Orders Link */}
            <Link
              href="/orders"
              className="flex items-center space-x-1.5 px-3 py-1.5 text-xs font-semibold rounded-xl bg-slate-900 border border-slate-700/80 hover:bg-slate-800 text-slate-200 transition-colors"
              title="Xem danh sách đơn hàng đã đặt"
            >
              <span>📦</span>
              <span className="hidden sm:inline">Đơn hàng</span>
            </Link>

            {/* Cart Button */}
            <button
              onClick={() => setIsCartOpen(true)}
              className="relative p-2 rounded-xl bg-slate-900 border border-slate-700/80 hover:bg-slate-800 text-slate-200 transition-colors"
              title="Mở giỏ hàng"
            >
              <span className="text-lg">🛒</span>
              {totalItems > 0 && (
                <span className="absolute -top-1.5 -right-1.5 w-5 h-5 rounded-full bg-blue-600 text-white text-[10px] font-black flex items-center justify-center ring-2 ring-slate-950 animate-bounce">
                  {totalItems}
                </span>
              )}
            </button>

            {/* Demo User Avatar Button */}
            <button
              onClick={() => setIsModalOpen(true)}
              className="flex items-center space-x-2 p-1.5 sm:px-3 sm:py-1.5 rounded-xl bg-slate-900 border border-slate-700/80 hover:bg-slate-800 text-slate-200 transition-colors"
              title="Đổi tài khoản kiểm thử"
            >
              <span className="text-base">{user.avatar}</span>
              <div className="text-left hidden sm:block">
                <div className="text-xs font-semibold leading-tight text-white max-w-[110px] truncate">
                  {user.name}
                </div>
                <div className="text-[10px] text-emerald-400 leading-tight">Đang kết nối</div>
              </div>
            </button>

            {/* Theme Toggle */}
            <ThemeToggle />
          </div>
        </div>

        {/* Categories Bar */}
        {setSelectedCategory && (
          <div className="flex items-center space-x-2 py-2 overflow-x-auto no-scrollbar border-t border-slate-800/60">
            <span className="text-xs text-slate-400 font-semibold flex-shrink-0 mr-1">Danh mục:</span>
            {categories.map((c) => {
              const active = selectedCategory === c.id;
              return (
                <button
                  key={c.id}
                  onClick={() => setSelectedCategory(c.id)}
                  className={`text-xs px-3 py-1 rounded-full whitespace-nowrap transition-all font-medium ${
                    active
                      ? 'bg-blue-600 text-white font-bold shadow-sm shadow-blue-500/30'
                      : 'bg-slate-900/60 hover:bg-slate-800 text-slate-400 hover:text-slate-200 border border-slate-800'
                  }`}
                >
                  {c.label}
                </button>
              );
            })}
          </div>
        )}
      </div>
    </header>
  );
}
