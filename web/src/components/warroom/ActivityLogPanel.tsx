'use client';

import React from 'react';

export interface ActivityLogEntry {
  id: string;
  timestamp: string;
  type: string;
  message: string;
  recovery_time_ms?: number;
}

interface ActivityLogPanelProps {
  logs: ActivityLogEntry[];
}

export default function ActivityLogPanel({ logs }: ActivityLogPanelProps) {
  return (
    <div className="p-6 rounded-2xl border border-[var(--card-border)] bg-[var(--card-bg)]">
      <div className="flex items-center justify-between mb-4">
        <h3 className="font-bold text-base text-white">Nhật Ký Hoạt Động & Chaos Recovery (Real-time)</h3>
        <span className="text-xs text-slate-400">{logs.length} bản ghi</span>
      </div>

      <div className="max-h-64 overflow-y-auto space-y-2 pr-2 font-mono text-xs">
        {logs.length === 0 ? (
          <p className="text-slate-500 py-8 text-center font-sans">Chưa có sự kiện nào được ghi nhận.</p>
        ) : (
          logs.slice().reverse().map((log) => {
            const isChaos = log.type === 'chaos_detected';
            const isRecovery = log.type === 'recovered';

            return (
              <div
                key={log.id}
                className={`p-2.5 rounded-lg border ${
                  isChaos
                    ? 'bg-red-500/10 border-red-500/30 text-red-300'
                    : isRecovery
                    ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-300'
                    : 'bg-slate-900/60 border-slate-800 text-slate-300'
                }`}
              >
                <div className="flex justify-between items-baseline mb-1">
                  <span className="text-slate-400 text-[10px]">
                    {new Date(log.timestamp).toLocaleTimeString('vi-VN')}
                  </span>
                  {log.recovery_time_ms && (
                    <span className="px-1.5 py-0.5 rounded bg-emerald-500/20 text-emerald-300 font-bold text-[10px]">
                      Phục hồi sau: {(log.recovery_time_ms / 1000).toFixed(2)}s
                    </span>
                  )}
                </div>
                <div>{log.message}</div>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
