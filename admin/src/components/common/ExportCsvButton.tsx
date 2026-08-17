"use client";
import Button from "@/components/ui/button/Button";
import { exportCSV } from "@/lib/csv";

interface ExportCsvButtonProps {
  filename: string;
  headers: string[];
  rows: (string | number)[][];
  disabled?: boolean;
}

export default function ExportCsvButton({
  filename,
  headers,
  rows,
  disabled,
}: ExportCsvButtonProps) {
  return (
    <Button
      variant="outline"
      size="sm"
      disabled={disabled || rows.length === 0}
      onClick={() => exportCSV(filename, headers, rows)}
    >
      Export CSV
    </Button>
  );
}
