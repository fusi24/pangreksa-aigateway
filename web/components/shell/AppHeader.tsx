"use client";

import {
  Header,
  HeaderName,
  HeaderGlobalBar,
  HeaderGlobalAction,
} from "@carbon/react";
import { Light, Asleep, Notification, UserAvatar } from "@carbon/icons-react";
import { useUIStore } from "@/store/ui";
import { useRouter } from "next/navigation";

interface AppHeaderProps {
  /** Displayed in the header alongside the app name. */
  orgId: string;
}

/**
 * Carbon application header with theme toggle, notifications, and user avatar.
 */
export function AppHeader({ orgId: _ }: AppHeaderProps) {
  const { toggleTheme, theme } = useUIStore();
  const router = useRouter();

  async function handleLogout() {
    await fetch("/api/auth/logout", { method: "POST" });
    router.push("/login");
    router.refresh();
  }

  return (
    <Header aria-label="Pangreksa Console">
      <HeaderName href="/dashboard" prefix="Pangreksa">
        Console
      </HeaderName>

      <HeaderGlobalBar>
        <HeaderGlobalAction
          aria-label={theme === "white" ? "Switch to dark mode" : "Switch to light mode"}
          onClick={toggleTheme}
          tooltipAlignment="end"
        >
          {theme === "white" ? <Asleep /> : <Light />}
        </HeaderGlobalAction>

        <HeaderGlobalAction
          aria-label="Notifications"
          tooltipAlignment="end"
        >
          <Notification />
        </HeaderGlobalAction>

        <HeaderGlobalAction
          aria-label="Sign out"
          onClick={handleLogout}
          tooltipAlignment="end"
        >
          <UserAvatar />
        </HeaderGlobalAction>
      </HeaderGlobalBar>
    </Header>
  );
}
