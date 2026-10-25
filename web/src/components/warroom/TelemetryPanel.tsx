'use client';

import React from 'react';

export interface ArchitectureTelemetry {
  arch: string;
  healthy: boolean;
  status: string;
  rps: number;
  p50_latency_ms: number;
  p95_latency_ms: number;
  p99_latency_ms: number;
  status_2xx: number;
  status_409: number;
  status_429: number;
  status_5xx: number;
  remaining_stock: number;
  oversell_count: number;
}

interface TelemetryPanelProps {
  title: string;
  badge: string;
  arch: 'baseline' | 'optimized';
  telemetry?: ArchitectureTelemetry;
}

export default function TelemetryPanel({ title, badge, arch, telemetry }: TelemetryPanelProps) {
  const isHealthy = telemetry?.healthy ?? false;
  const isOptimized = arch === 'optimized';
  const hasOversell = (telemetry?.oversell_count ?? 0) > 0;

  return (
    <div className={`p-6 rounded-2xl border ${isOptimized ? 'border-blue-500/30 bg-blue-950/10' : 'border-amber-500/30 bg-amber-950/10'} flex flex-col justify-between`}>
      {/* Top Header */}
      <div>
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center space-x-2">
            <span className={`w-3 h-3 rounded-full ${isHealthy ? 'bg-emerald-400 animate-pulse' : 'bg-red-500'}`} />
            <h2 className="text-xl font-extrabold tracking-tight text-white">{title}</h2>
          </div>
          <span className={`text-xs px-2.5 py-1 rounded-full font-bold uppercase ${
            isOptimized ? 'bg-blue-500/20 text-blue-300' : 'bg-amber-500/20 text-amber-300'
          }`}>
            {badge}
          </span>
        </div>

        {/* Invariant Indicator: Oversell Total */}
        <div className={`p-4 rounded-xl mb-6 border text-center transition-all ${
          hasOversell
            ? 'bg-red-500/20 border-red-500 text-red-300 animate-bounce'
            : isOptimized
            ? 'spin-border-wrapper'
            : 'bg-slate-900/60 border-slate-700 text-slate-300'
        }`}>
          <div className={!hasOversell && isOptimized ? 'spin-border-inner p-3' : ''}>
            <span className="text-xs uppercase font-bold tracking-wider block mb-1">
              {hasOversell ? '🚨 VI PHẠM INVARIANT: OVERSELL PHÁT HIỆN' : '🛡️ Invariant Check: Oversell Total'}
            </span>
            <span className={`text-2xl font-black ${hasOversell ? 'text-red-400' : 'text-emerald-400'}`}>
              {telemetry?.oversell_count ?? 0} VI PHẠM
            </span>
          </div>
        </div>

        {/* Metrics Grid */}
        <div className="grid grid-cols-2 gap-4 mb-6">
          <div className="p-3 rounded-xl bg-slate-900/60 border border-slate-800">
            <span className="text-xs text-slate-400 block">Tải Thông Lượng (RPS)</span>
            <span className="text-2xl font-black text-white">{telemetry?.rps.toFixed(1) ?? '0.0'}</span>
          </div>
          <div className="p-3 rounded-xl bg-slate-900/60 border border-slate-800">
            <span className="text-xs text-slate-400 block">Độ Trễ p99</span>
            <span className="text-2xl font-black text-blue-400">{telemetry?.p99_latency_ms.toFixed(1) ?? '0.0'} ms</span>
          </div>
          <div className="p-3 rounded-xl bg-slate-900/60 border border-slate-800">
            <span className="text-xs text-slate-400 block">Độ Trễ p50</span>
            <span className="text-xl font-bold text-slate-300">{telemetry?.p50_latency_ms.toFixed(1) ?? '0.0'} ms</span>
          </div>
          <div className="p-3 rounded-xl bg-slate-900/60 border border-slate-800">
            <span className="text-xs text-slate-400 block">Kho Còn Lại</span>
            <span className="text-xl font-bold text-emerald-400">{telemetry?.remaining_stock ?? 0}</span>
          </div>
        </div>

        {/* Status Distribution */}
        <div className="space-y-2 text-xs">
          <span className="font-bold text-slate-400 uppercase tracking-wider block mb-2">Phân Phối Mã HTTP</span>
          <div className="flex justify-between py-1 border-b border-slate-800">
            <span className="text-emerald-400">2xx Thành công:</span>
            <strong className="text-white">{telemetry?.status_2xx ?? 0}</strong>
          </div>
          <div className="flex justify-between py-1 border-b border-slate-800">
            <span className="text-amber-400">409 Hết hàng / Trùng:</span>
            <strong className="text-white">{telemetry?.status_409 ?? 0}</strong>
          </div>
          <div className="flex justify-between py-1 border-b border-slate-800">
            <span className="text-purple-400">429 Rate limit:</span>
            <strong className="text-white">{telemetry?.status_429 ?? 0}</strong>
          </div>
          <div className="flex justify-between py-1">
            <span className="text-red-400">5xx Lỗi hệ thống:</span>
            <strong className="text-red-400 font-bold">{telemetry?.status_5xx ?? 0}</strong>
          </div>
        </div>
      </div>
    </div>
  );
}
