"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Modal, TextInput, Select, SelectItem, Toggle,
  Tag, InlineNotification, SkeletonText, NumberInput,
} from "@carbon/react";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { CrudTable } from "@/components/config/CrudTable";
import { CodeBlock } from "@/components/telemetry/CodeBlock";
import { useNotificationStore } from "@/store/notifications";
import type { GuardrailRecord } from "@/types/api";

const guardrailSchema = z.object({
  name: z.string().min(1),
  type: z.string().min(1),
  hook: z.enum(["pre_request", "post_response", "streaming"]),
  mode: z.enum(["block", "flag", "audit"]),
  enforcement: z.enum(["hard", "soft"]),
  enabled: z.boolean(),
  priority: z.number().int().min(1),
  config_json: z.string().min(1, "Config JSON required"),
});
type GuardrailFormValues = z.infer<typeof guardrailSchema>;

const QUERY_KEY = ["config", "guardrails"];

/** Guardrail Policy Manager — ordered rules with priority drag-and-drop hint. */
export default function GuardrailsPage() {
  const qc = useQueryClient();
  const { add: notify } = useNotificationStore();
  const [modalOpen, setModalOpen] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [configPreview, setConfigPreview] = useState<string>("");

  const { data, isLoading, error } = useQuery<{ guardrails?: GuardrailRecord[] }>({
    queryKey: QUERY_KEY,
    queryFn: async () => {
      const res = await fetch("/api/config/guardrails");
      if (!res.ok) throw new Error("Failed to load guardrails");
      return res.json() as Promise<{ guardrails?: GuardrailRecord[] }>;
    },
  });

  const { register, handleSubmit, reset, control, watch, formState: { errors, isSubmitting } } =
    useForm<GuardrailFormValues>({
      resolver: zodResolver(guardrailSchema),
      defaultValues: { enabled: true, priority: 1, hook: "pre_request", mode: "block", enforcement: "hard", config_json: "{}" },
    });

  const enabledValue = watch("enabled");
  const configJsonValue = watch("config_json");

  const saveMutation = useMutation({
    mutationFn: async (values: GuardrailFormValues & { id?: string }) => {
      const { id, config_json, ...rest } = values;
      const body = { ...rest, config: JSON.parse(config_json) };
      const url = id ? `/api/config/guardrails/${id}` : "/api/config/guardrails";
      const res = await fetch(url, { method: id ? "PUT" : "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      if (!res.ok) { const err = (await res.json()) as { error?: string }; throw new Error(err.error ?? "Save failed"); }
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEY });
      setModalOpen(false); reset();
      notify("success", "Guardrail saved", "Config applied — daemons refresh within ~30s.");
    },
    onError: (err: Error) => notify("error", "Save failed", err.message),
  });

  const guardrails = (data?.guardrails ?? []).sort((a, b) => a.priority - b.priority);

  const rows = guardrails.map((g) => ({
    id: g.id,
    priority: String(g.priority),
    name: g.name,
    type: g.type,
    hook: g.hook,
    mode: g.mode,
    enforcement: g.enforcement,
    enabled: g.enabled ? "Yes" : "No",
    hit_count_7d: String(g.hit_count_7d),
  }));

  const headers = [
    { key: "priority", header: "Priority" },
    { key: "name", header: "Name" },
    { key: "type", header: "Type" },
    { key: "hook", header: "Hook" },
    { key: "mode", header: "Mode" },
    { key: "enforcement", header: "Enforcement" },
    { key: "enabled", header: "Enabled" },
    { key: "hit_count_7d", header: "Hits (7d)" },
  ];

  return (
    <>
      <h1 style={{ fontSize: "1.75rem", fontWeight: 600, marginBottom: "1.5rem" }}>Guardrail Policies</h1>
      {error && <InlineNotification kind="error" title="Failed to load guardrails" subtitle={(error as Error).message} />}
      {isLoading ? <SkeletonText paragraph /> : (
        <CrudTable title="Guardrails" description="Ordered evaluation chain — lower priority number = evaluated first."
          headers={headers} rows={rows} isLoading={isLoading} queryKey={QUERY_KEY}
          deleteUrl={(id) => `/api/config/guardrails/${id}`}
          onAdd={() => { reset(); setEditId(null); setModalOpen(true); }}
          onEdit={(id) => {
            const g = guardrails.find((x) => x.id === id);
            if (!g) return;
            reset({ name: g.name, type: g.type, hook: g.hook, mode: g.mode, enforcement: g.enforcement,
              enabled: g.enabled, priority: g.priority, config_json: JSON.stringify(g.config, null, 2) });
            setEditId(id); setConfigPreview(JSON.stringify(g.config, null, 2)); setModalOpen(true);
          }}
          renderCell={(header, value) => {
            if (header === "enabled") return <Tag type={value === "Yes" ? "green" : "gray"}>{value as string}</Tag>;
            if (header === "mode") return <Tag type={value === "block" ? "red" : value === "flag" ? "warm-gray" : "gray"}>{value as string}</Tag>;
            if (header === "enforcement") return <Tag type={value === "hard" ? "red" : "blue"}>{value as string}</Tag>;
            return value;
          }}
        />
      )}
      <Modal open={modalOpen} modalHeading={editId ? "Edit Guardrail" : "Create Guardrail"}
        primaryButtonText={isSubmitting ? "Saving…" : "Save"} secondaryButtonText="Cancel"
        onRequestSubmit={handleSubmit((v) => saveMutation.mutate(editId ? { ...v, id: editId } : v))}
        onRequestClose={() => { setModalOpen(false); reset(); }} size="lg">
        <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
          <TextInput id="gr-name" labelText="Name" {...register("name")} invalid={!!errors.name} invalidText={errors.name?.message} />
          <TextInput id="gr-type" labelText="Type (e.g. pii-detection)" {...register("type")} invalid={!!errors.type} />
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: "1rem" }}>
            <Select id="gr-hook" labelText="Hook" {...register("hook")}>
              <SelectItem value="pre_request" text="Pre-request" />
              <SelectItem value="post_response" text="Post-response" />
              <SelectItem value="streaming" text="Streaming" />
            </Select>
            <Select id="gr-mode" labelText="Mode" {...register("mode")}>
              <SelectItem value="block" text="Block" />
              <SelectItem value="flag" text="Flag" />
              <SelectItem value="audit" text="Audit" />
            </Select>
            <Select id="gr-enforcement" labelText="Enforcement" {...register("enforcement")}>
              <SelectItem value="hard" text="Hard" />
              <SelectItem value="soft" text="Soft" />
            </Select>
          </div>
          <Controller name="priority" control={control}
            render={({ field }) => (
              <NumberInput id="gr-priority" label="Priority (lower = first)" min={1} max={999}
                value={field.value} onChange={(_, { value }) => field.onChange(Number(value))} />
            )} />
          <Toggle id="gr-enabled" labelText="Enabled" toggled={enabledValue}
            onToggle={(v: boolean) => reset({ ...watch(), enabled: v })} labelA="Disabled" labelB="Enabled" />
          <div>
            <label style={{ display: "block", marginBottom: "0.25rem", fontSize: "0.875rem", fontWeight: 600 }}>Config JSON</label>
            <textarea id="gr-config" style={{ width: "100%", height: 120, fontFamily: "IBM Plex Mono, monospace", fontSize: "0.75rem", padding: "0.5rem" }}
              {...register("config_json")} onChange={(e) => { register("config_json").onChange(e); setConfigPreview(e.target.value); }} />
            {configPreview && (
              <div style={{ marginTop: "0.5rem" }}>
                <CodeBlock code={configPreview} language="json" maxLines={8} />
              </div>
            )}
          </div>
        </div>
      </Modal>
    </>
  );
}
