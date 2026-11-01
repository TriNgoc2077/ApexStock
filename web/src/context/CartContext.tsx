'use client';

import React, { createContext, useContext, useState, useEffect } from 'react';
import { FlashSaleItem } from '@/lib/api';

export interface CartItem {
  id: string; // unique cart entry id
  itemId: string; // flash sale item id
  productId: string;
  name: string;
  image: string;
  salePrice: number;
  originalPrice: number;
  quantity: number;
  maxPerUser: number;
  selectedColor?: string;
  selectedOption?: string;
}

interface CartContextType {
  items: CartItem[];
  addToCart: (item: FlashSaleItem, color?: string, option?: string) => void;
  updateQuantity: (cartId: string, delta: number) => void;
  removeFromCart: (cartId: string) => void;
  clearCart: () => void;
  totalItems: number;
  totalAmount: number;
  isCartOpen: boolean;
  setIsCartOpen: (open: boolean) => void;
  lastAddedName: string | null;
}

const CartContext = createContext<CartContextType | undefined>(undefined);

const CART_STORAGE_KEY = 'apexstock_cart_items_v1';

export function CartProvider({ children }: { children: React.ReactNode }) {
  const [items, setItems] = useState<CartItem[]>([]);
  const [isCartOpen, setIsCartOpen] = useState<boolean>(false);
  const [lastAddedName, setLastAddedName] = useState<string | null>(null);

  useEffect(() => {
    try {
      const saved = localStorage.getItem(CART_STORAGE_KEY);
      if (saved) {
        setItems(JSON.parse(saved));
      }
    } catch {
      // Ignore json errors
    }
  }, []);

  const saveItems = (newItems: CartItem[]) => {
    setItems(newItems);
    try {
      localStorage.setItem(CART_STORAGE_KEY, JSON.stringify(newItems));
    } catch {
      // Ignore storage errors
    }
  };

  const addToCart = (item: FlashSaleItem, color?: string, option?: string) => {
    const existingIndex = items.findIndex((i) => i.itemId === item.id);
    if (existingIndex > -1) {
      const updated = [...items];
      const current = updated[existingIndex];
      if (current.quantity < current.maxPerUser) {
        updated[existingIndex] = {
          ...current,
          quantity: current.quantity + 1,
        };
        saveItems(updated);
      }
    } else {
      const newItem: CartItem = {
        id: `cart-${Date.now()}-${Math.random().toString(36).substring(2, 6)}`,
        itemId: item.id,
        productId: item.product_id,
        name: item.product_name,
        image: item.product_image || '/images/products/iphone15.jpg',
        salePrice: item.sale_price,
        originalPrice: item.original_price,
        quantity: 1,
        maxPerUser: item.max_per_user || 1,
        selectedColor: color,
        selectedOption: option,
      };
      saveItems([...items, newItem]);
    }

    setLastAddedName(item.product_name);
    setIsCartOpen(true);
    setTimeout(() => setLastAddedName(null), 3000);
  };

  const updateQuantity = (cartId: string, delta: number) => {
    const updated = items
      .map((item) => {
        if (item.id === cartId) {
          const nextQty = item.quantity + delta;
          if (nextQty <= 0) return null;
          if (nextQty > item.maxPerUser) return item;
          return { ...item, quantity: nextQty };
        }
        return item;
      })
      .filter((item): item is CartItem => item !== null);

    saveItems(updated);
  };

  const removeFromCart = (cartId: string) => {
    const filtered = items.filter((i) => i.id !== cartId);
    saveItems(filtered);
  };

  const clearCart = () => {
    saveItems([]);
  };

  const totalItems = items.reduce((sum, item) => sum + item.quantity, 0);
  const totalAmount = items.reduce((sum, item) => sum + item.salePrice * item.quantity, 0);

  return (
    <CartContext.Provider
      value={{
        items,
        addToCart,
        updateQuantity,
        removeFromCart,
        clearCart,
        totalItems,
        totalAmount,
        isCartOpen,
        setIsCartOpen,
        lastAddedName,
      }}
    >
      {children}
    </CartContext.Provider>
  );
}

export function useCart() {
  const context = useContext(CartContext);
  if (!context) {
    throw new Error('useCart must be used within a CartProvider');
  }
  return context;
}
