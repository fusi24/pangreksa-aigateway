import {
  Document,
  Packer,
  Paragraph,
  Table,
  TableRow,
  TableCell,
  HeadingLevel,
  AlignmentType,
  WidthType,
  TextRun,
} from "docx";
import type { ReportData, ReportParams } from "@/types/api";

const BRAND_COLOR = "0f62fe";

/**
 * Builds a summary paragraph from report metadata.
 */
function buildSummarySection(data: ReportData): Paragraph[] {
  return Object.entries(data.summary).map(([key, value]) =>
    new Paragraph({
      children: [
        new TextRun({ text: `${key}: `, bold: true }),
        new TextRun(String(value)),
      ],
      spacing: { after: 100 },
    })
  );
}

/**
 * Builds a data table from report rows.
 */
function buildDataTable(data: ReportData): Table {
  const keys = data.rows.length > 0 ? Object.keys(data.rows[0] ?? {}) : [];

  const headerRow = new TableRow({
    children: keys.map(
      (key) =>
        new TableCell({
          children: [new Paragraph({ children: [new TextRun({ text: key, bold: true, color: "FFFFFF" })] })],
          shading: { fill: BRAND_COLOR, type: "clear", color: "auto" },
          width: { size: Math.floor(100 / keys.length), type: WidthType.PERCENTAGE },
        })
    ),
  });

  const dataRows = data.rows.map(
    (row, i) =>
      new TableRow({
        children: keys.map(
          (key) => {
            const cellOptions = i % 2 === 1
              ? { children: [new Paragraph(String(row[key] ?? ""))], shading: { fill: "f4f4f4", type: "clear" as const, color: "auto" } }
              : { children: [new Paragraph(String(row[key] ?? ""))] };
            return new TableCell(cellOptions);
          }
        ),
      })
  );

  return new Table({
    rows: [headerRow, ...dataRows],
    width: { size: 100, type: WidthType.PERCENTAGE },
  });
}

/**
 * Generates a DOCX report from the provided data.
 *
 * @param params - Report parameters (type, date range, scope)
 * @param data - Aggregated report data from the admin API
 * @returns A Blob containing the generated .docx file
 */
export async function generateDocx(
  params: ReportParams,
  data: ReportData
): Promise<Blob> {
  const doc = new Document({
    sections: [
      {
        children: [
          // Title page
          new Paragraph({
            text: "Pangreksa AI Gateway",
            heading: HeadingLevel.TITLE,
          }),
          new Paragraph({
            text: `${params.type.replace(/_/g, " ").toUpperCase()} Report`,
            heading: HeadingLevel.HEADING_1,
          }),
          new Paragraph({
            text: `Period: ${params.from} to ${params.to}`,
            alignment: AlignmentType.LEFT,
            spacing: { after: 200 },
          }),
          new Paragraph({
            text: `Scope: ${params.scope}${params.scope_id ? ` (${params.scope_id})` : ""}`,
            spacing: { after: 400 },
          }),

          // Summary section
          new Paragraph({
            text: "Summary",
            heading: HeadingLevel.HEADING_2,
            spacing: { after: 200 },
          }),
          ...buildSummarySection(data),

          // Data table
          ...(data.rows.length > 0
            ? [
                new Paragraph({
                  text: "Detail",
                  heading: HeadingLevel.HEADING_2,
                  spacing: { before: 400, after: 200 },
                }),
                buildDataTable(data),
              ]
            : [new Paragraph("No data available for the selected period.")]),

          // Footer
          new Paragraph({
            children: [
              new TextRun({ text: `Generated: ${data.meta.generated_at}  ·  By: ${data.meta.generated_by}`, color: "525252", size: 18 }),
            ],
            spacing: { before: 400 },
          }),
        ],
      },
    ],
  });

  return Packer.toBlob(doc);
}
