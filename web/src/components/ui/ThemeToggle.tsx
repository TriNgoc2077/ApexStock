'use client';

import React, { useEffect, useState } from 'react';

export default function ThemeToggle() {
  const [theme, setTheme] = useState<'dark' | 'light'>('dark');
  const [mounted, setMounted] = useState<boolean>(false);

  useEffect(() => {
    setMounted(true);
    const saved = localStorage.getItem('theme') as 'dark' | 'light' | null;
    const initialTheme = saved || 'dark';
    setTheme(initialTheme);
    applyTheme(initialTheme);
  }, []);

  const applyTheme = (t: 'dark' | 'light') => {
    document.documentElement.setAttribute('data-theme', t);
    if (t === 'light') {
      document.documentElement.classList.add('light');
      document.documentElement.classList.remove('dark');
      document.body.classList.add('light-mode');
    } else {
      document.documentElement.classList.add('dark');
      document.documentElement.classList.remove('light');
      document.body.classList.remove('light-mode');
    }
  };

  const toggleTheme = () => {
    const next = theme === 'dark' ? 'light' : 'dark';
    setTheme(next);
    localStorage.setItem('theme', next);
    applyTheme(next);
  };

  if (!mounted) {
    return (
      <div className="w-9 h-9 rounded-xl border border-slate-700 bg-slate-900" />
    );
  }

  return (
    <button
      onClick={toggleTheme}
      aria-label="Chuyển đổi giao diện Sáng / Tối"
      title={theme === 'dark' ? 'Chuyển sang Giao diện Sáng (Light Mode)' : 'Chuyển sang Giao diện Tối (Dark Mode)'}
      className={`flex items-center space-x-1.5 px-2.5 py-1.5 rounded-xl border transition-all text-xs font-semibold ${
        theme === 'dark'
          ? 'bg-slate-900 hover:bg-slate-800 text-amber-300 border-slate-700/80 shadow-inner'
          : 'bg-white hover:bg-slate-100 text-slate-800 border-slate-300 shadow-sm'
      }`}
    >
      <span className="text-sm">{theme === 'dark' ? '☀️' : '🌙'}</span>
      <span className="hidden sm:inline text-[11px] font-medium">
        {theme === 'dark' ? 'Sáng' : 'Tối'}
      </span>
    </button>
  );
}
