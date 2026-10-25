'use client';

import React, { useEffect, useState } from 'react';
import Link from 'next/link';
import AuroraHero from '@/components/ui/AuroraHero';
import TelemetryPanel, { ArchitectureTelemetry } from '@/components/warroom/TelemetryPanel';
import ActivityLogPanel, { ActivityLogEntry } from '@/components/warroom/ActivityLogPanel';
import RunControlBar from '@/components/warroom/RunControlBar';
import VerdictSummaryModal from '@/components/warroom/VerdictSummaryModal';

interface WarRoomSnapshot {
  timestamp: string;
  baseline: ArchitectureTelemetry;
  optimized: ArchitectureTelemetry;
  activity_logs: ActivityLogEntry[];
  run_state: string;
  is_benchmark: boolean;
  run_id?: string;
}

export default function WarRoomPage() {
  const [snapshot, setSnapshot] = useState<WarRoomSnapshot | null>(null);
  const [connected, setConnected] = useState<boolean>(false);
  const [isStale, setIsStale] = useState<boolean>(false);
  const [isRunning, setIsRunning] = useState<boolean>(false);
  const [showVerdict, setShowVerdict] = useState<boolean>(false);

  useEffect(() => {
    let eventSource: EventSource | null = null;
    let staleTimer: NodeJS.Timeout | null = null;
    let reconnectTimer: NodeJS.Timeout | null = null;

    const connect = () => {
      eventSource = new EventSource('http://localhost:9000/api/v1/warroom/stream');

      eventSource.onopen = () => {
        setConnected(true);
        setIsStale(false);
      };

      eventSource.onmessage = (event) => {
        try {
          const data: WarRoomSnapshot = JSON.parse(event.data);
          setSnapshot(data);
          setIsRunning(data.run_state.startsWith('running'));
          setIsStale(false);

          if (staleTimer) clearTimeout(staleTimer);
          staleTimer = setTimeout(() => setIsStale(true), 3000);
        } catch {
          // Ignore parse errors on heartbeat comments
        }
      };

      eventSource.onerror = () => {
        setConnected(false);
        setIsStale(true);
        eventSource?.close();
        if (reconnectTimer) clearTimeout(reconnectTimer);
        reconnectTimer = setTimeout(connect, 2000);
      };
    };

    connect();

    return () => {
      eventSource?.close();
      if (staleTimer) clearTimeout(staleTimer);
      if (reconnectTimer) clearTimeout(reconnectTimer);
    };
  }, []);

  const handleStartRun = async (mode: 'sequential' | 'parallel_demo', vus: number, duration: string) => {
    try {
      await fetch('http://localhost:9000/api/v1/warroom/run', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ mode, vus, duration }),
      });
    } catch {
      alert('Không thể kết nối đến War Room controller');
    }
  };

  return (
    <main className="min-h-screen p-6 sm:p-12 max-w-7xl mx-auto">
      {/* Top Header */}
      <header className="flex justify-between items-center mb-8">
        <div className="flex items-center space-x-3">
          <Link href="/" className="text-sm text-slate-400 hover:text-white mr-2">← Trang Sản Phẩm</Link>
          <span className="font-extrabold text-2xl tracking-tight text-white">War Room Benchmark ⚡</span>
        </div>
        <div className="flex items-center space-x-3">
          {snapshot && (
            <button
              onClick={() => setShowVerdict(true)}
              className="px-3 py-1 rounded-full text-xs font-bold bg-blue-500/20 text-blue-300 border border-blue-500/30 hover:bg-blue-500/30 transition-colors"
            >
              📊 Xem Verdict
            </button>
          )}
          {isStale && (
            <span className="px-3 py-1 rounded-full text-xs font-bold bg-amber-500/20 text-amber-300 border border-amber-500/30 animate-pulse">
              ⚠️ MẤT KẾT NỐI / DỮ LIỆU CŨ (STALE)
            </span>
          )}
          <span className={`px-3 py-1 rounded-full text-xs font-bold border ${
            connected
              ? 'bg-emerald-500/20 text-emerald-400 border-emerald-500/30'
              : 'bg-red-500/20 text-red-400 border-red-500/30'
          }`}>
            {connected ? '● LIVE STREAM KẾT NỐI' : '○ MẤT TÍN HIỆU'}
          </span>
        </div>
      </header>

      <AuroraHero
        badge="Live Telemetry & Chaos Defense"
        title="Trung Tâm Chỉ Huy So Sánh Hiệu Năng"
        subtitle="So sánh trực tiếp kiến trúc Baseline và Optimized dưới tải cao và kịch bản chaos injection."
      />

      <RunControlBar onStartRun={handleStartRun} isRunning={isRunning} />

      {/* Side-by-Side Comparison Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8 mb-8">
        <TelemetryPanel
          title="Baseline Architecture"
          badge="PostgreSQL Transactional (:8081)"
          arch="baseline"
          telemetry={snapshot?.baseline}
        />
        <TelemetryPanel
          title="Optimized Architecture"
          badge="Redis Lua + NATS JetStream (:8082)"
          arch="optimized"
          telemetry={snapshot?.optimized}
        />
      </div>

      {/* Activity Log & Chaos Events */}
      <ActivityLogPanel logs={snapshot?.activity_logs ?? []} />

      <VerdictSummaryModal
        isOpen={showVerdict}
        onClose={() => setShowVerdict(false)}
        baseline={snapshot?.baseline}
        optimized={snapshot?.optimized}
        isBenchmark={snapshot?.is_benchmark ?? true}
      />
    </main>
  );
}
