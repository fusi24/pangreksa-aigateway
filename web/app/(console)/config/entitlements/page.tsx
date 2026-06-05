"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Search, Button, TextInput, NumberInput, Select, SelectItem,
  InlineNotification, Tile, SkeletonText, Tag,
} from "@carbon/react";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useNotificationStore } from "@/store/notifications";
import type { EntitlementRecord } from "@/types/api";

const entitlementSchema = z.object({
  user_id: z.string().uuid(),
  allowed_prompts: z.string(),
  allowed_skills: z.string(),
  allowed_mcps: z.string(),
  budget_limit_usd: z.number().nullable(),
  rate_limit_rpm: z.number().int().nullable(),
  rate_limit_tpm: z.number().int().nullable(),
  data_scope: z.string(),
});
type EntitlementFormValues = z.infer<typeof entitlementSchema>;

interface UserResult { user_id: string; email: string; name: string }

/**
 * Entitlement Manager — search users and edit their allowed resources.
 */
export default function EntitlementsPage() {
  const qc = useQueryClient();
  const { add: notify } = useNotificationStore();
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedUser, setSelectedUser] = useState<UserResult | null>(null);

  const searchResults = useQuery<{ users: UserResult[] }>({
    queryKey: ["user-search", searchQuery],
    queryFn: async () => {
      if (!searchQuery.trim()) return { users: [] };
      const res = await fetch(`/api/config/entitlements/search?q=${encodeURIComponent(searchQuery)}`);
      if (!res.ok) return { users: [] };
      return res.json() as Promise<{ users: UserResult[] }>;
    },
    enabled: searchQuery.length > 2,
  });

  const entitlementQuery = useQuery<EntitlementRecord>({
    queryKey: ["entitlement", selectedUser?.user_id],
    queryFn: async () => {
      const res = await fetch(`/api/config/entitlements?user_id=${selectedUser!.user_id}`);
      if (!res.ok) throw new Error("Failed to load entitlement");
      return res.json() as Promise<EntitlementRecord>;
    },
    enabled: !!selectedUser,
  });

  const { register, handleSubmit, control, reset, formState: { errors, isSubmitting } } =
    useForm<EntitlementFormValues>({ resolver: zodResolver(entitlementSchema) });

  // Pre-fill form when entitlement loads
  if (entitlementQuery.data && selectedUser) {
    const e = entitlementQuery.data;
    reset({
      user_id: e.user_id,
      allowed_prompts: e.allowed_prompts.join(", "),
      allowed_skills: e.allowed_skills.join(", "),
      allowed_mcps: e.allowed_mcps.join(", "),
      budget_limit_usd: e.budget_limit_usd,
      rate_limit_rpm: e.rate_limit_rpm,
      rate_limit_tpm: e.rate_limit_tpm,
      data_scope: e.data_scope,
    });
  }

  const saveMutation = useMutation({
    mutationFn: async (values: EntitlementFormValues) => {
      const body = {
        ...values,
        allowed_prompts: values.allowed_prompts.split(",").map((s) => s.trim()).filter(Boolean),
        allowed_skills: values.allowed_skills.split(",").map((s) => s.trim()).filter(Boolean),
        allowed_mcps: values.allowed_mcps.split(",").map((s) => s.trim()).filter(Boolean),
      };
      const res = await fetch("/api/config/entitlements", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!res.ok) { const err = (await res.json()) as { error?: string }; throw new Error(err.error ?? "Save failed"); }
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["entitlement"] });
      notify("success", "Entitlement updated", "Config applied — daemons refresh within ~30s.");
    },
    onError: (err: Error) => notify("error", "Save failed", err.message),
  });

  return (
    <>
      <h1 style={{ fontSize: "1.75rem", fontWeight: 600, marginBottom: "1.5rem" }}>Entitlement Manager</h1>

      {/* User search */}
      <Tile style={{ marginBottom: "1.5rem" }}>
        <h3 style={{ marginBottom: "1rem" }}>Select User</h3>
        <Search id="user-search" labelText="Search by email or user ID" value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)} placeholder="user@example.com or UUID" />
        {searchResults.data?.users && searchResults.data.users.length > 0 && (
          <div style={{ marginTop: "0.5rem", border: "1px solid #e0e0e0", borderRadius: 4 }}>
            {searchResults.data.users.map((u) => (
              <button key={u.user_id}
                style={{ display: "block", width: "100%", padding: "0.5rem 1rem", textAlign: "left",
                  background: selectedUser?.user_id === u.user_id ? "#e8f1ff" : "transparent",
                  border: "none", cursor: "pointer", borderBottom: "1px solid #e0e0e0" }}
                onClick={() => setSelectedUser(u)}>
                <strong>{u.email}</strong>
                <span style={{ color: "#525252", fontSize: "0.75rem", marginLeft: "0.5rem", fontFamily: "IBM Plex Mono, monospace" }}>
                  {u.user_id.slice(0, 8)}…
                </span>
              </button>
            ))}
          </div>
        )}
        {selectedUser && (
          <div style={{ marginTop: "0.5rem" }}>
            Selected: <Tag type="blue">{selectedUser.email}</Tag>
          </div>
        )}
      </Tile>

      {/* Entitlement form */}
      {selectedUser && (
        <Tile>
          <h3 style={{ marginBottom: "1rem" }}>Entitlements for {selectedUser.email}</h3>
          {entitlementQuery.isLoading && <SkeletonText paragraph />}
          {entitlementQuery.error && (
            <InlineNotification kind="error" title="Failed to load entitlement"
              subtitle={(entitlementQuery.error as Error).message} />
          )}
          {(entitlementQuery.data || !entitlementQuery.isLoading) && !entitlementQuery.error && (
            <form onSubmit={handleSubmit((v) => saveMutation.mutate(v))}>
              <input type="hidden" {...register("user_id")} value={selectedUser.user_id} />
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "1rem" }}>
                <TextInput id="ent-prompts" labelText="Allowed prompts (FQN patterns, comma-separated)"
                  {...register("allowed_prompts")} helperText="e.g. chat_prompt:*:latest, code_assist:*" />
                <TextInput id="ent-skills" labelText="Allowed skills (comma-separated)"
                  {...register("allowed_skills")} helperText="e.g. summarize, translate" />
                <TextInput id="ent-mcps" labelText="Allowed MCP servers (comma-separated)"
                  {...register("allowed_mcps")} />
                <Select id="ent-scope" labelText="Data scope" {...register("data_scope")}>
                  <SelectItem value="org" text="Organization" />
                  <SelectItem value="own" text="Own data only" />
                  <SelectItem value="all" text="All (admin)" />
                </Select>
                <Controller name="budget_limit_usd" control={control}
                  render={({ field }) => (
                    <NumberInput id="ent-budget" label="Budget limit (USD/mo, 0 = unlimited)"
                      min={0} step={0.01} value={field.value ?? 0}
                      onChange={(_, { value }) => field.onChange(Number(value) || null)} />
                  )} />
                <Controller name="rate_limit_rpm" control={control}
                  render={({ field }) => (
                    <NumberInput id="ent-rpm" label="Rate limit (req/min, 0 = unlimited)"
                      min={0} value={field.value ?? 0}
                      onChange={(_, { value }) => field.onChange(Number(value) || null)} />
                  )} />
                <Controller name="rate_limit_tpm" control={control}
                  render={({ field }) => (
                    <NumberInput id="ent-tpm" label="Rate limit (tokens/min, 0 = unlimited)"
                      min={0} value={field.value ?? 0}
                      onChange={(_, { value }) => field.onChange(Number(value) || null)} />
                  )} />
              </div>
              {saveMutation.isError && (
                <InlineNotification kind="error" title="Save failed"
                  subtitle={(saveMutation.error as Error).message} lowContrast hideCloseButton
                  style={{ marginTop: "1rem" }} />
              )}
              <div style={{ marginTop: "1.5rem" }}>
                <Button type="submit" disabled={isSubmitting}>
                  {isSubmitting ? "Saving…" : "Save Entitlement"}
                </Button>
              </div>
            </form>
          )}
        </Tile>
      )}
    </>
  );
}
