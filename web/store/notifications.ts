"use client";

import { create } from "zustand";

export type NotificationKind = "success" | "error" | "info" | "warning";

export interface Notification {
  id: string;
  kind: NotificationKind;
  title: string;
  subtitle?: string;
}

interface NotificationState {
  notifications: Notification[];
  add: (kind: NotificationKind, title: string, subtitle?: string) => void;
  remove: (id: string) => void;
  clear: () => void;
}

/**
 * Global notification queue consumed by ToastContainer in the console layout.
 */
export const useNotificationStore = create<NotificationState>((set) => ({
  notifications: [],

  add: (kind, title, subtitle) =>
    set((s) => {
      const notification: Notification = subtitle !== undefined
        ? { id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`, kind, title, subtitle }
        : { id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`, kind, title };
      return { notifications: [...s.notifications, notification] };
    }),

  remove: (id) =>
    set((s) => ({
      notifications: s.notifications.filter((n) => n.id !== id),
    })),

  clear: () => set({ notifications: [] }),
}));
