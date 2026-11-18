"use client";

// The dashboard at work: one screen held in view while four short panels
// scroll past, each switching the screen it describes. Without script, or
// under 60rem, every panel carries its own screen inline instead.
import { useEffect, useRef, useState } from "react";

export interface Screen {
  id: string;
  title: string;
  line: string;
  src: string;
  alt: string;
}

export function DashboardRun({ screens }: { screens: Screen[] }) {
  const [active, setActive] = useState(0);
  const panels = useRef<(HTMLLIElement | null)[]>([]);

  useEffect(() => {
    const io = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (e.isIntersecting) setActive(Number((e.target as HTMLElement).dataset.index));
        }
      },
      { rootMargin: "-45% 0px -45% 0px" },
    );
    panels.current.forEach((p) => p && io.observe(p));
    return () => io.disconnect();
  }, []);

  return (
    <div className="run">
      <div className="run-stage" aria-hidden="true">
        <div className="run-frame print">
          {screens.map((s, i) => (
            <img
              key={s.id}
              src={s.src}
              alt=""
              width={2048}
              height={1536}
              loading={i === 0 ? "eager" : "lazy"}
              className={i === active ? "is-on" : undefined}
            />
          ))}
        </div>
      </div>
      <ol className="run-panels">
        {screens.map((s, i) => (
          <li
            key={s.id}
            data-index={i}
            ref={(el) => {
              panels.current[i] = el;
            }}
            className={i === active ? "is-on" : undefined}
          >
            <h3>{s.title}</h3>
            <p>{s.line}</p>
            <div className="run-inline print">
              <a href={s.src} aria-label={`Open the full-size screenshot: ${s.title}`}>
                <img src={s.src} alt={s.alt} width={2048} height={1536} loading="lazy" />
              </a>
            </div>
          </li>
        ))}
      </ol>
    </div>
  );
}
