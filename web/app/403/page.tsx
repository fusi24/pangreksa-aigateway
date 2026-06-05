"use client";

import Link from "next/link";
import { Tile, Button } from "@carbon/react";

/**
 * 403 Access Denied page — shown when RBAC middleware denies route access.
 */
export default function ForbiddenPage() {
  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "#f4f4f4",
      }}
    >
      <Tile style={{ maxWidth: 480, padding: "2rem", textAlign: "center" }}>
        <h1 style={{ fontSize: "3rem", fontWeight: 700, color: "#da1e28", marginBottom: "0.5rem" }}>
          403
        </h1>
        <h2 style={{ fontSize: "1.25rem", fontWeight: 600, marginBottom: "0.75rem" }}>
          Access Denied
        </h2>
        <p style={{ color: "#525252", marginBottom: "1.5rem" }}>
          You don&apos;t have permission to view this page.
          Contact your administrator to request access.
        </p>
        <Link href="/dashboard">
          <Button kind="primary">Go to Dashboard</Button>
        </Link>
      </Tile>
    </div>
  );
}
