"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Modal, TextInput, PasswordInput, Select, SelectItem,
  Tag, Button, InlineNotification, SkeletonText, InlineLoading,
} from "@carbon/react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { CrudTable } from "@/components/config/CrudTable";
import { CodeBlock } from "@/components/telemetry/CodeBlock";
import { useNotificationStore } from "@/store/notifications";
import { fmtTs } from "@/lib/utils/format";
import type { McpServerRecord, McpTestResult } from "@/types/api";

const mcpSchema = z.object({
  name: z.string().min(1),
  url: z.string().url("Must be a valid URL"),
  auth_type: z.enum(["none", "bearer", "basic"]),
  credentials: z.string().optional(),
});
type McpFormValues = z.infer<typeof mcpSchema>;

const QUERY_KEY = ["config", "mcp"];

/** MCP Server Registry CRUD page. */
export default function McpPage() {
  const qc = useQueryClient();
  const { add: notify } = useNotificationStore();
  const [modalOpen, setModalOpen] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<McpTestResult | null>(null);
  const [testingId, setTestingId] = useState<string | null>(null);

  const { data, isLoading, error } = useQuery<{ servers?: McpServerRecord[] }>({
    queryKey: QUERY_KEY,
    queryFn: async () => {
      const res = await fetch("/api/config/mcp");
      if (!res.ok) throw new Error("Failed to fetch MCP servers");
      return res.json() as Promise<{ servers?: McpServerRecord[] }>;
    },
  });

  const { register, handleSubmit, reset, watch, formState: { errors, isSubmitting } } =
    useForm<McpFormValues>({ resolver: zodResolver(mcpSchema), defaultValues: { auth_type: "none" } });

  const authType = watch("auth_type");

  const saveMutation = useMutation({
    mutationFn: async (values: McpFormValues & { id?: string }) => {
      const { id, ...body } = values;
      const url = id ? `/api/config/mcp/${id}` : "/api/config/mcp";
      const res = await fetch(url, {
        method: id ? "PUT" : "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!res.ok) { const err = (await res.json()) as { error?: string }; throw new Error(err.error ?? "Save failed"); }
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEY });
      setModalOpen(false); reset();
      notify("success", editId ? "MCP server updated" : "MCP server added");
    },
    onError: (err: Error) => notify("error", "Save failed", err.message),
  });

  const testMutation = useMutation({
    mutationFn: async (id: string) => {
      setTestingId(id);
      const res = await fetch(`/api/config/mcp/${id}/test`, { method: "POST" });
      if (!res.ok) throw new Error("Test failed");
      return res.json() as Promise<McpTestResult>;
    },
    onSuccess: (result) => { setTestResult(result); setTestingId(null); },
    onError: (err: Error) => { notify("error", "Test failed", err.message); setTestingId(null); },
  });

  const servers = data?.servers ?? [];

  const rows = servers.map((s) => ({
    id: s.id,
    name: s.name,
    url: s.url,
    auth_type: s.auth_type,
    tool_count: String(s.tool_count),
    last_tested_at: s.last_tested_at ? fmtTs(s.last_tested_at) : "Never",
    status: s.status,
    _test: s.id,
  }));

  const headers = [
    { key: "name", header: "Name" },
    { key: "url", header: "URL" },
    { key: "auth_type", header: "Auth" },
    { key: "tool_count", header: "Tools" },
    { key: "status", header: "Status" },
    { key: "last_tested_at", header: "Last Tested" },
    { key: "_test", header: "" },
  ];

  return (
    <>
      <h1 style={{ fontSize: "1.75rem", fontWeight: 600, marginBottom: "1.5rem" }}>MCP Server Registry</h1>
      {error && <InlineNotification kind="error" title="Failed to load MCP servers" subtitle={(error as Error).message} />}
      {isLoading ? <SkeletonText paragraph /> : (
        <CrudTable title="MCP Servers" headers={headers} rows={rows} isLoading={isLoading}
          queryKey={QUERY_KEY} deleteUrl={(id) => `/api/config/mcp/${id}`}
          onAdd={() => { reset(); setEditId(null); setModalOpen(true); }}
          onEdit={(id) => {
            const s = servers.find((x) => x.id === id);
            if (!s) return;
            reset({ name: s.name, url: s.url, auth_type: s.auth_type });
            setEditId(id); setModalOpen(true);
          }}
          renderCell={(header, value, rowId) => {
            if (header === "status") return <Tag type={value === "reachable" ? "green" : value === "unreachable" ? "red" : "gray"}>{value as string}</Tag>;
            if (header === "auth_type") return <Tag type="blue">{value as string}</Tag>;
            if (header === "_test") return (
              <Button kind="ghost" size="sm" onClick={() => testMutation.mutate(rowId)}
                disabled={testingId === rowId}>
                {testingId === rowId ? <InlineLoading /> : "Test"}
              </Button>
            );
            return value;
          }}
        />
      )}
      <Modal open={modalOpen} modalHeading={editId ? "Edit MCP Server" : "Add MCP Server"}
        primaryButtonText={isSubmitting ? "Saving…" : "Save"} secondaryButtonText="Cancel"
        onRequestSubmit={handleSubmit((v) => saveMutation.mutate(editId ? { ...v, id: editId } : v))}
        onRequestClose={() => { setModalOpen(false); reset(); }} size="md">
        <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
          <TextInput id="mcp-name" labelText="Name" {...register("name")} invalid={!!errors.name} invalidText={errors.name?.message} />
          <TextInput id="mcp-url" labelText="URL" type="url" {...register("url")} invalid={!!errors.url} invalidText={errors.url?.message} />
          <Select id="mcp-auth" labelText="Authentication" {...register("auth_type")}>
            <SelectItem value="none" text="None" />
            <SelectItem value="bearer" text="Bearer token" />
            <SelectItem value="basic" text="Basic auth" />
          </Select>
          {authType !== "none" && (
            <PasswordInput id="mcp-creds" labelText="Credentials" {...register("credentials")} helperText="Stored encrypted at rest." />
          )}
        </div>
      </Modal>
      <Modal open={testResult !== null} modalHeading="Test Result" passiveModal onRequestClose={() => setTestResult(null)} size="lg">
        {testResult && (
          <>
            <Tag type={testResult.reachable ? "green" : "red"}>{testResult.reachable ? "Reachable" : "Unreachable"}</Tag>
            {testResult.tools.length > 0 && (
              <div style={{ marginTop: "1rem" }}>
                <CodeBlock code={JSON.stringify(testResult.tools, null, 2)} language="json" maxLines={20} />
              </div>
            )}
          </>
        )}
      </Modal>
    </>
  );
}
