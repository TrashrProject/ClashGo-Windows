import { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import Sidebar from './components/Sidebar';
import Dashboard from './components/Dashboard';
import HomeView from './components/HomeView';
import DeveloperView from './components/DeveloperView';
import LicenseGate from './components/LicenseGate';
import Analytics from './components/Analytics';
import ConfigView from './components/ConfigView';
import SettingsView from './components/SettingsView';
import AccountView from './components/AccountView';
import { EventsOn } from '../wailsjs/runtime';
import {
  GetStats,
  GetAttackHistory,
  GetLatestAttackReplay,
  GetSessionReport,
  GetActivity,
  GetLogs,
  SaveConfig,
  StartBot,
  StopBot,
  IsRunning,
  ResetStats,
  GetConfig,
  GetStrategies,
  GetSystemDiagnostics,
  GetStartupReadiness,
  GetLatestBootReport,
  ExportDiagnostics,
  SetBlueStacksInstance,
  GetUpdateStatus,
  GetAppVersion,
  CheckForUpdate,
  DownloadUpdate,
  ApplyUpdate,
  InstallAndRestart,
  SkipCurrentVersion,
  ClearSkippedVersion,
  GetAccountConfig,
  GetLicenseState,
  GetMemberInterfaceLevel,
  SetSimpleMode,
  StartTestSession,
  StartQuickTestSession,
  GetPlayerProfile,
  GetVillageResourceHistory,
  SaveMemberInterfaceLevel,
} from '../wailsjs/go/main/App';
import { bot } from '../wailsjs/go/models';
import { InterfaceLevel, TabType, UpdateStatus, DEFAULT_UPDATE_STATUS, SystemDiagnostics, VillageResourceSnapshot, ActivityEvent, AttackReplayView, SessionReportView, BotStats, AttackReport } from './types';
import UpdateBanner from './components/UpdateBanner';
import './App.css';

/**
 * Defensive wrapper around the Wails-generated `EventsOn` that returns
 * a no-op unsubscribe when the Wails runtime bridge isn't available.
 *
 * Why this exists
 * ─────────────────────────────────────────────────────────────────
 * The generated `web/wailsjs/runtime/runtime.js` short-circuits via
 * `window.runtime?.EventsOnMultiple(eventName, callback)` — but when
 * `window.runtime` itself is undefined (e.g. running the React app
 * directly in Chrome via `npm run dev`, or a Wails WkWebView that
 * hasn't received its bridge yet), the inner `EventsOnMultiple`
 * function reads `.EventsOnMultiple` off `undefined` and throws an
 * `Uncaught TypeError`. That throw propagates out of the synchronous
 * useEffect body, React unmounts the whole `<App/>` (no error
 * boundary in the original tree), the `#root` container becomes
 * empty, `WebviewIsTransparent: true` lets Wails' dark-zinc
 * BackgroundColour (#09090b) show through, and the user reports a
 * "black screen" with no actionable signal.
 *
 * Wrapping EventsOn at the call site keeps the defensive measure
 * scoped to where it's needed and avoids introducing a class
 * component (the codebase uses React.FC + React.memo throughout).
 */
function safeEventsOn(
  eventName: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  callback: (...args: any[]) => void,
): () => void {
  try {
    return EventsOn(eventName, callback);
  } catch {
    // Wails runtime bridge isn't ready yet (development outside wails
    // dev, or the bridge hasn't injected). Return a no-op unsubscribe
    // so the cleanup chain in the useEffect still runs cleanly.
    return () => {};
  }
}

const friendlyBotErrorMessage = (value: string): string => {
  const raw = value.trim();
  const text = raw.toLowerCase();

  if (text.includes('bluestacks 5 was not detected') || text.includes('bluestacks player')) {
    return 'BlueStacks 5 n’est pas détecté. Vérifie son installation puis ouvre Paramètres > État Windows.';
  }
  if (text.includes('adb was not detected') || text.includes('adb executable') || text.includes('adb not found')) {
    return 'ADB n’est pas détecté. ClashGO peut utiliser Android platform-tools ou le HD-Adb de BlueStacks.';
  }
  if (text.includes('no bluestacks instance') || text.includes('preferred instance')) {
    return 'Aucune instance BlueStacks n’est disponible. Lance ton instance une fois puis réessaie.';
  }
  if (text.includes('license has expired') || text.includes('license expired')) {
    return 'Ta licence ClashGO est expirée. Renouvelle-la puis actualise ta licence dans Mon ClashGO.';
  }
  if (text.includes('license is invalid') || text.includes('license is invalid or revoked')) {
    return 'Ta licence ClashGO est invalide ou désactivée.';
  }
  if (text.includes('already activated on another machine') || text.includes('machine mismatch')) {
    return 'Cette licence est liée à un autre PC. Une réinitialisation de machine est nécessaire.';
  }
  if (text.includes('runtime assets missing')) {
    return 'Des fichiers nécessaires à ClashGO sont manquants. Ouvre Paramètres > État Windows pour voir lesquels.';
  }
  if (text.includes('precision calibration') || text.includes('deployment calibration') || text.includes('calibration')) {
    return 'La calibration de déploiement n’est pas prête. Ouvre Paramètres > État Windows puis relance le pré-contrôle.';
  }
  if (text.includes('strategy') && (text.includes('not found') || text.includes('missing') || text.includes('unavailable'))) {
    return 'La stratégie d’attaque sélectionnée est introuvable. Ouvre Automatisation > Comportement et choisis une stratégie disponible.';
  }
  if (text.includes('army') && (text.includes('mismatch') || text.includes('not ready'))) {
    return 'L’armée détectée ne correspond pas encore au plan de farm. ClashGO attend une composition fiable avant d’attaquer.';
  }
  if (text.includes('army') && (text.includes('uncertain') || text.includes('confidence'))) {
    return 'ClashGO n’est pas assez sûr de la composition de l’armée. L’attaque est mise en attente plutôt que de prendre un risque.';
  }
  if (text.includes('session test active')) {
    return 'Une session test est en cours. Attends sa fin ou arrête-la proprement avant de modifier ces réglages.';
  }
  if (text.includes('account service unavailable') || text.includes('clashgo account service unavailable')) {
    return 'Le service de profil Clash est temporairement indisponible. Le bot peut continuer avec les données locales déjà enregistrées.';
  }
  if (text.includes('startup was cancelled') || text.includes('boot cancelled')) {
    return 'Le démarrage du bot a été annulé.';
  }

  return raw || 'Une erreur inconnue a empêché le démarrage du bot.';
};

const normalizeBotErrorMessage = (payload: unknown, fallback: string): string => {
  if (typeof payload === 'string' && payload.trim()) return friendlyBotErrorMessage(payload);
  if (payload && typeof payload === 'object' && 'message' in payload) {
    const message = (payload as { message?: unknown }).message;
    if (typeof message === 'string' && message.trim()) return friendlyBotErrorMessage(message);
  }
  return friendlyBotErrorMessage(fallback);
};

const getInitialDarkMode = (): boolean => {
  try {
    const stored = localStorage.getItem('darkMode');
    if (stored !== null) return stored === 'true';
    // Default to LIGHT, not OS preference.
    //
    // macOS's system appearance is dark by default, so honoring
    // `prefers-color-scheme: dark` here stacks dark on top of dark:
    // Tailwind `bg-zinc-950` (#09090b) over Wails' macOS dark window
    // chrome over the WkWebView's transparent layer. The visual result
    // reads as "black screen" to the user even though every component
    // is technically painted. The SettingsView toggle still round-trips
    // the user's explicit choice via `localStorage`.
    return false;
  } catch {
    return false;
  }
};

const getInitialInterfaceLevel = (): InterfaceLevel => {
  try {
    const stored = localStorage.getItem('interfaceLevel');
    if (stored === 'developer' || stored === 'advanced' || stored === 'simple') return stored;
  } catch {
    // Fall through to the newcomer-safe default.
  }
  return 'simple';
};

const getInitialSidebarExpanded = (): boolean => {
  try {
    const stored = localStorage.getItem('sidebarExpanded');
    if (stored !== null) return stored === 'true';
    return true;
  } catch {
    return true;
  }
};

const createEmptyStats = (): BotStats => new bot.BotStats({
  attacks_completed: 0,
  search_skips: 0,
  total_gold: 0,
  total_elixir: 0,
  total_de: 0,
  stars_0: 0,
  stars_1: 0,
  stars_2: 0,
  stars_3: 0,
  uptime: 0,
  cpu_time_sec: 0,
  cpu_cores: 0,
  recovery_attempts: 0,
  recovery_successes: 0,
  bluestacks_restarts: 0,
  adb_health: {
    last_capture: null,
    avg_capture_ms: 0,
    consecutive_fails: 0,
    captures_total: 0,
    errors_total: 0,
    last_error: "",
  },
}) as unknown as BotStats;

function App() {
  const [tab, setTab] = useState<TabType>('dashboard');
  const [accountPage, setAccountPage] = useState<'account' | 'settings' | 'village'>('account');
  const [stats, setStats] = useState<BotStats>(() => createEmptyStats());
  const [isRunning, setIsRunning] = useState(false);
  const [isStarting, setIsStarting] = useState(false);
  const [history, setHistory] = useState<AttackReport[]>([]);
  const [resourceHistory, setResourceHistory] = useState<VillageResourceSnapshot[]>([]);
  const [activity, setActivity] = useState<ActivityEvent[]>([]);
  const [replay, setReplay] = useState<AttackReplayView>({ available: false, complete: false, events: [] });
  const [sessionReport, setSessionReport] = useState<SessionReportView | null>(null);
  const [logs, setLogs] = useState<string[]>([]);
  const [adbPort, setAdbPort] = useState(5555);
  const [darkMode, setDarkMode] = useState(getInitialDarkMode);
  const [sidebarExpanded, setSidebarExpanded] = useState(getInitialSidebarExpanded);
  const [interfaceLevel, setInterfaceLevel] = useState<InterfaceLevel>(getInitialInterfaceLevel);
  const [playerTag, setPlayerTag] = useState('');
  const [accountReady, setAccountReady] = useState(false);
  const [licenseAccessReady, setLicenseAccessReady] = useState(false);
  const [licenseActivated, setLicenseActivated] = useState(false);
  const [licenseEnforced, setLicenseEnforced] = useState(true);
  const [licenseRole, setLicenseRole] = useState<'member' | 'developer' | 'admin' | ''>('');
  const [licenseMemberName, setLicenseMemberName] = useState('');
  const [licensePlan, setLicensePlan] = useState('');
  const [licenseExpiresAt, setLicenseExpiresAt] = useState('');

  // Updater state — pushed via `updater_status` event from Go.
  const [updateStatus, setUpdateStatus] = useState<UpdateStatus>(DEFAULT_UPDATE_STATUS);
  const [appVersion, setAppVersion] = useState('');
  const [systemDiagnostics, setSystemDiagnostics] = useState<SystemDiagnostics | null>(null);
  const [updateDismissed, setUpdateDismissed] = useState(false);
  const [botError, setBotError] = useState('');
  const [botDiagnosticPath, setBotDiagnosticPath] = useState('');
  const [memberNotice, setMemberNotice] = useState('');
  const [testSessionActive, setTestSessionActive] = useState(false);
  const [startupCheck, setStartupCheck] = useState<{
    ready: boolean;
    checks: Array<{ id: string; label: string; ok: boolean; blocking?: boolean; message: string; action?: string; action_label?: string }>;
  } | null>(null);
  const [latestBootReport, setLatestBootReport] = useState<{
    started_at?: string;
    completed_at?: string;
    outcome?: string;
    final_error?: string;
    suggested_action?: string;
    recovery_used?: string[];
    attempts?: number;
  } | null>(null);
  const [startupCheckRunning, setStartupCheckRunning] = useState(false);
  const startupCheckAutoRan = useRef(false);
  const startInFlightRef = useRef(false);
  const testSessionInFlightRef = useRef(false);

  // Config states
  const [goldThreshold, setGoldThreshold] = useState(400000);
  const [elixirThreshold, setElixirThreshold] = useState(400000);
  const [deThreshold, setDeThreshold] = useState(2000);
  const [selectedStrategy, setSelectedStrategy] = useState('default');
  const [strategies, setStrategiesList] = useState<string[]>([]);
  const [searchEnabled, setSearchEnabled] = useState(true);
  const [upgradeWalls, setUpgradeWalls] = useState(false);
  const [stallTimer, setStallTimer] = useState(30);
  const [lootExitEnabled, setLootExitEnabled] = useState(false);
  const [lootExitPercent, setLootExitPercent] = useState(100);
  const [simpleMode, setSimpleMode] = useState(true);

  const syncMemberScopedView = useCallback(async (activated: boolean) => {
    startupCheckAutoRan.current = false;
    setStartupCheck(null);
    if (!activated) {
      // Clear every member-scoped surface immediately. This prevents the next
      // license (or the activation screen) from briefly displaying the
      // previous member's counters, diagnostics or logs while its own state
      // is being restored.
      setPlayerTag('');
      setStats(createEmptyStats());
      setHistory([]);
      setResourceHistory([]);
      setActivity([]);
      setReplay({ available: false, complete: false, events: [] });
      setSessionReport(null);
      setLogs([]);
      setLatestBootReport(null);
      setBotError('');
      setBotDiagnosticPath('');
      setMemberNotice('');
      setTestSessionActive(false);
      return;
    }

    const [configResult, accountResult, statsResult, historyResult, resourceResult, activityResult, replayResult, reportResult, bootResult] = await Promise.allSettled([
      GetConfig(),
      GetAccountConfig(),
      GetStats(),
      GetAttackHistory(),
      GetVillageResourceHistory(),
      GetActivity(),
      GetLatestAttackReplay(),
      GetSessionReport(),
      GetLatestBootReport(),
    ]);

    if (configResult.status === 'fulfilled') {
      const conf = configResult.value;
      setGoldThreshold(conf.search.min_loot_gold);
      setElixirThreshold(conf.search.min_loot_elixir);
      setDeThreshold(conf.search.min_loot_de);
      setSearchEnabled(conf.search.enabled);
      setUpgradeWalls(conf.upgrade.upgrade_walls);
      setSelectedStrategy(conf.attack.strategy_file);
      setStallTimer(conf.attack.stall_timer_seconds);
      setLootExitEnabled(conf.attack.loot_exit_enabled ?? false);
      setLootExitPercent(conf.attack.loot_exit_percent ?? 100);
      setSimpleMode(conf.automation?.simple_mode ?? true);
    }
    if (accountResult.status === 'fulfilled') {
      setPlayerTag(accountResult.value?.player_tag || '');
    }
    if (statsResult.status === 'fulfilled') {
      setStats(statsResult.value as unknown as BotStats);
    }
    if (historyResult.status === 'fulfilled') {
      setHistory((historyResult.value ?? []) as unknown as AttackReport[]);
    }
    if (resourceResult.status === 'fulfilled') {
      setResourceHistory((resourceResult.value ?? []) as VillageResourceSnapshot[]);
    }
    if (activityResult.status === 'fulfilled') {
      setActivity((activityResult.value ?? []) as unknown as ActivityEvent[]);
    }
    if (replayResult.status === 'fulfilled') {
      setReplay((replayResult.value ?? { available: false, complete: false, events: [] }) as unknown as AttackReplayView);
    }
    if (reportResult.status === 'fulfilled') {
      const report = reportResult.value as unknown as SessionReportView;
      setSessionReport(report && (report.attacks || 0) > 0 ? report : null);
    }
    if (bootResult.status === 'fulfilled') {
      setLatestBootReport((bootResult.value || null) as typeof latestBootReport);
    }
  }, []);

  const handleLicenseReady = useCallback((state: { activated: boolean; role?: string; member_name?: string; plan?: string; expires_at?: string }, policy: { enforced: boolean }) => {
    const role = state?.role === 'admin'
      ? 'admin'
      : state?.role === 'developer'
        ? 'developer'
        : state?.activated
          ? 'member'
          : '';
    setLicenseActivated(Boolean(state?.activated));
    setLicenseEnforced(Boolean(policy.enforced));
    setLicenseRole(role);
    setLicenseMemberName(state?.member_name || '');
    setLicensePlan(state?.plan || '');
    setLicenseExpiresAt(state?.expires_at || '');

    if (role === 'developer' || role === 'admin') {
      setInterfaceLevel('developer');
    } else if (state?.activated) {
      void GetMemberInterfaceLevel()
        .then((saved: unknown) => {
          const level: InterfaceLevel = saved === 'advanced' ? 'advanced' : 'simple';
          setInterfaceLevel(level);
        })
        .catch(() => {
          setInterfaceLevel((current) => current === 'developer' ? 'simple' : current);
        });
      setTab((current) => current === 'developer' ? 'dashboard' : current);
    } else {
      setInterfaceLevel((current) => current === 'developer' ? 'simple' : current);
      setTab((current) => current === 'developer' ? 'dashboard' : current);
    }

    void syncMemberScopedView(Boolean(state?.activated));

    if (!policy.enforced || state?.activated) {
      setLicenseAccessReady(true);
    }
  }, [syncMemberScopedView]);

  const handleInterfaceLevelChange = useCallback((level: InterfaceLevel) => {
    setInterfaceLevel(level);
    if (level !== 'developer') {
      void SaveMemberInterfaceLevel(level).catch((err: unknown) => {
        console.warn('Failed to save member interface level:', err);
      });
    }
  }, []);

  useEffect(() => {
    const init = async () => {
      try {
        const [conf, running, strats, account] = await Promise.all([
          GetConfig(),
          IsRunning(),
          GetStrategies(),
          GetAccountConfig()
        ]);

        setGoldThreshold(conf.search.min_loot_gold);
        setElixirThreshold(conf.search.min_loot_elixir);
        setDeThreshold(conf.search.min_loot_de);
        setSearchEnabled(conf.search.enabled);
        setUpgradeWalls(conf.upgrade.upgrade_walls);
        setSelectedStrategy(conf.attack.strategy_file);
        setStallTimer(conf.attack.stall_timer_seconds);
        setLootExitEnabled(conf.attack.loot_exit_enabled ?? false);
        setLootExitPercent(conf.attack.loot_exit_percent ?? 100);
        const configuredSimpleMode = conf.automation?.simple_mode ?? true;
        setSimpleMode(configuredSimpleMode);
        setIsRunning(running);
        setIsStarting(false);
        // Never let a null from the Go side reach the Config page — a
        // nil slice marshals to JSON null, and ConfigView dereferences
        // `strategies.length`. `?? []` keeps the UI resilient even if a
        // future binding regresses to null.
        setStrategiesList(strats ?? []);
        setAdbPort(conf.device.adb_port);
        setPlayerTag(account?.player_tag || '');

        // Zero-config startup: if the user linked a tag previously, refresh
        // the public profile immediately in the background. This also lets
        // the Go backend auto-select the matching farm HDV before the user
        // ever opens the Account page.
        if (account?.player_tag) {
          void GetPlayerProfile().catch((err: unknown) => {
            console.warn('Background account sync failed:', err);
          });
        }
      } catch (err) {
        console.error('Init failed:', err);
      } finally {
        setAccountReady(true);
      }

      // Pull the embedded app version + initial updater snapshot.
      try {
        const v = await GetAppVersion();
        setAppVersion(v);
      } catch (err) {
        console.warn('GetAppVersion failed:', err);
      }
      try {
        const s = await GetUpdateStatus();
        setUpdateStatus(s);
      } catch (err) {
        console.warn('GetUpdateStatus failed:', err);
      }
      try {
        const license = await GetLicenseState();
        if (license?.activated && (license.role === 'developer' || license.role === 'admin')) {
          setLicenseActivated(true);
          setLicenseRole(license.role);
          setLicenseMemberName(license.member_name || '');
          setLicensePlan(license.plan || '');
          setLicenseExpiresAt(license.expires_at || '');
          setInterfaceLevel('developer');
        } else if (license?.activated) {
          setLicenseActivated(true);
          setLicenseRole('member');
          setLicenseMemberName(license.member_name || '');
          setLicensePlan(license.plan || '');
          setLicenseExpiresAt(license.expires_at || '');
          try {
            const savedLevel = await GetMemberInterfaceLevel();
            setInterfaceLevel(savedLevel === 'advanced' ? 'advanced' : 'simple');
          } catch {
            setInterfaceLevel('simple');
          }
        } else {
          setLicenseActivated(false);
          setLicenseRole('');
          setLicenseMemberName('');
          setLicensePlan('');
          setLicenseExpiresAt('');
          setInterfaceLevel((current) => current === 'developer' ? 'simple' : current);
        }
      } catch (err) {
        console.warn('GetLicenseState failed:', err);
      }
    };
    init();

    const fetchFastData = async () => {
      try {
        const [s, a] = await Promise.all([
          GetStats(),
          GetActivity(),
        ]);
        setStats(s as unknown as BotStats);
        setActivity((a ?? []) as unknown as ActivityEvent[]);
      } catch (err) {
        console.error('Fast data fetch failed:', err);
      }
    };

    const fetchHistory = async () => {
      try {
        const h = await GetAttackHistory();
        setHistory((h ?? []) as unknown as AttackReport[]);
      } catch (err) {
        console.warn('Attack history refresh failed:', err);
      }
    };

    const fetchLogs = async () => {
      try {
        const l = await GetLogs();
        setLogs(l ?? []);
      } catch (err) {
        console.warn('Log refresh failed:', err);
      }
    };

    const fetchResourceHistory = async () => {
      try {
        const rh = await GetVillageResourceHistory();
        setResourceHistory((rh ?? []) as VillageResourceSnapshot[]);
      } catch (err) {
        console.warn('Resource history refresh failed:', err);
      }
    };

    const fetchReplay = async () => {
      try {
        const latest = await GetLatestAttackReplay();
        setReplay((latest ?? { available: false, complete: false, events: [] }) as unknown as AttackReplayView);
      } catch (err) {
        console.warn('Attack replay refresh failed:', err);
      }
    };

    const fetchSessionReport = async () => {
      try {
        const report = await GetSessionReport();
        const typed = report as unknown as SessionReportView;
        setSessionReport(typed && (typed.attacks || 0) > 0 ? typed : null);
      } catch (err) {
        console.warn('Session report refresh failed:', err);
      }
    };

    void fetchFastData();
    void fetchHistory();
    void fetchLogs();
    void fetchResourceHistory();
    void fetchReplay();
    void fetchSessionReport();

    // Keep the high-frequency poll tiny: only live counters + compact activity.
    // History is event-driven at attack completion, logs do not need 2 Hz, and
    // replay changes only after an attack. This removes avoidable Wails IPC and
    // JSON work while the bot is farming.
    const fastInterval = setInterval(fetchFastData, 2000);
    const logInterval = setInterval(fetchLogs, 4000);
    const historyInterval = setInterval(fetchHistory, 30000); // recovery fallback
    const resourceInterval = setInterval(fetchResourceHistory, 15000);
    const replayInterval = setInterval(fetchReplay, 30000); // recovery fallback
    const sessionReportInterval = setInterval(fetchSessionReport, 30000); // cold-start/stop fallback

    const fetchDiagnostics = async () => {
      try {
        const d = await GetSystemDiagnostics();
        setSystemDiagnostics(d as SystemDiagnostics);
      } catch (err) {
        console.warn('GetSystemDiagnostics failed:', err);
      }
    };
    fetchDiagnostics();
    const diagnosticsInterval = setInterval(fetchDiagnostics, 5000);

    // The 1 Hz screenshot poll used to live here. It moved into
    // <Feed/>'s own useEffect so it only runs when the Live View tab
    // is mounted, instead of burning the WailsIPC bridge every second
    // regardless of tab. The Live View tab has been removed entirely
    // (see the 'feed' tab deletion in Sidebar.tsx / types.ts).

    // Subscribe to updater_status events pushed every 2s from Go.
    // `safeEventsOn` wraps the runtime bridge so a missing
    // `window.runtime` (e.g. `npm run dev` opened in Chrome directly,
    // or a WkWebView whose bridge hasn't injected yet) doesn't throw
    // out of this synchronous useEffect body — that throw propagates
    // and unmounts the React tree, which under `WebviewIsTransparent:
    // true` presents as the dark-zinc window frame.
    const unsubUpdater = safeEventsOn("updater_status", (data: UpdateStatus) => {
      if (data && typeof data === 'object') {
        setUpdateStatus(data);
      }
    });

    const unsubLicense = safeEventsOn("license_state", (payload: { activated?: boolean; role?: string; member_name?: string; plan?: string; expires_at?: string; error?: string }) => {
      const role = payload?.role === 'admin'
        ? 'admin'
        : payload?.role === 'developer'
          ? 'developer'
          : payload?.activated
            ? 'member'
            : '';
      setLicenseActivated(Boolean(payload?.activated));
      setLicenseRole(role);
      setLicenseMemberName(payload?.member_name || '');
      setLicensePlan(payload?.plan || '');
      setLicenseExpiresAt(payload?.expires_at || '');
      setLicenseAccessReady(Boolean(payload?.activated));
      startupCheckAutoRan.current = false;
      setStartupCheck(null);
      void syncMemberScopedView(Boolean(payload?.activated));
      if (payload?.activated) {
        void GetStartupReadiness()
          .then((result: unknown) => {
            setStartupCheck(result as unknown as {
              ready: boolean;
              checks: Array<{ id: string; label: string; ok: boolean; blocking?: boolean; message: string; action?: string; action_label?: string }>;
            });
            startupCheckAutoRan.current = true;
          })
          .catch(() => {
            startupCheckAutoRan.current = false;
          });
      }

      if (role === 'developer' || role === 'admin') {
        setInterfaceLevel('developer');
      } else if (payload?.activated) {
        void GetMemberInterfaceLevel()
          .then((saved: unknown) => setInterfaceLevel(saved === 'advanced' ? 'advanced' : 'simple'))
          .catch(() => setInterfaceLevel('simple'));
        setTab((current) => current === 'developer' ? 'dashboard' : current);
      } else {
        setInterfaceLevel((current) => current === 'developer' ? 'simple' : current);
        setTab((current) => current === 'developer' ? 'dashboard' : current);
      }

    });

    // StartBot returns running=true immediately while the boot runs in
    // the background (BlueStacks launch + ADB connect can take minutes
    // on a cold start). When the boot fails, Go emits bot_error /
    // bot_init_failed — without listening, the sidebar stays on
    // "STOP BOT" forever and Stop becomes a confusing no-op (there's
    // no bot to stop). Flip the button back to START on either event.
    const refreshBootReport = () => {
      window.setTimeout(() => {
        void GetLatestBootReport()
          .then((report: unknown) => setLatestBootReport((report || null) as typeof latestBootReport))
          .catch(() => {});
      }, 250);
    };

    const unsubBotError = safeEventsOn("bot_error", (payload: unknown) => {
      setTestSessionActive(false);
      setIsStarting(false);
      setIsRunning(false);
      setBotError(normalizeBotErrorMessage(payload, 'Le bot n’a pas pu démarrer.'));
      refreshBootReport();
    });
    const unsubBotInitFailed = safeEventsOn("bot_init_failed", (payload: unknown) => {
      setTestSessionActive(false);
      setIsStarting(false);
      setIsRunning(false);
      setBotError(normalizeBotErrorMessage(payload, 'L’initialisation BlueStacks / ADB a échoué.'));
      refreshBootReport();
    });
    const unsubBotStarted = safeEventsOn("bot_started", () => {
      setIsStarting(false);
      setIsRunning(true);
      setBotError('');
      refreshBootReport();
    });
    const unsubBotStopped = safeEventsOn("bot_stopped", () => {
      setTestSessionActive(false);
      setIsStarting(false);
      setIsRunning(false);
      setBotError('');
      void fetchFastData();
      void fetchHistory();
      void fetchSessionReport();
    });
    const unsubMemberTestRestored = safeEventsOn("member_test_session_restored", (payload: unknown) => {
      setTestSessionActive(false);
      const message = normalizeBotErrorMessage(payload, 'Session test terminée · tes réglages personnels ont été restaurés.');
      setMemberNotice(message || 'Session test terminée · tes réglages personnels ont été restaurés.');
      window.setTimeout(() => setMemberNotice(''), 8000);
    });

    const unsubBotBootCancelled = safeEventsOn("bot_boot_cancelled", (payload: unknown) => {
      setIsStarting(false);
      setIsRunning(false);
      setBotError(normalizeBotErrorMessage(payload, 'Le démarrage du bot a été annulé.'));
      refreshBootReport();
    });

    const unsubAttackHistory = safeEventsOn("attack_history_updated", (payload: unknown) => {
      if (Array.isArray(payload)) {
        setHistory(payload as unknown as AttackReport[]);
      }
      // Deployment traces and session aggregates are ready at the attack
      // boundary, so refresh them event-driven instead of adding hot polling.
      void fetchReplay();
      void fetchSessionReport();
    });
    const unsubStatsUpdated = safeEventsOn("stats_updated", (payload: bot.BotStats) => {
      if (payload && typeof payload === 'object') {
        setStats(payload as unknown as BotStats);
      }
    });

    return () => {
      clearInterval(fastInterval);
      clearInterval(logInterval);
      clearInterval(historyInterval);
      clearInterval(resourceInterval);
      clearInterval(replayInterval);
      clearInterval(sessionReportInterval);
      clearInterval(diagnosticsInterval);
      unsubUpdater();
      unsubLicense();
      unsubBotError();
      unsubBotInitFailed();
      unsubBotStarted();
      unsubBotStopped();
      unsubMemberTestRestored();
      unsubBotBootCancelled();
      unsubAttackHistory();
      unsubStatsUpdated();
    };
  }, [syncMemberScopedView]);

  useEffect(() => {
    if (darkMode) {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }
    try {
      localStorage.setItem('darkMode', String(darkMode));
    } catch (e) {
      console.warn('Failed to save darkMode preference:', e);
    }
  }, [darkMode]);

  useEffect(() => {
    try {
      localStorage.setItem('sidebarExpanded', String(sidebarExpanded));
    } catch (e) {
      console.warn('Failed to save sidebarExpanded preference:', e);
    }
  }, [sidebarExpanded]);

  useEffect(() => {
    try {
      localStorage.setItem('interfaceLevel', interfaceLevel);
    } catch (e) {
      console.warn('Failed to save interface level:', e);
    }
    if (tab === 'developer' && licenseRole !== 'developer' && licenseRole !== 'admin') {
      setTab('dashboard');
      return;
    }
    // Settings stays hidden from the Simple sidebar, but may be opened
    // contextually from Home when ClashGO detects a Windows/ADB problem.
    if (interfaceLevel === 'simple' && (tab === 'activity' || tab === 'analytics' || tab === 'developer')) {
      setTab('dashboard');
    }
  }, [interfaceLevel, tab, licenseRole]);

  const saveSettings = async () => {
    await SaveConfig(
      goldThreshold,
      elixirThreshold,
      deThreshold,
      upgradeWalls,
      selectedStrategy,
      searchEnabled,
      stallTimer,
      lootExitEnabled,
      lootExitPercent,
    );
  };

  const handleStartupCheck = useCallback(async () => {
    if (startupCheckRunning) return;
    setStartupCheckRunning(true);
    try {
      const result = await GetStartupReadiness();
      setStartupCheck(result as unknown as {
        ready: boolean;
        checks: Array<{ id: string; label: string; ok: boolean; blocking?: boolean; message: string; action?: string; action_label?: string }>;
      });
    } catch (err) {
      console.warn('Startup readiness check failed:', err);
      setStartupCheck({
        ready: false,
        checks: [{
          id: 'internal',
          label: 'Diagnostic',
          ok: false,
          message: 'Le pré-contrôle n’a pas pu être exécuté.',
        }],
      });
    } finally {
      setStartupCheckRunning(false);
    }
  }, [startupCheckRunning]);

  useEffect(() => {
    if (!accountReady || !licenseAccessReady || startupCheckAutoRan.current) return;
    startupCheckAutoRan.current = true;
    void handleStartupCheck();
  }, [accountReady, licenseAccessReady, handleStartupCheck]);

  useEffect(() => {
    if (!accountReady || !licenseAccessReady || isRunning || isStarting) return;

    let cancelled = false;
    const refreshQuietly = async () => {
      try {
        const result = await GetStartupReadiness();
        if (cancelled) return;
        setStartupCheck(result as unknown as {
          ready: boolean;
          checks: Array<{ id: string; label: string; ok: boolean; blocking?: boolean; message: string; action?: string; action_label?: string }>;
        });
        startupCheckAutoRan.current = true;
      } catch {
        // Keep the last known preflight state. The explicit "Tout vérifier"
        // action still surfaces an error if the user asks for a manual check.
      }
    };

    const timer = window.setInterval(() => {
      void refreshQuietly();
    }, 15_000);

    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [accountReady, licenseAccessReady, isRunning, isStarting]);

  const refreshStartupReadiness = useCallback(async () => {
    startupCheckAutoRan.current = true;
    setStartupCheckRunning(true);
    try {
      const result = await GetStartupReadiness();
      setStartupCheck(result as unknown as {
        ready: boolean;
        checks: Array<{ id: string; label: string; ok: boolean; blocking?: boolean; message: string; action?: string; action_label?: string }>;
      });
    } catch (err) {
      console.warn('Startup readiness refresh failed:', err);
      setStartupCheck(null);
      startupCheckAutoRan.current = false;
    } finally {
      setStartupCheckRunning(false);
    }
  }, []);

  const handleStart = async () => {
    if (startInFlightRef.current || isRunning || isStarting) return;
    startInFlightRef.current = true;
    setBotError('');
    setBotDiagnosticPath('');
    try {
      setStartupCheckRunning(true);
      const readiness = await GetStartupReadiness();
      const typedReadiness = readiness as unknown as {
        ready: boolean;
        checks: Array<{ id: string; label: string; ok: boolean; blocking?: boolean; message: string; action?: string; action_label?: string }>;
      };
      setStartupCheck(typedReadiness);
      setStartupCheckRunning(false);

      if (!typedReadiness.ready) {
        const firstBlocked = typedReadiness.checks.find((check) => !check.ok && check.blocking !== false);
        const blockedMessage = firstBlocked
          ? `${firstBlocked.label} : ${firstBlocked.message}`
          : 'La configuration ClashGO n’est pas prête.';
        setBotError(friendlyBotErrorMessage(blockedMessage));
        return;
      }

      const res = await StartBot(goldThreshold, elixirThreshold, deThreshold, upgradeWalls, searchEnabled);
      if (res.running) {
        // "running=true" from StartBot means the asynchronous boot was
        // accepted, not that the runtime is already active. Keep the UI in
        // STARTING until Go emits bot_started after b.Start() succeeds.
        setIsStarting(true);
        setIsRunning(false);
      } else {
        setIsStarting(false);
        setIsRunning(false);
        if (res.message) {
          setBotError(friendlyBotErrorMessage(res.message));
        }
      }
    } catch (err) {
      console.error('Start failed:', err);
      setStartupCheckRunning(false);
      setIsStarting(false);
      setIsRunning(false);
      setBotError(friendlyBotErrorMessage(err instanceof Error ? err.message : String(err)));
    } finally {
      startInFlightRef.current = false;
    }
  };

  const handleStartQuickTestSession = async () => {
    if (testSessionInFlightRef.current || startInFlightRef.current || isRunning || isStarting) return;
    testSessionInFlightRef.current = true;
    setBotError('');
    setBotDiagnosticPath('');
    try {
      setStartupCheckRunning(true);
      const readiness = await GetStartupReadiness();
      const typedReadiness = readiness as unknown as {
        ready: boolean;
        checks: Array<{ id: string; label: string; ok: boolean; blocking?: boolean; message: string; action?: string; action_label?: string }>;
      };
      setStartupCheck(typedReadiness);
      setStartupCheckRunning(false);

      if (!typedReadiness.ready) {
        const firstBlocked = typedReadiness.checks.find((check) => !check.ok && check.blocking !== false);
        setBotError(friendlyBotErrorMessage(firstBlocked
          ? `${firstBlocked.label} : ${firstBlocked.message}`
          : 'La configuration ClashGO n’est pas prête.'));
        return;
      }

      const res = await StartQuickTestSession(goldThreshold, elixirThreshold, deThreshold, upgradeWalls, searchEnabled);
      await syncMemberScopedView(true);

      if (res.running) {
        setTestSessionActive(true);
        setIsStarting(true);
        setIsRunning(false);
      } else {
        setIsStarting(false);
        setIsRunning(false);
        if (res.message) setBotError(friendlyBotErrorMessage(res.message));
      }
    } catch (err) {
      console.error('Quick test session start failed:', err);
      setStartupCheckRunning(false);
      setIsStarting(false);
      setIsRunning(false);
      setBotError(friendlyBotErrorMessage(err instanceof Error ? err.message : String(err)));
    } finally {
      testSessionInFlightRef.current = false;
    }
  };

  const handleStartTestSession = async () => {
    if (testSessionInFlightRef.current || startInFlightRef.current || isRunning || isStarting) return;
    testSessionInFlightRef.current = true;
    setBotError('');
    setBotDiagnosticPath('');
    try {
      setStartupCheckRunning(true);
      const readiness = await GetStartupReadiness();
      const typedReadiness = readiness as unknown as {
        ready: boolean;
        checks: Array<{ id: string; label: string; ok: boolean; blocking?: boolean; message: string; action?: string; action_label?: string }>;
      };
      setStartupCheck(typedReadiness);
      setStartupCheckRunning(false);

      if (!typedReadiness.ready) {
        const firstBlocked = typedReadiness.checks.find((check) => !check.ok && check.blocking !== false);
        setBotError(friendlyBotErrorMessage(firstBlocked
          ? `${firstBlocked.label} : ${firstBlocked.message}`
          : 'La configuration ClashGO n’est pas prête.'));
        return;
      }

      const res = await StartTestSession(goldThreshold, elixirThreshold, deThreshold, upgradeWalls, searchEnabled);
      await syncMemberScopedView(true);

      if (res.running) {
        setTestSessionActive(true);
        setIsStarting(true);
        setIsRunning(false);
      } else {
        setIsStarting(false);
        setIsRunning(false);
        if (res.message) setBotError(friendlyBotErrorMessage(res.message));
      }
    } catch (err) {
      console.error('Test session start failed:', err);
      setStartupCheckRunning(false);
      setIsStarting(false);
      setIsRunning(false);
      setBotError(friendlyBotErrorMessage(err instanceof Error ? err.message : String(err)));
    } finally {
      testSessionInFlightRef.current = false;
    }
  };

  const handleStop = async () => {
    try {
      const res = await StopBot();
      setTestSessionActive(false);
      setIsStarting(false);
      setIsRunning(res.running);
    } catch (err) {
      console.error('Stop failed:', err);
    }
  };

  const handleSetBlueStacksInstance = async (instance: string): Promise<void> => {
    await SetBlueStacksInstance(instance);
    const d = await GetSystemDiagnostics();
    setSystemDiagnostics(d as SystemDiagnostics);
    await refreshStartupReadiness();
  };

  const handleExportDiagnostics = async (): Promise<string> => {
    try {
      return await ExportDiagnostics();
    } catch (err) {
      console.error('ExportDiagnostics failed:', err);
      throw err;
    }
  };

  const handleBotDiagnosticExport = async () => {
    setBotDiagnosticPath('');
    try {
      const path = await handleExportDiagnostics();
      setBotDiagnosticPath(path);
    } catch {
      setBotDiagnosticPath('Échec de l’export du diagnostic — consulte la console système.');
    }
  };

  const handleReset = async () => {
    try {
      await ResetStats();
      const s = await GetStats();
      setStats(s as unknown as BotStats);
    } catch (err) {
      console.error('Reset failed:', err);
    }
  };

  // --- Updater handlers (bound to Wails methods on App) ---
  const handleUpdaterCheck = async () => {
    try {
      const s = await CheckForUpdate();
      setUpdateStatus(s);
      setUpdateDismissed(false);
    } catch (err) {
      console.error('CheckForUpdate failed:', err);
    }
  };
  const handleUpdaterDownload = async (): Promise<string> => {
    // Returns the local path; UpdateStatus will reflect via the
    // 2s event ticker (state will flip to 'ready').
    const path = await DownloadUpdate();
    const s = await GetUpdateStatus();
    setUpdateStatus(s);
    return path;
  };
  const handleUpdaterApply = async () => {
    await ApplyUpdate();
  };
  const handleUpdaterOneClick = async () => {
    // The Go-side InstallAndRestart takes care of stopping the bot,
    // saving stats, marking the restarting state, spawning the helper,
    // and exiting the process after a 1s IPC flush window.
    await InstallAndRestart();
    // The status will flip to 'restarting' and the React side will
    // switch to the non-dismissible splash automatically via the
    // updater_status event listener.
  };
  const handleUpdaterSkip = async () => {
    await SkipCurrentVersion();
    const s = await GetUpdateStatus();
    setUpdateStatus(s);
  };
  const handleUpdaterClearSkip = async () => {
    await ClearSkippedVersion();
    const s = await GetUpdateStatus();
    setUpdateStatus(s);
  };

  const dashboardProps = useMemo(() => ({
    stats,
    history,
    activity,
    replay,
    sessionReport,
    logs,
  }), [stats, history, activity, replay, sessionReport, logs]);

  // ADB connection state — drives the header status pill. Labels stop
  // calling the local ADB server "localhost:{port}" because the bot
  // can legitimately attach to remote devices via `adb connect
  // <ip>:{port}`; the port number is just the local ADB server's
  // listen port, not the device host regardless.
  const adbState: 'connected' | 'degraded' | 'disconnected' | 'awaiting' =
    !isRunning
      ? 'awaiting'
      : stats.adb_health.consecutive_fails === 0
        ? 'connected'
        : stats.adb_health.consecutive_fails < 5
          ? 'degraded'
          : 'disconnected';
  const adbStateLabel =
    adbState === 'connected'
      ? 'Connecté'
      : adbState === 'degraded'
        ? 'Dégradé'
        : adbState === 'disconnected'
          ? 'Déconnecté'
          : 'En attente';

  const licenseExpiryNotice = useMemo(() => {
    if (!licenseActivated || !licenseExpiresAt) return null;
    const expiry = Date.parse(licenseExpiresAt);
    if (!Number.isFinite(expiry)) return null;
    const remainingMs = expiry - Date.now();
    if (remainingMs <= 0) {
      return { urgent: true, text: 'Ta licence ClashGO est expirée.' };
    }
    const remainingHours = Math.ceil(remainingMs / (60 * 60 * 1000));
    if (remainingHours <= 72) {
      const text = remainingHours <= 24
        ? 'Ta licence ClashGO expire aujourd’hui.'
        : `Ta licence ClashGO expire dans ${Math.ceil(remainingHours / 24)} jours.`;
      return { urgent: remainingHours <= 24, text };
    }
    return null;
  }, [licenseActivated, licenseExpiresAt]);

  const windowsPreflightReady = useMemo(() => (
    systemDiagnostics
      ? Boolean(
          systemDiagnostics.assets_ready &&
          systemDiagnostics.emulator?.adb_found &&
          systemDiagnostics.emulator?.bluestacks_player_found &&
          systemDiagnostics.emulator?.preferred_instance
        )
      : false
  ), [systemDiagnostics]);

  // The backend pre-control is the source of truth whenever available.
  // This keeps the sidebar button aligned with the exact checks StartBot will
  // enforce (license, runtime assets, BlueStacks/ADB, strategy and member
  // pacing). The Clash account remains advisory and never blocks startup.
  const blockingStartupCheck = startupCheck?.checks.find((check) => !check.ok && check.blocking !== false);
  const startReady = startupCheck
    ? startupCheck.ready
    : licenseAccessReady && windowsPreflightReady;
  const startBlockedReason = blockingStartupCheck
    ? `${blockingStartupCheck.label} : ${blockingStartupCheck.message}`
    : !licenseAccessReady
      ? 'Active ta licence dans Mon ClashGO.'
      : !systemDiagnostics
        ? 'Vérification de l’environnement Windows en cours…'
        : !windowsPreflightReady
          ? 'Vérifie BlueStacks et ADB dans Paramètres > État Windows.'
          : '';

  const readinessIssues = useMemo(() => {
    if (!systemDiagnostics) return [] as string[];
    const issues: string[] = [];
    if (!systemDiagnostics.assets_ready) {
      const missing = systemDiagnostics.missing_assets ?? [];
      issues.push(missing.length
        ? `Fichiers ClashGO manquants : ${missing.slice(0, 3).join(', ')}`
        : 'Certains fichiers nécessaires à ClashGO sont manquants.');
    }
    if (!systemDiagnostics.emulator?.bluestacks_player_found) {
      issues.push('BlueStacks 5 n’est pas détecté.');
    }
    if (!systemDiagnostics.emulator?.adb_found) {
      issues.push('ADB n’est pas détecté.');
    }
    if (systemDiagnostics.emulator?.bluestacks_player_found && !systemDiagnostics.emulator?.bluestacks_running) {
      issues.push('BlueStacks est installé mais ne semble pas démarré.');
    }
    if (!systemDiagnostics.emulator?.preferred_instance) {
      issues.push('Aucune instance BlueStacks utilisable n’est sélectionnée.');
    }
    if (systemDiagnostics.emulator?.adb_setting_present && !systemDiagnostics.emulator?.adb_enabled) {
      issues.push('ADB est désactivé dans la configuration BlueStacks.');
    }
    return issues;
  }, [systemDiagnostics]);

  const tabTitle: Record<TabType, string> = {
    dashboard: 'Accueil',
    config: 'Automatisation',
    account: 'Mon ClashGO',
    activity: 'Activité',
    analytics: 'Statistiques',
    settings: 'Paramètres',
    developer: licenseRole === 'admin' ? 'Administration' : 'Support',
  };

  const configProps = useMemo(() => ({
    goldThreshold, setGoldThreshold,
    elixirThreshold, setElixirThreshold,
    deThreshold, setDeThreshold,
    selectedStrategy, setSelectedStrategy,
    strategies,
    searchEnabled, setSearchEnabled,
    upgradeWalls, setUpgradeWalls,
    stallTimer, setStallTimer,
    lootExitEnabled, setLootExitEnabled,
    lootExitPercent, setLootExitPercent,
    simpleMode,
    testSessionActive,
    onSetSimpleMode: async (enabled: boolean) => {
      const level: InterfaceLevel = enabled ? 'simple' : 'advanced';
      // The Go automation mode is authoritative. Reflect it immediately after
      // the backend commit succeeds; the interface-level preference is a
      // secondary convenience and must never leave the UI showing the opposite
      // runtime mode if its own persistence fails.
      await SetSimpleMode(enabled);
      setSimpleMode(enabled);
      if (interfaceLevel !== 'developer') {
        setInterfaceLevel(level);
      }
      try {
        await SaveMemberInterfaceLevel(level);
      } catch (err) {
        console.warn('Automation mode changed but interface preference could not be saved:', err);
      }
      await refreshStartupReadiness();
    },
    onSave: async () => {
      // Errors intentionally bubble so ConfigView's save-status
      // indicator can show a red "Save failed" pill back to the user.
      // Previously this catch swallowed the error and only logged it,
      // which made save feel broken when SaveConfig (the Wails IPC)
      // rejected (e.g. backend down, malformed payload).
      await saveSettings();
      await refreshStartupReadiness();
    }
  }), [
    goldThreshold, elixirThreshold, deThreshold,
    selectedStrategy, strategies, searchEnabled, upgradeWalls, stallTimer,
    lootExitEnabled, lootExitPercent, simpleMode, testSessionActive, refreshStartupReadiness
  ]);

  if (!licenseAccessReady) {
    return <LicenseGate onReady={handleLicenseReady} />;
  }

  return (
    <div className="app-shell bg-zinc-50 dark:bg-zinc-950 text-zinc-950 dark:text-zinc-50 transition-colors duration-500" style={{ display: 'flex', width: '100vw', height: '100vh' }}>
      <Sidebar 
        tab={tab}
        interfaceLevel={interfaceLevel}
        setTab={setTab}
        expanded={sidebarExpanded}
        setExpanded={setSidebarExpanded}
        running={isRunning}
        starting={isStarting}
        onStart={handleStart}
        onStop={handleStop}
        licenseActivated={licenseActivated}
        licenseRole={licenseRole}
        memberName={licenseMemberName}
        licensePlan={licensePlan}
        startReady={startReady}
        startBlockedReason={startBlockedReason}
      />

      <main 
        className={`flex-1 bg-zinc-50 dark:bg-zinc-950 transition-[margin-left,background-color] duration-300 ease-[cubic-bezier(0.4,0,0.2,1)] min-h-screen overflow-y-auto ${sidebarExpanded ? 'ml-64' : 'ml-20'}`}
      >
        <div className="draggable sticky top-0 left-0 right-0 h-12 z-40 bg-transparent pointer-events-auto" />
        <div className="max-w-[1600px] mx-auto p-4 md:p-6 lg:p-8 xl:p-12 pt-0 -mt-12">
          <header className="mb-8 flex justify-between items-end draggable">
            <div className="space-y-1">
              <div className="flex items-center gap-3">
                <div className="flex gap-1">
                  <span className={`w-1.5 h-1.5 rounded-full ${isRunning ? 'bg-emerald-500 animate-pulse' : 'bg-zinc-300 dark:bg-zinc-800'}`}></span>
                  <span className="w-1.5 h-1.5 bg-zinc-300 dark:bg-zinc-800 rounded-full"></span>
                </div>
                <h2 className="text-[11px] text-zinc-500 dark:text-zinc-500 uppercase tracking-[0.4em] font-black">ClashGO</h2>
              </div>
              <h1 className="font-headline text-5xl font-bold tracking-tight text-zinc-950 dark:text-white">{tabTitle[tab]}</h1>
            </div>
            <div className="flex gap-4 items-center">
              {!updateDismissed && (
                <UpdateBanner
                  status={updateStatus}
                  appVersion={appVersion}
                  isBotRunning={isRunning || isStarting}
                  onCheckNow={handleUpdaterCheck}
                  onDownload={handleUpdaterDownload}
                  onApply={handleUpdaterApply}
                  onUpdateAndRestart={handleUpdaterOneClick}
                  onSkip={handleUpdaterSkip}
                  onClearSkip={handleUpdaterClearSkip}
                  onDismiss={() => setUpdateDismissed(true)}
                />
              )}
              <div className="bg-white dark:bg-zinc-900 px-6 py-3.5 rounded-2xl border border-zinc-100/50 dark:border-zinc-800/50 flex items-center gap-4 shadow-premium dark:shadow-none no-drag backdrop-blur-md" title={`ADB server port ${adbPort} — ${adbStateLabel}`}>
                <div className="relative">
                  <div className={`w-2.5 h-2.5 rounded-full ${
                    adbState === 'connected' ? 'bg-emerald-500'
                    : adbState === 'disconnected' ? 'bg-rose-500 animate-pulse'
                    : 'bg-amber-500 animate-pulse'
                  }`}></div>
                  {adbState === 'connected' && <div className="absolute inset-0 w-2.5 h-2.5 rounded-full bg-emerald-500 animate-ping opacity-20"></div>}
                </div>
                <span className={`text-[11px] font-black uppercase tracking-widest ${
                  adbState === 'connected' ? 'text-zinc-500 dark:text-zinc-400'
                  : adbState === 'disconnected' ? 'text-rose-500'
                  : 'text-amber-600 dark:text-amber-400'
                }`}>ADB: {adbPort} · {adbStateLabel}</span>
              </div>
            </div>
          </header>

          {licenseExpiryNotice && (
            <section className={
              'mb-4 rounded-2xl border px-4 py-3 ' +
              (licenseExpiryNotice.urgent
                ? 'border-rose-200 bg-rose-50 text-rose-700 dark:border-rose-900/50 dark:bg-rose-950/30 dark:text-rose-300'
                : 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-300')
            }>
              <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-3">
                  <span className="material-symbols-outlined text-lg">{licenseExpiryNotice.urgent ? 'error' : 'schedule'}</span>
                  <div>
                    <div className="text-xs font-black uppercase tracking-wider">Licence</div>
                    <div className="mt-0.5 text-sm font-semibold">{licenseExpiryNotice.text}</div>
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => setTab('account')}
                  className="shrink-0 rounded-xl border border-current/20 px-4 py-2 text-[10px] font-black uppercase tracking-widest"
                >
                  Mon ClashGO
                </button>
              </div>
            </section>
          )}

          {memberNotice && (
            <section className="mb-4 no-drag rounded-2xl border border-emerald-500/30 bg-emerald-500/10 px-5 py-3 text-emerald-700 dark:text-emerald-300" role="status">
              <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-3">
                  <span className="material-symbols-outlined text-lg">task_alt</span>
                  <div>
                    <div className="text-[9px] font-black uppercase tracking-[0.2em]">Session test</div>
                    <div className="mt-0.5 text-sm font-semibold">{memberNotice}</div>
                  </div>
                </div>
                <button
                  type="button"
                  onClick={() => setMemberNotice('')}
                  className="rounded-xl p-2 text-emerald-600/70 transition hover:bg-emerald-500/10 hover:text-emerald-700 dark:text-emerald-300"
                  aria-label="Fermer le message"
                >
                  <span className="material-symbols-outlined text-lg">close</span>
                </button>
              </div>
            </section>
          )}

          {botError && (
            <section className="mb-6 no-drag rounded-2xl border border-rose-500/30 bg-rose-500/10 px-5 py-4 shadow-sm" role="alert">
              <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                <div className="min-w-0">
                  <div className="flex items-center gap-2 text-rose-600 dark:text-rose-400">
                    <span className="material-symbols-outlined text-lg">error</span>
                    <span className="text-[10px] font-black uppercase tracking-[0.22em]">Démarrage du bot impossible</span>
                  </div>
                  <p className="mt-1 break-words text-sm font-semibold text-zinc-800 dark:text-zinc-200">{botError}</p>
                  {botDiagnosticPath && (
                    <p className="mt-2 break-all text-[10px] font-mono text-zinc-500 dark:text-zinc-400">{botDiagnosticPath}</p>
                  )}
                </div>
                <div className="flex shrink-0 flex-wrap gap-2">
                  <button
                    type="button"
                    onClick={() => setTab('settings')}
                    className="rounded-xl border border-zinc-300/70 bg-white px-4 py-2 text-[10px] font-black uppercase tracking-widest text-zinc-700 transition hover:bg-zinc-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-200 dark:hover:bg-zinc-800"
                  >
                    État Windows
                  </button>
                  <button
                    type="button"
                    onClick={() => void handleBotDiagnosticExport()}
                    className="rounded-xl bg-rose-600 px-4 py-2 text-[10px] font-black uppercase tracking-widest text-white transition hover:bg-rose-500"
                  >
                    Exporter le diagnostic
                  </button>
                  <button
                    type="button"
                    onClick={() => { setBotError(''); setBotDiagnosticPath(''); }}
                    className="rounded-xl px-3 py-2 text-zinc-500 transition hover:bg-rose-500/10 hover:text-rose-600 dark:text-zinc-400"
                    aria-label="Fermer l’erreur de démarrage"
                  >
                    <span className="material-symbols-outlined text-lg">close</span>
                  </button>
                </div>
              </div>
            </section>
          )}

          {tab === 'dashboard' && (
            <HomeView
              stats={stats}
              history={history}
              activity={activity}
              sessionReport={sessionReport}
              testSessionActive={testSessionActive}
              running={isRunning}
              starting={isStarting}
              onStart={handleStart}
              onStartTestSession={handleStartTestSession}
              onStartQuickTestSession={handleStartQuickTestSession}
              onStop={handleStop}
              onOpenAutomation={() => setTab('config')}
              onOpenAccount={() => {
                setAccountPage('account');
                setTab('account');
              }}
              onOpenMemberSettings={() => {
                setAccountPage('settings');
                setTab('account');
              }}
              onOpenVillage={() => {
                setAccountPage('village');
                setTab('account');
              }}
              onOpenSettings={() => setTab('settings')}
              licenseReady={licenseActivated}
              licenseRequired={licenseEnforced}
              memberName={licenseMemberName}
              licensePlan={licensePlan}
              licenseExpiresAt={licenseExpiresAt}
              accountLinked={Boolean(playerTag)}
              windowsReady={systemDiagnostics ? windowsPreflightReady : null}
              readinessIssues={readinessIssues}
              startupCheck={startupCheck}
              startupCheckRunning={startupCheckRunning}
              onRunStartupCheck={() => void handleStartupCheck()}
              latestBootReport={latestBootReport}
            />
          )}
          {tab === 'activity' && <Dashboard {...dashboardProps} />}
          {tab === 'account' && (
            <AccountView
              playerTag={playerTag}
              interfaceLevel={interfaceLevel}
              initialPage={accountPage}
              testSessionActive={testSessionActive}
              onInterfaceLevelChange={handleInterfaceLevelChange}
              onAccountChanged={(tag) => {
                setPlayerTag(tag);
                if (tag) setTab('account');
              }}
              onReadinessChanged={() => { void refreshStartupReadiness(); }}
            />
          )}
          {tab === 'analytics' && <Analytics stats={stats} resourceHistory={resourceHistory} history={history as any} />}
          {tab === 'config' && <ConfigView {...configProps} />}
          {tab === 'developer' && <DeveloperView />}
          {tab === 'settings' && (
            <SettingsView
              stats={stats}
              isRunning={isRunning}
              isStarting={isStarting}
              adbPort={adbPort}
              darkMode={darkMode}
              setDarkMode={setDarkMode}
              onResetStats={handleReset}
              appVersion={appVersion}
              updateStatus={updateStatus}
              onCheckUpdates={handleUpdaterCheck}
              onClearSkip={handleUpdaterClearSkip}
              systemDiagnostics={systemDiagnostics}
              onExportDiagnostics={handleExportDiagnostics}
              onSetBlueStacksInstance={handleSetBlueStacksInstance}
            />
          )}
        </div>
      </main>

    </div>
  );
}

export default App;
