import { useEffect, useMemo, useState } from "react";
import codexLogo from "./assets/provider-codex.svg";
import claudeLogo from "./assets/provider-claude.svg";
import kimiLogo from "./assets/provider-kimi.svg";
import {
  ActivateProfile,
  AddAccount,
  GetConfig,
  GetInitialSnapshot,
  GetSnapshot,
  HideToTray,
  LogoutProfile,
  RefreshSnapshot,
  SetAutoRotateCodex,
  SetAutoRotateThreshold,
} from "../wailsjs/go/main/App";
import { WindowIsMinimised } from "../wailsjs/runtime/runtime";

type ProfileCard = {
  id: string;
  label: string;
  email: string;
  provider: string;
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
  const [providerFilter, setProviderFilter] = useState<string>("all");
  const [showAddModal, setShowAddModal] = useState<boolean>(false);
  const [showSettingsModal, setShowSettingsModal] = useState<boolean>(false);
  const [autoRotateEnabled, setAutoRotateEnabled] = useState<boolean>(false);
  const [autoRotateThreshold, setAutoRotateThreshold] = useState<number>(5);

  const profileCount = snapshot.profiles.length;

  useEffect(() => {
    void loadInitial();
    void loadConfig();
    const timer = window.setInterval(() => {
      void loadCurrent();
    }, 15000);
    return () => window.clearInterval(timer);
  }, []);

  async function loadConfig() {
    try {
      const config = await GetConfig();
      // Wails bindings use snake_case for Go struct fields
      const enabled = (config as any).auto_rotate_codex;
      const threshold = (config as any).auto_rotate_threshold;
      if (typeof enabled === "boolean") {
        setAutoRotateEnabled(enabled);
      }
      if (typeof threshold === "number") {
        setAutoRotateThreshold(threshold);
      }
    } catch {
      // ignore config loading errors
    }
  }

  async function toggleAutoRotate(value: boolean) {
    setAutoRotateEnabled(value);
    try {
      await SetAutoRotateCodex(value);
    } catch {
      // ignore
    }
  }

  async function updateThreshold(value: number) {
    setAutoRotateThreshold(value);
    try {
      await SetAutoRotateThreshold(value);
    } catch {
      // ignore
    }
  }

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
    setShowAddModal(false);
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

  async function onAdd(provider: string) {
    const result = await AddAccount(provider);
    applyAction(result);
  }

  const providerOptions = useMemo(() => {
    const values = Array.from(new Set(snapshot.profiles.map((profile) => profile.provider || "unknown")));
    return values.sort((left, right) => left.localeCompare(right));
  }, [snapshot.profiles]);

  useEffect(() => {
    if (providerFilter !== "all" && !providerOptions.includes(providerFilter)) {
      setProviderFilter("all");
    }
  }, [providerFilter, providerOptions]);

  const sortedProfiles = useMemo(() => {
    if (providerFilter === "all") {
      return snapshot.profiles;
    }
    return snapshot.profiles.filter((profile) => (profile.provider || "unknown") === providerFilter);
  }, [providerFilter, snapshot.profiles]);

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
          <label className="provider-filter" title="Filter by provider">
            <select
              aria-label="Filter by provider"
              value={providerFilter}
              onChange={(event) => setProviderFilter(event.target.value)}
            >
              <option value="all">All providers</option>
              {providerOptions.map((provider) => (
                <option key={provider} value={provider}>
                  {providerLabel(provider)}
                </option>
              ))}
            </select>
          </label>
          <button className="ghost-btn" onClick={() => void refresh()}>
            Refresh now
          </button>
          <button className="settings-btn" title="Settings" onClick={() => setShowSettingsModal(true)} aria-label="Open settings">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="3" />
              <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
            </svg>
          </button>
          <button className="solid-btn" onClick={() => setShowAddModal(true)}>
            Add account
          </button>
        </div>
      </header>

      <main className="dashboard-grid">
        {sortedProfiles.map((profile) => (
          <article
            key={profile.id}
            className={`account-card ${profile.isActive ? `active-card active-card-${(profile.provider || "unknown").toLowerCase()}` : ""}`}
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
              <div className="provider-mark provider-mark-footer" title={providerLabel(profile.provider)}>
                <img src={providerLogo(profile.provider)} alt={providerLabel(profile.provider)} />
                <span>{providerLabel(profile.provider)}</span>
              </div>
            </footer>
          </article>
        ))}
      </main>

      {showAddModal ? (
        <div className="modal-backdrop" onClick={() => setShowAddModal(false)}>
          <div className="modal-card" onClick={(event) => event.stopPropagation()}>
            <div className="modal-header">
              <div>
                <h3>Add account</h3>
                <p>Choose the provider to start a login flow in a separate console window.</p>
              </div>
              <button className="modal-close" onClick={() => setShowAddModal(false)} aria-label="Close add account dialog">
                ×
              </button>
            </div>
            <div className="provider-choice-grid">
              <button className="provider-choice provider-choice-codex" onClick={() => void onAdd("codex")}>
                <img src={codexLogo} alt="Codex" />
                <strong>Codex</strong>
                <span>Open a Codex login console</span>
              </button>
              <button className="provider-choice provider-choice-claude" onClick={() => void onAdd("claude")}>
                <img src={claudeLogo} alt="Claude" />
                <strong>Claude</strong>
                <span>Open a Claude login console</span>
              </button>
              <button className="provider-choice provider-choice-kimi" onClick={() => void onAdd("kimi")}>
                <img src={kimiLogo} alt="Kimi" />
                <strong>Kimi</strong>
                <span>Open a Kimi login console</span>
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {showSettingsModal ? (
        <div className="modal-backdrop" onClick={() => setShowSettingsModal(false)}>
          <div className="modal-card" onClick={(event) => event.stopPropagation()}>
            <div className="modal-header">
              <div>
                <h3>Settings</h3>
                <p>Configure auto-rotate and switching behavior.</p>
              </div>
              <button className="modal-close" onClick={() => setShowSettingsModal(false)} aria-label="Close settings dialog">
                ×
              </button>
            </div>
            <div className="settings-body">
              <div className="settings-row">
                <div className="settings-label">
                  <strong>Auto-rotate Codex accounts (5H-First)</strong>
                  <span>Automatically switch to another cached account when the active one drops below the threshold.</span>
                </div>
                <label className="toggle-switch">
                  <input
                    type="checkbox"
                    checked={autoRotateEnabled}
                    onChange={(e) => void toggleAutoRotate(e.target.checked)}
                  />
                  <span className="toggle-slider" />
                </label>
              </div>
              <div className="settings-row">
                <div className="settings-label">
                  <strong>Switch threshold (%)</strong>
                  <span>Percentage of remaining 5H quota that triggers a switch.</span>
                </div>
                <div className="slider-control">
                  <input
                    type="range"
                    min={1}
                    max={20}
                    value={autoRotateThreshold}
                    onChange={(e) => void updateThreshold(Number(e.target.value))}
                  />
                  <span className="slider-value">{autoRotateThreshold}%</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function providerLabel(provider: string) {
  const normalized = (provider || "").trim().toLowerCase();
  switch (normalized) {
    case "codex":
      return "Codex";
    case "claude":
      return "Claude";
    case "kimi":
      return "Kimi";
    default:
      return normalized ? normalized.charAt(0).toUpperCase() + normalized.slice(1) : "Unknown";
  }
}

function providerLogo(provider: string) {
  const normalized = (provider || "").trim().toLowerCase();
  switch (normalized) {
    case "claude":
      return claudeLogo;
    case "kimi":
      return kimiLogo;
    case "codex":
    default:
      return codexLogo;
  }
}

export default App;
