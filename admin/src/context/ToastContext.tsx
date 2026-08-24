"use client";
import React, {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
} from "react";

export type ToastKind = "success" | "error" | "info";

interface Toast {
  id: number;
  kind: ToastKind;
  title: string;
  message?: string;
}

interface ToastContextValue {
  success: (title: string, message?: string) => void;
  error: (title: string, message?: string) => void;
  info: (title: string, message?: string) => void;
}

const ToastContext = createContext<ToastContextValue | undefined>(undefined);

const TOAST_DURATION_MS = 4000;

export const ToastProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(1);

  const dismiss = useCallback((id: number) => {
    setToasts((current) => current.filter((toast) => toast.id !== id));
  }, []);

  const push = useCallback(
    (kind: ToastKind, title: string, message?: string) => {
      const id = nextId.current++;
      setToasts((current) => [...current.slice(-3), { id, kind, title, message }]);
      window.setTimeout(() => dismiss(id), TOAST_DURATION_MS);
    },
    [dismiss]
  );

  const value = useMemo<ToastContextValue>(
    () => ({
      success: (title, message) => push("success", title, message),
      error: (title, message) => push("error", title, message),
      info: (title, message) => push("info", title, message),
    }),
    [push]
  );

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div
        aria-live="polite"
        className="fixed bottom-4 right-4 z-99999 flex w-full max-w-sm flex-col gap-2"
      >
        {toasts.map((toast) => (
          <ToastCard key={toast.id} toast={toast} onDismiss={() => dismiss(toast.id)} />
        ))}
      </div>
    </ToastContext.Provider>
  );
};

const kindStyles: Record<
  ToastKind,
  { ring: string; iconBg: string; icon: React.ReactNode }
> = {
  success: {
    ring: "border-success-500/40",
    iconBg: "bg-success-50 text-success-600 dark:bg-success-500/10 dark:text-success-400",
    icon: (
      <path
        fillRule="evenodd"
        clipRule="evenodd"
        d="M16.7045 5.2955C17.0986 5.68963 17.0986 6.32537 16.7045 6.7195L8.3695 15.0545C7.97537 15.4486 7.33963 15.4486 6.9455 15.0545L3.2955 11.4045C2.90137 11.0104 2.90137 10.3746 3.2955 9.9805C3.68963 9.58637 4.32537 9.58637 4.7195 9.9805L7.6575 12.9185L15.2805 5.2955C15.6746 4.90137 16.3104 4.90137 16.7045 5.2955Z"
        fill="currentColor"
      />
    ),
  },
  error: {
    ring: "border-error-500/40",
    iconBg: "bg-error-50 text-error-600 dark:bg-error-500/10 dark:text-error-400",
    icon: (
      <path
        fillRule="evenodd"
        clipRule="evenodd"
        d="M10 2C5.58172 2 2 5.58172 2 10C2 14.4183 5.58172 18 10 18C14.4183 18 18 14.4183 18 10C18 5.58172 14.4183 2 10 2ZM10 6.25C10.4142 6.25 10.75 6.58579 10.75 7V11C10.75 11.4142 10.4142 11.75 10 11.75C9.58579 11.75 9.25 11.4142 9.25 11V7C9.25 6.58579 9.58579 6.25 10 6.25ZM10 13.25C9.58579 13.25 9.25 13.5858 9.25 14C9.25 14.4142 9.58579 14.75 10 14.75C10.4142 14.75 10.75 14.4142 10.75 14C10.75 13.5858 10.4142 13.25 10 13.25Z"
        fill="currentColor"
      />
    ),
  },
  info: {
    ring: "border-brand-500/40",
    iconBg: "bg-brand-50 text-brand-600 dark:bg-brand-500/10 dark:text-brand-400",
    icon: (
      <path
        fillRule="evenodd"
        clipRule="evenodd"
        d="M10 2C5.58172 2 2 5.58172 2 10C2 14.4183 5.58172 18 10 18C14.4183 18 18 14.4183 18 10C18 5.58172 14.4183 2 10 2ZM9.25 9.25C9.25 8.83579 9.58579 8.5 10 8.5C10.4142 8.5 10.75 8.83579 10.75 9.25V13.75C10.75 14.1642 10.4142 14.5 10 14.5C9.58579 14.5 9.25 14.1642 9.25 13.75V9.25ZM10 6C9.58579 6 9.25 6.33579 9.25 6.75C9.25 7.16421 9.58579 7.5 10 7.5C10.4142 7.5 10.75 7.16421 10.75 6.75C10.75 6.33579 10.4142 6 10 6Z"
        fill="currentColor"
      />
    ),
  },
};

function ToastCard({
  toast,
  onDismiss,
}: {
  toast: Toast;
  onDismiss: () => void;
}) {
  const styles = kindStyles[toast.kind];
  return (
    <div
      role="status"
      className={`flex items-start gap-3 rounded-xl border bg-white p-4 shadow-theme-lg dark:bg-gray-900 ${styles.ring}`}
    >
      <span
        className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-full ${styles.iconBg}`}
      >
        <svg width="18" height="18" viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg">
          {styles.icon}
        </svg>
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-gray-800 dark:text-white/90">
          {toast.title}
        </p>
        {toast.message && (
          <p className="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {toast.message}
          </p>
        )}
      </div>
      <button
        onClick={onDismiss}
        aria-label="Dismiss notification"
        className="shrink-0 text-gray-400 transition-colors hover:text-gray-700 dark:hover:text-white"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
          <path
            fillRule="evenodd"
            clipRule="evenodd"
            d="M6.04289 16.5413C5.65237 16.9318 5.65237 17.565 6.04289 17.9555C6.43342 18.346 7.06658 18.346 7.45711 17.9555L11.9987 13.4139L16.5408 17.956C16.9313 18.3466 17.5645 18.3466 17.955 17.956C18.3455 17.5655 18.3455 16.9323 17.955 16.5418L13.4129 11.9997L17.955 7.4576C18.3455 7.06707 18.3455 6.43391 17.955 6.04338C17.5645 5.65286 16.9313 5.65286 16.5408 6.04338L11.9987 10.5855L7.45711 6.0439C7.06658 5.65338 6.43342 5.65338 6.04289 6.0439C5.65237 6.43442 5.65237 7.06759 6.04289 7.45811L10.5845 11.9997L6.04289 16.5413Z"
            fill="currentColor"
          />
        </svg>
      </button>
    </div>
  );
}

export const useToast = () => {
  const context = useContext(ToastContext);
  if (context === undefined) {
    throw new Error("useToast must be used within a ToastProvider");
  }
  return context;
};
