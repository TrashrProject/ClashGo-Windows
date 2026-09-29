import React from 'react';
import { BotStats, UpdateStatus, SystemDiagnostics } from '../types';
import { GetLatestAttackTrace, GetLatestBootReport } from '../../wailsjs/go/main/App';

type BootReportView = {
  started_at?: string;
  completed_at?: string;
  outcome?: string;
  final_error?: string;
  suggested_action?: string;
  recovery_used?: string[];
  attempts?: number;
  steps?: Array<{
    name?: string;
    result?: string;
    detail?: string;
    duration_ns?: number;
  }>;
};

const friendlyBootAction = (value?: string): string => {
  const raw = String(value || '').trim();
  const text = raw.toLowerCase();
  if (!raw) return 'Relance le démarrage. ClashGO réessaiera automatiquement les récupérations les plus sûres.';
  if (text.includes('adb') && text.includes('enabled')) return 'Vérifie que BlueStacks est lancé et que l’accès ADB est activé.';
  if (text.includes('restart bluestacks') || text.includes('relaunch bluestacks')) return 'Redémarre BlueStacks puis relance ClashGO.';
  if (text.includes('wait') || text.includes('initializing')) return 'Attends quelques secondes que BlueStacks termine son démarrage puis réessaie.';
  if (text.includes('clash of clans') || text.includes('startapp')) return 'Ouvre Clash of Clans une fois dans BlueStacks puis relance le bot.';
  if (text.includes('screen') || text.includes('capture')) return 'Vérifie que BlueStacks affiche bien le village puis réessaie.';
  return raw;
};

interface SettingsViewProps {
  stats: BotStats;
  isRunning: boolean;
  isStarting: boolean;
  adbPort: number;
  darkMode: boolean;
  setDarkMode: (val: boolean) => void;
  onResetStats: () => void;
  appVersion: string;
  updateStatus: UpdateStatus;
  onCheckUpdates: () => void;
  onClearSkip: () => void;
  systemDiagnostics: SystemDiagnostics | null;
  onExportDiagnostics: () => Promise<string>;
  onSetBlueStacksInstance: (instance: string) => Promise<void>;
}

