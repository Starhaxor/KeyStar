import React from "react";
import Label from "./Label";

interface FieldProps {
  id: string;
  label: React.ReactNode;
  name: string;
  children: React.ReactNode;
  description?: React.ReactNode;
  error?: React.ReactNode;
}

const controlElements = new Set(["input", "select", "textarea"]);

function connectControl(
  children: React.ReactNode,
  controlProps: Record<string, string | boolean | undefined>
): React.ReactNode {
  return React.Children.map(children, (child) => {
    if (!React.isValidElement<{ children?: React.ReactNode }>(child)) {
      return child;
    }

    if (typeof child.type === "string" && controlElements.has(child.type)) {
      return React.cloneElement(child, controlProps);
    }

    if (child.props.children) {
      return React.cloneElement(child, undefined, connectControl(child.props.children, controlProps));
    }

    return child;
  });
}

export default function Field({
  id,
  label,
  name,
  children,
  description,
  error,
}: FieldProps) {
  const descriptionId = description ? `${id}-description` : undefined;
  const errorId = error ? `${id}-error` : undefined;
  const describedBy = [descriptionId, errorId].filter(Boolean).join(" ") || undefined;

  return (
    <div>
      <Label htmlFor={id}>{label}</Label>
      {connectControl(children, {
        id,
        name,
        "aria-invalid": error ? true : undefined,
        "aria-describedby": describedBy,
      })}
      {description && (
        <p id={descriptionId} className="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
          {description}
        </p>
      )}
      {error && (
        <p id={errorId} className="mt-1.5 text-xs text-error-500" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}
