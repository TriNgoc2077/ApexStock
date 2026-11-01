'use client';

import React, { useState } from 'react';
import { useAuth } from '@/context/AuthContext';

export default function UserModal() {
  const { user, presetUsers, selectUserById, isModalOpen, setIsModalOpen, setUser } = useAuth();
  const [customId, setCustomId] = useState('');
  const [customName, setCustomName] = useState('');
  const [error, setError] = useState<string | null>(null);

  if (!isModalOpen) return null;

  const handleCustomSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!customId.trim()) {
      setError('Vui lòng nhập User UUID');
      return;
    }
    // Simple UUID check
    if (customId.trim().length !== 36) {
      setError('UUID phải đủ 36 ký tự (chuẩn RFC 4122)');
      return;
    }

    setUser({
      id: customId.trim(),
      name: customName.trim() || 'Người dùng tuỳ chỉnh',
      email: `${customId.slice(0, 8)}@apexstock.test`,
      role: 'Custom Tester',
      avatar: '👤',
      balance: 10000000,
    });
    setIsModalOpen(false);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm animate-fade-in">
      <div className="relative w-full max-w-lg rounded-2xl bg-slate-900 border border-slate-700/80 p-6 shadow-2xl text-slate-100">
        {/* Header */}
        <div className="flex items-center justify-between pb-4 mb-4 border-b border-slate-800">
          <div className="flex items-center space-x-3">
            <span className="text-2xl">👤</span>
            <div>
              <h3 className="text-lg font-bold">Quản Lý Phiên Đăng Nhập Demo</h3>
              <p className="text-xs text-slate-400">Chọn tài khoản test có sẵn trong cơ sở dữ liệu ApexStock</p>
            </div>
          </div>
          <button
            onClick={() => setIsModalOpen(false)}
            className="p-1 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800 transition-colors"
          >
            ✕
          </button>
        </div>

        {/* Current Active User Banner */}
        <div className="mb-6 p-4 rounded-xl bg-blue-950/40 border border-blue-500/30">
          <div className="text-xs font-semibold text-blue-400 uppercase tracking-wider mb-2">
            Tài Khoản Đang Hoạt Động
          </div>
          <div className="flex items-center space-x-3">
            <div className="w-12 h-12 rounded-full bg-blue-600/20 border border-blue-500/40 flex items-center justify-center text-2xl">
              {user.avatar}
            </div>
            <div className="flex-1 min-w-0">
              <div className="flex items-center space-x-2">
                <span className="font-bold text-white truncate">{user.name}</span>
                <span className="text-[10px] px-2 py-0.5 rounded-full bg-emerald-500/20 text-emerald-400 border border-emerald-500/30">
                  Online
                </span>
              </div>
              <p className="text-xs text-slate-400">{user.email} • {user.role}</p>
              <p className="text-[11px] font-mono text-slate-500 truncate mt-0.5">ID: {user.id}</p>
            </div>
          </div>
        </div>

        {/* Quick Switch Preset Users */}
        <div className="mb-6">
          <div className="text-xs font-semibold text-slate-300 uppercase tracking-wider mb-2">
            Chuyển Đổi Nhanh Tài Khoản (Đã Seed Database)
          </div>
          <div className="space-y-2 max-h-48 overflow-y-auto pr-1">
            {presetUsers.map((u) => {
              const isSelected = u.id === user.id;
              return (
                <button
                  key={u.id}
                  onClick={() => {
                    selectUserById(u.id);
                    setIsModalOpen(false);
                  }}
                  className={`w-full flex items-center justify-between p-3 rounded-xl border text-left transition-all ${
                    isSelected
                      ? 'bg-blue-600/20 border-blue-500 text-white'
                      : 'bg-slate-800/40 border-slate-700/60 hover:bg-slate-800 hover:border-slate-600 text-slate-300'
                  }`}
                >
                  <div className="flex items-center space-x-3">
                    <span className="text-xl">{u.avatar}</span>
                    <div>
                      <div className="text-sm font-semibold flex items-center space-x-2">
                        <span>{u.name}</span>
                        {isSelected && <span className="text-xs text-blue-400 font-normal">(Hiện tại)</span>}
                      </div>
                      <span className="text-xs text-slate-400">{u.role}</span>
                    </div>
                  </div>
                  <div className="text-right">
                    <span className="text-xs font-mono text-slate-400">
                      {u.id.split('-').pop()}
                    </span>
                  </div>
                </button>
              );
            })}
          </div>
        </div>

        {/* Custom User ID Form */}
        <form onSubmit={handleCustomSubmit} className="pt-4 border-t border-slate-800">
          <div className="text-xs font-semibold text-slate-300 uppercase tracking-wider mb-2">
            Nhập ID Kiểm Thử Riêng (Tùy Chọn)
          </div>
          {error && <div className="text-xs text-red-400 mb-2">{error}</div>}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 mb-3">
            <input
              type="text"
              placeholder="Tên người dùng..."
              value={customName}
              onChange={(e) => setCustomName(e.target.value)}
              className="px-3 py-2 text-xs rounded-lg bg-slate-800 border border-slate-700 text-white focus:outline-none focus:border-blue-500"
            />
            <input
              type="text"
              placeholder="33333333-3333-3333-3333-000000000001"
              value={customId}
              onChange={(e) => setCustomId(e.target.value)}
              className="px-3 py-2 text-xs font-mono rounded-lg bg-slate-800 border border-slate-700 text-white focus:outline-none focus:border-blue-500"
            />
          </div>
          <div className="flex justify-end space-x-2">
            <button
              type="button"
              onClick={() => setIsModalOpen(false)}
              className="px-4 py-2 text-xs rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 transition-colors"
            >
              Đóng
            </button>
            <button
              type="submit"
              className="px-4 py-2 text-xs font-bold rounded-lg bg-blue-600 hover:bg-blue-500 text-white shadow-md shadow-blue-500/30 transition-colors"
            >
              Áp Dụng
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
