/**
 * Auth layout — minimal centered container, no Carbon Shell.
 */
export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
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
      {children}
    </div>
  );
}
