import { redirect } from "next/navigation";

/** Root page redirects to the dashboard. */
export default function RootPage() {
  redirect("/dashboard");
}
