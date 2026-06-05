"use client";

import {
  SideNav,
  SideNavItems,
  SideNavLink,
  SideNavMenu,
  SideNavMenuItem,
} from "@carbon/react";
import {
  Dashboard,
  Activity,
  Analytics,
  Settings,
  Report,
  DataVis_4,
} from "@carbon/icons-react";
import { usePathname } from "next/navigation";
import type { ConsolePermission } from "@/types/rbac";

interface NavItem {
  href?: string;
  label: string;
  icon: React.ComponentType;
  permission?: ConsolePermission;
  children?: Array<{ href: string; label: string; permission?: ConsolePermission }>;
}

const NAV_ITEMS: NavItem[] = [
  {
    href: "/dashboard",
    label: "Observability",
    icon: Dashboard,
    permission: "console.observability.read",
  },
  {
    href: "/monitor/topology",
    label: "Live Monitoring",
    icon: Activity,
    permission: "console.monitor.read",
  },
  {
    label: "Telemetry",
    icon: Analytics,
    permission: "console.telemetry.read",
    children: [
      { href: "/telemetry/metrics", label: "Metrics" },
      { href: "/telemetry/logs", label: "Logs" },
      { href: "/telemetry/traces", label: "Traces" },
    ],
  },
  {
    label: "Configuration",
    icon: DataVis_4,
    permission: "gateway.prompt_registry.read",
    children: [
      { href: "/config/prompts", label: "Prompt Registry" },
      { href: "/config/skills", label: "Skills" },
      { href: "/config/mcp", label: "MCP Servers" },
      { href: "/config/guardrails", label: "Guardrails" },
      { href: "/config/policies", label: "Policies" },
      { href: "/config/entitlements", label: "Entitlements" },
      { href: "/config/audit", label: "Audit Trail" },
    ],
  },
  {
    href: "/reports",
    label: "Reports",
    icon: Report,
    permission: "console.reports.read",
  },
  {
    href: "/settings/api-keys",
    label: "Settings",
    icon: Settings,
    permission: "console.admin.read",
  },
];

interface AppSideNavProps {
  /** Permission list from server session — used to hide inaccessible items. */
  permissions: ConsolePermission[];
}

/**
 * Carbon side navigation with permission-filtered items and active state.
 * Visibility filtering is UI-only — middleware enforces at route level.
 */
export function AppSideNav({ permissions }: AppSideNavProps) {
  const pathname = usePathname();

  function canSee(permission?: ConsolePermission): boolean {
    if (!permission) return true;
    return permissions.includes(permission);
  }

  function isActive(href: string): boolean {
    return pathname === href || pathname.startsWith(href + "/");
  }

  return (
    <SideNav aria-label="Main navigation" isFixedNav expanded>
      <SideNavItems>
        {NAV_ITEMS.filter((item) => canSee(item.permission)).map((item) => {
          const Icon = item.icon;

          if (item.children) {
            const visibleChildren = item.children.filter((c) =>
              canSee(c.permission)
            );
            if (visibleChildren.length === 0) return null;

            return (
              <SideNavMenu
                key={item.label}
                title={item.label}
                renderIcon={Icon}
                defaultExpanded={visibleChildren.some((c) => isActive(c.href))}
              >
                {visibleChildren.map((child) => (
                  <SideNavMenuItem
                    key={child.href}
                    href={child.href}
                    isActive={isActive(child.href)}
                  >
                    {child.label}
                  </SideNavMenuItem>
                ))}
              </SideNavMenu>
            );
          }

          return (
            <SideNavLink
              key={item.label}
              href={item.href ?? "#"}
              renderIcon={Icon}
              isActive={isActive(item.href ?? "")}
            >
              {item.label}
            </SideNavLink>
          );
        })}
      </SideNavItems>
    </SideNav>
  );
}
