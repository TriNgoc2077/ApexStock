'use client';

import React, { useState } from 'react';

interface RunControlBarProps {
  onStartRun: (mode: 'sequential' | 'parallel_demo', vus: number, duration: string) => Promise<void>;
  isRunning: boolean;
}

export default function RunControlBar({ onStartRun, isRunning }: RunControlBarProps) {
  const [vus, setVus] = useState<number>(1000);
  const [duration, setDuration] = useState<string>('60s');
  const [isParallelDemo, setIsParallelDemo] = useState<boolean>(false);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onStartRun(isParallelDemo ? 'parallel_demo' : 'sequential', vus, duration);
  };

  return (
    <form onSubmit={handleSubmit} className="p-4 rounded-xl border border-[var(--card-border)] bg-[var(--card-bg)] mb-8 flex flex-wrap items-center justify-between gap-4">
      <div className="flex flex-wrap items-center gap-6">
        <div>
          <label className="text-xs font-semibold text-slate-400 block mb-1">Mức Tải Người Dùng (VUs):</label>
          <select
            value={vus}
            onChange={(e) => setVus(Number(e.target.value))}
            disabled={isRunning}
            className="px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-700 text-xs text-white"
          >
            <option value={100}>100 VUs (Smoke)</option>
            <option value={500}>500 VUs (Medium)</option>
            <option value={1000}>1,000 VUs (Full Load)</option>
          </select>
        </div>

        <div>
          <label className="text-xs font-semibold text-slate-400 block mb-1">Thời Gian Chạy:</label>
          <select
            value={duration}
            onChange={(e) => setDuration(e.target.value)}
            disabled={isRunning}
            className="px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-700 text-xs text-white"
          >
            <option value="30s">30 giây</option>
            <option value="60s">60 giây</option>
          </select>
        </div>

        <div className="flex items-center space-x-2 pt-4">
          <input
            type="checkbox"
            id="parallelToggle"
            checked={isParallelDemo}
            onChange={(e) => setIsParallelDemo(e.target.checked)}
            disabled={isRunning}
            className="rounded border-slate-700"
          />
          <label htmlFor="parallelToggle" className="text-xs text-amber-300 font-semibold cursor-pointer">
            Chế độ chạy song song (Chỉ dùng cho Visual Demo - D0.11)
          </label>
        </div>
      </div>

      <button
        type="submit"
        disabled={isRunning}
        className={`px-6 py-2.5 rounded-xl font-black text-sm transition-all ${
          isRunning
            ? 'bg-slate-800 text-slate-500 cursor-wait animate-pulse'
            : isParallelDemo
            ? 'bg-amber-600 hover:bg-amber-500 text-white shadow-lg shadow-amber-500/25'
            : 'bg-blue-600 hover:bg-blue-500 text-white shadow-lg shadow-blue-500/25'
        }`}
      >
        {isRunning ? '⏳ Đang Chạy Benchmark...' : isParallelDemo ? '▶️ Bắt Đầu Demo Song Song' : '🚀 Bắt Đầu Benchmark Tuần Tự (Chuẩn D0.11)'}
      </button>
    </form>
  );
}
