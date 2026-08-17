import React from "react";

type Tone = "blue" | "green" | "yellow" | "red" | "gray";

const toneClasses: Record<Tone, string> = {
  blue: "bg-brand-50 text-brand-600 dark:bg-brand-500/10 dark:text-brand-400",
  green:
    "bg-success-50 text-success-600 dark:bg-success-500/10 dark:text-success-400",
  yellow:
    "bg-warning-50 text-warning-600 dark:bg-warning-500/10 dark:text-warning-400",
  red: "bg-error-50 text-error-600 dark:bg-error-500/10 dark:text-error-400",
  gray: "bg-gray-100 text-gray-700 dark:bg-white/[0.05] dark:text-gray-300",
};

export function actionTone(action: string): Tone {
  const upper = action.toUpperCase();
  if (
    upper.includes("FAILED") ||
    upper.includes("RATE_LIMITED") ||
    upper.includes("REJECTED") ||
    upper.includes("REVOKED") ||
    upper.includes("DENIED")
  ) {
    return "red";
  }
  if (upper.includes("MFA")) return "yellow";
  if (upper.includes("LOGIN") || upper.includes("LOGOUT")) return "blue";
  if (upper.includes("CREATED") || upper.includes("ENABLED")) return "green";
  if (upper.includes("UPDATED") || upper.includes("RESET")) return "yellow";
  return "gray";
}

export default function ActionBadge({ action }: { action: string }) {
  const tone = actionTone(action);
  return (
    <span
      className={`inline-flex rounded-full px-2.5 py-1 text-xs font-medium ${toneClasses[tone]}`}
    >
      {action}
    </span>
  );
}
