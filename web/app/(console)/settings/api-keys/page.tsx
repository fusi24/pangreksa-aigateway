"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  DataTable,
  Table,
  TableHead,
  TableRow,
  TableHeader,
  TableBody,
  TableCell,
  TableContainer,
  TableToolbar,
  TableToolbarContent,
  TableToolbarSearch,
  Button,
  Modal,
  TextInput,
  InlineNotification,
  CodeSnippet,
  Tag,
  SkeletonText,
} from "@carbon/react";
import { Add, TrashCan } from "@carbon/icons-react";
import { useNotificationStore } from "@/store/notifications";
import { fmtTs, fmtRelative } from "@/lib/utils/format";
import type { ApiKeyRecord, ApiKeyCreateResponse } from "@/types/api";

const HEADERS = [
  { key: "name", header: "Name" },
  { key: "prefix", header: "Token prefix" },
  { key: "type", header: "Type" },
  { key: "created_at", header: "Created" },
  { key: "last_used_at", header: "Last used" },
  { key: "actions", header: "" },
];

/**
 * API Key Management page — list, create, and revoke PAT/VAT keys.
 */
export default function ApiKeysPage() {
  const qc = useQueryClient();
  const { add: notify } = useNotificationStore();

  const [createOpen, setCreateOpen] = useState(false);
  const [newKeyName, setNewKeyName] = useState("");
  const [createdToken, setCreatedToken] = useState<string | null>(null);
  const [tokenCopied, setTokenCopied] = useState(false);
  const [revokeId, setRevokeId] = useState<string | null>(null);

  const { data, isLoading, error } = useQuery<{ keys: ApiKeyRecord[] }>({
    queryKey: ["api-keys"],
    queryFn: async () => {
      const res = await fetch("/api/settings/api-keys");
      if (!res.ok) throw new Error("Failed to load API keys");
      return res.json() as Promise<{ keys: ApiKeyRecord[] }>;
    },
  });

  const createMutation = useMutation({
    mutationFn: async (name: string) => {
      const res = await fetch("/api/settings/api-keys", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
      });
      if (!res.ok) {
        const err = (await res.json()) as { error: string };
        throw new Error(err.error);
      }
      return res.json() as Promise<ApiKeyCreateResponse>;
    },
    onSuccess: (result) => {
      qc.invalidateQueries({ queryKey: ["api-keys"] });
      setCreatedToken(result.token);
      setCreateOpen(false);
      setNewKeyName("");
    },
    onError: (err: Error) => {
      notify("error", "Failed to create key", err.message);
    },
  });

  const revokeMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`/api/settings/api-keys?id=${id}`, {
        method: "DELETE",
      });
      if (!res.ok) throw new Error("Revoke failed");
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["api-keys"] });
      setRevokeId(null);
      notify("success", "API key revoked");
    },
    onError: (err: Error) => {
      notify("error", "Failed to revoke key", err.message);
    },
  });

  const rows = (data?.keys ?? []).map((k) => ({
    id: k.id,
    name: k.name,
    prefix: k.prefix,
    type: k.type,
    created_at: fmtTs(k.created_at),
    last_used_at: k.last_used_at ? fmtRelative(k.last_used_at) : "Never",
    actions: k.id,
  }));

  if (error) {
    return (
      <InlineNotification
        kind="error"
        title="Failed to load API keys"
        subtitle={(error as Error).message}
      />
    );
  }

  return (
    <>
      <div style={{ marginBottom: "1rem" }}>
        <h2>API Keys</h2>
        <p style={{ color: "#525252" }}>
          Manage Personal Access Tokens (PAT) for your account.
          Token values are shown only once at creation time.
        </p>
      </div>

      {isLoading ? (
        <SkeletonText paragraph />
      ) : (
        <DataTable rows={rows} headers={HEADERS}>
          {({ rows: tableRows, headers, getTableProps, getRowProps }) => (
            <TableContainer title="API Keys" description="Your active access tokens">
              <TableToolbar>
                <TableToolbarContent>
                  <TableToolbarSearch />
                  <Button
                    renderIcon={Add}
                    onClick={() => setCreateOpen(true)}
                  >
                    Create key
                  </Button>
                </TableToolbarContent>
              </TableToolbar>
              <Table {...getTableProps()}>
                <TableHead>
                  <TableRow>
                    {headers.map((h) => (
                      <TableHeader key={h.key}>
                        {h.header}
                      </TableHeader>
                    ))}
                  </TableRow>
                </TableHead>
                <TableBody>
                  {tableRows.map((row) => {
                    const { key: rowKey, ...rowProps } = getRowProps({ row });
                    return (
                    <TableRow key={rowKey} {...rowProps}>
                      {row.cells.map((cell) => (
                        <TableCell key={cell.id}>
                          {cell.info.header === "type" ? (
                            <Tag type={cell.value === "PAT" ? "blue" : "purple"}>
                              {cell.value as string}
                            </Tag>
                          ) : cell.info.header === "actions" ? (
                            <Button
                              kind="danger--ghost"
                              size="sm"
                              renderIcon={TrashCan}
                              iconDescription="Revoke"
                              hasIconOnly
                              onClick={() => setRevokeId(cell.value as string)}
                            />
                          ) : (
                            (cell.value as string)
                          )}
                        </TableCell>
                      ))}
                    </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </DataTable>
      )}

      {/* Create Key Modal */}
      <Modal
        open={createOpen}
        modalHeading="Create API Key"
        primaryButtonText={createMutation.isPending ? "Creating…" : "Create"}
        secondaryButtonText="Cancel"
        onRequestSubmit={() => {
          if (newKeyName.trim()) createMutation.mutate(newKeyName.trim());
        }}
        onRequestClose={() => {
          setCreateOpen(false);
          setNewKeyName("");
        }}
      >
        <TextInput
          id="key-name"
          labelText="Key name"
          placeholder="e.g. CI pipeline token"
          value={newKeyName}
          onChange={(e) => setNewKeyName(e.target.value)}
          helperText="Give this key a descriptive name so you can identify it later."
        />
      </Modal>

      {/* Token Display Modal — shown once after creation */}
      <Modal
        open={createdToken !== null}
        modalHeading="API Key Created"
        primaryButtonText="I have copied this token"
        primaryButtonDisabled={!tokenCopied}
        secondaryButtonText={undefined}
        onRequestSubmit={() => {
          setCreatedToken(null);
          setTokenCopied(false);
        }}
        onRequestClose={() => {
          setCreatedToken(null);
          setTokenCopied(false);
        }}
        preventCloseOnClickOutside
      >
        <InlineNotification
          kind="warning"
          title="Save your token now"
          subtitle="This is the only time your token will be displayed. It cannot be recovered."
          lowContrast
          hideCloseButton
        />
        <div style={{ marginTop: "1rem" }}>
          <CodeSnippet
            type="single"
            feedback="Copied!"
            onClick={() => setTokenCopied(true)}
          >
            {createdToken ?? ""}
          </CodeSnippet>
        </div>
      </Modal>

      {/* Revoke Confirm Modal */}
      <Modal
        open={revokeId !== null}
        danger
        modalHeading="Revoke API Key"
        primaryButtonText={revokeMutation.isPending ? "Revoking…" : "Revoke"}
        secondaryButtonText="Cancel"
        onRequestSubmit={() => {
          if (revokeId) revokeMutation.mutate(revokeId);
        }}
        onRequestClose={() => setRevokeId(null)}
      >
        <p>
          This action is immediate and cannot be undone. Any application using this
          key will stop working.
        </p>
      </Modal>
    </>
  );
}
