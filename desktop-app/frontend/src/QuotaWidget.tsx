import { useEffect, useRef, useState } from "react";
import { SkipForward, X } from "lucide-react";
import { Close, GetOpacity, GetStatus, GetSwitchResult, SetOpacity, SwitchNext } from "../wailsjs/go/main/Widget";
import { WindowSetSize } from "../wailsjs/runtime/runtime";

type Status = Awaited<ReturnType<typeof GetStatus>>;

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
  const hasData = Boolean(summary) && summary !== "-";
  return (
    <div style={{ marginTop: 7 }}>
      <div style={{ display: "flex", justifyContent: "space-between", fontSize: 11 }}>
        <span style={{ opacity: 0.75 }}>{label}</span>
        <span>
          <b style={{ color: hasData ? barColor(percent) : "#999" }}>{hasData ? `${percent}%` : "-"}</b>
          {hasData && resetText(summary) && <span style={{ opacity: 0.75 }}> · reset {resetText(summary)}</span>}
        </span>
      </div>
      <div style={{ height: 5, marginTop: 3, borderRadius: 3, background: "rgba(255,255,255,0.15)" }}>
        <div style={{ width: `${hasData ? percent : 0}%`, height: "100%", borderRadius: 3, background: barColor(percent) }} />
      </div>
    </div>
  );
}

const iconButton: React.CSSProperties = {
  ...noDrag,
  display: "flex",
  background: "none",
  border: "none",
  color: "#e6faff",
  cursor: "pointer",
  padding: 2,
};

export default function QuotaWidget() {
  const [status, setStatus] = useState<Status | null>(null);
  const [switching, setSwitching] = useState(false);
  const [notice, setNotice] = useState("");
  const boxRef = useRef<HTMLDivElement>(null);
  const opacityRef = useRef(0.85);

  const load = () => void GetStatus().then(setStatus).catch(() => undefined);

  useEffect(() => {
    document.documentElement.style.background = "#080a10";
    document.body.style.background = "#080a10";
    document.body.classList.add("widget-mode");
    void GetOpacity().then((v) => (opacityRef.current = v));
    load();
    const timer = window.setInterval(load, 5000);
    return () => window.clearInterval(timer);
  }, []);

  // Keep the window exactly as tall as the content; the user only drags the width.
  useEffect(() => {
    const el = boxRef.current;
    if (!el) return;
    const observer = new ResizeObserver(() => {
      const height = Math.ceil(el.getBoundingClientRect().height);
      if (Math.abs(window.innerHeight - height) > 1) WindowSetSize(window.innerWidth, height);
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  async function switchNext() {
    if (switching) return;
    setSwitching(true);
    setNotice("");
    const requestedAt = await SwitchNext();
    let message = "";
    for (let i = 0; i < 120 && !message; i++) {
      await new Promise((r) => setTimeout(r, 500));
      message = await GetSwitchResult(requestedAt);
    }
    load();
    setSwitching(false);
    setNotice(message || "Hết thời gian chờ, app chính có đang chạy không?");
    window.setTimeout(() => setNotice(""), 6000);
  }

  // Mouse wheel over the widget: up = more solid, down = more see-through.
  function onWheel(e: React.WheelEvent) {
    const next = Math.min(1, Math.max(0.3, opacityRef.current + (e.deltaY < 0 ? 0.05 : -0.05)));
    opacityRef.current = next;
    void SetOpacity(next);
  }

  const active = status?.active;

  return (
    <div
      ref={boxRef}
      onWheel={onWheel}
      title="Lăn chuột để chỉnh độ trong suốt"
      style={{
        ...drag,
        position: "relative",
        padding: "9px 11px 10px",
        color: "#e6faff",
        fontFamily: "'JetBrains Mono', Consolas, monospace",
        background: "#080a10",
        border: "1px solid rgba(0, 243, 255, 0.4)",
        borderRadius: 8,
        cursor: "move",
        userSelect: "none",
        textShadow: "0 1px 2px rgba(0,0,0,0.9)",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
        <span style={{ flex: 1, minWidth: 0, fontSize: 12, fontWeight: 700, color: "#00f3ff", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {active ? active.email : "Chưa có acc Codex active"}
        </span>
        <button onClick={() => void switchNext()} disabled={switching} title="Chuyển sang acc còn nhiều quota nhất" style={iconButton}>
          <SkipForward size={14} />
        </button>
        <button onClick={() => void Close()} title="Đóng widget" style={{ ...iconButton, opacity: 0.7 }}>
          <X size={14} />
        </button>
      </div>

      {status && (
        <div style={{ marginTop: 3, fontSize: 10, opacity: 0.85 }}>
          {status.total} acc · <span style={{ color: "#00ff9d" }}>{status.available} còn quota</span> ·{" "}
          <span style={{ color: "#ff3b5c" }}>{status.out} hết</span>
        </div>
      )}

      {active && (
        <>
          <Bar label="5H" percent={active.primaryPercent} summary={active.primarySummary} />
          <Bar label="WEEKLY" percent={active.secondaryPercent} summary={active.secondarySummary} />
          {active.lastError && active.lastError !== "-" && (
            <div style={{ marginTop: 5, fontSize: 10, color: "#ff3b5c" }}>{active.lastError}</div>
          )}
        </>
      )}

      {notice && <div style={{ marginTop: 6, fontSize: 10, color: "#ffd166" }}>{notice}</div>}

      {switching && (
        <div
          style={{
            ...noDrag,
            position: "absolute",
            inset: 0,
            borderRadius: 8,
            background: "rgba(8, 10, 16, 0.9)",
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            justifyContent: "center",
            gap: 6,
            fontSize: 11,
            color: "#00f3ff",
          }}
        >
          <SkipForward size={16} className="loading-spinner" />
          Đang tìm acc còn quota...
        </div>
      )}
    </div>
  );
}
