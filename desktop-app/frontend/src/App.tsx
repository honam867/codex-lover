import { useEffect, useMemo, useRef, useState } from "react";
import codexLogo from "./assets/provider-codex.svg";
import claudeLogo from "./assets/provider-claude.svg";
import kimiLogo from "./assets/provider-kimi.svg";
import {
  Activity,
  Ban,
  Check,
  Cpu,
  FileSpreadsheet,
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
  AddShop,
  CheckSingleProfileHealth,
  ExportProfilesToExcel,
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
  RemoveShop,
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
  shopName: string;
  customerName: string;
  note: string;
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

type ViewMode = "card" | "list";
type AudienceValue = "personal" | "customer";
type HealthFilter = "ok" | "limited" | "failed" | "no_auth" | "unknown";
type AudienceSort = "" | "customer_first" | "personal_first";
type MonthSort = "" | "newest" | "oldest";
type HealthSort = "" | "ok_first";
type PriceSort = "" | "asc" | "desc";
type FilterMenuKey = "audience" | "health" | "provider" | "shop";

type FilterOption = {
  value: string;
  label: string;
};

const ZOOM_LEVELS = [0.85, 1, 1.15];

const AUDIENCE_OPTIONS: Array<[AudienceValue, string]> = [
  ["customer", "Khách hàng"],
  ["personal", "Cá nhân"],
];

const HEALTH_OPTIONS: Array<[HealthFilter, string]> = [
  ["ok", "Probe OK"],
  ["limited", "Quota limited"],
  ["failed", "Check failed"],
  ["no_auth", "No auth"],
];

function App() {
  const [snapshot, setSnapshot] = useState<Snapshot>({ generatedAt: "", profiles: [] });
  const [busyProfile, setBusyProfile] = useState<string>("");
  const [healthPickMode, setHealthPickMode] = useState<boolean>(false);
  const [checkingProfile, setCheckingProfile] = useState<string>("");
  const [exportingExcel, setExportingExcel] = useState<boolean>(false);
  const [statusText, setStatusText] = useState<string>("SYSTEM_READY");
  const [providerFilter, setProviderFilter] = useState<string>("all");
  const [audienceFilters, setAudienceFilters] = useState<AudienceValue[]>([]);
  const [healthFilters, setHealthFilters] = useState<HealthFilter[]>([]);
  const [providerFilters, setProviderFilters] = useState<string[]>([]);
  const [shopFilters, setShopFilters] = useState<string[]>([]);
  const [openFilterMenu, setOpenFilterMenu] = useState<FilterMenuKey | "">("");
  const [audienceSort, setAudienceSort] = useState<AudienceSort>("");
  const [monthSort, setMonthSort] = useState<MonthSort>("");
  const [healthSort, setHealthSort] = useState<HealthSort>("");
  const [priceSort, setPriceSort] = useState<PriceSort>("");
  const [viewMode, setViewMode] = useState<ViewMode>("card");
  const [zoomIndex, setZoomIndex] = useState<number>(1);
  const [sidebarCollapsed, setSidebarCollapsed] = useState<boolean>(false);
  const [showAddModal, setShowAddModal] = useState<boolean>(false);
  const [showSettingsModal, setShowSettingsModal] = useState<boolean>(false);
  const [autoRotateEnabled, setAutoRotateEnabled] = useState<boolean>(false);
  const [autoRotateThreshold, setAutoRotateThreshold] = useState<number>(5);
  const [systemStatus, setSystemStatus] = useState<SystemStatus>({ hasCodexCli: true, codexInstallUrl: "https://github.com/openai/codex" });
  const [trigger, setTrigger] = useState<TriggerConfig>(DEFAULT_TRIGGER);
  const [lastRun, setLastRun] = useState<TriggerRun | null>(null);
  const [topNPreview, setTopNPreview] = useState<string[]>([]);
  const [deletionHistory, setDeletionHistory] = useState<DeletedAccountRecord[]>([]);
  const [showDeletionLog, setShowDeletionLog] = useState<boolean>(false);
  const [shopCatalog, setShopCatalog] = useState<string[]>([]);
  const [newShopName, setNewShopName] = useState<string>("");
  const [showShopSuggestions, setShowShopSuggestions] = useState<boolean>(false);
  const [editProfile, setEditProfile] = useState<ProfileCard | null>(null);
  const [editDate, setEditDate] = useState<string>("");
  const [editPrice, setEditPrice] = useState<number>(0);
  const [editAudience, setEditAudience] = useState<string>("personal");
  const [editShopName, setEditShopName] = useState<string>("");
  const [editCustomerName, setEditCustomerName] = useState<string>("");
  const [showEditShopSuggestions, setShowEditShopSuggestions] = useState<boolean>(false);
  const [editNote, setEditNote] = useState<string>("");
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
    setShowDeletionLog(false);
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
      const shops = (config as any).shops;
      if (typeof enabled === "boolean") setAutoRotateEnabled(enabled);
      if (typeof threshold === "number") setAutoRotateThreshold(threshold);
      if (Array.isArray(shops)) setShopCatalog(shops.filter((shop) => typeof shop === "string"));
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

  async function saveNewShop() {
    const name = newShopName.trim();
    if (!name) return;
    const shops = await AddShop(name);
    setShopCatalog(shops ?? []);
    setNewShopName("");
    setShowShopSuggestions(false);
  }

  async function deleteShop(name: string) {
    const shops = await RemoveShop(name);
    setShopCatalog(shops ?? []);
    if (editShopName === name) setEditShopName("");
    setShopFilters((current) => current.filter((shop) => !sameText(shop, name)));
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
    if (checkingProfile) return;
    setHealthPickMode((enabled) => !enabled);
    setStatusText(healthPickMode ? "SYSTEM_READY" : "SELECT_ACCOUNT_TO_CHECK");
  }

  async function checkSingleProfileHealth(profile: ProfileCard) {
    if (!healthPickMode || checkingProfile || profile.provider.toLowerCase() !== "codex") return;
    const confirmed = window.confirm(
      `Check trạng thái cho ${profile.label || profile.email}? ` +
        "Request này sẽ dùng cached Codex auth, có thể refresh token và có thể ảnh hưởng quota window. Tiếp tục?"
    );
    if (!confirmed) return;
    setStatusText(`CHECKING ${profile.label || profile.email}...`);
    setCheckingProfile(profile.id);
    try {
      const result = await CheckSingleProfileHealth(profile.id);
      applyAction(result);
      setStatusText(result.error ? `ERROR: ${result.error}` : "SELECT_ACCOUNT_TO_CHECK");
    } finally {
      setCheckingProfile("");
    }
  }

  async function exportExcel() {
    if (exportingExcel) return;
    setExportingExcel(true);
    setStatusText("EXPORTING_EXCEL...");
    try {
      const result = await ExportProfilesToExcel(sortedProfiles as any);
      applyAction(result);
    } catch {
      setStatusText("ERROR: EXPORT FAILED");
    } finally {
      setExportingExcel(false);
    }
  }

  function applyAction(result: ActionResponse) {
    setSnapshot(result.snapshot);
    setStatusText(result.error ? `ERROR: ${result.error}` : result.message || `CORE_LOADED: ${result.snapshot.generatedAt}`);
    setBusyProfile("");
    setShowAddModal(false);
  }

  function handleProfileClick(profile: ProfileCard) {
    if (healthPickMode) {
      void checkSingleProfileHealth(profile);
      return;
    }
    if (profile.provider.toLowerCase() === "codex") openEdit(profile);
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
    setEditShopName(profile.shopName || "");
    setEditCustomerName(profile.customerName || "");
    setShowEditShopSuggestions(false);
    setEditNote(profile.note || "");
  }

  async function saveEdit() {
    if (!editProfile) return;
    const shopName = editShopName.trim();
    if (shopName && !shopCatalog.some((shop) => sameText(shop, shopName))) {
      const shops = await AddShop(shopName);
      setShopCatalog(shops ?? []);
    }
    const result = await UpdateProfileMeta(editProfile.id, editDate, editPrice, editAudience, shopName, editCustomerName, editNote);
    setEditProfile(null);
    applyAction(result);
  }

  async function savePrice() {
    if (!pricePrompt) return;
    const result = await UpdateProfileMeta(pricePrompt.id, pricePrompt.createdAtISO || "", promptPrice, promptAudience, pricePrompt.shopName || "", pricePrompt.customerName || "", pricePrompt.note || "");
    setPricePrompt(null);
    applyAction(result);
  }

  const providerOptions = useMemo(() => {
    return Array.from(new Set(snapshot.profiles.map((p) => p.provider || "unknown"))).sort();
  }, [snapshot.profiles]);

  const shopOptions = useMemo(() => {
    return uniqueText([
      ...shopCatalog,
      ...snapshot.profiles.map((profile) => profile.shopName || ""),
    ]).sort((a, b) => a.localeCompare(b));
  }, [shopCatalog, snapshot.profiles]);

  const sortedProfiles = useMemo(() => {
    const filtered = snapshot.profiles.filter((profile) => {
      const provider = profile.provider || "unknown";
      if (providerFilter !== "all" && provider !== providerFilter) return false;
      if (providerFilters.length > 0 && !providerFilters.includes(provider)) return false;
      if (audienceFilters.length > 0 && profile.provider.toLowerCase() === "codex") {
        if (!audienceFilters.includes(normalizeAudience(profile.audience))) return false;
      }
      if (healthFilters.length > 0 && profile.provider.toLowerCase() === "codex") {
        if (!healthFilters.includes(normalizeHealth(profile.healthStatus))) return false;
      }
      if (shopFilters.length > 0) {
        if (profile.provider.toLowerCase() !== "codex") return false;
        if (!shopFilters.some((shop) => sameText(shop, profile.shopName))) return false;
      }
      return true;
    });

    if (!audienceSort && !monthSort && !healthSort && !priceSort) return filtered;
    return [...filtered].sort((a, b) => compareProfiles(a, b, { audienceSort, monthSort, healthSort, priceSort }));
  }, [audienceFilters, audienceSort, healthFilters, healthSort, monthSort, priceSort, providerFilter, providerFilters, shopFilters, snapshot.profiles]);

  const codexProfiles = useMemo(
    () => snapshot.profiles.filter((p) => p.provider.toLowerCase() === "codex"),
    [snapshot.profiles]
  );

  const editShopOptions = useMemo(() => {
    const current = editShopName.trim();
    if (current && !shopCatalog.some((shop) => shop.toLowerCase() === current.toLowerCase())) {
      return [...shopCatalog, current];
    }
    return shopCatalog;
  }, [editShopName, shopCatalog]);

  const shopSuggestions = useMemo(() => {
    const q = newShopName.trim().toLowerCase();
    if (!q) return shopCatalog;
    return shopCatalog.filter((shop) => shop.toLowerCase().includes(q));
  }, [newShopName, shopCatalog]);

  const editShopSuggestions = useMemo(() => {
    const q = editShopName.trim().toLowerCase();
    if (!q) return editShopOptions;
    return editShopOptions.filter((shop) => shop.toLowerCase().includes(q));
  }, [editShopName, editShopOptions]);

  const canCreateEditShop = Boolean(editShopName.trim()) && !shopCatalog.some((shop) => sameText(shop, editShopName));

  useEffect(() => {
    if (providerFilter !== "all" && !providerOptions.includes(providerFilter)) {
      setProviderFilter("all");
    }
    setProviderFilters((current) => current.filter((provider) => providerOptions.includes(provider)));
  }, [providerFilter, providerOptions]);

  useEffect(() => {
    setShopFilters((current) => current.filter((shop) => shopOptions.some((option) => sameText(option, shop))));
  }, [shopOptions]);

  const zoomLevel = ZOOM_LEVELS[zoomIndex] ?? 1;
  const hasActiveFilters = audienceFilters.length > 0 || healthFilters.length > 0 || providerFilters.length > 0 || shopFilters.length > 0;
  const hasActiveSorts = Boolean(audienceSort || monthSort || healthSort || priceSort);
  const toggleFilterMenu = (menu: FilterMenuKey) => setOpenFilterMenu((current) => current === menu ? "" : menu);
  const clearAllFilterSort = () => {
    setAudienceFilters([]);
    setHealthFilters([]);
    setProviderFilters([]);
    setShopFilters([]);
    setAudienceSort("");
    setMonthSort("");
    setHealthSort("");
    setPriceSort("");
  };

  return (
    <div className={clsx("app-shell", sidebarCollapsed && "app-shell-sidebar-collapsed")} style={{ "--dashboard-zoom": zoomLevel } as any}>
      <aside className="sidebar">
        <div className="sidebar-logo">
          <Cpu size={24} className="text-neon" />
          <h1>CODEX // CORE</h1>
          <button
            onClick={() => setSidebarCollapsed((value) => !value)}
            className="sidebar-toggle"
            title={sidebarCollapsed ? "Open sidebar" : "Close sidebar"}
          >
            {sidebarCollapsed ? ">" : "<"}
          </button>
        </div>

        <nav className="nav-group">
          <span className="nav-label">Modules</span>
          <button 
            onClick={() => { setProviderFilter('all'); setProviderFilters([]); }}
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
                onClick={() => { setProviderFilter(p); setProviderFilters([]); }}
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
          <div className="top-actions">
            <button onClick={() => void refresh()} className="cyber-btn flex items-center gap-2">
              <RefreshCw size={14} /> Sync All
            </button>
            <button
              onClick={() => void checkHealth()}
              className={clsx("cyber-btn flex items-center gap-2", healthPickMode && "cyber-btn-solid", checkingProfile && "cyber-btn-loading")}
              disabled={Boolean(checkingProfile)}
              title="Bật chế độ chọn account Codex trên trang để check riêng từng account"
            >
              {checkingProfile ? <RefreshCw size={14} className="loading-spinner" /> : <ShieldCheck size={14} />}
              {checkingProfile ? "Đang check..." : healthPickMode ? "Chọn tài khoản" : "Check trạng thái"}
            </button>
            <button
              onClick={() => void exportExcel()}
              className={clsx("cyber-btn flex items-center gap-2", exportingExcel && "cyber-btn-loading")}
              disabled={exportingExcel || sortedProfiles.length === 0}
              title="Export các card đang hiển thị ra Excel, không gồm quota và health"
            >
              {exportingExcel ? <RefreshCw size={14} className="loading-spinner" /> : <FileSpreadsheet size={14} />}
              {exportingExcel ? "Đang export..." : "Export Excel"}
            </button>
            <button onClick={() => setShowAddModal(true)} className="cyber-btn cyber-btn-solid flex items-center gap-2">
              <Plus size={14} /> New Link
            </button>
          </div>
        </header>

        <div className="dashboard-toolbar">
          <div className="toolbar-primary-row">
            <div className="toolbar-left">
              <div className="status-bar">
                <div className="status-dot" />
                <span>{statusText}</span>
              </div>
              <div className="view-switcher">
                <span className="control-label">VIEW</span>
                <button onClick={() => setViewMode("card")} className={clsx("control-chip", viewMode === "card" && "active")}>CARD</button>
                <button onClick={() => setViewMode("list")} className={clsx("control-chip", viewMode === "list" && "active")}>LIST</button>
              </div>
              <div className="view-switcher">
                <span className="control-label">ZOOM</span>
                <button onClick={() => setZoomIndex((value) => Math.max(0, value - 1))} className="control-chip" disabled={zoomIndex === 0}>-</button>
                <span className="zoom-readout">{Math.round(zoomLevel * 100)}%</span>
                <button onClick={() => setZoomIndex((value) => Math.min(ZOOM_LEVELS.length - 1, value + 1))} className="control-chip" disabled={zoomIndex === ZOOM_LEVELS.length - 1}>+</button>
              </div>
            </div>
            <div className="toolbar-right">
              <div className="filter-sort-row">
                <span className="filter-sort-label">FILTER</span>
                <MultiFilterMenu
                  label="Audience"
                  options={AUDIENCE_OPTIONS.map(([value, label]) => ({ value, label }))}
                  selected={audienceFilters}
                  open={openFilterMenu === "audience"}
                  onToggleOpen={() => toggleFilterMenu("audience")}
                  onToggle={(value) => setAudienceFilters((current) => toggleFilterValue(current, value as AudienceValue))}
                />
                <MultiFilterMenu
                  label="Health"
                  options={HEALTH_OPTIONS.map(([value, label]) => ({ value, label }))}
                  selected={healthFilters}
                  open={openFilterMenu === "health"}
                  onToggleOpen={() => toggleFilterMenu("health")}
                  onToggle={(value) => setHealthFilters((current) => toggleFilterValue(current, value as HealthFilter))}
                />
                <MultiFilterMenu
                  label="Provider"
                  options={providerOptions.map((provider) => ({ value: provider, label: provider.toUpperCase() }))}
                  selected={providerFilters}
                  open={openFilterMenu === "provider"}
                  onToggleOpen={() => toggleFilterMenu("provider")}
                  onToggle={(value) => setProviderFilters((current) => toggleFilterValue(current, value))}
                />
                <MultiFilterMenu
                  label="Shop"
                  options={shopOptions.map((shop) => ({ value: shop, label: shop }))}
                  selected={shopFilters}
                  open={openFilterMenu === "shop"}
                  onToggleOpen={() => toggleFilterMenu("shop")}
                  onToggle={(value) => setShopFilters((current) => toggleFilterValue(current, value))}
                  emptyLabel="Chưa có shop"
                />
              </div>
              <div className="filter-sort-row">
                <span className="filter-sort-label">SORT</span>
                <select className="filter-select" value={audienceSort} onChange={(event) => setAudienceSort(event.target.value as AudienceSort)}>
                  <option value="">Audience</option>
                  <option value="customer_first">Khách hàng trước</option>
                  <option value="personal_first">Cá nhân trước</option>
                </select>
                <select className="filter-select" value={monthSort} onChange={(event) => setMonthSort(event.target.value as MonthSort)}>
                  <option value="">Month</option>
                  <option value="newest">Tháng mới trước</option>
                  <option value="oldest">Tháng cũ trước</option>
                </select>
                <select className="filter-select" value={healthSort} onChange={(event) => setHealthSort(event.target.value as HealthSort)}>
                  <option value="">Quota</option>
                  <option value="ok_first">Probe OK trước</option>
                </select>
                <select className="filter-select" value={priceSort} onChange={(event) => setPriceSort(event.target.value as PriceSort)}>
                  <option value="">Price</option>
                  <option value="asc">Giá thấp - cao</option>
                  <option value="desc">Giá cao - thấp</option>
                </select>
              </div>
            </div>
          </div>
          {(hasActiveFilters || hasActiveSorts) && (
            <div className="active-filter-tags">
              {audienceFilters.map((value) => (
                <button key={`audience-${value}`} className="filter-tag" onClick={() => setAudienceFilters((current) => current.filter((item) => item !== value))}>
                  Audience: {audienceLabel(value)} <X size={10} />
                </button>
              ))}
              {healthFilters.map((value) => (
                <button key={`health-${value}`} className="filter-tag" onClick={() => setHealthFilters((current) => current.filter((item) => item !== value))}>
                  Health: {healthLabel(value)} <X size={10} />
                </button>
              ))}
              {providerFilters.map((value) => (
                <button key={`provider-${value}`} className="filter-tag" onClick={() => setProviderFilters((current) => current.filter((item) => item !== value))}>
                  Provider: {value.toUpperCase()} <X size={10} />
                </button>
              ))}
              {shopFilters.map((value) => (
                <button key={`shop-${value}`} className="filter-tag" onClick={() => setShopFilters((current) => current.filter((item) => !sameText(item, value)))}>
                  Shop: {value} <X size={10} />
                </button>
              ))}
              {audienceSort && <button className="filter-tag filter-tag-sort" onClick={() => setAudienceSort("")}>Sort: {audienceSortLabel(audienceSort)} <X size={10} /></button>}
              {monthSort && <button className="filter-tag filter-tag-sort" onClick={() => setMonthSort("")}>Sort: {monthSortLabel(monthSort)} <X size={10} /></button>}
              {healthSort && <button className="filter-tag filter-tag-sort" onClick={() => setHealthSort("")}>Sort: Probe OK trước <X size={10} /></button>}
              {priceSort && <button className="filter-tag filter-tag-sort" onClick={() => setPriceSort("")}>Sort: {priceSortLabel(priceSort)} <X size={10} /></button>}
              <button className="filter-tag-clear" onClick={clearAllFilterSort}>Clear all</button>
            </div>
          )}
          {healthPickMode && (
            <div className="health-pick-hint">
              Click một Codex card đang hiển thị để check riêng account đó. Bấm lại Chọn tài khoản để thoát.
            </div>
          )}
        </div>

        <div className={clsx("dashboard-grid", viewMode === "list" && "dashboard-list")}>
          {sortedProfiles.map((profile) => (
            <article
              key={profile.id}
              onClick={() => handleProfileClick(profile)}
              className={clsx(
                "account-card",
                profile.provider.toLowerCase() === "codex" && "account-card-clickable",
                healthPickMode && profile.provider.toLowerCase() === "codex" && "account-card-health-selectable",
                checkingProfile === profile.id && "account-card-health-checking",
                profile.provider.toLowerCase() === "codex" && normalizeAudience(profile.audience) === "customer" && "account-card-customer",
                profile.isActive && `active active-${profile.provider.toLowerCase()}`,
                profile.provider.toLowerCase() === "codex" && isUnhealthy(profile.healthStatus) && "account-card-health-danger",
                profile.provider.toLowerCase() === "codex" && profile.blocked && "account-card-blocked"
              )}
            >
              <img src={getProviderLogo(profile.provider)} className="provider-corner-logo" alt="" aria-hidden="true" />
              <div className="card-head flex justify-between items-start mb-4">
                <div className="min-w-0 flex-1">
                  <h3 className="card-title truncate" title={profile.label}>{profile.label.toUpperCase()}</h3>
                  <p className="card-email truncate" title={profile.email}>{profile.email}</p>
                </div>
              </div>

              <div className="card-usage space-y-5">
                {profile.provider.toLowerCase() === "codex" ? (
                  <div className="meter-block">
                    <div className="meter-label">
                      <span>Quota: WEEKLY</span>
                      <span className="text-neon">{renderQuotaSummary(profile.primarySummary)}</span>
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
                        <span className="text-neon">{renderQuotaSummary(profile.primarySummary)}</span>
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
                        <span className="text-neon">{renderQuotaSummary(profile.secondarySummary)}</span>
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
                (profile.price > 0 || profile.shopName || profile.customerName || profile.createdAtText || profile.endAtText || profile.healthMessage) && (
                  <div className="card-meta">
                    {profile.price > 0 && (
                      <div className="card-meta-row">
                        <span className="text-dim">Giá</span>
                        <span>{formatVND(profile.price)}</span>
                      </div>
                    )}
                    {profile.shopName && (
                      <div className="card-meta-row">
                        <span className="text-dim">Tên shop</span>
                        <span>{profile.shopName}</span>
                      </div>
                    )}
                    {profile.customerName && (
                      <div className="card-meta-row">
                        <span className="text-dim">Tên khách</span>
                        <strong className="card-customer-name">{profile.customerName}</strong>
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
                        <span>
                          {profile.endAtText}
                          {profile.daysRemainingText && (
                            <strong className="card-day-count"> ({profile.daysRemainingText})</strong>
                          )}
                        </span>
                      </div>
                    )}
                    {profile.daysUsedText && (
                      <div className="card-meta-row">
                        <span className="text-dim">Used</span>
                        <strong className="card-day-count">{profile.daysUsedText}</strong>
                      </div>
                    )}
                    {profile.healthMessage && (
                      <div className={clsx("card-meta-row", healthTextClass(profile.healthStatus))}>
                        <span className="text-dim">Health</span>
                        <span>
                          {formatHealthMessage(profile.healthMessage)}
                          {profile.healthCheckedAtText && profile.healthCheckedAtText !== "-" ? ` · ${profile.healthCheckedAtText}` : ""}
                        </span>
                      </div>
                    )}
                  </div>
                )}

              <div className="card-actions">
                <div className="card-badges">
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
                <div className="card-action-buttons">
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
          <div className="modal-content settings-modal" onClick={e => e.stopPropagation()}>
            <button className="modal-close settings-modal-close" onClick={() => setShowSettingsModal(false)}><X size={18} /></button>
             <div className="settings-modal-header">
              <h2 className="text-neon text-lg font-bold">CORE_CONFIGURATION</h2>
            </div>
            <div className="settings-modal-body">
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
                <div>
                  <div className="font-bold text-sm">DANH MỤC SHOP</div>
                </div>
                <div className="shop-catalog-add">
                  <div className="shop-combobox">
                    <input
                      className="modal-input"
                      value={newShopName}
                      placeholder="Tên shop"
                      onFocus={() => setShowShopSuggestions(true)}
                      onBlur={() => window.setTimeout(() => setShowShopSuggestions(false), 120)}
                      onChange={(e) => { setNewShopName(e.target.value); setShowShopSuggestions(true); }}
                      onKeyDown={(e) => { if (e.key === "Enter") void saveNewShop(); }}
                    />
                    {showShopSuggestions && (
                      <div className="shop-suggestion-list">
                        {shopSuggestions.length === 0 && <div className="shop-suggestion-empty">Không có shop khớp</div>}
                        {shopSuggestions.map((shop) => (
                          <button
                            key={shop}
                            type="button"
                            className="shop-suggestion-item"
                            onMouseDown={(e) => e.preventDefault()}
                            onClick={() => { setNewShopName(shop); setShowShopSuggestions(false); }}
                          >
                            <span>{shop}</span>
                            <X size={11} onClick={(e) => { e.stopPropagation(); void deleteShop(shop); }} />
                          </button>
                        ))}
                      </div>
                    )}
                  </div>
                  <button className="cyber-btn cyber-btn-solid" onClick={() => void saveNewShop()}>THÊM</button>
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
                <button className="settings-collapse-toggle" onClick={() => setShowDeletionLog((value) => !value)}>
                  <span>DELETION_LOG</span>
                  <span>{deletionHistory.length} item(s) {showDeletionLog ? "▲" : "▼"}</span>
                </button>
                {showDeletionLog && (
                  deletionHistory.length === 0 ? (
                    <div className="text-[10px] text-dim mt-3">No deletions yet.</div>
                  ) : (
                    <div className="deletion-log mt-3">
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
                  )
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
              <h2 className="modal-title">Chỉnh sửa</h2>
              <button className="modal-close" onClick={() => setEditProfile(null)}><X size={20} /></button>
            </div>
            <div className="modal-form">
              <div className="modal-field-row">
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
                  <div className="modal-label">GIÁ TIỀN</div>
                  <input
                    type="text"
                    inputMode="numeric"
                    className="modal-input"
                    value={formatVNDInput(editPrice)}
                    onChange={(e) => setEditPrice(parseVNDInput(e.target.value))}
                  />
                </div>
              </div>
              <div className="modal-field">
                <div className="modal-label">TÊN SHOP</div>
                <div className="shop-combobox">
                  <input
                    className="modal-input"
                    value={editShopName}
                    placeholder="Chọn hoặc tạo shop"
                    onFocus={() => setShowEditShopSuggestions(true)}
                    onBlur={() => window.setTimeout(() => setShowEditShopSuggestions(false), 120)}
                    onChange={(e) => { setEditShopName(e.target.value); setShowEditShopSuggestions(true); }}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        e.preventDefault();
                        setShowEditShopSuggestions(false);
                      }
                    }}
                  />
                  {showEditShopSuggestions && (
                    <div className="shop-suggestion-list">
                      {editShopSuggestions.length === 0 && !canCreateEditShop && <div className="shop-suggestion-empty">Không có shop khớp</div>}
                      {editShopSuggestions.map((shop) => (
                        <button
                          key={shop}
                          type="button"
                          className="shop-suggestion-item"
                          onMouseDown={(e) => e.preventDefault()}
                          onClick={() => { setEditShopName(shop); setShowEditShopSuggestions(false); }}
                        >
                          <span>{shop}</span>
                        </button>
                      ))}
                      {canCreateEditShop && (
                        <button
                          type="button"
                          className="shop-suggestion-item shop-suggestion-create"
                          onMouseDown={(e) => e.preventDefault()}
                          onClick={() => setShowEditShopSuggestions(false)}
                        >
                          <span>Tạo shop: {editShopName.trim()}</span>
                        </button>
                      )}
                    </div>
                  )}
                </div>
                {canCreateEditShop && <div className="modal-hint">Shop mới sẽ được thêm vào danh mục khi lưu account.</div>}
              </div>
              <div className="modal-field">
                <div className="modal-label">TÊN KHÁCH HÀNG</div>
                <input
                  className="modal-input"
                  value={editCustomerName}
                  placeholder="Tên khách để quản lý"
                  onChange={(e) => setEditCustomerName(e.target.value)}
                />
              </div>
              <div className="modal-field">
                <div className="modal-label">GHI CHÚ</div>
                <textarea
                  className="modal-input modal-textarea"
                  value={editNote}
                  placeholder="Ghi chú nội bộ, không hiển thị trên card"
                  onChange={(e) => setEditNote(e.target.value)}
                />
              </div>
              <div className="modal-field">
                <div className="modal-label">PHÂN LOẠI SỬ DỤNG</div>
                <div className="audience-radio-group audience-radio-inline">
                  <label className="audience-radio-row">
                    <input
                      type="radio"
                      name="edit-audience"
                      value="personal"
                      checked={editAudience === "personal"}
                      onChange={() => setEditAudience("personal")}
                    />
                    <span>Cá nhân</span>
                  </label>
                  <label className="audience-radio-row">
                    <input
                      type="radio"
                      name="edit-audience"
                      value="customer"
                      checked={editAudience === "customer"}
                      onChange={() => setEditAudience("customer")}
                    />
                    <span>Khách hàng</span>
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

function MultiFilterMenu({
  label,
  options,
  selected,
  open,
  onToggleOpen,
  onToggle,
  emptyLabel = "Không có lựa chọn",
}: {
  label: string;
  options: FilterOption[];
  selected: string[];
  open: boolean;
  onToggleOpen: () => void;
  onToggle: (value: string) => void;
  emptyLabel?: string;
}) {
  const activeText = selected.length > 0 ? `${label} (${selected.length})` : label;
  return (
    <div className="filter-menu">
      <button type="button" className={clsx("filter-menu-button", selected.length > 0 && "active", open && "open")} onClick={onToggleOpen}>
        <span>{activeText}</span>
        <span>{open ? "▲" : "▼"}</span>
      </button>
      {open && (
        <div className="filter-menu-panel">
          {options.length === 0 && <div className="filter-menu-empty">{emptyLabel}</div>}
          {options.map((option) => {
            const checked = selected.some((value) => sameText(value, option.value));
            return (
              <button
                key={option.value}
                type="button"
                className={clsx("filter-menu-option", checked && "selected")}
                onClick={() => onToggle(option.value)}
              >
                <span>{option.label}</span>
                <span>{checked ? "✓" : ""}</span>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

const formatVND = (value: number): string =>
  `${new Intl.NumberFormat("vi-VN").format(value)} ₫`;

const formatVNDInput = (value: number): string =>
  value > 0 ? formatVND(value) : "";

const parseVNDInput = (value: string): number => {
  const digits = value.replace(/\D/g, "");
  return digits ? Number(digits) : 0;
};

const renderQuotaSummary = (summary: string) => {
  const match = summary.match(/^(.*?\bresets\s+)(\d{4}-\d{2}-\d{2})(.*)$/i);
  if (!match) return summary;
  return (
    <>
      {match[1]}
      <strong className="quota-reset-date">{match[2]}</strong>
      {match[3]}
    </>
  );
};

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
  return normalized === "failed" || normalized === "no_auth";
};

const isHealthy = (status: string) => status.toLowerCase() === "ok";

const isLimited = (status: string) => status.toLowerCase() === "limited";

const healthTextClass = (status: string) => {
  if (isHealthy(status)) return "card-meta-health-ok";
  if (isLimited(status)) return "card-meta-health-limited";
  if (isUnhealthy(status)) return "card-meta-health-error";
  return "";
};

const normalizeAudience = (value: string) => value.toLowerCase() === "customer" ? "customer" : "personal";

const sameText = (a: string, b: string): boolean =>
  a.trim().toLowerCase() === b.trim().toLowerCase();

const uniqueText = (values: string[]): string[] => {
  const out: string[] = [];
  for (const value of values) {
    const trimmed = value.trim();
    if (!trimmed || out.some((item) => sameText(item, trimmed))) continue;
    out.push(trimmed);
  }
  return out;
};

const toggleFilterValue = <T extends string>(current: T[], value: T): T[] =>
  current.some((item) => sameText(item, value))
    ? current.filter((item) => !sameText(item, value))
    : [...current, value];

const normalizeHealth = (value: string): HealthFilter => {
  const normalized = value.toLowerCase();
  if (normalized === "ok") return "ok";
  if (normalized === "limited") return "limited";
  if (normalized === "no_auth") return "no_auth";
  if (normalized === "failed") return "failed";
  return "unknown";
};

const formatHealthMessage = (value: string): string =>
  value.replace(/^\s*skipped:\s*/i, "");

const audienceLabel = (value: AudienceValue): string =>
  value === "customer" ? "Khách hàng" : "Cá nhân";

const healthLabel = (value: HealthFilter): string => {
  switch (value) {
    case "ok": return "Probe OK";
    case "limited": return "Quota limited";
    case "no_auth": return "No auth";
    default: return "Check failed";
  }
};

const audienceSortLabel = (value: AudienceSort): string =>
  value === "customer_first" ? "Khách hàng trước" : "Cá nhân trước";

const monthSortLabel = (value: MonthSort): string =>
  value === "newest" ? "Tháng mới trước" : "Tháng cũ trước";

const priceSortLabel = (value: PriceSort): string =>
  value === "asc" ? "Giá thấp - cao" : "Giá cao - thấp";

type ProfileSortOptions = {
  audienceSort: AudienceSort;
  monthSort: MonthSort;
  healthSort: HealthSort;
  priceSort: PriceSort;
};

const compareProfiles = (a: ProfileCard, b: ProfileCard, options: ProfileSortOptions): number => {
  const audienceComparison = compareAudience(a, b, options.audienceSort);
  if (audienceComparison !== 0) return audienceComparison;

  const monthComparison = compareMonth(a, b, options.monthSort);
  if (monthComparison !== 0) return monthComparison;

  const healthComparison = compareHealth(a, b, options.healthSort);
  if (healthComparison !== 0) return healthComparison;

  const priceComparison = comparePrice(a, b, options.priceSort);
  if (priceComparison !== 0) return priceComparison;

  return (a.label || a.email).localeCompare(b.label || b.email);
};

const compareAudience = (a: ProfileCard, b: ProfileCard, sort: AudienceSort): number => {
  if (!sort) return 0;
  const order = sort === "customer_first" ? ["customer", "personal"] : ["personal", "customer"];
  return order.indexOf(normalizeAudience(a.audience)) - order.indexOf(normalizeAudience(b.audience));
};

const compareHealth = (a: ProfileCard, b: ProfileCard, sort: HealthSort): number => {
  if (!sort) return 0;
  const rank = (profile: ProfileCard) => normalizeHealth(profile.healthStatus) === "ok" ? 0 : 1;
  return rank(a) - rank(b);
};

const compareMonth = (a: ProfileCard, b: ProfileCard, sort: MonthSort): number => {
  if (!sort) return 0;
  const aTime = Date.parse(a.createdAtISO || "") || 0;
  const bTime = Date.parse(b.createdAtISO || "") || 0;
  return sort === "newest" ? bTime - aTime : aTime - bTime;
};

const comparePrice = (a: ProfileCard, b: ProfileCard, sort: PriceSort): number => {
  if (!sort) return 0;
  return sort === "asc" ? a.price - b.price : b.price - a.price;
};

export default App;
