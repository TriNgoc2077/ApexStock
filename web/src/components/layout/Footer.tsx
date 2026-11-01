'use client';

import React from 'react';
import Link from 'next/link';

export default function Footer() {
  return (
    <footer className="w-full bg-slate-950 border-t border-slate-800 text-slate-400 mt-20 pt-12 pb-8">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-8 pb-10 border-b border-slate-800/80">
          {/* Col 1: Brand & Purpose */}
          <div className="space-y-4">
            <div className="flex items-center space-x-2.5">
              <div className="w-8 h-8 rounded-lg bg-blue-600 flex items-center justify-center font-bold text-white shadow-md shadow-blue-500/20">
                ⚡
              </div>
              <span className="font-extrabold text-lg text-white tracking-tight">ApexStock Demo</span>
            </div>
            <p className="text-xs text-slate-400 leading-relaxed">
              Hệ thống bán hàng chớp nhoáng (Flash Sale) và Thương mại điện tử phân tán với hai kiến trúc đối chiếu:
              <strong> Baseline</strong> (PostgreSQL khóa bi quan) và <strong>Optimized</strong> (Redis In-Memory + NATS JetStream).
            </p>
            <div className="flex items-center space-x-2 text-xs">
              <span className="px-2 py-1 rounded bg-slate-900 border border-slate-800 text-emerald-400 font-mono">
                ✓ 0% Oversell Invariant
              </span>
            </div>
          </div>

          {/* Col 2: Technology Stack */}
          <div>
            <h4 className="text-xs font-bold text-white uppercase tracking-wider mb-4">Kiến Trúc Công Nghệ</h4>
            <ul className="space-y-2 text-xs">
              <li className="flex items-center space-x-2">
                <span className="w-1.5 h-1.5 rounded-full bg-blue-500"></span>
                <span>Go 1.24 (High-Performance Engine)</span>
              </li>
              <li className="flex items-center space-x-2">
                <span className="w-1.5 h-1.5 rounded-full bg-indigo-500"></span>
                <span>PostgreSQL 17 (ACID Data Store)</span>
              </li>
              <li className="flex items-center space-x-2">
                <span className="w-1.5 h-1.5 rounded-full bg-red-500"></span>
                <span>Redis 7 (Atomic Lua Scripts)</span>
              </li>
              <li className="flex items-center space-x-2">
                <span className="w-1.5 h-1.5 rounded-full bg-teal-500"></span>
                <span>NATS JetStream (Async Worker Queue)</span>
              </li>
              <li className="flex items-center space-x-2">
                <span className="w-1.5 h-1.5 rounded-full bg-purple-500"></span>
                <span>Next.js 15 & React 19 Frontend</span>
              </li>
            </ul>
          </div>

          {/* Col 3: Testing & Benchmark */}
          <div>
            <h4 className="text-xs font-bold text-white uppercase tracking-wider mb-4">Kiểm Thử Tải Trọng</h4>
            <ul className="space-y-2 text-xs">
              <li>
                <Link href="/war-room" className="hover:text-blue-400 transition-colors flex items-center space-x-1">
                  <span>📊</span>
                  <span>Bảng điều khiển War Room Dashboard</span>
                </Link>
              </li>
              <li className="hover:text-slate-300">Kịch bản K6 Sustainable Throughput (10k RPS)</li>
              <li className="hover:text-slate-300">Kịch bản K6 Maximum RPS Shock Load (40k RPS)</li>
              <li className="hover:text-slate-300">Chaos Engineering & Invariant Verifier</li>
              <li className="hover:text-slate-300">SePay Webhook Simulator & Auto Restock</li>
            </ul>
          </div>

          {/* Col 4: Project & Author */}
          <div>
            <h4 className="text-xs font-bold text-white uppercase tracking-wider mb-4">Thông Tin Đề Tài</h4>
            <div className="space-y-2 text-xs">
              <p><strong>Đồ án:</strong> Báo Cáo Thực Tập Tốt Nghiệp</p>
              <p><strong>Học viên / Tác giả:</strong> Cao Trí Ngọc</p>
              <p><strong>Mục tiêu:</strong> Thiết kế kiến trúc giải quyết bài toán nghẽn cổ chai và Race Condition trong Flash Sale</p>
              <p className="pt-2 text-[11px] text-slate-500">
                Tích hợp cổng thanh toán mô phỏng VietQR SePay thời gian thực.
              </p>
            </div>
          </div>
        </div>

        {/* Bottom bar */}
        <div className="pt-6 flex flex-col sm:flex-row justify-between items-center text-xs text-slate-500">
          <p>© 2026 ApexStock Platform. Nghiên cứu & Phát triển phục vụ Hội đồng Đánh giá Báo cáo Thực tập.</p>
          <div className="flex space-x-4 mt-4 sm:mt-0">
            <span className="text-emerald-400">● Cụm API Baseline :8081 Active</span>
            <span className="text-blue-400">● Cụm API Optimized :8082 Active</span>
          </div>
        </div>
      </div>
    </footer>
  );
}
