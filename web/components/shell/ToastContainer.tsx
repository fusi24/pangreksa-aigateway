"use client";

import { ToastNotification } from "@carbon/react";
import { useNotificationStore } from "@/store/notifications";
import { useEffect } from "react";

/**
 * Renders all queued toast notifications from the Zustand notification store.
 * Auto-dismisses each notification after 5 seconds.
 */
export function ToastContainer() {
  const { notifications, remove } = useNotificationStore();

  useEffect(() => {
    if (notifications.length === 0) return;
    const lastNotif = notifications[notifications.length - 1];
    if (!lastNotif) return;
    const id = lastNotif.id;
    const timer = setTimeout(() => remove(id), 5000);
    return () => clearTimeout(timer);
  }, [notifications, remove]);

  if (notifications.length === 0) return null;

  return (
    <div
      style={{
        position: "fixed",
        bottom: "1rem",
        right: "1rem",
        zIndex: 9000,
        display: "flex",
        flexDirection: "column",
        gap: "0.5rem",
        maxWidth: 400,
      }}
    >
      {notifications.map((n) => {
        const props = n.subtitle !== undefined
          ? { kind: n.kind, title: n.title, subtitle: n.subtitle, caption: "", onClose: () => remove(n.id), timeout: 0 }
          : { kind: n.kind, title: n.title, caption: "", onClose: () => remove(n.id), timeout: 0 };
        return <ToastNotification key={n.id} {...props} />;
      })}
    </div>
  );
}
