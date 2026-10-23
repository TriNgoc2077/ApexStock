import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'ApexStock Demo',
  description: 'Flash sale concurrency comparison: Baseline vs Optimized',
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
