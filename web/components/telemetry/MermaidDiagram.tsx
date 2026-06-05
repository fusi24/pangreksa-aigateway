"use client";

import { useEffect, useRef, useId } from "react";

interface MermaidDiagramProps {
  /** Mermaid diagram definition string. */
  definition: string;
}

/**
 * Renders a static Mermaid diagram.
 * Mermaid is dynamically imported to avoid SSR issues.
 * Uses a stable unique ID from React's useId.
 */
export function MermaidDiagram({ definition }: MermaidDiagramProps) {
  const ref = useRef<HTMLDivElement>(null);
  const rawId = useId();
  // Mermaid requires alphanumeric IDs — strip React's colon separators
  const id = `mermaid-${rawId.replace(/[^a-zA-Z0-9]/g, "")}`;

  useEffect(() => {
    if (!ref.current) return;

    import("mermaid").then((m) => {
      m.default.initialize({
        startOnLoad: false,
        theme: "default",
        securityLevel: "strict",
      });

      m.default.render(id, definition).then(({ svg }) => {
        if (ref.current) {
          ref.current.innerHTML = svg;
        }
      }).catch((err: unknown) => {
        if (ref.current) {
          const message = err instanceof Error ? err.message : "Diagram render error";
          ref.current.innerHTML = `<p style="color:#da1e28;font-size:0.875rem">${message}</p>`;
        }
      });
    });
  }, [definition, id]);

  return (
    <div
      ref={ref}
      role="img"
      aria-label="Diagram"
      style={{ overflowX: "auto" }}
    />
  );
}
