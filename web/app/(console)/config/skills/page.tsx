"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Modal,
  TextInput,
  TextArea,
  Toggle,
  InlineNotification,
  Tag,
  SkeletonText,
} from "@carbon/react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { CrudTable } from "@/components/config/CrudTable";
import { useNotificationStore } from "@/store/notifications";
import { fmtTs, truncate } from "@/lib/utils/format";
import type { SkillRecord } from "@/types/api";

const skillSchema = z.object({
  name: z.string().min(1).max(64),
  description: z.string().max(200, "Max 200 characters"),
  content: z.string().min(1, "SKILL.md content required"),
  preload: z.boolean(),
});
type SkillFormValues = z.infer<typeof skillSchema>;

const QUERY_KEY = ["config", "skills"];

/**
 * Skill Registry CRUD page.
 */
export default function SkillsPage() {
  const qc = useQueryClient();
  const { add: notify } = useNotificationStore();
  const [modalOpen, setModalOpen] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);

  const { data, isLoading, error } = useQuery<{ skills?: SkillRecord[] }>({
    queryKey: QUERY_KEY,
    queryFn: async () => {
      const res = await fetch("/api/config/skills");
      if (!res.ok) throw new Error("Failed to fetch skills");
      return res.json() as Promise<{ skills?: SkillRecord[] }>;
    },
  });

  const {
    register,
    handleSubmit,
    reset,
    setValue,
    watch,
    formState: { errors, isSubmitting },
  } = useForm<SkillFormValues>({
    resolver: zodResolver(skillSchema),
    defaultValues: { preload: false },
  });

  const saveMutation = useMutation({
    mutationFn: async (values: SkillFormValues & { id?: string }) => {
      const { id, ...body } = values;
      const url = id ? `/api/config/skills/${id}` : "/api/config/skills";
      const method = id ? "PUT" : "POST";
      const res = await fetch(url, {
        method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!res.ok) {
        const err = (await res.json()) as { error?: string };
        throw new Error(err.error ?? "Save failed");
      }
    },
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: QUERY_KEY });
      setModalOpen(false);
      setEditId(null);
      reset();
      notify("success", vars.id ? "Skill updated" : "Skill created",
        "Config applied — daemons refresh within ~30s.");
    },
    onError: (err: Error) => notify("error", "Save failed", err.message),
  });

  function openCreate() {
    reset({ preload: false, name: "", description: "", content: "" });
    setEditId(null);
    setModalOpen(true);
  }

  function openEdit(id: string) {
    const skill = data?.skills?.find((s) => s.id === id);
    if (!skill) return;
    reset({ name: skill.name, description: skill.description, content: skill.content, preload: skill.preload });
    setEditId(id);
    setModalOpen(true);
  }

  const preloadValue = watch("preload");
  const descriptionValue = watch("description");

  const rows = (data?.skills ?? []).map((s) => ({
    id: s.id,
    name: s.name,
    description: truncate(s.description, 60),
    preload: s.preload ? "Yes" : "No",
    usage_7d: String(s.usage_7d),
    updated_at: fmtTs(s.updated_at),
  }));

  const headers = [
    { key: "name", header: "Name" },
    { key: "description", header: "Description" },
    { key: "preload", header: "Preload" },
    { key: "usage_7d", header: "Uses (7d)" },
    { key: "updated_at", header: "Updated" },
  ];

  return (
    <>
      <h1 style={{ fontSize: "1.75rem", fontWeight: 600, marginBottom: "1.5rem" }}>
        Skill Registry
      </h1>

      {error && <InlineNotification kind="error" title="Failed to load skills"
        subtitle={(error as Error).message} style={{ marginBottom: "1rem" }} />}

      {isLoading ? <SkeletonText paragraph /> : (
        <CrudTable
          title="Skills"
          description="Skills extend the gateway with reusable logic."
          headers={headers}
          rows={rows}
          isLoading={isLoading}
          queryKey={QUERY_KEY}
          deleteUrl={(id) => `/api/config/skills/${id}`}
          onAdd={openCreate}
          onEdit={openEdit}
          renderCell={(header, value) => {
            if (header === "preload") {
              return <Tag type={value === "Yes" ? "green" : "gray"}>{value as string}</Tag>;
            }
            return value;
          }}
        />
      )}

      <Modal
        open={modalOpen}
        modalHeading={editId ? "Edit Skill" : "Create Skill"}
        primaryButtonText={isSubmitting ? "Saving…" : "Save"}
        secondaryButtonText="Cancel"
        onRequestSubmit={handleSubmit((v) => saveMutation.mutate(editId ? { ...v, id: editId } : v))}
        onRequestClose={() => { setModalOpen(false); reset(); }}
        size="lg"
      >
        {saveMutation.isError && (
          <InlineNotification kind="error" title="Save failed"
            subtitle={(saveMutation.error as Error).message} lowContrast hideCloseButton />
        )}
        <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
          <TextInput id="skill-name" labelText="Name" {...register("name")}
            invalid={!!errors.name} invalidText={errors.name?.message} />
          <div>
            <TextArea id="skill-desc" labelText="Description (max 200 chars)"
              {...register("description")} rows={2}
              invalid={!!errors.description} invalidText={errors.description?.message} />
            <div style={{ color: "#525252", fontSize: "0.75rem", textAlign: "right" }}>
              {(descriptionValue ?? "").length}/200
            </div>
          </div>
          <TextArea id="skill-content" labelText="SKILL.md content"
            {...register("content")} rows={12}
            invalid={!!errors.content} invalidText={errors.content?.message} />
          <Toggle
            id="skill-preload"
            labelText="Preload skill on daemon startup"
            toggled={preloadValue}
            onToggle={(checked: boolean) => setValue("preload", checked)}
            labelA="Off"
            labelB="On"
          />
        </div>
      </Modal>
    </>
  );
}
