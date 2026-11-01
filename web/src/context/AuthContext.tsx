'use client';

import React, { createContext, useContext, useState, useEffect } from 'react';

export interface DemoUser {
  id: string;
  name: string;
  email: string;
  role: string;
  avatar: string;
  balance: number;
}

export const PRESET_USERS: DemoUser[] = [
  {
    id: '33333333-3333-3333-3333-000000000001',
    name: 'Cao Trí Ngọc',
    email: 'user1@apexstock.test',
    role: 'Sinh viên thực tập (Demo Host)',
    avatar: '👨‍💻',
    balance: 50000000,
  },
  {
    id: '33333333-3333-3333-3333-000000000002',
    name: 'ThS. Nguyễn Văn A',
    email: 'user2@apexstock.test',
    role: 'Giám khảo Hội đồng 1',
    avatar: '👨‍🏫',
    balance: 100000000,
  },
  {
    id: '33333333-3333-3333-3333-000000000003',
    name: 'TS. Trần Thị B',
    email: 'user3@apexstock.test',
    role: 'Giám khảo Hội đồng 2',
    avatar: '👩‍🏫',
    balance: 100000000,
  },
  {
    id: '33333333-3333-3333-3333-000000000004',
    name: 'Khách hàng VIP #4',
    email: 'user4@apexstock.test',
    role: 'Người mua kiểm thử Quota',
    avatar: '🛍️',
    balance: 30000000,
  },
  {
    id: '33333333-3333-3333-3333-000000000005',
    name: 'Khách hàng VIP #5',
    email: 'user5@apexstock.test',
    role: 'Người mua kiểm thử Concurrency',
    avatar: '⚡',
    balance: 45000000,
  },
];

interface AuthContextType {
  user: DemoUser;
  presetUsers: DemoUser[];
  setUser: (user: DemoUser) => void;
  selectUserById: (id: string) => void;
  isModalOpen: boolean;
  setIsModalOpen: (open: boolean) => void;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

const STORAGE_KEY = 'apexstock_current_user_id';

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [currentUser, setCurrentUser] = useState<DemoUser>(PRESET_USERS[0]);
  const [isModalOpen, setIsModalOpen] = useState<boolean>(false);
  const [isMounted, setIsMounted] = useState<boolean>(false);

  useEffect(() => {
    setIsMounted(true);
    const savedId = localStorage.getItem(STORAGE_KEY);
    if (savedId) {
      const found = PRESET_USERS.find((u) => u.id === savedId);
      if (found) {
        setCurrentUser(found);
      }
    }
  }, []);

  const selectUserById = (id: string) => {
    const found = PRESET_USERS.find((u) => u.id === id);
    if (found) {
      setCurrentUser(found);
      localStorage.setItem(STORAGE_KEY, found.id);
    }
  };

  const handleSetUser = (u: DemoUser) => {
    setCurrentUser(u);
    localStorage.setItem(STORAGE_KEY, u.id);
  };

  return (
    <AuthContext.Provider
      value={{
        user: currentUser,
        presetUsers: PRESET_USERS,
        setUser: handleSetUser,
        selectUserById,
        isModalOpen,
        setIsModalOpen,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}
