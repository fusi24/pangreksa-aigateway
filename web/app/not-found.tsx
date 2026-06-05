"use client";

import Link from "next/link";
import { Tile, Button } from "@carbon/react";

/**
 * 404 Not Found page.
 */
export default function NotFoundPage() {
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
        <h1 style={{ fontSize: "3rem", fontWeight: 700, color: "#0f62fe", marginBottom: "0.5rem" }}>
          404
        </h1>
        <h2 style={{ fontSize: "1.25rem", fontWeight: 600, marginBottom: "0.75rem" }}>
          Page Not Found
        </h2>
        <p style={{ color: "#525252", marginBottom: "1.5rem" }}>
          The page you are looking for does not exist.
        </p>
        <Link href="/dashboard">
          <Button kind="primary">Go to Dashboard</Button>
        </Link>
      </Tile>
    </div>
  );
}
