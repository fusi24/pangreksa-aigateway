"use client";

import { useMemo } from "react";
import { CodeSnippet } from "@carbon/react";
import hljs from "highlight.js/lib/core";
import json from "highlight.js/lib/languages/json";
import yaml from "highlight.js/lib/languages/yaml";
import markdown from "highlight.js/lib/languages/markdown";
import go from "highlight.js/lib/languages/go";
import sql from "highlight.js/lib/languages/sql";
import bash from "highlight.js/lib/languages/bash";

// Register only required language packs to minimize bundle size
hljs.registerLanguage("json", json);
hljs.registerLanguage("yaml", yaml);
hljs.registerLanguage("markdown", markdown);
hljs.registerLanguage("go", go);
hljs.registerLanguage("sql", sql);
hljs.registerLanguage("bash", bash);

export type CodeLanguage = "json" | "yaml" | "markdown" | "go" | "sql" | "bash";

interface CodeBlockProps {
  code: string;
  language: CodeLanguage;
  /** Collapse after N lines. Defaults to 15. */
  maxLines?: number;
}

/**
 * Syntax-highlighted code block using Highlight.js inside a Carbon CodeSnippet.
 * Uses dangerouslySetInnerHTML — safe because hljs.highlight() output is
 * HTML-escaped (only <span class="hljs-*"> elements).
 */
export function CodeBlock({ code, language, maxLines = 15 }: CodeBlockProps) {
  const highlighted = useMemo(() => {
    try {
      return hljs.highlight(code, { language }).value;
    } catch {
      // Fallback to plain text if highlighting fails
      return code.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
    }
  }, [code, language]);

  return (
    <CodeSnippet
      type="multi"
      feedback="Copied to clipboard"
      maxCollapsedNumberOfRows={maxLines}
    >
      <code dangerouslySetInnerHTML={{ __html: highlighted }} />
    </CodeSnippet>
  );
}
