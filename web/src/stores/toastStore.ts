import { create } from 'zustand';

interface ToastItem {
  id: string;
  type: 'success' | 'error' | 'info';
  message: string;
  duration?: number;
}

interface ToastStore {
  toasts: ToastItem[];
  add: (toast: Omit<ToastItem, 'id'>) => void;
  remove: (id: string) => void;
}

let counter = 0;

export const useToast = create<ToastStore>((set) => ({
  toasts: [],
  add: (toast) => {
    const id = String(++counter);
    set((s) => ({ toasts: [...s.toasts, { ...toast, id }] }));
    setTimeout(() => {
      set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) }));
    }, toast.duration || 4000);
  },
  remove: (id) => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),
}));

// Shorthand helpers
export const toast = {
  success: (message: string) => useToast.getState().add({ type: 'success', message }),
  error: (message: string) => useToast.getState().add({ type: 'error', message }),
  info: (message: string) => useToast.getState().add({ type: 'info', message }),
};
