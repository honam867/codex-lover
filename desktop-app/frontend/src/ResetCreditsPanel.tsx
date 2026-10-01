import { useEffect, useRef, useState } from "react";
import clsx from "clsx";
import { GetResetCredits, UseResetCredit } from "../wailsjs/go/main/App";

type ResetCreditItem = {
  id: string;
  title: string;
  expiresText: string;
};

type ResetHistoryItem = {
  id: string;
  kind: string;
  text: string;
  atText: string;
};

type ResetCreditsView = {
  availableCount: number;
  credits: ResetCreditItem[];
  history: ResetHistoryItem[];
  historyError?: string;
  error?: string;
};

export type ResetActionResult = Awaited<ReturnType<typeof UseResetCredit>>;

type Props = {
  profileId: string;
  profileLabel: string;
  primaryPercent: number;
  secondaryPercent: number;
  hasSecondary: boolean;
  onAction: (result: ResetActionResult) => void;
};

type Notice = { text: string; isError: boolean };

export function ResetCreditsPanel({ profileId, profileLabel, primaryPercent, secondaryPercent, hasSecondary, onAction }: Props) {
  const [view, setView] = useState<ResetCreditsView | null>(null);
  const [loading, setLoading] = useState<boolean>(false);
  const [subTab, setSubTab] = useState<"available" | "history">("available");
  const [busyCreditId, setBusyCreditId] = useState<string>("");
  const [notice, setNotice] = useState<Notice | null>(null);
  const loadSeq = useRef<number>(0);

  async function load() {
    // Only the latest request may update state, so overlapping loads cannot
    // overwrite newer data or leak into another account after a switch.
    const seq = ++loadSeq.current;
    setLoading(true);
    try {
      const result = (await GetResetCredits(profileId)) as unknown as ResetCreditsView;
      if (seq === loadSeq.current) setView(result);
    } catch (err) {
      if (seq === loadSeq.current) setView({ availableCount: 0, credits: [], history: [], error: String(err) });
    } finally {
      if (seq === loadSeq.current) setLoading(false);
    }
  }

  useEffect(() => {
    setView(null);
    setNotice(null);
    setSubTab("available");
    void load();
  }, [profileId]);

  async function redeem(credit: ResetCreditItem) {
    const quota = hasSecondary
      ? `5H còn ${primaryPercent}% · WEEKLY còn ${secondaryPercent}%`
      : `5H còn ${primaryPercent}%`;
    const confirmed = window.confirm(
      `Dùng 1 lượt đặt lại cho ${profileLabel}?\n\n` +
        `${credit.title}\nQuota hiện tại: ${quota}\n\n` +
        "Thao tác này không hoàn tác được.",
    );
    if (!confirmed) return;

    setBusyCreditId(credit.id);
    setNotice(null);
    try {
      const result = await UseResetCredit(profileId, credit.id);
      setNotice({ text: result.error || result.message, isError: Boolean(result.error) });
      onAction(result);
      await load();
    } catch (err) {
      setNotice({ text: String(err), isError: true });
    } finally {
      setBusyCreditId("");
    }
  }

  const credits = view?.credits ?? [];
  const history = view?.history ?? [];

  return (
    <div className="reset-panel">
      <h3 className="reset-title">Đặt lại giới hạn sử dụng</h3>
      <p className="reset-sub">Dùng một lượt đặt lại để khôi phục giới hạn trong 5 giờ, giới hạn hằng tuần hoặc cả hai</p>

      {notice && <div className={clsx("reset-notice", notice.isError && "reset-notice-error")}>{notice.text}</div>}

      <div className="reset-box">
        <div className="reset-tabs">
          <button
            type="button"
            className={clsx("reset-tab", subTab === "available" && "reset-tab-active")}
            onClick={() => setSubTab("available")}
          >
            Có sẵn <span className="reset-tab-count">{view && !view.error ? view.availableCount : ""}</span>
          </button>
          <button
            type="button"
            className={clsx("reset-tab", subTab === "history" && "reset-tab-active")}
            onClick={() => setSubTab("history")}
          >
            Lịch sử
          </button>
        </div>

        {loading && !view && <div className="reset-empty">Đang tải...</div>}

        {view?.error && (
          <div className="reset-empty reset-error">
            <span>Không tải được lượt đặt lại: {view.error}</span>
            <button type="button" className="reset-use-btn" onClick={() => void load()} disabled={loading}>
              Thử lại
            </button>
          </div>
        )}

        {view && !view.error && subTab === "available" && (
          credits.length === 0 ? (
            <div className="reset-empty">Không có lượt đặt lại nào</div>
          ) : (
            credits.map((credit) => (
              <div key={credit.id} className="reset-row">
                <div className="reset-row-main">
                  <div className="reset-row-title">{credit.title}</div>
                  {credit.expiresText && <div className="reset-row-sub">{credit.expiresText}</div>}
                </div>
                <button
                  type="button"
                  className="reset-use-btn"
                  disabled={Boolean(busyCreditId)}
                  onClick={() => void redeem(credit)}
                >
                  {busyCreditId === credit.id ? "Đang đặt lại..." : "Dùng lượt đặt lại"}
                </button>
              </div>
            ))
          )
        )}

        {view && !view.error && subTab === "history" && (
          view.historyError ? (
            <div className="reset-empty reset-error">Không tải được lịch sử: {view.historyError}</div>
          ) : history.length === 0 ? (
            <div className="reset-empty">Chưa có lịch sử</div>
          ) : (
            history.map((item) => (
              <div key={item.id} className="reset-row">
                <div className="reset-row-main">
                  <div className="reset-row-title">{item.text}</div>
                  {item.atText && <div className="reset-row-sub">{item.atText}</div>}
                </div>
              </div>
            ))
          )
        )}
      </div>
    </div>
  );
}
