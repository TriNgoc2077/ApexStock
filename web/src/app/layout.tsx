import type { Metadata } from 'next';
import './globals.css';
import { AuthProvider } from '@/context/AuthContext';
import { CartProvider } from '@/context/CartContext';
import UserModal from '@/components/auth/UserModal';

export const metadata: Metadata = {
  title: 'ApexStock — Nền Tảng Thương Mại Điện Tử & Flash Sale Chịu Tải Cao',
  description: 'Hệ thống bán hàng chớp nhoáng với kiến trúc phân tán chịu tải cao, đối chiếu Baseline vs Optimized không oversell.',
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="vi" data-theme="dark" suppressHydrationWarning>
      <head>
        <script
          dangerouslySetInnerHTML={{
            __html: `
              (function() {
                try {
                  var saved = localStorage.getItem('theme');
                  if (saved === 'light') {
                    document.documentElement.setAttribute('data-theme', 'light');
                    document.documentElement.classList.add('light');
                    document.documentElement.classList.remove('dark');
                  } else {
                    document.documentElement.setAttribute('data-theme', 'dark');
                    document.documentElement.classList.add('dark');
                    document.documentElement.classList.remove('light');
                  }
                } catch(e) {}
              })();
            `,
          }}
        />
      </head>
      <body className="min-h-screen bg-[var(--background)] text-[var(--foreground)] antialiased flex flex-col transition-colors duration-200">
        <AuthProvider>
          <CartProvider>
            {children}
            <UserModal />
          </CartProvider>
        </AuthProvider>
      </body>
    </html>
  );
}
