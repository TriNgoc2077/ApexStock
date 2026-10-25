'use client';

import React from 'react';

interface AuroraHeroProps {
  title: string;
  subtitle: string;
  badge?: string;
}

export default function AuroraHero({ title, subtitle, badge }: AuroraHeroProps) {
  return (
    <div className="relative overflow-hidden py-16 px-6 sm:px-12 rounded-2xl mb-12 border border-[var(--card-border)] bg-[var(--card-bg)]">
      <div className="absolute inset-0 aurora-bg -z-10 pointer-events-none" />
      <div className="max-w-3xl mx-auto text-center">
        {badge && (
          <span className="inline-block px-3 py-1 mb-4 text-xs font-semibold tracking-wider uppercase rounded-full bg-blue-500/10 text-blue-400 border border-blue-500/20">
            {badge}
          </span>
        )}
        <h1 className="text-4xl sm:text-5xl font-extrabold tracking-tight text-[var(--foreground)] mb-4">
          {title}
        </h1>
        <p className="text-lg text-slate-300 max-w-2xl mx-auto">
          {subtitle}
        </p>
      </div>
    </div>
  );
}
