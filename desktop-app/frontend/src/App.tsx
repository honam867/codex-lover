import { useEffect, useMemo, useState } from "react";
import {
  ActivateProfile,
  AddAccount,
  GetInitialSnapshot,
  GetSnapshot,
  HideToTray,
  LogoutProfile,
  RefreshSnapshot,
} from "../wailsjs/go/main/App";
import { WindowIsMinimised } from "../wailsjs/runtime/runtime";

type ProfileCard = {
  id: string;
  label: string;
  email: string;
  plan: string;
  authStatus: string;
  freshness: string;
  isActive: boolean;
  primaryPercent: number;
  primarySummary: string;
  secondaryPercent: number;
  secondarySummary: string;
  lastError: string;
  canLoginFromCache: boolean;
  lastRefreshedAtText: string;
};

type Snapshot = {
  generatedAt: string;
  profiles: ProfileCard[];
};

type ActionResponse = {
  message: string;
  error?: string;
  snapshot: Snapshot;
};

const badgeTone = (kind: string) => {
  switch (kind.toLowerCase()) {
    case "active":
    case "fresh":
      return "green";
    case "logged_out":
    case "cached":
      return "amber";
    case "error":
      return "red";
    default:
      return "slate";
  }
};

const meterTone = (percent: number) => {
  if (percent <= 20) {
    return "danger";
  }
  if (percent <= 40) {
    return "warning";
  }
  return "healthy";
};

function App() {
  const [snapshot, setSnapshot] = useState<Snapshot>({ generatedAt: "", profiles: [] });
  const [busyProfile, setBusyProfile] = useState<string>("");
  const [statusText, setStatusText] = useState<string>("Loading...");

  const profileCount = snapshot.profiles.length;

  useEffect(() => {
    void loadInitial();
    const timer = window.setInterval(() => {
      void loadCurrent();
    }, 15000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    let cancelled = false;
    let inFlight = false;

    const timer = window.setInterval(async () => {
      if (cancelled || inFlight) {
        return;
      }
      inFlight = true;
      try {
        const minimised = await WindowIsMinimised();
        if (minimised) {
          await HideToTray();
        }
      } catch {
        // ignore minimise polling errors
      } finally {
        inFlight = false;
      }
    }, 250);

    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, []);

  async function loadInitial() {
    const initial = await GetInitialSnapshot();
    setSnapshot(initial);
    setStatusText(`Last refresh: ${initial.generatedAt}   Accounts: ${initial.profiles.length}   Auto refresh: every 15s`);
  }

  async function loadCurrent() {
    const current = await GetSnapshot();
    setSnapshot(current);
    setStatusText(`Last refresh: ${current.generatedAt}   Accounts: ${current.profiles.length}   Auto refresh: every 15s`);
  }

  async function refresh() {
    const result = await RefreshSnapshot();
    applyAction(result);
  }

  function applyAction(result: ActionResponse) {
    setSnapshot(result.snapshot);
    setStatusText(result.error ? `${result.message}: ${result.error}` : result.message || `Last refresh: ${result.snapshot.generatedAt}`);
    setBusyProfile("");
  }

  async function onActivate(profileId: string) {
    setBusyProfile(profileId);
    const result = await ActivateProfile(profileId);
    applyAction(result);
  }

  async function onDelete(profileId: string, label: string) {
    const confirmed = window.confirm(`Delete account ${label}? This removes cached auth and local account data.`);
    if (!confirmed) return;
    setBusyProfile(profileId);
    const result = await LogoutProfile(profileId);
    applyAction(result);
  }

  async function onAdd() {
    const result = await AddAccount();
    applyAction(result);
  }

  const sortedProfiles = useMemo(() => snapshot.profiles, [snapshot]);

  return (
    <div className="app-shell">
      <div className="bg-orb orb-a" />
      <div className="bg-orb orb-b" />
      <header className="topbar">
        <div>
          <h1>Account Management</h1>
          <p>{statusText || `Accounts: ${profileCount}`}</p>
        </div>
        <div className="topbar-actions">
          <button className="ghost-btn" onClick={() => void refresh()}>
            Refresh now
          </button>
          <button className="solid-btn" onClick={() => void onAdd()}>
            Add account
          </button>
        </div>
      </header>

      <main className="dashboard-grid">
        {sortedProfiles.map((profile) => (
          <article
            key={profile.id}
            className={`account-card ${profile.isActive ? "active-card" : ""}`}
          >
            <div className="card-header">
              <div className="card-title-row">
                <h2 title={profile.label}>{profile.label}</h2>
              </div>
              <div className="badge-row">
                <span className="mini-badge plan-badge">{profile.plan}</span>
                <span className={`mini-badge ${badgeTone(profile.authStatus)}`}>{profile.authStatus}</span>
                <span className={`mini-badge ${badgeTone(profile.freshness)}`}>{profile.freshness}</span>
              </div>
            </div>

            <p className="email-line" title={profile.email}>{profile.email}</p>

            <div className="meter-block">
              <div className="meter-row">
                <div className="meter-label">5H</div>
                <div className="meter-summary" title={profile.primarySummary}>{profile.primarySummary}</div>
              </div>
              <div className="meter-track">
                <div
                  className={`meter-fill meter-${meterTone(profile.primaryPercent)}`}
                  style={{ width: `${profile.primaryPercent}%` }}
                />
              </div>
            </div>

            <div className="meter-block">
              <div className="meter-row">
                <div className="meter-label">WEEKLY</div>
                <div className="meter-summary" title={profile.secondarySummary}>{profile.secondarySummary}</div>
              </div>
              <div className="meter-track">
                <div
                  className={`meter-fill meter-${meterTone(profile.secondaryPercent)}`}
                  style={{ width: `${profile.secondaryPercent}%` }}
                />
              </div>
            </div>

            <div className="card-actions">
              {profile.canLoginFromCache ? (
                <button
                  className="ghost-btn small-btn"
                  disabled={busyProfile === profile.id}
                  onClick={() => void onActivate(profile.id)}
                >
                  Log in
                </button>
              ) : (
                <span className="placeholder-action" />
              )}
              <button
                className="danger-btn small-btn"
                disabled={busyProfile === profile.id}
                onClick={() => void onDelete(profile.id, profile.label)}
              >
                Delete
              </button>
            </div>

            <footer className="card-footer">
              <span>{profile.lastRefreshedAtText}</span>
            </footer>
          </article>
        ))}
      </main>
    </div>
  );
}

export default App;
