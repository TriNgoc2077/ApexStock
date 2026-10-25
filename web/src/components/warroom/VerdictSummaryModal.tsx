'use client';

import React from 'react';
import { ArchitectureTelemetry } from './TelemetryPanel';

interface VerdictSummaryModalProps {
  isOpen: boolean;
  onClose: () => void;
  baseline?: ArchitectureTelemetry;
  optimized?: ArchitectureTelemetry;
  isBenchmark: boolean;
}

export default function VerdictSummaryModal({
  isOpen,
  onClose,
  baseline,
  optimized,
  isBenchmark,
}: VerdictSummaryModalProps) {
  if (!isOpen) return null;

  const baselineP99 = baseline?.p99_latency_ms ?? 0;
  const optimizedP99 = optimized?.p99_latency_ms ?? 0;
  const speedup = baselineP99 > 0 && optimizedP99 > 0 ? (baselineP99 / optimizedP99).toFixed(1) : 'N/A';
  const baselineOversell = baseline?.oversell_count ?? 0;
  const optimizedOversell = optimized?.oversell_count ?? 0;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4">
      <div className="bg-slate-900 border border-slate-700 rounded-2xl max-w-xl w-full p-6 shadow-2xl relative">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-xl font-black text-white">Kết Quả Đánh Giá Hiệu Năng (Benchmark Verdict)</h3>
          <button
            onClick={onClose}
            className="text-slate-400 hover:text-white text-lg font-bold p-1"
          >
            ✕
          </button>
        </div>

        <div className="mb-4">
          <span className={`text-xs px-2.5 py-1 rounded-full font-bold uppercase ${
            isBenchmark ? 'bg-blue-500/20 text-blue-300' : 'bg-amber-500/20 text-amber-300'
          }`}>
            {isBenchmark ? 'Đánh Giá Chính Thức (D0.11 Sequential)' : 'Demo Trực Quan (Non-benchmark)'}
          </span>
        </div>

        <div className="space-y-4 mb-6 text-sm">
          <div className="p-4 rounded-xl bg-slate-950/50 border border-slate-800">
            <span className="text-xs text-slate-400 block mb-1">Kiểm Tra Invariant (Không Oversell)</span>
            <div className="flex justify-between items-center">
              <span>Optimized (Redis Lua + JetStream):</span>
              <strong className={optimizedOversell === 0 ? 'text-emerald-400' : 'text-red-400'}>
                {optimizedOversell === 0 ? '✅ HỢP LỆ (0 Oversell)' : `🚨 VI PHẠM (${optimizedOversell})`}
              </strong>
            </div>
            <div className="flex justify-between items-center mt-2">
              <span>Baseline (PostgreSQL Transaction):</span>
              <strong className={baselineOversell === 0 ? 'text-emerald-400' : 'text-red-400'}>
                {baselineOversell === 0 ? '✅ HỢP LỆ (0 Oversell)' : `🚨 VI PHẠM (${baselineOversell})`}
              </strong>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="p-3 rounded-xl bg-slate-950/50 border border-slate-800">
              <span className="text-xs text-slate-400 block">Baseline p99</span>
              <span className="text-xl font-bold text-amber-400">{baselineP99.toFixed(1)} ms</span>
            </div>
            <div className="p-3 rounded-xl bg-slate-950/50 border border-slate-800">
              <span className="text-xs text-slate-400 block">Optimized p99</span>
              <span className="text-xl font-bold text-blue-400">{optimizedP99.toFixed(1)} ms</span>
            </div>
          </div>

          {speedup !== 'N/A' && (
            <div className="p-3 rounded-xl bg-blue-500/10 border border-blue-500/30 text-center">
              <span className="text-xs text-blue-300 block mb-0.5">Tốc Độ Cải Thiện Độ Trễ p99</span>
              <span className="text-2xl font-black text-blue-400">{speedup}x</span>
            </div>
          )}
        </div>

        <button
          onClick={onClose}
          className="w-full py-2.5 rounded-xl bg-slate-800 hover:bg-slate-700 text-white font-bold text-sm transition-colors"
        >
          Đóng
        </button>
      </div>
    </div>
  );
}
