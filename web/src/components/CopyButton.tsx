"use client";

import { useState } from "react";

/** Copies text to the clipboard; says "Copied" only after the copy succeeded. */
export function CopyButton({ text, label }: { text: string; label: string }) {
  const [state, setState] = useState<"idle" | "copied" | "failed">("idle");

  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setState("copied");
    } catch {
      setState("failed");
    }
    setTimeout(() => setState("idle"), 2400);
  }

  return (
    <button type="button" className="btn btn--quiet btn--sm" onClick={copy} aria-label={label}>
      <span aria-live="polite">{state === "copied" ? "Copied" : state === "failed" ? "Copy failed, select it" : "Copy"}</span>
    </button>
  );
}
