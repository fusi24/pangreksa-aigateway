import { redirect } from "next/navigation";
import { getSession } from "@/lib/auth/session";
import { AppHeader } from "@/components/shell/AppHeader";
import { AppSideNav } from "@/components/shell/AppSideNav";
import { ThemeProvider } from "@/components/shell/ThemeProvider";
import { ToastContainer } from "@/components/shell/ToastContainer";

/**
 * Console layout — rendered on every authenticated route.
 * Reads the session server-side and passes permissions to the SideNav.
 * Redirects to /login if the session is missing or invalid.
 */
export default async function ConsoleLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const session = await getSession();

  if (!session) {
    redirect("/login");
  }

  return (
    <ThemeProvider>
      <AppHeader orgId={session.org_id} />
      <AppSideNav permissions={session.permissions} />
      {/* Plain div replaces Carbon Content — Content uses context, incompatible with RSC */}
      <div style={{ marginLeft: "16rem", marginTop: "3rem", padding: "2rem" }}>
        {children}
      </div>
      <ToastContainer />
    </ThemeProvider>
  );
}
