"use client";
import Button from "@/components/ui/button/Button";
import { exportCSV } from "@/lib/csv";
import { useState } from "react";

interface ExportCsvButtonProps {
  filename: string;
  headers: string[];
  rows: (string | number)[][];
  loadAllRows?: () => Promise<(string | number)[][]>;
  disabled?: boolean;
}

export default function ExportCsvButton({
  filename,
  headers,
  rows,
  loadAllRows,
  disabled,
}: ExportCsvButtonProps) {
  const [exporting, setExporting] = useState(false);
  async function exportRows() {
    setExporting(true);
    try { exportCSV(filename, headers, loadAllRows ? await loadAllRows() : rows); }
    finally { setExporting(false); }
  }
  return (
    <Button
      variant="outline"
      size="sm"
      disabled={disabled || rows.length === 0 || exporting}
      onClick={() => void exportRows()}
    >
      {exporting ? "Preparing…" : "Export CSV"}
    </Button>
  );
}
