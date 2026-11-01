'use client';

import React, { useState, useEffect } from 'react';

interface CountdownTimerProps {
  endTime?: string;
}

export default function CountdownTimer({ endTime }: CountdownTimerProps) {
  const [timeLeft, setTimeLeft] = useState<{ hours: number; minutes: number; seconds: number }>({
    hours: 2,
    minutes: 45,
    seconds: 18,
  });

  useEffect(() => {
    const target = endTime ? new Date(endTime).getTime() : Date.now() + 2 * 3600 * 1000 + 45 * 60 * 1000;

    const timer = setInterval(() => {
      const now = Date.now();
      const diff = Math.max(0, target - now);

      const hours = Math.floor(diff / (1000 * 60 * 60));
      const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));
      const seconds = Math.floor((diff % (1000 * 60)) / 1000);

      setTimeLeft({ hours, minutes, seconds });
    }, 1000);

    return () => clearInterval(timer);
  }, [endTime]);

  const pad = (n: number) => String(n).padStart(2, '0');

  return (
    <div className="flex items-center space-x-1.5 text-xs font-mono font-bold">
      <span className="px-2 py-1 rounded bg-slate-900 border border-slate-700 text-white shadow-inner">
        {pad(timeLeft.hours)}
      </span>
      <span className="text-amber-400 font-sans font-black animate-pulse">:</span>
      <span className="px-2 py-1 rounded bg-slate-900 border border-slate-700 text-white shadow-inner">
        {pad(timeLeft.minutes)}
      </span>
      <span className="text-amber-400 font-sans font-black animate-pulse">:</span>
      <span className="px-2 py-1 rounded bg-slate-900 border border-slate-700 text-white shadow-inner">
        {pad(timeLeft.seconds)}
      </span>
    </div>
  );
}
