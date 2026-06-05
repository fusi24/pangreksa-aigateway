"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Modal,
  TextInput,
  TextArea,
  Tag,
  Button,
  InlineNotification,
  SkeletonText,
  Tabs,
  Tab,
  TabList,
  TabPanels,
  TabPanel,
} from "@carbon/react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { CrudTable } from "@/components/config/CrudTable";
import { CodeBlock } from "@/components/telemetry/CodeBlock";
import { MermaidDiagram } from "@/components/telemetry/MermaidDiagram";
import { useNotificationStore } from "@/store/notifications";
import { fmtTs, truncate } from "@/lib/utils/format";
import type { PromptsResponse, PromptVersion } from "@/types/api";

const promptSchema = z.object({
  repo: z.string().min(1),
  name: z.string().min(1),
  content: z.string().min(1),
});
type PromptFormValues = z.infer<typeof promptSchema>;

const QUERY_KEY = ["config", "prompts"];

/**
 * Prompt Registry CRUD page with version history Mermaid diagram.
 */
export default function PromptsPage() {
  const qc = useQueryClient();
  const { add: notify } = useNotificationStore();
  const [createOpen, setCreateOpen] = useState(false);
  const [viewPrompt, setViewPrompt] = useState<PromptVersion | null>(null);
  const [historyPrompt, setHistoryPrompt] = useState<PromptVersion[] | null>(null);

  const { data, isLoading, error } = useQuery<PromptsResponse>({
    queryKey: QUERY_KEY,
    queryFn: async () => {
      const res = await fetch("/api/config/prompts");
      if (!res.ok) throw new Error("Failed to fetch prompts");
      return res.json() as Promise<PromptsResponse>;
    },
  });

  const { register, handleSubmit, reset, formState: { errors, isSubmitting } } =
    useForm<PromptFormValues>({ resolver: zodResolver(promptSchema) });

  const createMutation = useMutation({
    mutationFn: async (values: PromptFormValues) => {
      const res = await fetch(`/api/config/prompts/${values.repo}/${values.name}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ content: values.content, config: { variables: [], guardrails: [] } }),
      });
      if (!res.ok) {
        const err = (await res.json()) as { error?: string };
        throw new Error(err.error ?? "Create failed");
      }
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEY });
      setCreateOpen(false);
      reset();
      notify("success", "Prompt version created", "Config applied — daemons refresh within ~30s.");
    },
    onError: (err: Error) => notify("error", "Create failed", err.message),
  });

  const allPrompts = data?.repos.flatMap((r) => r.prompts) ?? [];

  const rows = allPrompts.map((p) => ({
    id: p.fqn,
    fqn: truncate(p.fqn, 40),
    repo: p.repo,
    name: p.name,
    version: `v${p.version}`,
    active: p.is_active ? "Active" : "",
    created_at: fmtTs(p.created_at),
  }));

  const headers = [
    { key: "fqn", header: "FQN" },
    { key: "repo", header: "Repo" },
    { key: "name", header: "Name" },
    { key: "version", header: "Version" },
    { key: "active", header: "Status" },
    { key: "created_at", header: "Created" },
  ];

  // Build Mermaid gitGraph for version history
  function buildVersionHistory(versions: PromptVersion[]): string {
    const sorted = [...versions].sort((a, b) => a.version - b.version);
    const lines = ["gitGraph", "   commit id: \"init\""];
    sorted.forEach((v) => {
      lines.push(`   commit id: "v${v.version}"${v.is_active ? ' tag: "active"' : ""}`);
    });
    return lines.join("\n");
  }

  return (
    <>
      <h1 style={{ fontSize: "1.75rem", fontWeight: 600, marginBottom: "1.5rem" }}>
        Prompt Registry
      </h1>

      {error && <InlineNotification kind="error" title="Failed to load prompts"
        subtitle={(error as Error).message} style={{ marginBottom: "1rem" }} />}

      {isLoading ? <SkeletonText paragraph /> : (
        <CrudTable
          title="Prompts"
          description="Immutable prompt versions — each save creates a new version."
          headers={headers}
          rows={rows}
          isLoading={isLoading}
          queryKey={QUERY_KEY}
          deleteUrl={(id) => `/api/config/prompts/${encodeURIComponent(id)}`}
          onAdd={() => { reset(); setCreateOpen(true); }}
          onEdit={(id) => {
            const p = allPrompts.find((pr) => pr.fqn === id);
            if (p) setViewPrompt(p);
          }}
          renderCell={(header, value) => {
            if (header === "active" && value) return <Tag type="green">Active</Tag>;
            return value;
          }}
        />
      )}

      {/* Create new version */}
      <Modal
        open={createOpen}
        modalHeading="Create Prompt Version"
        primaryButtonText={isSubmitting ? "Creating…" : "Create"}
        secondaryButtonText="Cancel"
        onRequestSubmit={handleSubmit((v) => createMutation.mutate(v))}
        onRequestClose={() => setCreateOpen(false)}
        size="lg"
      >
        {createMutation.isError && (
          <InlineNotification kind="error" title="Create failed"
            subtitle={(createMutation.error as Error).message} lowContrast hideCloseButton />
        )}
        <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
          <TextInput id="prompt-repo" labelText="Repository" {...register("repo")}
            invalid={!!errors.repo} invalidText={errors.repo?.message} />
          <TextInput id="prompt-name" labelText="Prompt name" {...register("name")}
            invalid={!!errors.name} invalidText={errors.name?.message} />
          <TextArea id="prompt-content" labelText="Prompt content (Markdown)" {...register("content")}
            rows={14} invalid={!!errors.content} invalidText={errors.content?.message} />
        </div>
      </Modal>

      {/* View content drawer */}
      <Modal
        open={viewPrompt !== null}
        modalHeading={viewPrompt ? `${viewPrompt.fqn}` : ""}
        passiveModal
        onRequestClose={() => setViewPrompt(null)}
        size="lg"
      >
        {viewPrompt && (
          <Tabs>
            <TabList aria-label="Prompt tabs">
              <Tab>Content</Tab>
              <Tab>Version History</Tab>
            </TabList>
            <TabPanels>
              <TabPanel>
                <CodeBlock code={viewPrompt.content} language="markdown" maxLines={40} />
              </TabPanel>
              <TabPanel>
                <MermaidDiagram
                  definition={buildVersionHistory(
                    allPrompts.filter((p) => p.repo === viewPrompt.repo && p.name === viewPrompt.name)
                  )}
                />
              </TabPanel>
            </TabPanels>
          </Tabs>
        )}
      </Modal>
    </>
  );
}