const SettingsView: React.FC<SettingsViewProps> = React.memo(({
  stats, isRunning, isStarting, adbPort, darkMode, setDarkMode, onResetStats,
  appVersion, updateStatus, onCheckUpdates, onClearSkip, systemDiagnostics, onExportDiagnostics, onSetBlueStacksInstance,
}) => {
  // Destructive action protection: the first click only ARMS the reset
  // (visual shift + "click again" prompt); a second click within 4s
  // actually fires it. Prevents fat-finger stat wipes.
  const [resetArmed, setResetArmed] = React.useState(false);
  const [diagnosticsPath, setDiagnosticsPath] = React.useState('');
  const [diagnosticsBusy, setDiagnosticsBusy] = React.useState(false);
  const [instanceBusy, setInstanceBusy] = React.useState(false);
  const [instanceMessage, setInstanceMessage] = React.useState('');
  const [traceOpen, setTraceOpen] = React.useState(false);
  const [latestTrace, setLatestTrace] = React.useState('');
  const [traceBusy, setTraceBusy] = React.useState(false);
  const [bootReport, setBootReport] = React.useState<BootReportView | null>(null);
  const [settingsPage, setSettingsPage] = React.useState<'general' | 'windows' | 'diagnostic'>(() => {
    try {
      const saved = localStorage.getItem('clashgo_settings_page');
      return saved === 'windows' || saved === 'diagnostic' ? saved : 'general';
    } catch {
      return 'general';
    }
  });
  const resetTimerRef = React.useRef<number | null>(null);

  React.useEffect(() => {
    try { localStorage.setItem('clashgo_settings_page', settingsPage); } catch {}
  }, [settingsPage]);

  React.useEffect(() => () => {
    if (resetTimerRef.current) window.clearTimeout(resetTimerRef.current);
  }, []);

  React.useEffect(() => {
    let active = true;
    const load = async () => {
      try {
        const report = await GetLatestBootReport();
        if (active) setBootReport((report || null) as BootReportView | null);
      } catch {
        if (active) setBootReport(null);
      }
    };
    void load();
    const id = window.setInterval(load, 10000);
    return () => {
      active = false;
      window.clearInterval(id);
    };
  }, []);

  const handleInstanceChange = async (instance: string) => {
    if (instanceBusy || isRunning || isStarting) return;
    setInstanceBusy(true);
    setInstanceMessage('');
    try {
      await onSetBlueStacksInstance(instance);
      setInstanceMessage(instance ? 'Instance enregistrée' : 'Sélection automatique activée');
    } catch (err) {
      setInstanceMessage(err instanceof Error ? err.message : String(err));
    } finally {
      setInstanceBusy(false);
    }
  };

  const handleExportDiagnostics = async () => {
    if (diagnosticsBusy) return;
    setDiagnosticsBusy(true);
    try {
      const path = await onExportDiagnostics();
      setDiagnosticsPath(path);
    } catch {
      setDiagnosticsPath('Échec de l’export — consulte le journal app.log');
    } finally {
      setDiagnosticsBusy(false);
    }
  };

  const handleLoadTrace = async () => {
    if (traceBusy) return;
    setTraceBusy(true);
    try {
      const trace = await GetLatestAttackTrace();
      setLatestTrace(trace || '');
      setTraceOpen(true);
    } catch {
      setLatestTrace('');
      setTraceOpen(true);
    } finally {
      setTraceBusy(false);
    }
  };

  const handleResetClick = () => {
    if (isRunning || isStarting) return;
    if (!resetArmed) {
      setResetArmed(true);
      resetTimerRef.current = window.setTimeout(() => setResetArmed(false), 4000);
      return;
    }
    if (resetTimerRef.current) window.clearTimeout(resetTimerRef.current);
    setResetArmed(false);
    onResetStats();
  };

  const preferredInstance = systemDiagnostics?.emulator.instances?.find((i) => i.preferred);
  const runtimeReady = systemDiagnostics?.assets_ready ?? false;
  const playerReady = systemDiagnostics?.emulator.bluestacks_player_found ?? false;
  const adbReady = systemDiagnostics?.emulator.adb_found ?? false;
  const overallReady = runtimeReady && playerReady && adbReady && !!preferredInstance;

  return (
    <div className="bg-white dark:bg-zinc-900 p-6 md:p-8 rounded-[2.5rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none max-w-5xl mx-auto transition-all duration-500">

      <div className="flex justify-between items-center mb-12">
        <div>
          <h3 className="text-2xl font-bold text-zinc-950 dark:text-white mb-2 tracking-tight">Paramètres système</h3>
          <p className="text-sm text-zinc-500 dark:text-zinc-500 font-medium">État de l’application, de Windows et de la connexion BlueStacks.</p>
        </div>
        <div className="w-14 h-14 rounded-2xl bg-zinc-50 dark:bg-zinc-800 flex items-center justify-center border border-zinc-100 dark:border-zinc-700 shadow-sm transition-colors">
          <span className="material-symbols-outlined text-zinc-500 dark:text-zinc-500 text-2xl">memory</span>
        </div>
      </div>

      <div className="mb-6 inline-flex rounded-2xl border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950/40 p-1.5">
        {([
          ['general', 'Général', 'settings'],
          ['windows', 'Windows', 'desktop_windows'],
          ['diagnostic', 'Diagnostic', 'monitor_heart'],
        ] as const).map(([id, label, icon]) => (
          <button
            key={id}
            type="button"
            onClick={() => setSettingsPage(id)}
            className={
              'flex items-center gap-2 rounded-xl px-4 py-2.5 text-[10px] font-black uppercase tracking-[0.16em] transition ' +
              (settingsPage === id
                ? 'bg-zinc-950 text-white dark:bg-white dark:text-zinc-950'
                : 'text-zinc-500 hover:text-zinc-950 dark:hover:text-white')
            }
          >
            <span className="material-symbols-outlined text-base">{icon}</span>
            {label}
          </button>
        ))}
      </div>

      <div className="space-y-4">
        <div className={(settingsPage === 'windows' ? '' : 'hidden ') + "bg-zinc-950 dark:bg-black text-white p-6 rounded-2xl border border-zinc-800 shadow-xl"}>
          <div className="flex items-center justify-between mb-5">
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-500">État Windows</div>
              <div className="text-lg font-black mt-1">{overallReady ? 'Prêt à démarrer' : 'Configuration requise'}</div>
            </div>
            <div className={`w-3 h-3 rounded-full ${overallReady ? 'bg-emerald-400 shadow-[0_0_14px_rgba(52,211,153,.7)]' : 'bg-amber-400 animate-pulse'}`}></div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            {[
              { label: 'Fichiers nécessaires', ok: runtimeReady, value: runtimeReady ? 'Prêt' : `${systemDiagnostics?.missing_assets?.length ?? 0} manquant(s)` },
              { label: 'BlueStacks 5', ok: playerReady, value: playerReady ? 'Détecté' : 'Introuvable' },
              { label: 'ADB', ok: adbReady, value: adbReady ? 'Détecté' : 'Introuvable' },
              { label: 'Instance', ok: !!preferredInstance, value: preferredInstance ? `${preferredInstance.name} · ${preferredInstance.adb_port}` : 'Non détectée' },
              { label: 'BlueStacks lancé', ok: systemDiagnostics?.emulator.bluestacks_running ?? false, value: systemDiagnostics?.emulator.bluestacks_running ? 'Lancé' : 'Arrêté' },
              { label: 'Accès ADB', ok: systemDiagnostics?.emulator.adb_enabled ?? false, value: systemDiagnostics?.emulator.adb_enabled ? 'Activé' : (systemDiagnostics?.emulator.adb_setting_present ? 'Désactivé · correction auto au démarrage' : 'Vérifier les réglages BlueStacks') },
            ].map((item) => (
              <div key={item.label} className="rounded-xl border border-zinc-800 bg-zinc-900/70 p-3 min-w-0">
                <div className="flex items-center gap-2">
                  <div className={`w-1.5 h-1.5 rounded-full shrink-0 ${item.ok ? 'bg-emerald-400' : 'bg-amber-400'}`}></div>
                  <span className="text-[9px] font-black uppercase tracking-wider text-zinc-500 truncate">{item.label}</span>
                </div>
                <div className="text-xs font-bold mt-1.5 truncate" title={item.value}>{item.value}</div>
              </div>
            ))}
          </div>
          {systemDiagnostics && !runtimeReady && systemDiagnostics.missing_assets.length > 0 && (
            <div className="mt-4 rounded-xl border border-amber-500/20 bg-amber-500/10 px-4 py-3 text-[10px] font-mono text-amber-300 break-words">
              Manquant : {systemDiagnostics.missing_assets.join(', ')}
            </div>
          )}

          {/* Automatique selection is the default. Manual instance choice is
              intentionally tucked away so normal users never need to touch it. */}
          <details className="mt-4 rounded-xl border border-zinc-800 bg-zinc-900/70 overflow-hidden">
            <summary className="cursor-pointer list-none p-4 flex items-center justify-between gap-4">
              <div className="min-w-0">
                <div className="text-[9px] font-black uppercase tracking-[0.2em] text-zinc-500">Connexion avancée</div>
                <div className="text-[11px] text-zinc-300 mt-1">
                  Instance BlueStacks · {systemDiagnostics?.configured_instance
                    ? `Manuelle : ${systemDiagnostics.configured_instance}`
                    : `Automatique : ${systemDiagnostics?.emulator.preferred_instance || 'détection…'}`}
                </div>
              </div>
              <span className="material-symbols-outlined text-zinc-500">tune</span>
            </summary>
            <div className="border-t border-zinc-800 p-4">
              <div className="flex items-center justify-between gap-4">
                <div className="text-[11px] text-zinc-400 max-w-[250px]">
                  Laisse sur Automatique sauf si ClashGO a détecté la mauvaise instance BlueStacks.
                </div>
                <select
                  value={systemDiagnostics?.configured_instance || ''}
                  disabled={instanceBusy || isRunning || isStarting || !systemDiagnostics}
                  onChange={(e) => handleInstanceChange(e.target.value)}
                  className="max-w-[220px] rounded-xl border border-zinc-700 bg-zinc-950 px-3 py-2 text-xs font-bold text-white outline-none focus:border-emerald-500 disabled:opacity-50"
                  aria-label="Sélection de l’instance BlueStacks"
                >
                  <option value="">Automatique</option>
                  {(systemDiagnostics?.emulator.instances ?? []).map((inst) => (
                    <option key={inst.name} value={inst.name}>
                      {inst.name} · ADB {inst.adb_port}
                    </option>
                  ))}
                </select>
              </div>
              {(isRunning || isStarting) && (
                <div className="mt-2 text-[9px] font-medium text-amber-400">
                  Arrête ClashGO avant de changer d’instance BlueStacks.
                </div>
              )}
              {instanceMessage && !(isRunning || isStarting) && (
                <div className="mt-2 text-[9px] font-medium text-zinc-400">{instanceMessage}</div>
              )}
            </div>
          </details>
        </div>

        {settingsPage === 'windows' && bootReport?.outcome === 'failed' && (
          <div className="rounded-2xl border border-amber-500/20 bg-amber-500/10 p-5">
            <div className="flex items-start gap-4">
              <div className="grid h-11 w-11 shrink-0 place-items-center rounded-xl bg-amber-500/10">
                <span className="material-symbols-outlined text-amber-500">build_circle</span>
              </div>
              <div className="min-w-0 flex-1">
                <div className="text-[9px] font-black uppercase tracking-[0.2em] text-amber-500">Dernier démarrage interrompu</div>
                <div className="mt-1 text-sm font-black text-zinc-950 dark:text-white">
                  {friendlyBootAction(bootReport.suggested_action)}
                </div>
                <div className="mt-2 flex flex-wrap gap-2 text-[9px] font-bold uppercase tracking-wider text-zinc-500">
                  <span>{bootReport.attempts || 0} tentative(s)</span>
                  {(bootReport.recovery_used || []).length > 0 && (
                    <span>· récupération auto utilisée</span>
                  )}
                </div>
                <details className="mt-3">
                  <summary className="cursor-pointer text-[10px] font-black uppercase tracking-widest text-zinc-500">
                    Voir le détail technique
                  </summary>
                  <div className="mt-3 rounded-xl bg-zinc-950 px-4 py-3 text-[10px] font-mono text-zinc-300 break-words">
                    {bootReport.final_error || 'Aucune erreur détaillée enregistrée.'}
                  </div>
                </details>
              </div>
            </div>
          </div>
        )}

        {/* Sombre Mode Toggle */}
        <button
          type="button"
          role="switch"
          aria-checked={darkMode}
          onClick={() => setDarkMode(!darkMode)}
          className={(settingsPage === 'general' ? '' : 'hidden ') + "w-full flex justify-between items-center bg-zinc-50/50 dark:bg-zinc-800/30 p-6 rounded-2xl border border-zinc-100/50 dark:border-zinc-800/50 hover:bg-white dark:hover:bg-zinc-800/60 hover:shadow-premium dark:hover:shadow-none transition-all duration-300 group cursor-pointer text-left"}
        >
          <div className="flex items-center gap-5">
            <div className="w-12 h-12 rounded-xl bg-white dark:bg-zinc-800 flex items-center justify-center border border-zinc-100 dark:border-zinc-700 group-hover:scale-105 transition-all duration-300 shadow-sm">
              <span className="material-symbols-outlined text-xl text-zinc-500 dark:text-zinc-500">
                {darkMode ? 'dark_mode' : 'light_mode'}
              </span>
            </div>
            <div className="flex flex-col">
              <span className="text-[10px] font-black text-zinc-500 dark:text-zinc-500 uppercase tracking-[0.2em] mb-0.5">Thème de l’application</span>
              <span className="text-sm font-bold text-zinc-950 dark:text-white">{darkMode ? 'Sombre' : 'Clair'}</span>
            </div>
          </div>
          <div 
            className={`w-12 h-6 rounded-full p-1 transition-all duration-500 ease-in-out relative ${darkMode ? 'bg-zinc-700' : 'bg-zinc-200'}`}
          >
            <div className={`w-4 h-4 rounded-full bg-white dark:bg-zinc-400 transition-all duration-500 ease-in-out shadow-md ${darkMode ? 'translate-x-6' : 'translate-x-0'}`}></div>
          </div>
        </button>

        <details className={(settingsPage === 'diagnostic' ? '' : 'hidden ') + "rounded-2xl border border-zinc-100 dark:border-zinc-800 bg-zinc-50/40 dark:bg-zinc-950/20 overflow-hidden"}>
          <summary className="cursor-pointer list-none flex items-center justify-between gap-4 p-5">
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.2em] text-zinc-500">Mesures techniques</div>
              <div className="mt-1 text-sm font-bold text-zinc-950 dark:text-white">ADB, captures, CPU et récupération</div>
              <div className="mt-1 text-[10px] text-zinc-400">À ouvrir uniquement pour le diagnostic ou le support.</div>
            </div>
            <span className="material-symbols-outlined text-zinc-400">expand_more</span>
          </summary>
          <div className="border-t border-zinc-100 dark:border-zinc-800 p-4 space-y-3">
            {[
              { label: 'État de connexion', value: stats.adb_health.consecutive_fails === 0 ? 'Optimal' : 'Interrompu', status: stats.adb_health.consecutive_fails === 0 ? 'success' : 'error', icon: 'hub', detail: stats.adb_health.last_error },
              { label: 'Port ADB', value: adbPort.toString(), status: 'info', icon: 'router' },
              { label: 'Latence capture', value: isNaN(stats.adb_health.avg_capture_ms) ? '0ms' : `${stats.adb_health.avg_capture_ms.toFixed(1)}ms`, status: stats.adb_health.avg_capture_ms < 200 ? 'success' : 'info', icon: 'speed' },
              { label: 'Captures réussies', value: stats.adb_health.captures_total > 0 ? `${((stats.adb_health.captures_total / Math.max(1, stats.adb_health.captures_total + stats.adb_health.errors_total)) * 100).toFixed(1)}%` : '—', status: stats.adb_health.errors_total === 0 ? 'success' : 'info', icon: 'monitoring' },
              { label: 'Erreurs ADB', value: stats.adb_health.errors_total.toLocaleString(), status: stats.adb_health.consecutive_fails > 0 ? 'error' : 'success', icon: 'error' },
              // cpu_time_sec is device-independent (absolute CPU seconds since
              // start). cpu_cores is a fraction of one core; scaled by the host's
              // logical core count only to render a familiar 0-100% number.
              { label: 'Temps CPU', value: `${stats.cpu_time_sec.toFixed(1)}s`, status: 'info', icon: 'schedule' },
              { label: 'Utilisation CPU', value: isNaN(stats.cpu_cores) ? '0%' : `${(stats.cpu_cores * (navigator.hardwareConcurrency || 1) * 100).toFixed(1)}%`, status: stats.cpu_cores < 0.5 ? 'success' : 'info', icon: 'memory' },
              { label: 'Récupérations réussies', value: stats.recovery_attempts > 0 ? `${stats.recovery_successes}/${stats.recovery_attempts}` : '0/0', status: stats.recovery_attempts === stats.recovery_successes ? 'success' : 'info', icon: 'healing' },
              { label: 'Redémarrages BlueStacks', value: stats.bluestacks_restarts.toLocaleString(), status: stats.bluestacks_restarts === 0 ? 'success' : 'info', icon: 'restart_alt' },
            ].map((item, i) => (

            <div key={i} className="flex justify-between items-center bg-zinc-50/50 dark:bg-zinc-800/30 p-6 rounded-2xl border border-zinc-100/50 dark:border-zinc-800/50 hover:bg-white dark:hover:bg-zinc-800/60 hover:shadow-premium dark:hover:shadow-none transition-all duration-300 group">
              <div className="flex items-center gap-5">
                <div className="w-12 h-12 rounded-xl bg-white dark:bg-zinc-900 flex items-center justify-center border border-zinc-100 dark:border-zinc-800 group-hover:scale-105 transition-all duration-300 shadow-sm">
                  <span className="material-symbols-outlined text-xl text-zinc-500 dark:text-zinc-500">{item.icon}</span>
                </div>
                <div className="flex flex-col min-w-0">
                  <span className="text-[10px] font-black text-zinc-500 dark:text-zinc-500 uppercase tracking-[0.2em] mb-0.5">{item.label}</span>
                  <span className={`text-sm font-bold tracking-tight ${item.status === 'error' ? 'text-rose-600 dark:text-rose-400' : 'text-zinc-950 dark:text-white'}`}>{item.value}</span>
                  {item.detail && (
                    <span className="mt-1 text-[10px] font-mono text-rose-500/70 truncate max-w-[220px]" title={item.detail}>{item.detail}</span>
                  )}
                </div>
              </div>
              <div className="flex items-center gap-3">
                 <div className={`w-2 h-2 rounded-full ${
                   item.status === 'success' ? 'bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.4)]' :
                   item.status === 'error' ? 'bg-rose-500 animate-pulse shadow-[0_0_8px_rgba(244,63,94,0.4)]' : 'bg-zinc-300 dark:bg-zinc-600'
                 }`}></div>
              </div>
            </div>
        ))}
          </div>
        </details>

        {/* Mise à jour row — surfaces current version + a manual check
            button so users can force a refresh without waiting for the
            6h background poller. Rendered as a keyboard-accessible
            div[role=button] because it contains a real <button>
            (Réactiver les notifications) — nesting buttons would be
            invalid HTML. */}
        <div
          role="button"
          tabIndex={0}
          onClick={onCheckUpdates}
          onKeyDown={(e) => {
            // Ignore keydowns originating from the nested "Resume
            // notifications" button — otherwise pressing Space/Enter
            // there would also trigger a manual update check.
            if (e.target !== e.currentTarget) return;
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault();
              onCheckUpdates();
            }
          }}
          className={(settingsPage === 'general' ? '' : 'hidden ') + "w-full flex justify-between items-center bg-zinc-50/50 dark:bg-zinc-800/30 p-6 rounded-2xl border border-zinc-100/50 dark:border-zinc-800/50 hover:bg-white dark:hover:bg-zinc-800/60 hover:shadow-premium dark:hover:shadow-none transition-all duration-300 group cursor-pointer text-left"}
        >
          <div className="flex items-center gap-5">
            <div className="w-12 h-12 rounded-xl bg-white dark:bg-zinc-900 flex items-center justify-center border border-zinc-100 dark:border-zinc-800 group-hover:scale-105 transition-all duration-300 shadow-sm">
              <span className="material-symbols-outlined text-xl text-zinc-500 dark:text-zinc-500">system_update</span>
            </div>
            <div className="flex flex-col">
              <span className="text-[10px] font-black text-zinc-500 dark:text-zinc-500 uppercase tracking-[0.2em] mb-0.5">
                Version de l’application
              </span>
              <span className="text-sm font-bold tracking-tight text-zinc-950 dark:text-white tabular-nums">
                v{appVersion || '0.0.0'}
                {updateStatus.available && (
                  <span className="ml-3 text-[10px] font-black uppercase tracking-widest text-emerald-500">
                    Mise à jour {updateStatus.latest_version} disponible
                  </span>
                )}
              </span>
            </div>
          </div>
          <div className="flex items-center gap-3">
            {updateStatus.skip_version && (
              <button
                onClick={(e) => { e.stopPropagation(); onClearSkip(); }}
                className="text-[10px] font-black text-zinc-500 dark:text-zinc-500 uppercase tracking-[0.2em] hover:text-zinc-700 dark:hover:text-zinc-300 transition-colors"
                title={`Réactiver les notifications pour v${updateStatus.skip_version}`}
              >
                Réactiver les notifications
              </button>
            )}
            <span className="material-symbols-outlined text-zinc-300 dark:text-zinc-700 group-hover:translate-x-1 transition-transform">refresh</span>
          </div>
        </div>


        <div className={(settingsPage === 'diagnostic' ? '' : 'hidden ') + "rounded-2xl border border-zinc-100/60 dark:border-zinc-800/60 bg-zinc-50/40 dark:bg-zinc-950/20 overflow-hidden"}>
          <button
            type="button"
            onClick={() => setTraceOpen(v => !v)}
            className="w-full flex items-center justify-between gap-4 p-5 text-left"
          >
            <div className="flex items-center gap-4">
              <div className="w-11 h-11 rounded-xl bg-white dark:bg-zinc-900 border border-zinc-100 dark:border-zinc-800 flex items-center justify-center">
                <span className="material-symbols-outlined text-zinc-500">bug_report</span>
              </div>
              <div>
                <div className="text-[10px] font-black uppercase tracking-[0.2em] text-zinc-500">Diagnostic avancé</div>
                <div className="text-sm font-bold text-zinc-950 dark:text-white">Dernière trace d’attaque</div>
                <div className="text-[10px] text-zinc-400 mt-0.5">Utile uniquement pour le dépannage. Un utilisateur normal peut ignorer cette section.</div>
              </div>
            </div>
            <span className={`material-symbols-outlined text-zinc-400 transition-transform ${traceOpen ? 'rotate-180' : ''}`}>expand_more</span>
          </button>

          {traceOpen && (
            <div className="border-t border-zinc-100 dark:border-zinc-800 p-5">
              <div className="flex items-center justify-between gap-3 mb-3">
                <span className="text-[10px] font-black uppercase tracking-widest text-zinc-400">Trace structurée de l’armée</span>
                <button
                  type="button"
                  onClick={() => void handleLoadTrace()}
                  disabled={traceBusy}
                  className="px-3 py-2 rounded-lg bg-zinc-950 dark:bg-white text-white dark:text-zinc-950 text-[10px] font-black disabled:opacity-40"
                >
                  {traceBusy ? 'Chargement…' : 'Actualiser'}
                </button>
              </div>
              <pre className="max-h-64 overflow-auto rounded-xl bg-zinc-950 text-zinc-300 p-4 text-[10px] leading-relaxed font-mono whitespace-pre-wrap break-words">
                {latestTrace || 'Aucune trace d’attaque pour le moment. ClashGO en crée automatiquement après un déploiement de farm.'}
              </pre>
            </div>
          )}
        </div>

        <button
          type="button"
          onClick={handleExportDiagnostics}
          disabled={diagnosticsBusy}
          className={(settingsPage === 'diagnostic' ? '' : 'hidden ') + "w-full flex justify-between items-center bg-zinc-50/50 dark:bg-zinc-800/30 p-6 rounded-2xl border border-zinc-100/50 dark:border-zinc-800/50 hover:bg-white dark:hover:bg-zinc-800/60 transition-all duration-300 group disabled:opacity-60"}
        >
          <div className="flex items-center gap-5 min-w-0">
            <div className="w-12 h-12 rounded-xl bg-white dark:bg-zinc-900 flex items-center justify-center border border-zinc-100 dark:border-zinc-800 shadow-sm">
              <span className="material-symbols-outlined text-xl text-zinc-500">folder_zip</span>
            </div>
            <div className="flex flex-col text-left min-w-0">
              <span className="text-[10px] font-black text-zinc-500 uppercase tracking-[0.2em] mb-0.5">Support</span>
              <span className="text-sm font-bold text-zinc-950 dark:text-white">{diagnosticsBusy ? 'Création du diagnostic…' : 'Exporter le diagnostic'}</span>
              {diagnosticsPath && <span className="text-[9px] font-mono text-zinc-500 truncate max-w-[360px]" title={diagnosticsPath}>{diagnosticsPath}</span>}
            </div>
          </div>
          <span className="material-symbols-outlined text-zinc-400">download</span>
        </button>

        {/* Reset Section — armed-confirm to protect against misclicks. */}
        <div className={(settingsPage === 'diagnostic' ? '' : 'hidden ') + "pt-8 mt-8 border-t border-zinc-50 dark:border-zinc-800/50"}>
           <button
             onClick={handleResetClick}
             disabled={isRunning || isStarting}
             aria-live="polite"
             className={`w-full flex justify-between items-center p-6 rounded-2xl border transition-all duration-300 group disabled:opacity-50 disabled:cursor-not-allowed ${
               resetArmed
                 ? 'bg-rose-500 border-rose-600 text-white shadow-[0_0_30px_-6px_rgba(244,63,94,0.5)]'
                 : 'bg-rose-50/50 dark:bg-rose-950/10 border-rose-100/50 dark:border-rose-900/20 hover:bg-rose-100/50 dark:hover:bg-rose-950/20'
             }`}
           >
             <div className="flex items-center gap-5">
               <div className={`w-12 h-12 rounded-xl flex items-center justify-center border transition-all duration-300 shadow-sm ${
                 resetArmed ? 'bg-white/20 border-white/30' : 'bg-white dark:bg-zinc-900 border-rose-100 dark:border-rose-900 group-hover:scale-105'
               }`}>
                 <span className={`material-symbols-outlined text-xl ${resetArmed ? 'text-white animate-pulse' : 'text-rose-500 dark:text-rose-400'}`} style={{ fontVariationSettings: "'FILL' 1" }}>{resetArmed ? 'warning' : 'delete_forever'}</span>
               </div>
               <div className="flex flex-col text-left">
                 <span className={`text-[10px] font-black uppercase tracking-[0.2em] mb-0.5 ${resetArmed ? 'text-white/80' : 'text-rose-600 dark:text-rose-500'}`}>Zone sensible</span>
                 <span className={`text-sm font-bold ${resetArmed ? 'text-white' : 'text-rose-600 dark:text-rose-400'}`}>
                   {isRunning || isStarting ? 'Attends l’arrêt complet du bot avant de réinitialiser' : (resetArmed ? 'Cliquer encore pour confirmer — efface toutes les statistiques' : 'Réinitialiser toutes les statistiques')}
                 </span>
               </div>
             </div>
             <span className={`material-symbols-outlined transition-transform ${resetArmed ? 'text-white animate-pulse' : 'text-rose-400 dark:text-rose-800 group-hover:translate-x-1'}`}>
               {resetArmed ? 'error' : 'chevron_right'}
             </span>
           </button>
        </div>
      </div>
    </div>
  );
});

export default SettingsView;
