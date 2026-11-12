import { useEffect, useState } from "react";
import { Close, GetActive } from "../wailsjs/go/main/Widget";

type Card = Awaited<ReturnType<typeof GetActive>>;

const drag = { "--wails-draggable": "drag" } as React.CSSProperties;
const noDrag = { "--wails-draggable": "no-drag" } as React.CSSProperties;

function barColor(percent: number) {
  if (percent > 50) return "#00ff9d";
  if (percent > 20) return "#ffb000";
  return "#ff3b5c";
}

// "84% left  resets 2026-11-17 14:28" -> "17/11 14:28"
function resetText(summary: string) {
  const m = summary.match(/resets \d{4}-(\d{2})-(\d{2}) (\d{2}:\d{2})/);
  return m ? `${m[2]}/${m[1]} ${m[3]}` : "";
}

function Bar({ label, percent, summary }: { label: string; percent: number; summary: string }) {
  const hasData = summary && summary !== "-";
  return (
    <div style={{ marginTop: 8 }}>
      <div style={{ display: "flex", justifyContent: "space-between", fontSize: 11 }}>
        <span style={{ opacity: 0.7 }}>{label}</span>
        <span>
          <b style={{ color: hasData ? barColor(percent) : "#888" }}>{hasData ? `${percent}%` : "-"}</b>
          {hasData && resetText(summary) && <span style={{ opacity: 0.7 }}> · reset {resetText(summary)}</span>}
        </span>
      </div>
      <div style={{ height: 6, marginTop: 3, borderRadius: 3, background: "rgba(255,255,255,0.12)" }}>
        <div style={{ width: `${hasData ? percent : 0}%`, height: "100%", borderRadius: 3, background: barColor(percent) }} />
      </div>
    </div>
  );
}

export default function QuotaWidget() {
  const [card, setCard] = useState<Card | null>(null);

  useEffect(() => {
    document.documentElement.style.background = "transparent";
    document.body.style.background = "transparent";
    document.body.classList.add("widget-mode");
    const load = () => void GetActive().then((c) => setCard(c ?? null)).catch(() => setCard(null));
    load();
    const timer = window.setInterval(load, 5000);
    return () => window.clearInterval(timer);
  }, []);

  return (
    <div
      style={{
        ...drag,
        height: "100vh",
        boxSizing: "border-box",
        padding: "10px 12px",
        color: "#e6faff",
        fontFamily: "'JetBrains Mono', Consolas, monospace",
        background: "rgba(8, 10, 16, 0.45)",
        border: "1px solid rgba(0, 243, 255, 0.35)",
        borderRadius: 10,
        cursor: "move",
        userSelect: "none",
      }}
    >
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 8 }}>
        <span style={{ fontSize: 12, fontWeight: 700, color: "#00f3ff", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {card ? card.email : "No active Codex account"}
        </span>
        <button
          onClick={() => void Close()}
          title="Đóng widget"
          style={{ ...noDrag, background: "none", border: "none", color: "#e6faff", opacity: 0.6, cursor: "pointer", fontSize: 14, padding: 0 }}
        >
          ×
        </button>
      </div>
      {card && (
        <>
          <Bar label="5H" percent={card.primaryPercent} summary={card.primarySummary} />
          <Bar label="WEEKLY" percent={card.secondaryPercent} summary={card.secondarySummary} />
          {card.lastError && card.lastError !== "-" && (
            <div style={{ marginTop: 6, fontSize: 10, color: "#ff3b5c" }}>{card.lastError}</div>
          )}
        </>
      )}
    </div>
  );
}
