import { useEffect, useMemo, useState } from "react";
import codexLogo from "./assets/provider-codex.svg";
import claudeLogo from "./assets/provider-claude.svg";
import kimiLogo from "./assets/provider-kimi.svg";
import {
  Activity,
  Plus,
  RefreshCw,
  Settings,
  Trash2,
  LayoutDashboard,
  ShieldCheck,
  Cpu,
  X
} from "lucide-react";
import {
  ActivateProfile,
  AddAccount,
  GetConfig,
  GetInitialSnapshot,
  GetSnapshot,
  GetSystemStatus,
  LogoutProfile,
  OpenCodexInstallPage,
  RefreshSnapshot,
  SetAutoRotateCodex,
  SetAutoRotateThreshold,
} from "../wailsjs/go/main/App";
import clsx from "clsx";

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

type SystemStatus = {
  hasCodexCli: boolean;
  codexInstallUrl: string;
};

function App() {
  const [snapshot, setSnapshot] = useState<Snapshot>({ generatedAt: "", profiles: [] });
  const [busyProfile, setBusyProfile] = useState<string>("");
  const [statusText, setStatusText] = useState<string>("SYSTEM_READY");
  const [providerFilter, setProviderFilter] = useState<string>("all");
  const [showAddModal, setShowAddModal] = useState<boolean>(false);
  const [showSettingsModal, setShowSettingsModal] = useState<boolean>(false);
  const [autoRotateEnabled, setAutoRotateEnabled] = useState<boolean>(false);
  const [autoRotateThreshold, setAutoRotateThreshold] = useState<number>(5);
  const [systemStatus, setSystemStatus] = useState<SystemStatus>({ hasCodexCli: true, codexInstallUrl: "https://github.com/openai/codex" });

  useEffect(() => {
    void loadInitial();
    void loadConfig();
    void loadSystemStatus();
  }, []);

  useEffect(() => {
    if (!systemStatus.hasCodexCli) return;
    const timer = window.setInterval(() => {
      void loadCurrent();
    }, 15000);
    return () => window.clearInterval(timer);
  }, [systemStatus.hasCodexCli]);

  async function loadSystemStatus() {
    try {
      const next = await GetSystemStatus();
      setSystemStatus(next);
      setStatusText(next.hasCodexCli ? "SYSTEM_READY" : "CODEX_REQUIRED");
    } catch {
      setSystemStatus((current) => ({ ...current, hasCodexCli: false }));
      setStatusText("SYSTEM_CHECK_FAILED");
    }
  }

  async function openCodexInstallPage() {
    try {
      await OpenCodexInstallPage();
    } catch {
      setStatusText("OPEN_INSTALL_LINK_FAILED");
    }
  }

  async function loadConfig() {
    try {
      const config = await GetConfig();
      const enabled = (config as any).auto_rotate_codex;
      const threshold = (config as any).auto_rotate_threshold;
      if (typeof enabled === "boolean") setAutoRotateEnabled(enabled);
      if (typeof threshold === "number") setAutoRotateThreshold(threshold);
    } catch {}
  }

  async function loadInitial() {
    const initial = await GetInitialSnapshot();
    setSnapshot(initial);
  }

  async function loadCurrent() {
    if (!systemStatus.hasCodexCli) return;
    const current = await GetSnapshot();
    setSnapshot(current);
  }

  async function refresh() {
    setStatusText("SYNCHRONIZING...");
    const result = await RefreshSnapshot();
    applyAction(result);
  }

  function applyAction(result: ActionResponse) {
    setSnapshot(result.snapshot);
    setStatusText(result.error ? `ERROR: ${result.error}` : `CORE_LOADED: ${result.snapshot.generatedAt}`);
    setBusyProfile("");
    setShowAddModal(false);
  }

  async function onActivate(profileId: string) {
    setBusyProfile(profileId);
    const result = await ActivateProfile(profileId);
    applyAction(result);
  }

  async function onDelete(profileId: string, label: string) {
    if (!window.confirm(`CONFIRM WIPE FOR ${label.toUpperCase()}?`)) return;
    setBusyProfile(profileId);
    const result = await LogoutProfile(profileId);
    applyAction(result);
  }

  async function onAdd(provider: string) {
    const result = await AddAccount(provider);
    applyAction(result);
  }

  const providerOptions = useMemo(() => {
    return Array.from(new Set(snapshot.profiles.map((p) => p.provider || "unknown"))).sort();
  }, [snapshot.profiles]);

  const sortedProfiles = useMemo(() => {
    if (providerFilter === "all") return snapshot.profiles;
    return snapshot.profiles.filter((p) => (p.provider || "unknown") === providerFilter);
  }, [providerFilter, snapshot.profiles]);

  useEffect(() => {
    if (providerFilter !== "all" && !providerOptions.includes(providerFilter)) {
      setProviderFilter("all");
    }
  }, [providerFilter, providerOptions]);

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="sidebar-logo">
          <Cpu size={24} className="text-neon" />
          <h1>CODEX // CORE</h1>
        </div>

        <nav className="nav-group">
          <span className="nav-label">Modules</span>
          <button 
            onClick={() => setProviderFilter('all')}
            className={clsx("cyber-btn w-full flex items-center gap-2 mb-2", providerFilter === 'all' && 'cyber-btn-solid')}
          >
            <LayoutDashboard size={14} /> Main Dashboard
          </button>
        </nav>

        <nav className="nav-group">
          <span className="nav-label">Source Filter</span>
          <div className="space-y-2">
            {['all', ...providerOptions].map(p => (
              <button
                key={p}
                onClick={() => setProviderFilter(p)}
                className={clsx(
                  "cyber-btn w-full flex items-center justify-between text-[10px]",
                  providerFilter === p && "cyber-btn-solid"
                )}
              >
                <span>{p.toUpperCase()}</span>
                {p !== 'all' && <img src={getProviderLogo(p)} className="w-3 h-3 grayscale brightness-200" />}
              </button>
            ))}
          </div>
        </nav>

        <div className="mt-auto">
          <button onClick={() => setShowSettingsModal(true)} className="cyber-btn w-full flex items-center gap-2">
            <Settings size={14} /> Core Settings
          </button>
        </div>
      </aside>

      <main className="main-content">
        <header className="top-nav">
          <div className="status-bar">
            <div className="status-dot" />
            <span>{statusText}</span>
          </div>

          <div className="flex gap-4">
            <button onClick={() => void refresh()} className="cyber-btn flex items-center gap-2">
              <RefreshCw size={14} /> Sync All
            </button>
            <button onClick={() => setShowAddModal(true)} className="cyber-btn cyber-btn-solid flex items-center gap-2">
              <Plus size={14} /> New Link
            </button>
          </div>
        </header>

        <div className="dashboard-grid">
          {sortedProfiles.map((profile) => (
            <article 
              key={profile.id} 
              className={clsx(
                "account-card", 
                profile.isActive && `active active-${profile.provider.toLowerCase()}`
              )}
            >
              <div className="flex justify-between items-start mb-4">
                <div className="min-w-0 flex-1">
                  <h3 className="card-title truncate" title={profile.label}>{profile.label.toUpperCase()}</h3>
                  <p className="card-email truncate" title={profile.email}>{profile.email}</p>
                </div>
                <div className="flex flex-col items-end gap-2">
                  <div className="provider-badge">
                    <img src={getProviderLogo(profile.provider)} className="provider-icon-mini" />
                    <span className="text-[9px]">{profile.provider.toUpperCase()}</span>
                  </div>
                </div>
              </div>

              <div className="space-y-5">
                <div className="meter-block">
                  <div className="meter-label">
                    <span>Quota: 5H</span>
                    <span className="text-neon">{profile.primarySummary}</span>
                  </div>
                  <div className="meter-track">
                    <div 
                      className={clsx("meter-fill", meterTone(profile.primaryPercent))}
                      style={{ width: `${profile.primaryPercent}%` }}
                    />
                  </div>
                </div>

                <div className="meter-block">
                  <div className="meter-label">
                    <span>Quota: WEEKLY</span>
                    <span className="text-neon">{profile.secondarySummary}</span>
                  </div>
                  <div className="meter-track">
                    <div 
                      className={clsx("meter-fill", meterTone(profile.secondaryPercent))}
                      style={{ width: `${profile.secondaryPercent}%` }}
                    />
                  </div>
                </div>
              </div>

              <div className="flex justify-between items-center mt-6 pt-4 border-t border-dashed border-[rgba(0,243,255,0.1)]">
                <span className={clsx("text-[9px] px-2 py-0.5 rounded", badgeClass(profile.authStatus))}>
                  {profile.authStatus.replace('_', ' ')}
                </span>
                <div className="flex gap-2">
                  {profile.canLoginFromCache && (
                    <button 
                      onClick={() => void onActivate(profile.id)}
                      className="cyber-btn p-1.5"
                      disabled={busyProfile === profile.id}
                      title="RE-AUTHENTICATE"
                    >
                      <ShieldCheck size={14} />
                    </button>
                  )}
                  <button 
                    onClick={() => void onDelete(profile.id, profile.label)}
                    className="cyber-btn cyber-btn-danger p-1.5"
                    disabled={busyProfile === profile.id}
                    title="TERMINATE"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>
            </article>
          ))}
        </div>
      </main>

      {showAddModal && (
        <div className="modal-overlay" onClick={() => setShowAddModal(false)}>
          <div className="modal-content" onClick={e => e.stopPropagation()}>
            <div className="flex justify-between items-center mb-6">
              <h2 className="text-neon text-lg font-bold">ESTABLISH_UPLINK</h2>
              <button onClick={() => setShowAddModal(false)}><X size={20} /></button>
            </div>
            <div className="grid grid-cols-1 gap-4">
              {['codex', 'claude', 'kimi'].map(p => (
                <button 
                  key={p} 
                  onClick={() => void onAdd(p)}
                  className="cyber-btn flex items-center justify-between group py-4"
                >
                  <div className="flex items-center gap-3">
                    <img src={getProviderLogo(p)} className="w-6 h-6" />
                    <span className="font-bold">{p.toUpperCase()}</span>
                  </div>
                  <span className="text-[10px] opacity-50 group-hover:opacity-100">EXEC_LOGIN.SH _</span>
                </button>
              ))}
            </div>
          </div>
        </div>
      )}

      {showSettingsModal && (
        <div className="modal-overlay" onClick={() => setShowSettingsModal(false)}>
          <div className="modal-content" onClick={e => e.stopPropagation()}>
             <div className="flex justify-between items-center mb-6">
              <h2 className="text-neon text-lg font-bold">CORE_CONFIGURATION</h2>
              <button onClick={() => setShowSettingsModal(false)}><X size={20} /></button>
            </div>
            <div className="space-y-8">
              <div className="flex justify-between items-center bg-[rgba(0,243,255,0.05)] p-4 border border-[rgba(0,243,255,0.1)]">
                <div>
                  <div className="font-bold text-sm">AUTO_ROTATION_PROTOCOL</div>
                  <div className="text-[10px] text-dim">Automatic account switching on exhaustion</div>
                </div>
                <label className="relative inline-flex items-center cursor-pointer">
                  <input 
                    type="checkbox" 
                    className="sr-only peer"
                    checked={autoRotateEnabled} 
                    onChange={async e => {
                      const next = e.target.checked;
                      setAutoRotateEnabled(next);
                      try {
                        await SetAutoRotateCodex(next);
                      } catch {
                        setAutoRotateEnabled(!next);
                        setStatusText("ERROR: SAVE FAILED");
                      }
                    }}
                  />
                  <div className="w-11 h-6 bg-gray-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-neon-cyan"></div>
                </label>
              </div>
              <div className="bg-[rgba(0,243,255,0.05)] p-4 border border-[rgba(0,243,255,0.1)]">
                <div className="flex justify-between mb-4">
                  <span className="font-bold text-sm">SWITCH_THRESHOLD</span>
                  <span className="text-neon">{autoRotateThreshold}%</span>
                </div>
                <input 
                  type="range" 
                  min={1} max={20} 
                  className="w-full h-1 bg-gray-700 rounded-lg appearance-none cursor-pointer accent-neon-cyan"
                  value={autoRotateThreshold}
                  onChange={async e => {
                    const next = Number(e.target.value);
                    setAutoRotateThreshold(next);
                    try {
                      await SetAutoRotateThreshold(next);
                    } catch {
                      void loadConfig();
                      setStatusText("ERROR: SAVE FAILED");
                    }
                  }}
                />
                <div className="flex justify-between text-[8px] text-dim mt-2">
                  <span>1%</span>
                  <span>10%</span>
                  <span>20%</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      )}

      {!systemStatus.hasCodexCli && (
        <div className="modal-overlay modal-overlay-blocking">
          <div className="modal-content prerequisite-modal">
            <div className="prerequisite-label">SYSTEM REQUIREMENT</div>
            <h2 className="prerequisite-title">INSTALL CODEX FIRST</h2>
            <p className="prerequisite-copy">
              Codex CLI is not installed on this machine yet. This app currently depends on Codex for login and the main runtime flow.
            </p>
            <p className="prerequisite-copy prerequisite-copy-dim">
              Install Codex, then return here and click re-check.
            </p>
            <div className="prerequisite-actions">
              <button onClick={() => void openCodexInstallPage()} className="cyber-btn cyber-btn-solid">
                Download Codex
              </button>
              <button onClick={() => void loadSystemStatus()} className="cyber-btn">
                Re-check
              </button>
            </div>
            {systemStatus.codexInstallUrl && (
              <div className="prerequisite-link">{systemStatus.codexInstallUrl}</div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

const getProviderLogo = (p: string) => {
  switch (p.toLowerCase()) {
    case 'codex': return codexLogo;
    case 'claude': return claudeLogo;
    case 'kimi': return kimiLogo;
    default: return codexLogo;
  }
};

const badgeClass = (status: string) => {
  switch (status.toLowerCase()) {
    case 'active': return 'bg-[rgba(0,255,159,0.1)] text-neon-green border border-[rgba(0,255,159,0.2)]';
    case 'error': return 'bg-[rgba(255,0,85,0.1)] text-neon-pink border border-[rgba(255,0,85,0.2)]';
    default: return 'bg-[rgba(128,128,144,0.1)] text-text-dim border border-[rgba(128,128,144,0.2)]';
  }
};

const meterTone = (percent: number) => {
  if (percent <= 20) return "danger";
  if (percent <= 40) return "warning";
  return "healthy";
};

export default App;
