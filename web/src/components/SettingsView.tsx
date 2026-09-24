import React from 'react';
import { BotStats, UpdateStatus, SystemDiagnostics } from '../types';
import { GetLatestAttackTrace } from '../../wailsjs/go/main/App';

interface SettingsViewProps {
  stats: BotStats;
  isRunning: boolean;
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
  stats, isRunning, adbPort, darkMode, setDarkMode, onResetStats,
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
  const resetTimerRef = React.useRef<number | null>(null);

  React.useEffect(() => () => {
    if (resetTimerRef.current) window.clearTimeout(resetTimerRef.current);
  }, []);

  const handleInstanceChange = async (instance: string) => {
    if (instanceBusy) return;
    setInstanceBusy(true);
    setInstanceMessage('');
    try {
      await onSetBlueStacksInstance(instance);
      setInstanceMessage(instance ? 'Instance saved' : 'Automatic selection enabled');
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
      setDiagnosticsPath('Export failed — check app.log');
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
    if (isRunning) return;
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
    <div className="bg-white dark:bg-zinc-900 p-10 rounded-[3rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none max-w-2xl mx-auto transition-all duration-500">

      <div className="flex justify-between items-center mb-12">
        <div>
          <h3 className="text-2xl font-bold text-zinc-950 dark:text-white mb-2 tracking-tight">System Settings</h3>
          <p className="text-sm text-zinc-500 dark:text-zinc-500 font-medium">Core application and connection status.</p>
        </div>
        <div className="w-14 h-14 rounded-2xl bg-zinc-50 dark:bg-zinc-800 flex items-center justify-center border border-zinc-100 dark:border-zinc-700 shadow-sm transition-colors">
          <span className="material-symbols-outlined text-zinc-500 dark:text-zinc-500 text-2xl">memory</span>
        </div>
      </div>

      <div className="space-y-4">
        <div className="bg-zinc-950 dark:bg-black text-white p-6 rounded-2xl border border-zinc-800 shadow-xl">
          <div className="flex items-center justify-between mb-5">
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-500">Windows Readiness</div>
              <div className="text-lg font-black mt-1">{overallReady ? 'Ready to launch' : 'Setup required'}</div>
              <div className="text-[11px] text-zinc-400 mt-1">
                {overallReady
                  ? 'Everything needed to start the bot is detected.'
                  : 'Fix the orange items below. Advanced details are optional.'}
              </div>
            </div>
            <div className={`w-3 h-3 rounded-full ${overallReady ? 'bg-emerald-400 shadow-[0_0_14px_rgba(52,211,153,.7)]' : 'bg-amber-400 animate-pulse'}`}></div>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            {[
              { label: 'BlueStacks', ok: playerReady, value: playerReady ? 'Installed' : 'Needs attention' },
              { label: 'Game connection', ok: adbReady && !!preferredInstance, value: adbReady && !!preferredInstance ? 'Available' : 'Needs attention' },
              { label: 'ClashGO files', ok: runtimeReady, value: runtimeReady ? 'Ready' : 'Needs attention' },
            ].map((item) => (
              <div key={item.label} className="rounded-xl border border-zinc-800 bg-zinc-900/70 p-3 min-w-0">
                <div className="flex items-center gap-2">
                  <div className={`w-1.5 h-1.5 rounded-full shrink-0 ${item.ok ? 'bg-emerald-400' : 'bg-amber-400'}`}></div>
                  <span className="text-[9px] font-black uppercase tracking-wider text-zinc-500 truncate">{item.label}</span>
                </div>
                <div className="text-xs font-bold mt-1.5 truncate">{item.value}</div>
              </div>
            ))}
          </div>

          <details className="mt-4 rounded-xl border border-zinc-800 bg-zinc-900/70 overflow-hidden">
            <summary className="cursor-pointer list-none p-4 flex items-center justify-between gap-4">
              <div>
                <div className="text-[9px] font-black uppercase tracking-[0.2em] text-zinc-500">Technical details</div>
                <div className="text-[11px] text-zinc-300 mt-1">Only open this when troubleshooting.</div>
              </div>
              <span className="material-symbols-outlined text-zinc-500">terminal</span>
            </summary>
            <div className="border-t border-zinc-800 p-4">
              <div className="grid grid-cols-2 gap-3">
                {[
                  { label: 'Runtime assets', ok: runtimeReady, value: runtimeReady ? 'Ready' : `${systemDiagnostics?.missing_assets?.length ?? 0} missing` },
                  { label: 'BlueStacks 5', ok: playerReady, value: playerReady ? 'Detected' : 'Not found' },
                  { label: 'ADB', ok: adbReady, value: adbReady ? 'Detected' : 'Not found' },
                  { label: 'Instance', ok: !!preferredInstance, value: preferredInstance ? `${preferredInstance.name} · ${preferredInstance.adb_port}` : 'Not detected' },
                  { label: 'BlueStacks running', ok: systemDiagnostics?.emulator.bluestacks_running ?? false, value: systemDiagnostics?.emulator.bluestacks_running ? 'Running' : 'Stopped' },
                  { label: 'ADB access', ok: systemDiagnostics?.emulator.adb_enabled ?? false, value: systemDiagnostics?.emulator.adb_enabled ? 'Enabled' : (systemDiagnostics?.emulator.adb_setting_present ? 'Disabled · auto-fix on Start' : 'Check BlueStacks settings') },
                ].map((item) => (
                  <div key={item.label} className="rounded-xl border border-zinc-800 bg-zinc-950/70 p-3 min-w-0">
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
                  Missing: {systemDiagnostics.missing_assets.join(', ')}
                </div>
              )}
            </div>
          </details>

          {/* Automatic selection is the default. Manual instance choice is
              intentionally tucked away so normal users never need to touch it. */}
          <details className="mt-4 rounded-xl border border-zinc-800 bg-zinc-900/70 overflow-hidden">
            <summary className="cursor-pointer list-none p-4 flex items-center justify-between gap-4">
              <div className="min-w-0">
                <div className="text-[9px] font-black uppercase tracking-[0.2em] text-zinc-500">Advanced connection</div>
                <div className="text-[11px] text-zinc-300 mt-1">
                  BlueStacks instance · {systemDiagnostics?.configured_instance
                    ? `Manual: ${systemDiagnostics.configured_instance}`
                    : `Automatic: ${systemDiagnostics?.emulator.preferred_instance || 'detecting'}`}
                </div>
              </div>
              <span className="material-symbols-outlined text-zinc-500">tune</span>
            </summary>
            <div className="border-t border-zinc-800 p-4">
              <div className="flex items-center justify-between gap-4">
                <div className="text-[11px] text-zinc-400 max-w-[250px]">
                  Beginners should leave this on Automatic. Change it only if ClashGO clearly detected the wrong BlueStacks instance.
                </div>
                <select
                  value={systemDiagnostics?.configured_instance || ''}
                  disabled={instanceBusy || !systemDiagnostics}
                  onChange={(e) => handleInstanceChange(e.target.value)}
                  className="max-w-[220px] rounded-xl border border-zinc-700 bg-zinc-950 px-3 py-2 text-xs font-bold text-white outline-none focus:border-emerald-500 disabled:opacity-50"
                  aria-label="BlueStacks instance selection"
                >
                  <option value="">Automatic</option>
                  {(systemDiagnostics?.emulator.instances ?? []).map((inst) => (
                    <option key={inst.name} value={inst.name}>
                      {inst.name} · ADB {inst.adb_port}
                    </option>
                  ))}
                </select>
              </div>
              {instanceMessage && (
                <div className="mt-2 text-[9px] font-medium text-zinc-400">{instanceMessage}</div>
              )}
            </div>
          </details>
        </div>

        {/* Dark Mode Toggle */}
        <button
          type="button"
          role="switch"
          aria-checked={darkMode}
          onClick={() => setDarkMode(!darkMode)}
          className="w-full flex justify-between items-center bg-zinc-50/50 dark:bg-zinc-800/30 p-6 rounded-2xl border border-zinc-100/50 dark:border-zinc-800/50 hover:bg-white dark:hover:bg-zinc-800/60 hover:shadow-premium dark:hover:shadow-none transition-all duration-300 group cursor-pointer text-left"
        >
          <div className="flex items-center gap-5">
            <div className="w-12 h-12 rounded-xl bg-white dark:bg-zinc-800 flex items-center justify-center border border-zinc-100 dark:border-zinc-700 group-hover:scale-105 transition-all duration-300 shadow-sm">
              <span className="material-symbols-outlined text-xl text-zinc-500 dark:text-zinc-500">
                {darkMode ? 'dark_mode' : 'light_mode'}
              </span>
            </div>
            <div className="flex flex-col">
              <span className="text-[10px] font-black text-zinc-500 dark:text-zinc-500 uppercase tracking-[0.2em] mb-0.5">App Theme</span>
              <span className="text-sm font-bold text-zinc-950 dark:text-white">{darkMode ? 'Dark' : 'Light'}</span>
            </div>
          </div>
          <div 
            className={`w-12 h-6 rounded-full p-1 transition-all duration-500 ease-in-out relative ${darkMode ? 'bg-zinc-700' : 'bg-zinc-200'}`}
          >
            <div className={`w-4 h-4 rounded-full bg-white dark:bg-zinc-400 transition-all duration-500 ease-in-out shadow-md ${darkMode ? 'translate-x-6' : 'translate-x-0'}`}></div>
          </div>
        </button>

        <details className="rounded-2xl border border-zinc-100/60 dark:border-zinc-800/60 bg-zinc-50/40 dark:bg-zinc-950/20 overflow-hidden">
          <summary className="cursor-pointer list-none p-5 flex items-center justify-between gap-4">
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.2em] text-zinc-500">Technical performance</div>
              <div className="text-sm font-bold text-zinc-950 dark:text-white mt-1">Connection, capture and recovery details</div>
              <div className="text-[10px] text-zinc-400 mt-0.5">Normal users can leave this closed.</div>
            </div>
            <span className="material-symbols-outlined text-zinc-400">monitoring</span>
          </summary>
          <div className="border-t border-zinc-100 dark:border-zinc-800 p-4 space-y-4">
          {[
            { label: 'Connection Status', value: stats.adb_health.consecutive_fails === 0 ? 'Optimal' : 'Interrupted', status: stats.adb_health.consecutive_fails === 0 ? 'success' : 'error', icon: 'hub', detail: stats.adb_health.last_error },
            { label: 'ADB Port', value: adbPort.toString(), status: 'info', icon: 'router' },
            { label: 'Capture Latency', value: isNaN(stats.adb_health.avg_capture_ms) ? '0ms' : `${stats.adb_health.avg_capture_ms.toFixed(1)}ms`, status: stats.adb_health.avg_capture_ms < 200 ? 'success' : 'info', icon: 'speed' },
            { label: 'Capture Success', value: stats.adb_health.captures_total > 0 ? `${((stats.adb_health.captures_total / Math.max(1, stats.adb_health.captures_total + stats.adb_health.errors_total)) * 100).toFixed(1)}%` : '—', status: stats.adb_health.errors_total === 0 ? 'success' : 'info', icon: 'monitoring' },
            { label: 'ADB Errors', value: stats.adb_health.errors_total.toLocaleString(), status: stats.adb_health.consecutive_fails > 0 ? 'error' : 'success', icon: 'error' },
            // cpu_time_sec is device-independent (absolute CPU seconds since
            // start). cpu_cores is a fraction of one core; scaled by the host's
            // logical core count only to render a familiar 0-100% number.
            { label: 'CPU Time', value: `${stats.cpu_time_sec.toFixed(1)}s`, status: 'info', icon: 'schedule' },
            { label: 'CPU Usage', value: isNaN(stats.cpu_cores) ? '0%' : `${(stats.cpu_cores * (navigator.hardwareConcurrency || 1) * 100).toFixed(1)}%`, status: stats.cpu_cores < 0.5 ? 'success' : 'info', icon: 'memory' },
            { label: 'Recovery Success', value: stats.recovery_attempts > 0 ? `${stats.recovery_successes}/${stats.recovery_attempts}` : '0/0', status: stats.recovery_attempts === stats.recovery_successes ? 'success' : 'info', icon: 'healing' },
            { label: 'BlueStacks Restarts', value: stats.bluestacks_restarts.toLocaleString(), status: stats.bluestacks_restarts === 0 ? 'success' : 'info', icon: 'restart_alt' },
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

        {/* Update row — surfaces current version + a manual check
            button so users can force a refresh without waiting for the
            6h background poller. Rendered as a keyboard-accessible
            div[role=button] because it contains a real <button>
            (Resume notifications) — nesting buttons would be
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
          className="w-full flex justify-between items-center bg-zinc-50/50 dark:bg-zinc-800/30 p-6 rounded-2xl border border-zinc-100/50 dark:border-zinc-800/50 hover:bg-white dark:hover:bg-zinc-800/60 hover:shadow-premium dark:hover:shadow-none transition-all duration-300 group cursor-pointer text-left"
        >
          <div className="flex items-center gap-5">
            <div className="w-12 h-12 rounded-xl bg-white dark:bg-zinc-900 flex items-center justify-center border border-zinc-100 dark:border-zinc-800 group-hover:scale-105 transition-all duration-300 shadow-sm">
              <span className="material-symbols-outlined text-xl text-zinc-500 dark:text-zinc-500">system_update</span>
            </div>
            <div className="flex flex-col">
              <span className="text-[10px] font-black text-zinc-500 dark:text-zinc-500 uppercase tracking-[0.2em] mb-0.5">
                App Version
              </span>
              <span className="text-sm font-bold tracking-tight text-zinc-950 dark:text-white tabular-nums">
                v{appVersion || '0.0.0'}
                {updateStatus.available && (
                  <span className="ml-3 text-[10px] font-black uppercase tracking-widest text-emerald-500">
                    Update {updateStatus.latest_version} available
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
                title={`Resume notifications for v${updateStatus.skip_version}`}
              >
                Resume notifications
              </button>
            )}
            <span className="material-symbols-outlined text-zinc-300 dark:text-zinc-700 group-hover:translate-x-1 transition-transform">refresh</span>
          </div>
        </div>


        <div className="rounded-2xl border border-zinc-100/60 dark:border-zinc-800/60 bg-zinc-50/40 dark:bg-zinc-950/20 overflow-hidden">
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
                <div className="text-[10px] font-black uppercase tracking-[0.2em] text-zinc-500">Advanced diagnostics</div>
                <div className="text-sm font-bold text-zinc-950 dark:text-white">Latest attack state trace</div>
                <div className="text-[10px] text-zinc-400 mt-0.5">Only useful for troubleshooting — normal users can ignore this.</div>
              </div>
            </div>
            <span className={`material-symbols-outlined text-zinc-400 transition-transform ${traceOpen ? 'rotate-180' : ''}`}>expand_more</span>
          </button>

          {traceOpen && (
            <div className="border-t border-zinc-100 dark:border-zinc-800 p-5">
              <div className="flex items-center justify-between gap-3 mb-3">
                <span className="text-[10px] font-black uppercase tracking-widest text-zinc-400">Structured army trace</span>
                <button
                  type="button"
                  onClick={() => void handleLoadTrace()}
                  disabled={traceBusy}
                  className="px-3 py-2 rounded-lg bg-zinc-950 dark:bg-white text-white dark:text-zinc-950 text-[10px] font-black disabled:opacity-40"
                >
                  {traceBusy ? 'Loading…' : 'Refresh trace'}
                </button>
              </div>
              <pre className="max-h-64 overflow-auto rounded-xl bg-zinc-950 text-zinc-300 p-4 text-[10px] leading-relaxed font-mono whitespace-pre-wrap break-words">
                {latestTrace || 'No attack trace yet. ClashGO creates one automatically after a farm-profile deployment.'}
              </pre>
            </div>
          )}
        </div>

        <button
          type="button"
          onClick={handleExportDiagnostics}
          disabled={diagnosticsBusy}
          className="w-full flex justify-between items-center bg-zinc-50/50 dark:bg-zinc-800/30 p-6 rounded-2xl border border-zinc-100/50 dark:border-zinc-800/50 hover:bg-white dark:hover:bg-zinc-800/60 transition-all duration-300 group disabled:opacity-60"
        >
          <div className="flex items-center gap-5 min-w-0">
            <div className="w-12 h-12 rounded-xl bg-white dark:bg-zinc-900 flex items-center justify-center border border-zinc-100 dark:border-zinc-800 shadow-sm">
              <span className="material-symbols-outlined text-xl text-zinc-500">folder_zip</span>
            </div>
            <div className="flex flex-col text-left min-w-0">
              <span className="text-[10px] font-black text-zinc-500 uppercase tracking-[0.2em] mb-0.5">Support</span>
              <span className="text-sm font-bold text-zinc-950 dark:text-white">{diagnosticsBusy ? 'Creating bundle…' : 'Export Diagnostics'}</span>
              {diagnosticsPath && <span className="text-[9px] font-mono text-zinc-500 truncate max-w-[360px]" title={diagnosticsPath}>{diagnosticsPath}</span>}
            </div>
          </div>
          <span className="material-symbols-outlined text-zinc-400">download</span>
        </button>

        {/* Reset Section — armed-confirm to protect against misclicks. */}
        <div className="pt-8 mt-8 border-t border-zinc-50 dark:border-zinc-800/50">
           <button
             onClick={handleResetClick}
             disabled={isRunning}
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
                 <span className={`text-[10px] font-black uppercase tracking-[0.2em] mb-0.5 ${resetArmed ? 'text-white/80' : 'text-rose-600 dark:text-rose-500'}`}>Danger Zone</span>
                 <span className={`text-sm font-bold ${resetArmed ? 'text-white' : 'text-rose-600 dark:text-rose-400'}`}>
                   {isRunning ? 'Stop the bot before resetting' : (resetArmed ? 'Click again to confirm — wipes all stats' : 'Reset All Statistics')}
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
