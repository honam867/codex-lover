import { useEffect, useMemo, useRef, useState } from "react";
import codexLogo from "./assets/provider-codex.svg";
import claudeLogo from "./assets/provider-claude.svg";
import kimiLogo from "./assets/provider-kimi.svg";
import {
  Activity,
  Ban,
  Check,
  Cpu,
  LayoutDashboard,
  Plus,
  RefreshCw,
  Settings,
  ShieldCheck,
  Trash2,
  X,
} from "lucide-react";
import {
  ActivateProfile,
  AddAccount,
  CheckProfileHealth,
  GetConfig,
  GetDeletionHistory,
  GetInitialSnapshot,
  GetSnapshot,
  GetSystemStatus,
  GetTriggerSettings,
  GetLastTriggerRun,
  LogoutProfile,
  OpenCodexInstallPage,
  PreviewTriggerSelection,
  RefreshSnapshot,
  SaveTriggerSettings,
  SetAutoRotateCodex,
  SetAutoRotateThreshold,
  SetProfileBlocked,
  TriggerNow,
  UpdateProfileMeta,
} from "../wailsjs/go/main/App";
import clsx from "clsx";

type ProfileCard = {
  id: string;
  label: string;
  email: string;
  provider: string;
  audience: string;
  plan: string;
  authStatus: string;
  freshness: string;
  isActive: boolean;
  blocked: boolean;
  primaryPercent: number;
  primarySummary: string;
  secondaryPercent: number;
  secondarySummary: string;
  lastError: string;
  canLoginFromCache: boolean;
  lastRefreshedAtText: string;
  createdAtText: string;
  lastTriggeredAtText: string;
  lastTriggeredModel: string;
  price: number;
  createdAtISO: string;
  healthStatus: string;
  healthMessage: string;
  healthCheckedAtText: string;
  endAtText: string;
  daysRemainingText: string;
  daysUsedText: string;
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

type TriggerMode = "all" | "top_n" | "custom";

type TriggerConfig = {
  enabled: boolean;
  time_of_day: string;
  mode: TriggerMode;
  count: number;
  profile_ids: string[];
  grace_minutes: number;
};

type TriggerAccountResult = {
  profile_id: string;
  label: string;
  status: string;
  model_used?: string;
  verified: boolean;
  error?: string;
};

type TriggerRun = {
  ran_at: string;
  manual: boolean;
  results: TriggerAccountResult[];
};

type DeletedAccountRecord = {
  profile_id: string;
  label: string;
  email?: string;
  provider: string;
  deleted_at: string;
};

const TIME_SLOTS: string[] = Array.from({ length: 48 }, (_, i) => {
  const h = String(Math.floor(i / 2)).padStart(2, "0");
  const m = i % 2 === 0 ? "00" : "30";
  return `${h}:${m}`;
});

const DEFAULT_TRIGGER: TriggerConfig = {
  enabled: false,
  time_of_day: "08:00",
  mode: "all",
  count: 2,
  profile_ids: [],
  grace_minutes: 60,
};

type AudienceFilter = "all" | "personal" | "customer";

function App() {
  const [snapshot, setSnapshot] = useState<Snapshot>({ generatedAt: "", profiles: [] });
  const [busyProfile, setBusyProfile] = useState<string>("");
  const [checkingHealth, setCheckingHealth] = useState<boolean>(false);
  const [statusText, setStatusText] = useState<string>("SYSTEM_READY");
  const [providerFilter, setProviderFilter] = useState<string>("all");
  const [audienceFilter, setAudienceFilter] = useState<AudienceFilter>("all");
  const [showAddModal, setShowAddModal] = useState<boolean>(false);
  const [showSettingsModal, setShowSettingsModal] = useState<boolean>(false);
  const [autoRotateEnabled, setAutoRotateEnabled] = useState<boolean>(false);
  const [autoRotateThreshold, setAutoRotateThreshold] = useState<number>(5);
  const [systemStatus, setSystemStatus] = useState<SystemStatus>({ hasCodexCli: true, codexInstallUrl: "https://github.com/openai/codex" });
  const [trigger, setTrigger] = useState<TriggerConfig>(DEFAULT_TRIGGER);
  const [lastRun, setLastRun] = useState<TriggerRun | null>(null);
  const [topNPreview, setTopNPreview] = useState<string[]>([]);
  const [deletionHistory, setDeletionHistory] = useState<DeletedAccountRecord[]>([]);
  const [editProfile, setEditProfile] = useState<ProfileCard | null>(null);
  const [editDate, setEditDate] = useState<string>("");
  const [editPrice, setEditPrice] = useState<number>(0);
  const [editAudience, setEditAudience] = useState<string>("personal");
  const [pricePrompt, setPricePrompt] = useState<ProfileCard | null>(null);
  const [promptPrice, setPromptPrice] = useState<number>(0);
  const [promptAudience, setPromptAudience] = useState<string>("personal");
  const seenCodexIds = useRef<Set<string>>(new Set());
  const seededCodexIds = useRef<boolean>(false);

  useEffect(() => {
    if (!seededCodexIds.current) return;
    const codex = snapshot.profiles.filter((p) => p.provider.toLowerCase() === "codex");
    const fresh = codex.find((p) => !seenCodexIds.current.has(p.id));
    if (fresh) {
      codex.forEach((p) => seenCodexIds.current.add(p.id));
      if (fresh.price === 0 && !pricePrompt) {
        setPricePrompt(fresh);
        setPromptPrice(0);
        setPromptAudience(normalizeAudience(fresh.audience));
      }
    }
  }, [snapshot.profiles, pricePrompt]);

  useEffect(() => {
    void loadInitial();
    void loadConfig();
    void loadSystemStatus();
    void loadTrigger();
  }, []);

  useEffect(() => {
    if (!systemStatus.hasCodexCli) return;
    const timer = window.setInterval(() => {
      void loadCurrent();
    }, 15000);
    return () => window.clearInterval(timer);
  }, [systemStatus.hasCodexCli]);

  useEffect(() => {
    if (!showSettingsModal) return;
    void (async () => {
      try {
        const h = (await GetDeletionHistory()) as unknown as DeletedAccountRecord[];
        setDeletionHistory(h ?? []);
      } catch {
        setDeletionHistory([]);
      }
    })();
  }, [showSettingsModal]);

  useEffect(() => {
    if (!trigger.enabled || trigger.mode !== "top_n") {
      setTopNPreview([]);
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const preview = (await PreviewTriggerSelection(trigger as any)) as unknown as { selectedIds: string[] };
        if (!cancelled) setTopNPreview(preview.selectedIds ?? []);
      } catch {
        if (!cancelled) setTopNPreview([]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [trigger.enabled, trigger.mode, trigger.count, snapshot.profiles]);

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

  async function loadTrigger() {
    try {
      const t = (await GetTriggerSettings()) as unknown as TriggerConfig;
      setTrigger({ ...DEFAULT_TRIGGER, ...t, profile_ids: t.profile_ids ?? [] });
    } catch {}
    try {
      const run = (await GetLastTriggerRun()) as unknown as TriggerRun | null;
      setLastRun(run);
    } catch {}
  }

  async function saveTrigger(next: TriggerConfig) {
    setTrigger(next);
    try {
      await SaveTriggerSettings(next as any);
    } catch {
      setStatusText("ERROR: SAVE FAILED");
    }
  }

  async function onTriggerNow() {
    setStatusText("TRIGGERING...");
    try {
      const result = await TriggerNow();
      applyAction(result);
      const run = (await GetLastTriggerRun()) as unknown as TriggerRun | null;
      setLastRun(run);
    } catch {
      setStatusText("ERROR: TRIGGER FAILED");
    }
  }

  function toggleCustomProfile(id: string) {
    const has = trigger.profile_ids.includes(id);
    const profile_ids = has
      ? trigger.profile_ids.filter((p) => p !== id)
      : [...trigger.profile_ids, id];
    void saveTrigger({ ...trigger, profile_ids });
  }

  async function loadInitial() {
    const initial = await GetInitialSnapshot();
    initial.profiles
      .filter((p) => p.provider.toLowerCase() === "codex")
      .forEach((p) => seenCodexIds.current.add(p.id));
    seededCodexIds.current = true;
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

  async function checkHealth() {
    if (checkingHealth) return;
    const confirmed = window.confirm(
      "Check trạng thái sẽ gửi request Codex tối giản bằng cached auth cho từng account Codex. " +
        "Request này có thể refresh token và có thể mở/ảnh hưởng quota window. Tiếp tục?"
    );
    if (!confirmed) return;
    setStatusText("CHECKING_ACCOUNT_STATUS...");
    setCheckingHealth(true);
    try {
      const result = await CheckProfileHealth();
      applyAction(result);
    } finally {
      setCheckingHealth(false);
    }
  }

  function applyAction(result: ActionResponse) {
    setSnapshot(result.snapshot);
    setStatusText(result.error ? `ERROR: ${result.error}` : result.message || `CORE_LOADED: ${result.snapshot.generatedAt}`);
    setBusyProfile("");
    setShowAddModal(false);
  }

  async function onActivate(profileId: string) {
    setBusyProfile(profileId);
    const result = await ActivateProfile(profileId);
    applyAction(result);
  }

  async function onSetBlocked(profileId: string, blocked: boolean) {
    setBusyProfile(profileId);
    const result = await SetProfileBlocked(profileId, blocked);
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

  function openEdit(profile: ProfileCard) {
    setEditProfile(profile);
    setEditDate(profile.createdAtISO || "");
    setEditPrice(profile.price || 0);
    setEditAudience(normalizeAudience(profile.audience));
  }

  async function saveEdit() {
    if (!editProfile) return;
    const result = await UpdateProfileMeta(editProfile.id, editDate, editPrice, editAudience);
    setEditProfile(null);
    applyAction(result);
  }

  async function savePrice() {
    if (!pricePrompt) return;
    const result = await UpdateProfileMeta(pricePrompt.id, pricePrompt.createdAtISO || "", promptPrice, promptAudience);
    setPricePrompt(null);
    applyAction(result);
  }

  const providerOptions = useMemo(() => {
    return Array.from(new Set(snapshot.profiles.map((p) => p.provider || "unknown"))).sort();
  }, [snapshot.profiles]);

  const sortedProfiles = useMemo(() => {
    return snapshot.profiles.filter((p) => {
      if (providerFilter !== "all" && (p.provider || "unknown") !== providerFilter) return false;
      if (audienceFilter !== "all" && p.provider.toLowerCase() === "codex") {
        return normalizeAudience(p.audience) === audienceFilter;
      }
      return true;
    });
  }, [audienceFilter, providerFilter, snapshot.profiles]);

  const codexProfiles = useMemo(
    () => snapshot.profiles.filter((p) => p.provider.toLowerCase() === "codex"),
    [snapshot.profiles]
  );

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
            <button
              onClick={() => void checkHealth()}
              className={clsx("cyber-btn flex items-center gap-2", checkingHealth && "cyber-btn-loading")}
              disabled={checkingHealth}
              title="Sends a minimal authenticated Codex probe for cached Codex accounts"
            >
              {checkingHealth ? <RefreshCw size={14} className="loading-spinner" /> : <ShieldCheck size={14} />}
              {checkingHealth ? "Đang check..." : "Check trạng thái"}
            </button>
            <button onClick={() => setShowAddModal(true)} className="cyber-btn cyber-btn-solid flex items-center gap-2">
              <Plus size={14} /> New Link
            </button>
          </div>
        </header>

        <div className="audience-filter-bar">
          {([
            ["all", "Tất cả"],
            ["personal", "Cá nhân"],
            ["customer", "Khách hàng"],
          ] as Array<[AudienceFilter, string]>).map(([value, label]) => (
            <button
              key={value}
              onClick={() => setAudienceFilter(value)}
              className={clsx("audience-filter-btn", audienceFilter === value && "active")}
            >
              {label}
            </button>
          ))}
        </div>

        <div className="dashboard-grid">
          {sortedProfiles.map((profile) => (
            <article
              key={profile.id}
              onClick={() => { if (profile.provider.toLowerCase() === "codex") openEdit(profile); }}
              className={clsx(
                "account-card",
                profile.provider.toLowerCase() === "codex" && "account-card-clickable",
                profile.provider.toLowerCase() === "codex" && normalizeAudience(profile.audience) === "customer" && "account-card-customer",
                profile.isActive && `active active-${profile.provider.toLowerCase()}`,
                profile.provider.toLowerCase() === "codex" && isUnhealthy(profile.healthStatus) && "account-card-health-danger",
                profile.provider.toLowerCase() === "codex" && profile.blocked && "account-card-blocked"
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
                  {profile.provider.toLowerCase() === "codex" && normalizeAudience(profile.audience) === "customer" && (
                    <span className="audience-badge-customer">KHÁCH</span>
                  )}
                </div>
              </div>

              <div className="space-y-5">
                {profile.provider.toLowerCase() === "codex" ? (
                  <div className="meter-block">
                    <div className="meter-label">
                      <span>Quota: WEEKLY</span>
                      <span className="text-neon">{profile.primarySummary}</span>
                    </div>
                    <div className="meter-track">
                      <div
                        className={clsx("meter-fill", meterTone(profile.primaryPercent))}
                        style={{ width: `${profile.primaryPercent}%` }}
                      />
                    </div>
                  </div>
                ) : (
                  <>
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
                  </>
                )}
              </div>

              {profile.provider.toLowerCase() === "codex" &&
                (profile.price > 0 || profile.lastTriggeredAtText || profile.createdAtText || profile.endAtText || profile.healthMessage) && (
                  <div className="card-meta">
                    {profile.price > 0 && (
                      <div className="card-meta-row">
                        <span className="text-dim">Giá</span>
                        <span>{formatVND(profile.price)}</span>
                      </div>
                    )}
                    {profile.lastTriggeredAtText && (
                      <div className="card-meta-row">
                        <span className="text-dim">Trigger</span>
                        <span>
                          {profile.lastTriggeredAtText}
                          {profile.lastTriggeredModel ? ` · ${profile.lastTriggeredModel}` : ""}
                        </span>
                      </div>
                    )}
                    {profile.createdAtText && (
                      <div className="card-meta-row">
                        <span className="text-dim">Added</span>
                        <span>{profile.createdAtText}</span>
                      </div>
                    )}
                    {profile.endAtText && (
                      <div className="card-meta-row">
                        <span className="text-dim">End</span>
                        <span>{profile.endAtText}{profile.daysRemainingText ? ` (${profile.daysRemainingText})` : ""}</span>
                      </div>
                    )}
                    {profile.daysUsedText && (
                      <div className="card-meta-row">
                        <span className="text-dim">Used</span>
                        <span>{profile.daysUsedText}</span>
                      </div>
                    )}
                    {profile.healthMessage && (
                      <div className={clsx("card-meta-row", healthTextClass(profile.healthStatus))}>
                        <span className="text-dim">Health</span>
                        <span>
                          {profile.healthMessage}
                          {profile.healthCheckedAtText && profile.healthCheckedAtText !== "-" ? ` · ${profile.healthCheckedAtText}` : ""}
                        </span>
                      </div>
                    )}
                  </div>
                )}

              <div className="flex justify-between items-center mt-6 pt-4 border-t border-dashed border-[rgba(0,243,255,0.1)]">
                <div className="flex items-center gap-2">
                  <span className={clsx("text-[9px] px-2 py-0.5 rounded", badgeClass(profile.authStatus))}>
                    {profile.authStatus.replace('_', ' ')}
                  </span>
                  {profile.provider.toLowerCase() === "codex" && profile.blocked && (
                    <span className="blocked-badge">BLOCKED</span>
                  )}
                  {profile.provider.toLowerCase() === "codex" && isHealthy(profile.healthStatus) && (
                    <span className="health-badge-ok">CHECK OK</span>
                  )}
                  {profile.provider.toLowerCase() === "codex" && isUnhealthy(profile.healthStatus) && (
                    <span className="health-badge">CHECK FAILED</span>
                  )}
                </div>
                <div className="flex gap-2">
                  {profile.canLoginFromCache && !profile.blocked && (
                    <button
                      onClick={(e) => { e.stopPropagation(); void onActivate(profile.id); }}
                      className="cyber-btn p-1.5"
                      disabled={busyProfile === profile.id}
                      title="RE-AUTHENTICATE"
                    >
                      <ShieldCheck size={14} />
                    </button>
                  )}
                  {profile.provider.toLowerCase() === "codex" && (
                    <button
                      onClick={(e) => { e.stopPropagation(); void onSetBlocked(profile.id, !profile.blocked); }}
                      className={clsx("cyber-btn p-1.5", !profile.blocked && "cyber-btn-danger")}
                      disabled={busyProfile === profile.id}
                      title={profile.blocked ? "UNBLOCK ACCOUNT" : "BLOCK ACCOUNT"}
                    >
                      {profile.blocked ? <Check size={14} /> : <Ban size={14} />}
                    </button>
                  )}
                  <button
                    onClick={(e) => { e.stopPropagation(); void onDelete(profile.id, profile.label); }}
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

              <div className="bg-[rgba(0,243,255,0.05)] p-4 border border-[rgba(0,243,255,0.1)] space-y-4">
                <div className="flex justify-between items-center">
                  <div>
                    <div className="font-bold text-sm">AUTO_TRIGGER (OPENAI ONLY)</div>
                    <div className="text-[10px] text-dim">Open weekly quota window on a schedule</div>
                  </div>
                  <label className="relative inline-flex items-center cursor-pointer">
                    <input
                      type="checkbox"
                      className="sr-only peer"
                      checked={trigger.enabled}
                      onChange={(e) => void saveTrigger({ ...trigger, enabled: e.target.checked })}
                    />
                    <div className="w-11 h-6 bg-gray-700 rounded-full peer peer-checked:after:translate-x-full after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-neon-cyan"></div>
                  </label>
                </div>

                {trigger.enabled && (
                  <>
                    <div className="flex justify-between items-center">
                      <span className="text-[11px] text-dim">TRIGGER_TIME</span>
                      <select
                        className="trigger-select"
                        value={trigger.time_of_day}
                        onChange={(e) => void saveTrigger({ ...trigger, time_of_day: e.target.value })}
                      >
                        {TIME_SLOTS.map((slot) => (
                          <option key={slot} value={slot}>{slot}</option>
                        ))}
                      </select>
                    </div>

                    <div className="flex gap-2">
                      {(["all", "top_n", "custom"] as TriggerMode[]).map((m) => (
                        <button
                          key={m}
                          onClick={() => void saveTrigger({ ...trigger, mode: m })}
                          className={clsx("cyber-btn flex-1 text-[10px]", trigger.mode === m && "cyber-btn-solid")}
                        >
                          {m === "top_n" ? "TOP N" : m.toUpperCase()}
                        </button>
                      ))}
                    </div>

                    {trigger.mode === "top_n" && (
                      <>
                        <div className="flex justify-between items-center">
                          <span className="text-[11px] text-dim">ACCOUNT_COUNT (best weekly quota)</span>
                          <input
                            type="number"
                            min={1}
                            max={Math.max(1, codexProfiles.length)}
                            value={trigger.count}
                            onChange={(e) => void saveTrigger({ ...trigger, count: Math.max(1, Number(e.target.value)) })}
                            className="trigger-select w-16 text-center"
                          />
                        </div>

                        {topNPreview.length > 0 && (
                          <div className="trigger-preview">
                            <div className="text-[10px] text-dim mb-1">WILL TRIGGER:</div>
                            {topNPreview.map((id) => {
                              const p = codexProfiles.find((c) => c.id === id);
                              if (!p) return null;
                              return (
                                <div key={id} className="trigger-preview-row">
                                  <span className="trigger-pick-name" title={p.label}>{p.label}</span>
                                  <span className="trigger-pick-quota">WK {p.primaryPercent}%</span>
                                </div>
                              );
                            })}
                          </div>
                        )}
                      </>
                    )}

                    {trigger.mode === "custom" && (
                      <div className="trigger-picker">
                        {codexProfiles.map((p) => (
                          <label key={p.id} className={clsx("trigger-pick-row", trigger.profile_ids.includes(p.id) && "selected")}>
                            <input
                              type="checkbox"
                              checked={trigger.profile_ids.includes(p.id)}
                              onChange={() => toggleCustomProfile(p.id)}
                            />
                            <span className="trigger-pick-name" title={p.label}>{p.label}</span>
                            <span className="trigger-pick-quota">WK {p.primaryPercent}%</span>
                          </label>
                        ))}
                        {codexProfiles.length === 0 && (
                          <div className="text-[10px] text-dim">No Codex accounts.</div>
                        )}
                      </div>
                    )}

                    <button onClick={() => void onTriggerNow()} className="cyber-btn cyber-btn-solid w-full flex items-center justify-center gap-2">
                      <Activity size={14} /> TRIGGER NOW
                    </button>

                    {lastRun && (
                      <div className="trigger-lastrun">
                        <div className="text-[10px] text-dim mb-1">
                          LAST_RUN {new Date(lastRun.ran_at).toLocaleString()}
                        </div>
                        {lastRun.results.map((r) => (
                          <div key={r.profile_id} className="trigger-lastrun-row">
                            <span>{r.status === "opened" ? "✓" : r.status === "error" ? "✗" : "•"} {r.label}</span>
                            <span className="text-dim">
                              {r.status === "opened" ? (r.model_used || "opened") : r.status.replace("_", " ")}
                            </span>
                          </div>
                        ))}
                      </div>
                    )}
                  </>
                )}
              </div>

              <div className="bg-[rgba(0,243,255,0.05)] p-4 border border-[rgba(0,243,255,0.1)]">
                <div className="font-bold text-sm mb-3">DELETION_LOG</div>
                {deletionHistory.length === 0 ? (
                  <div className="text-[10px] text-dim">No deletions yet.</div>
                ) : (
                  <div className="deletion-log">
                    {deletionHistory.map((d, i) => (
                      <div key={`${d.profile_id}-${i}`} className="deletion-log-row">
                        <span className="deletion-log-name" title={d.email || d.label}>
                          {d.label || d.email || d.profile_id}
                        </span>
                        <span className="deletion-log-meta">
                          {(d.provider || "").toUpperCase()} · {new Date(d.deleted_at).toLocaleString()}
                        </span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      )}

      {editProfile && (
        <div className="modal-overlay" onClick={() => setEditProfile(null)}>
          <div className="modal-content" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h2 className="modal-title">EDIT_ACCOUNT</h2>
              <button className="modal-close" onClick={() => setEditProfile(null)}><X size={20} /></button>
            </div>
            <div className="modal-form">
              <div className="modal-field">
                <div className="modal-label">NGÀY ADD</div>
                <input
                  type="date"
                  className="modal-input"
                  value={editDate}
                  onClick={(e) => {
                    const el = e.currentTarget as HTMLInputElement & { showPicker?: () => void };
                    el.showPicker?.();
                  }}
                  onChange={(e) => setEditDate(e.target.value)}
                />
              </div>
              <div className="modal-field">
                <div className="modal-label">GIÁ TIỀN (VNĐ)</div>
                <input
                  type="number"
                  min={0}
                  className="modal-input"
                  value={editPrice}
                  onChange={(e) => setEditPrice(Math.max(0, Number(e.target.value)))}
                />
                {editPrice > 0 && <div className="modal-hint">{formatVND(editPrice)}</div>}
              </div>
              <div className="modal-field">
                <div className="modal-label">PHÂN LOẠI SỬ DỤNG</div>
                <div className="audience-radio-group">
                  <label className="audience-radio-row">
                    <input
                      type="radio"
                      name="edit-audience"
                      value="personal"
                      checked={editAudience === "personal"}
                      onChange={() => setEditAudience("personal")}
                    />
                    <span>Tài khoản sử dụng cho cá nhân</span>
                  </label>
                  <label className="audience-radio-row">
                    <input
                      type="radio"
                      name="edit-audience"
                      value="customer"
                      checked={editAudience === "customer"}
                      onChange={() => setEditAudience("customer")}
                    />
                    <span>Tài khoản sử dụng cho khách</span>
                  </label>
                </div>
              </div>
              <button onClick={() => void saveEdit()} className="cyber-btn cyber-btn-solid modal-btn-full">
                LƯU
              </button>
            </div>
          </div>
        </div>
      )}

      {pricePrompt && (
        <div className="modal-overlay" onClick={() => setPricePrompt(null)}>
          <div className="modal-content" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h2 className="modal-title">GIÁ TÀI KHOẢN</h2>
              <button className="modal-close" onClick={() => setPricePrompt(null)}><X size={20} /></button>
            </div>
            <div className="modal-sub" title={pricePrompt.email}>
              {pricePrompt.label || pricePrompt.email}
            </div>
            <input
              type="number"
              min={0}
              autoFocus
              placeholder="Nhập giá đã mua (VNĐ)"
              className="modal-input"
              value={promptPrice || ""}
              onChange={(e) => setPromptPrice(Math.max(0, Number(e.target.value)))}
            />
            {promptPrice > 0 && <div className="modal-hint">{formatVND(promptPrice)}</div>}
            <div className="modal-field mt-4">
              <div className="modal-label">PHÂN LOẠI SỬ DỤNG</div>
              <div className="audience-radio-group">
                <label className="audience-radio-row">
                  <input
                    type="radio"
                    name="prompt-audience"
                    value="personal"
                    checked={promptAudience === "personal"}
                    onChange={() => setPromptAudience("personal")}
                  />
                  <span>Tài khoản sử dụng cho cá nhân</span>
                </label>
                <label className="audience-radio-row">
                  <input
                    type="radio"
                    name="prompt-audience"
                    value="customer"
                    checked={promptAudience === "customer"}
                    onChange={() => setPromptAudience("customer")}
                  />
                  <span>Tài khoản sử dụng cho khách</span>
                </label>
              </div>
            </div>
            <div className="modal-actions">
              <button onClick={() => setPricePrompt(null)} className="cyber-btn modal-btn">SKIP</button>
              <button onClick={() => void savePrice()} className="cyber-btn cyber-btn-solid modal-btn">LƯU</button>
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

const formatVND = (value: number): string =>
  `${new Intl.NumberFormat("vi-VN").format(value)} ₫`;

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

const isUnhealthy = (status: string) => {
  const normalized = status.toLowerCase();
  return normalized === "failed" || normalized === "limited" || normalized === "no_auth";
};

const isHealthy = (status: string) => status.toLowerCase() === "ok";

const healthTextClass = (status: string) => {
  if (isHealthy(status)) return "card-meta-health-ok";
  if (isUnhealthy(status)) return "card-meta-health-error";
  return "";
};

const normalizeAudience = (value: string) => value.toLowerCase() === "customer" ? "customer" : "personal";

export default App;
