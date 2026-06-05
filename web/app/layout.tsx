import type { Metadata } from "next";
import { QueryProvider } from "@/components/providers/QueryProvider";
import "@carbon/styles/css/styles.css";

export const metadata: Metadata = {
  title: process.env.NEXT_PUBLIC_APP_NAME ?? "Pangreksa Console",
  description: "Pangreksa AI Gateway Administration Console",
};

/**
 * Root layout — provides TanStack Query context and Carbon global CSS.
 * Carbon Shell (Header + SideNav) lives in (console)/layout.tsx.
 */
export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body>
        <QueryProvider>{children}</QueryProvider>
      </body>
    </html>
  );
}
