"use client";

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
  InlineNotification,
} from "@carbon/react";
import { Add, TrashCan, Edit } from "@carbon/icons-react";
import { useState } from "react";
import { useQueryClient, useMutation } from "@tanstack/react-query";
import { useNotificationStore } from "@/store/notifications";

export interface CrudHeader {
  key: string;
  header: string;
}

export interface CrudRow {
  id: string;
  [key: string]: string | React.ReactNode;
}

interface CrudTableProps {
  title: string;
  description?: string;
  headers: CrudHeader[];
  rows: CrudRow[];
  isLoading: boolean;
  queryKey: string[];
  /** URL for DELETE /api/config/{entity}/{id} */
  deleteUrl: (id: string) => string;
  onAdd: () => void;
  onEdit: (id: string) => void;
  /** Extra columns rendered as React nodes — keyed by header key */
  renderCell?: (header: string, value: string | React.ReactNode, rowId: string) => React.ReactNode;
}

/**
 * Reusable CRUD table used by all 6 Configuration Manager pages.
 * Handles Delete confirmation, toolbar search, and Add/Edit actions.
 */
export function CrudTable({
  title,
  description,
  headers,
  rows,
  isLoading,
  queryKey,
  deleteUrl,
  onAdd,
  onEdit,
  renderCell,
}: CrudTableProps) {
  const qc = useQueryClient();
  const { add: notify } = useNotificationStore();
  const [deleteId, setDeleteId] = useState<string | null>(null);

  const deleteMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(deleteUrl(id), { method: "DELETE" });
      if (!res.ok) {
        const err = (await res.json()) as { error?: string };
        throw new Error(err.error ?? "Delete failed");
      }
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey });
      setDeleteId(null);
      notify("success", "Deleted successfully", "Config change applied — daemons will refresh within ~30s.");
    },
    onError: (err: Error) => {
      notify("error", "Delete failed", err.message);
    },
  });

  const allHeaders = [
    ...headers,
    { key: "_edit", header: "" },
    { key: "_delete", header: "" },
  ];

  const enrichedRows = rows.map((r) => ({
    ...r,
    _edit: r.id,
    _delete: r.id,
  }));

  return (
    <>
      <DataTable rows={enrichedRows} headers={allHeaders} isSortable>
        {({ rows: tableRows, headers: tableHeaders, getTableProps, getRowProps }) => (
          <TableContainer title={title} description={description}>
            <TableToolbar>
              <TableToolbarContent>
                <TableToolbarSearch />
                <Button renderIcon={Add} onClick={onAdd} disabled={isLoading}>
                  Add
                </Button>
              </TableToolbarContent>
            </TableToolbar>
            <Table {...getTableProps()} size="md">
              <TableHead>
                <TableRow>
                  {tableHeaders.map((h) => <TableHeader key={h.key}>{h.header}</TableHeader>)}
                </TableRow>
              </TableHead>
              <TableBody>
                {tableRows.map((row) => {
                  const { key: rk, ...rp } = getRowProps({ row });
                  return (
                    <TableRow key={rk} {...rp}>
                      {row.cells.map((cell) => {
                        if (cell.info.header === "_edit") {
                          return (
                            <TableCell key={cell.id} style={{ width: 40 }}>
                              <Button kind="ghost" size="sm" renderIcon={Edit} iconDescription="Edit" hasIconOnly
                                onClick={() => onEdit(cell.value as string)} />
                            </TableCell>
                          );
                        }
                        if (cell.info.header === "_delete") {
                          return (
                            <TableCell key={cell.id} style={{ width: 40 }}>
                              <Button kind="danger--ghost" size="sm" renderIcon={TrashCan} iconDescription="Delete" hasIconOnly
                                onClick={() => setDeleteId(cell.value as string)} />
                            </TableCell>
                          );
                        }
                        const rendered = renderCell
                          ? renderCell(cell.info.header, cell.value as string | React.ReactNode, row.id)
                          : (cell.value as React.ReactNode);
                        return <TableCell key={cell.id}>{rendered}</TableCell>;
                      })}
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </DataTable>

      <Modal
        open={deleteId !== null}
        danger
        modalHeading="Confirm Delete"
        primaryButtonText={deleteMutation.isPending ? "Deleting…" : "Delete"}
        secondaryButtonText="Cancel"
        onRequestSubmit={() => { if (deleteId) deleteMutation.mutate(deleteId); }}
        onRequestClose={() => setDeleteId(null)}
      >
        <p>This action cannot be undone. Daemons will refresh within ~30 seconds.</p>
        {deleteMutation.isError && (
          <InlineNotification kind="error" title="Delete failed"
            subtitle={(deleteMutation.error as Error).message} lowContrast hideCloseButton />
        )}
      </Modal>
    </>
  );
}
