import s from "./dashboard.module.css";

export function PageHead({
  title,
  description,
  actions,
}: {
  title: string;
  description?: React.ReactNode;
  actions?: React.ReactNode;
}) {
  return (
    <div className={s.pageHead}>
      <div>
        <h1>{title}</h1>
        {description && <p>{description}</p>}
      </div>
      {actions && <div className={s.actions}>{actions}</div>}
    </div>
  );
}

export function Panel({
  title,
  meta,
  children,
  className = "",
  flush = false,
}: {
  title?: string;
  meta?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
  flush?: boolean;
}) {
  return (
    <section className={`${s.panel} ${className}`}>
      {(title || meta) && (
        <div className={s.panelHead}>
          {title ? <h2>{title}</h2> : <span />}
          {meta && <span>{meta}</span>}
        </div>
      )}
      {flush ? children : <div className={s.panelBody}>{children}</div>}
    </section>
  );
}

export function Empty({ title, children }: { title: string; children?: React.ReactNode }) {
  return (
    <div className={s.empty}>
      <strong>{title}</strong>
      {children}
    </div>
  );
}

/** A URL in monospace, optionally linking to the page in a new tab. */
export function URLText({ url, link = true }: { url: string; link?: boolean }) {
  return link ? (
    <a className={s.url} href={url} target="_blank" rel="noreferrer noopener">
      {url}
    </a>
  ) : (
    <span className={s.url}>{url}</span>
  );
}

type Tone = "ok" | "bad" | "warn" | "info" | "plain";

export function Badge({ tone = "plain", children }: { tone?: Tone; children: React.ReactNode }) {
  return <span className={`${s.badge} ${tone !== "plain" ? s[`badge_${tone}`] : ""}`}>{children}</span>;
}

export function BreakerBadge({ state }: { state?: string }) {
  if (!state || state === "closed") return <Badge tone="ok">OK</Badge>;
  if (state === "open") return <Badge tone="bad">Paused (too many errors)</Badge>;
  if (state === "half-open") return <Badge tone="warn">Retrying</Badge>;
  return <Badge>{state}</Badge>;
}

export function statusTone(code: number): Tone {
  if (code >= 200 && code < 300) return "ok";
  if (code >= 300 && code < 400) return "info";
  if (code >= 400 && code < 500) return "warn";
  return "bad";
}

export { s as styles };
