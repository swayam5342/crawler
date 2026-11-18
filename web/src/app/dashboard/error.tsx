"use client";

// Catches a crash in one dashboard page so the header and navigation stay
// usable, instead of the whole app showing "This page couldn't load".
import { useEffect } from "react";
import s from "@/components/dashboard/dashboard.module.css";

export default function DashboardError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <div className={s.alert} role="alert">
      <strong>This page hit an error.</strong> {error.message || "Something went wrong while showing it."} The rest of the
      dashboard still works; use the tabs above, or{" "}
      <button type="button" className="btn btn-sm" onClick={reset}>
        try this page again
      </button>
    </div>
  );
}
