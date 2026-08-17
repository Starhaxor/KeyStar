// Client-side CSV export with a UTF-8 BOM so Excel opens Turkish characters
// correctly, and RFC-4180 escaping for commas, quotes and newlines.
export function exportCSV(
  filename: string,
  headers: string[],
  rows: (string | number)[][]
): void {
  const escape = (value: string | number): string => {
    const text = String(value ?? "");
    return /[",\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text;
  };
  const content = [headers, ...rows]
    .map((row) => row.map(escape).join(","))
    .join("\r\n");
  const blob = new Blob(["\uFEFF" + content], {
    type: "text/csv;charset=utf-8",
  });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
  URL.revokeObjectURL(url);
}
