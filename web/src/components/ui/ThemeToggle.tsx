'use client';

import React, { useEffect, useState } from 'react';

export default function ThemeToggle() {
  const [theme, setTheme] = useState<'dark' | 'light'>('dark');

  useEffect(() => {
    const saved = localStorage.getItem('theme') as 'dark' | 'light' | null;
    if (saved) {
      setTheme(saved);
      document.documentElement.setAttribute('data-theme', saved);
    } else {
      document.documentElement.setAttribute('data-theme', 'dark');
    }
  }, []);

  const toggleTheme = () => {
    const next = theme === 'dark' ? 'light' : 'dark';
    setTheme(next);
    localStorage.setItem('theme', next);
    document.documentElement.setAttribute('data-theme', next);
  };

  return (
    <button
      onClick={toggleTheme}
      aria-label="Toggle dark/light mode"
      className="p-2 rounded-lg border border-[var(--card-border)] bg-[var(--card-bg)] hover:opacity-80 transition-opacity focus:outline-none focus:ring-2 focus:ring-[var(--primary)]"
    >
      {theme === 'dark' ? '☀️' : '🌙'}
    </button>
  );
}
