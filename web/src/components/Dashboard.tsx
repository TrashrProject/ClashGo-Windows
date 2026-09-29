import React from 'react';
import { BotStats, AttackReport, ActivityEvent, AttackReplayView, SessionReportView } from '../types';
import { formatUptime, parseLogLine, LogSeverity } from '../utils';
import AutomationOverview from './AutomationOverview';

interface DashboardProps {
  stats: BotStats;
  history: AttackReport[];
  activity: ActivityEvent[];
  replay: AttackReplayView;
  sessionReport: SessionReportView | null;
  logs: string[];
}

const getTerminalAutoScroll = (): boolean => {
  try {
    const stored = localStorage.getItem('terminalAutoScroll');
    if (stored !== null) return stored === 'true';
    return true;
  } catch {
    return true;
  }
};

// Clipboard write with a fallback for webviews where the async clipboard
// API isn't granted (Wails WkWebView in some macOS versions).
const stageLabel = (value?: string): string => {
  const raw = String(value || '').trim().toLowerCase();
  const labels: Record<string, string> = {
    cooldown_intentional: 'pause programmée',
    preparation: 'préparation',
    search: 'recherche',
    deployment_protected: 'déploiement protégé',
    combat: 'combat',
    return_home: 'retour au village',
    learning: 'apprentissage',
  };
  return labels[raw] || (raw ? raw.split('_').join(' ') : 'apprentissage');
};

const sideLabel = (value?: string): string => {
  const raw = String(value || '').trim().toLowerCase();
  const labels: Record<string, string> = {
    left: 'gauche',
    right: 'droite',
    top: 'haut',
    bottom: 'bas',
    north: 'haut',
    south: 'bas',
    east: 'droite',
    west: 'gauche',
    unknown: 'inconnu',
    auto: 'auto',
  };
  return labels[raw] || (raw ? raw : 'auto');
};

const runtimeModeLabel = (value?: string): string => {
  const raw = String(value || '').trim().toLowerCase();
  if (raw === 'fast') return 'Rapide';
  if (raw === 'safe' || raw === 'cautious') return 'Prudent';
  if (raw === 'balanced' || raw === 'normal') return 'Équilibré';
  return value || 'Inconnu';
};

const copyText = async (text: string): Promise<void> => {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    document.execCommand('copy');
    document.body.removeChild(ta);
  }
};

const Dashboard: React.FC<DashboardProps> = React.memo(({
  stats,
  history,
  activity,
  replay,
  sessionReport,
  logs,
}) => {
  const containerRef = React.useRef<HTMLDivElement>(null);
  const [terminalAutoScroll, setTerminalAutoScroll] = React.useState(getTerminalAutoScroll);
  const [terminalHovered, setTerminalHovered] = React.useState(false);
  const [logFilter, setLogFilter] = React.useState('');
  const [severityFilter, setSeverityFilter] = React.useState<LogSeverity | 'all'>('all');
  const [copiedIdx, setCopiedIdx] = React.useState<number | null>(null);
  const [historyFilter, setHistoryFilter] = React.useState<'all' | 'complete' | 'partial'>('all');
  const [historyLimit, setHistoryLimit] = React.useState(10);
  const [activityPage, setActivityPage] = React.useState<'summary' | 'history' | 'console'>(() => {
    try {
      const saved = localStorage.getItem('clashgo_activity_page');
      return saved === 'history' || saved === 'console' ? saved : 'summary';
    } catch {
      return 'summary';
    }
  });
  const copiedTimerRef = React.useRef<number | null>(null);
  const uptimeHours = stats.uptime / (1e9 * 3600);

  const highLevelActivity = React.useMemo(() => {
    const numberField = (ev: ActivityEvent, key: string): number => {
      const raw = ev.fields?.[key];
      return typeof raw === 'number' && Number.isFinite(raw) ? raw : 0;
    };
    const textField = (ev: ActivityEvent, key: string): string => {
      const raw = ev.fields?.[key];
      return typeof raw === 'string' ? raw : '';
    };
    const rows: Array<{ at: string; icon: string; title: string; detail: string }> = [];
    for (const ev of activity ?? []) {
      if (rows.length >= 7) break;
      if (ev.type === 'target_found' && ev.fields?.accept !== true) continue;
      if (ev.type === 'target_skipped' || ev.type === 'state_changed') continue;

      if (ev.type === 'search_started') {
        rows.push({ at: ev.at, icon: 'search', title: 'Recherche en cours', detail: 'Recherche d’un village rentable' });
      } else if (ev.type === 'target_found') {
        rows.push({
          at: ev.at,
          icon: 'target',
          title: `Village accepté · ${numberField(ev, 'score')}/100`,
          detail: `${numberField(ev, 'gold').toLocaleString()} G · ${numberField(ev, 'elixir').toLocaleString()} E · ${numberField(ev, 'de').toLocaleString()} DE`,
        });
      } else if (ev.type === 'attack_started') {
        rows.push({
          at: ev.at,
          icon: 'bolt',
          title: 'Attaque lancée',
          detail: `${(numberField(ev, 'search_ms') / 1000).toFixed(1)} s recherche · ${numberField(ev, 'skips')} ignorés`,
        });
      } else if (ev.type === 'attack_finished') {
        rows.push({
          at: ev.at,
          icon: 'military_tech',
          title: `Attaque terminée · ${numberField(ev, 'stars')}★`,
          detail: `${numberField(ev, 'gold').toLocaleString()} G · ${(numberField(ev, 'deploy_ms') / 1000).toFixed(1)} s déploiement · ${(numberField(ev, 'cycle_ms') / 1000).toFixed(0)} s cycle`,
        });
      } else if (ev.type === 'recovery') {
        const stage = textField(ev, 'stage').toLowerCase();
        const method = textField(ev, 'method').toLowerCase();
        const methodLabel =
          method === 'game_restart' ? 'jeu relancé' :
          method === 'adb_reconnect' ? 'ADB reconnecté' :
          method === 'adb_server_reset' ? 'serveur ADB réinitialisé' :
          method === 'bluestacks_relaunch' ? 'BlueStacks relancé' :
          ev.fields?.bluestacks_restart === true ? 'BlueStacks relancé' :
          'récupération automatique';

        rows.push({
          at: ev.at,
          icon: stage === 'failed' ? 'warning' : 'healing',
          title: stage === 'success'
            ? 'Session récupérée'
            : stage === 'failed'
              ? 'Récupération à réessayer'
              : 'Récupération en cours',
          detail: stage === 'failed'
            ? methodLabel + ' · ClashGO réessaiera automatiquement'
            : methodLabel,
        });
      } else if (ev.type === 'speed_profile') {
        const mode = textField(ev, 'mode') || 'Balanced';
        const from = textField(ev, 'from');
        const nextTransitions = numberField(ev, 'next_transitions');
        const nextFirstPass = numberField(ev, 'next_first_pass_rate');
        const reactiveCapture = numberField(ev, 'fast_capture_ms');
        const modeReason = textField(ev, 'reason');
        const incident = textField(ev, 'incident');
        let reason = 'Sélectionné selon l’état actuel de la session';
        if (modeReason === 'safety_governor') {
          reason = `Protection de cadence${incident ? ` · ${incident.split('_').join(' ')}` : ''}`;
        } else if (nextTransitions >= 5 && nextFirstPass > 0 && nextFirstPass < 92) {
          reason = `Premier passage suivant ${nextFirstPass.toFixed(0)}% · ${nextTransitions.toFixed(0)} échantillons`;
        } else if (reactiveCapture > 0) {
          reason = `Capture réactive ${reactiveCapture.toFixed(0)}ms`;
        }
        rows.push({
          at: ev.at,
          icon: mode === 'Fast' ? 'speed' : mode === 'Safe' ? 'shield' : 'tune',
          title: `Mode de farm ${mode === 'Fast' ? 'Rapide' : mode === 'Safe' ? 'Prudent' : mode === 'Balanced' ? 'Équilibré' : mode}`,
          detail: from ? `${from} → ${mode} · ${reason}` : reason,
        });
      } else if (ev.type === 'return_home') {
        const ok = ev.fields?.success === true;
        rows.push({
          at: ev.at,
          icon: ok ? 'home' : 'home_work',
          title: ok ? 'Village prêt' : 'Retour au village de secours',
          detail: `${(numberField(ev, 'duration_ms') / 1000).toFixed(1)} s retour village`,
        });
      } else if (ev.type === 'anomaly') {
        const kind = textField(ev, 'kind') || 'performance_anomaly';

        if (kind === 'army_guard_rejected_target') {
          rows.push({
            at: ev.at,
            icon: 'shield',
            title: 'Base ignorée · armée non conforme',
            detail: 'Aucun déploiement effectué. ClashGO poursuit la recherche.',
          });
          continue;
        }
        if (kind === 'army_guard_uncertain') {
          rows.push({
            at: ev.at,
            icon: 'rule',
            title: 'Contrôle armée incertain',
            detail: 'La lecture n’était pas assez fiable pour bloquer l’attaque.',
          });
          continue;
        }
        if (kind === 'army_guard_unavailable') {
          rows.push({
            at: ev.at,
            icon: 'shield_question',
            title: 'Contrôle armée indisponible',
            detail: 'ClashGO n’a pas bloqué l’attaque sur une lecture non concluante.',
          });
          continue;
        }

        const duration = numberField(ev, 'duration_ms');
        const rawRegressions = ev.fields?.regressions;
        let regressionDetail = '';
        if (Array.isArray(rawRegressions) && rawRegressions.length > 0) {
          const first = rawRegressions[0] as { metric?: inconnu; delta_pct?: inconnu };
          const metric = typeof first.metric === 'string' ? first.metric.split('_').join(' ') : 'metric';
          const delta = typeof first.delta_pct === 'number' && Number.isFinite(first.delta_pct)
            ? ` · +${first.delta_pct.toFixed(0)}%`
            : '';
          regressionDetail = `${metric}${delta}`;
        }
        rows.push({
          at: ev.at,
          icon: 'monitor_heart',
          title: kind === 'performance_regression' ? 'Régression de performance' : 'Anomalie de performance',
          detail: regressionDetail || `${kind.split('_').join(' ')}${duration > 0 ? ` · ${(duration / 1000).toFixed(1)}s` : ''}`,
        });
      }
    }
    return rows;
  }, [activity]);

  const replayMap = React.useMemo(() => {
    const events = (replay?.events ?? []).filter((ev) => ev.kind === 'deploy');
    let maxX = 860;
    let maxY = 732;
    for (const ev of events) {
      maxX = Math.max(maxX, ev.p1?.x || 0, ev.p2?.x || 0, ev.slot_x || 0);
      maxY = Math.max(maxY, ev.p1?.y || 0, ev.p2?.y || 0, ev.slot_y || 0);
    }
    const categoryClass = (category?: string) => {
      switch ((category || '').toLowerCase()) {
        case 'hero': return 'text-amber-500';
        case 'siege':
        case 'cc': return 'text-rose-500';
        case 'spell': return 'text-violet-500';
        default: return 'text-sky-500';
      }
    };
    return { events, width: maxX, height: maxY, categoryClass };
  }, [replay]);

  React.useEffect(() => {
    try { localStorage.setItem('clashgo_activity_page', activityPage); } catch {}
  }, [activityPage]);

  React.useEffect(() => {
    try {
      localStorage.setItem('terminalAutoScroll', String(terminalAutoScroll));
    } catch (e) {
      console.warn('Failed to save terminalAutoScroll preference:', e);
    }
  }, [terminalAutoScroll]);

  // Unmount-only cleanup for the copy-feedback timer.
  React.useEffect(() => () => {
    if (copiedTimerRef.current) window.clearTimeout(copiedTimerRef.current);
  }, []);

  // Parse once per log update — the raw strings are stable between
  // polls, so memoizing on `logs` keeps re-renders cheap.
  const parsedLogs = React.useMemo(() => logs.map(parseLogLine), [logs]);

  const severityCounts = React.useMemo(() => {
    const counts: Record<LogSeverity, number> = { debug: 0, info: 0, success: 0, warn: 0, error: 0 };
    for (const l of parsedLogs) counts[l.level]++;
    return counts;
  }, [parsedLogs]);

  const filteredLogs = React.useMemo(() => {
    const needle = logFilter.trim().toLowerCase();
    if (!needle && severityFilter === 'all') return parsedLogs;
    return parsedLogs.filter((l) => {
      if (severityFilter !== 'all' && l.level !== severityFilter) return false;
      if (needle && !l.message.toLowerCase().includes(needle)) return false;
      return true;
    });
  }, [parsedLogs, logFilter, severityFilter]);

  // ANSI-free export: rebuild each line from the parsed structure so
  // the copied log never carries zerolog escape codes.
  const exportText = React.useMemo(() =>
    parsedLogs
      .map((l) => `${l.timestamp ? l.timestamp + ' | ' : ''}${l.level.toUpperCase()} | ${l.message}`)
      .join('\n'),
  [parsedLogs]);

  const copyLine = (idx: number, text: string) => {
    void copyText(text);
    setCopiedIdx(idx);
    if (copiedTimerRef.current) window.clearTimeout(copiedTimerRef.current);
    copiedTimerRef.current = window.setTimeout(() => setCopiedIdx(null), 1200);
  };

  // Wrap every (case-insensitive) occurrence sur the filter text in
  // <mark> so matches pop out while the message keeps its color.
  const highlightMatch = (message: string): React.ReactNode => {
    const needle = logFilter.trim().toLowerCase();
    if (!needle) return message;
    const lower = message.toLowerCase();
    const parts: React.ReactNode[] = [];
    let last = 0;
    let i = lower.indexOf(needle);
    while (i !== -1) {
      parts.push(message.slice(last, i));
      parts.push(<mark key={i}>{message.slice(i, i + needle.length)}</mark>);
      last = i + needle.length;
      i = lower.indexOf(needle, last);
    }
    parts.push(message.slice(last));
    return parts;
  };

  React.useEffect(() => {
    if (!terminalAutoScroll || terminalHovered) return;

    const container = containerRef.current;
    if (!container) return;

    // Smart scroll: only if already at bottom (within 100px). While
    // hovering, auto-scroll is suspended so the user can inspect/copy
    // freely — resuming the moment the pointer leaves.
    const isAtBottom = container.scrollHeight - container.scrollTop <= container.clientHeight + 100;
    if (isAtBottom) {
      container.scrollTop = container.scrollHeight;
    }
  }, [filteredLogs, terminalAutoScroll, terminalHovered]);

  const getRate = (total: number) => {
    if (uptimeHours < 0.01 || total === 0) return '+0/hr';
    const rate = total / uptimeHours;
    if (rate > 1e6) return `+${(rate / 1e6).toFixed(1)}M/hr`;
    if (rate > 1e3) return `+${(rate / 1e3).toFixed(0)}k/hr`;
    return `+${rate.toFixed(0)}/hr`;
  };

  const filteredHistory = React.useMemo(() => {
    const source = history ?? [];
    if (historyFilter === 'complete') return source.filter((rep) => rep.deploy_success);
    if (historyFilter === 'partial') return source.filter((rep) => !rep.deploy_success);
    return source;
  }, [history, historyFilter]);

  const visibleHistory = React.useMemo(
    () => filteredHistory.slice(0, historyLimit),
    [filteredHistory, historyLimit],
  );

  const latestAttack = history?.[0];
  const totalHistoryLoot = React.useMemo(() => {
    return (history ?? []).reduce((acc, rep) => ({
      gold: acc.gold + rep.gold_stolen + rep.bonus_gold,
      elixir: acc.elixir + rep.elixir_stolen + rep.bonus_elixir,
      de: acc.de + rep.dark_elixir_stolen + rep.bonus_de,
    }), { gold: 0, elixir: 0, de: 0 });
  }, [history]);

  const severityChips: { id: LogSeverity | 'all'; label: string; active: string; dot: string }[] = [
    { id: 'all', label: 'Tout', active: 'bg-zinc-100 dark:bg-zinc-800 text-zinc-900 dark:text-white border-zinc-200 dark:border-zinc-700', dot: 'bg-zinc-400' },
    { id: 'debug', label: 'Debug', active: 'bg-violet-500/15 text-violet-600 dark:text-violet-400 border-violet-500/40', dot: 'bg-violet-500' },
    { id: 'info', label: 'Info', active: 'bg-zinc-100 dark:bg-zinc-800 text-zinc-900 dark:text-white border-zinc-200 dark:border-zinc-700', dot: 'bg-zinc-400' },
    { id: 'success', label: 'Succès', active: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border-emerald-500/40', dot: 'bg-emerald-500' },
    { id: 'warn', label: 'Avert.', active: 'bg-amber-500/15 text-amber-600 dark:text-amber-400 border-amber-500/40', dot: 'bg-amber-500' },
    { id: 'error', label: 'Erreur', active: 'bg-rose-500/15 text-rose-600 dark:text-rose-400 border-rose-500/40', dot: 'bg-rose-500' },
  ];

  return (
    <div className="space-y-6">
      <AutomationOverview />

      <div className="sticky top-0 z-20 -mx-1 px-1 py-2 bg-zinc-50/90 dark:bg-zinc-950/90 backdrop-blur-xl overflow-x-auto">
        <div className="flex w-full min-w-max sm:w-auto sm:inline-flex rounded-2xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-1.5 shadow-sm">
          {([
            ['summary', 'Résumé', 'dashboard'],
            ['history', 'Historique', 'history'],
            ['console', 'Console', 'terminal'],
          ] as const).map(([id, label, icon]) => (
            <button
              key={id}
              type="button"
              onClick={() => setActivityPage(id)}
              className={
                'flex flex-1 sm:flex-none items-center justify-center gap-2 rounded-xl px-3 sm:px-4 py-2.5 text-[9px] sm:text-[10px] font-black uppercase tracking-[0.12em] sm:tracking-[0.16em] transition whitespace-nowrap ' +
                (activityPage === id
                  ? 'bg-zinc-950 text-white dark:bg-white dark:text-zinc-950'
                  : 'text-zinc-500 hover:text-zinc-950 dark:hover:text-white')
              }
            >
              <span className="material-symbols-outlined text-base">{icon}</span>
              {label}
            </button>
          ))}
        </div>
      </div>

      {activityPage === 'summary' && sessionReport && sessionReport.attacks > 0 && (
        <section className="bg-zinc-950 dark:bg-white rounded-[2.5rem] shadow-premium-lg overflow-hidden">
          <div className="px-6 py-5 flex flex-col xl:flex-row xl:items-center xl:justify-between gap-5">
            <div className="min-w-0">
              <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-500">Résumé de session</div>
              <div className="mt-1 flex flex-wrap items-center gap-3">
                <h3 className="text-xl font-black text-white dark:text-zinc-950 tracking-tight">
                  {sessionReport.attacks} attaque{sessionReport.attacks === 1 ? '' : 's'} · {sessionReport.speed_profile || 'Équilibré'}
                </h3>
                <span className="px-2.5 py-1 rounded-full bg-white/10 dark:bg-zinc-950/10 text-[9px] font-black uppercase tracking-widest text-zinc-400 dark:text-zinc-500">
                  Santé {sessionReport.health_score || 0}/100
                </span>
              </div>
              <div className="mt-2 text-[10px] font-bold text-zinc-500">
                Point limitant : {stageLabel(sessionReport.bottleneck)}
                {sessionReport.optimization_target ? ` · optimiser ${stageLabel(sessionReport.optimization_target)}` : ''}
                {sessionReport.top_strategy ? ` · ${sessionReport.top_strategy}` : ''}
                {sessionReport.top_deploy_side ? ` · côté ${sideLabel(sessionReport.top_deploy_side)}` : ''}
              </div>
            </div>

            <div className="grid grid-cols-2 md:grid-cols-4 xl:grid-cols-7 gap-2 min-w-0 xl:min-w-[760px]">
              {[
                { label: 'Or / h', value: new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(sessionReport.gold_per_hour || 0) },
                { label: 'Sans intervention', value: `${(sessionReport.zero_touch_rate || 0).toFixed(1)}%` },
                { label: 'Étoiles moy.', value: (sessionReport.average_stars || 0).toFixed(2) },
                { label: 'Série propre', value: `${sessionReport.current_zero_touch_streak || 0} / ${sessionReport.best_zero_touch_streak || 0}` },
                { label: 'Fin combat', value: sessionReport.average_battle_end_wait_ms > 0 ? `${(sessionReport.average_battle_end_wait_ms / 1000).toFixed(1)}s` : '—' },
                { label: 'Sorties anticipées', value: `${(sessionReport.early_exit_rate || 0).toFixed(1)}%` },
                { label: 'Meilleur O+E', value: new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(sessionReport.best_attack?.gold_plus_elixir || 0) },
              ].map((metric) => (
                <div key={metric.label} className="rounded-2xl bg-white/5 dark:bg-zinc-950/5 border border-white/10 dark:border-zinc-950/10 px-3 py-3">
                  <div className="text-[8px] font-black uppercase tracking-[0.16em] text-zinc-500">{metric.label}</div>
                  <div className="mt-1 text-lg font-black text-white dark:text-zinc-950 tabular-nums">{metric.value}</div>
                </div>
              ))}
            </div>
          </div>

          {(sessionReport.recommendations ?? []).length > 0 && (
            <div className="px-6 pb-5">
              <div className="rounded-[1.5rem] border border-white/10 dark:border-zinc-950/10 bg-white/5 dark:bg-zinc-950/5 p-4">
                <div className="flex items-center gap-2 text-[9px] font-black uppercase tracking-[0.18em] text-zinc-500">
                  <span className="material-symbols-outlined text-base">diagnosis</span>
                  Diagnostic de session
                </div>
                <div className="mt-3 grid grid-cols-1 lg:grid-cols-3 gap-2">
                  {(sessionReport.recommendations ?? []).slice(0, 3).map((recommendation, index) => (
                    <div key={`${index}-${recommendation}`} className="rounded-xl bg-black/10 dark:bg-white/5 px-3 py-2.5 text-[10px] font-bold leading-relaxed text-zinc-300 dark:text-zinc-600">
                      {recommendation}
                    </div>
                  ))}
                </div>
              </div>
            </div>
          )}
        </section>
      )}

      {/* Compact activity feed: high-level actions only, not raw diagnostics. */}
      <section className={(activityPage === 'summary' ? '' : 'hidden ') + "bg-white dark:bg-zinc-900 rounded-[2.5rem] border border-zinc-100/70 dark:border-zinc-800/70 shadow-premium dark:shadow-none overflow-hidden"}>
        <div className="px-6 py-5 flex items-center justify-between gap-4 border-b border-zinc-100 dark:border-zinc-800/70">
          <div>
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Activité en direct</div>
            <h3 className="mt-1 text-xl font-bold text-zinc-950 dark:text-white tracking-tight">Ce que fait le bot</h3>
          </div>
          <div className="px-3 py-1.5 rounded-full bg-zinc-100 dark:bg-zinc-800 text-[9px] font-black uppercase tracking-widest text-zinc-500">
            Santé {stats.health_score ?? 100}/100
          </div>
        </div>
        {highLevelActivity.length === 0 ? (
          <div className="px-6 py-8 text-sm font-medium text-zinc-400">En attente de la première action de farm…</div>
        ) : (
          <div className="divide-y divide-zinc-100 dark:divide-zinc-800/70">
            {highLevelActivity.map((item, index) => {
              const date = new Date(item.at);
              return (
                <div key={`${item.at}-${index}`} className="px-6 py-3.5 flex items-center gap-4">
                  <div className="w-9 h-9 rounded-xl bg-zinc-100 dark:bg-zinc-800 flex items-center justify-center shrink-0">
                    <span className="material-symbols-outlined text-zinc-500 text-lg">{item.icon}</span>
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="text-sm font-bold text-zinc-900 dark:text-zinc-100">{item.title}</div>
                    <div className="text-[10px] font-bold text-zinc-400 mt-0.5 truncate">{item.detail}</div>
                  </div>
                  <div className="text-[10px] font-black text-zinc-400 tabular-nums shrink-0">
                    {date.toLocaleTimeString([], { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </section>

      {activityPage === 'summary' && replay?.available && (
        <section className="bg-white dark:bg-zinc-900 rounded-[2.5rem] border border-zinc-100/70 dark:border-zinc-800/70 shadow-premium dark:shadow-none overflow-hidden">
          <div className="px-6 py-5 flex flex-wrap items-center justify-between gap-4 border-b border-zinc-100 dark:border-zinc-800/70">
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Replay de l’attaque</div>
              <h3 className="mt-1 text-xl font-bold text-zinc-950 dark:text-white tracking-tight">{replay.strategy || 'Dernier déploiement'}</h3>
            </div>
            <div className={`px-3 py-1.5 rounded-full text-[9px] font-black uppercase tracking-widest ${
              replay.complete
                ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
                : 'bg-amber-500/10 text-amber-600 dark:text-amber-400'
            }`}>
              {replay.complete ? 'Complet' : 'Partiel'}
            </div>
          </div>

          {(replay.events ?? []).length === 0 ? (
            <div className="px-6 py-8 text-sm font-medium text-zinc-400">Aucune action de déploiement enregistrée dans la dernière trace.</div>
          ) : (
            <>
              <div className="px-6 pt-5">
                <div className="grid grid-cols-1 xl:grid-cols-[1.5fr_auto] gap-4 items-start">
                  <div className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-zinc-950 overflow-hidden relative">
                    <div className="absolute top-4 left-4 z-10">
                      <div className="text-[9px] font-black uppercase tracking-[0.22em] text-zinc-500">Carte de déploiement</div>
                      <div className="mt-1 text-xs font-bold text-zinc-300">{replayMap.events.length} actions de déploiement enregistrées</div>
                    </div>
                    <svg
                      viewBox={`0 0 ${replayMap.width} ${replayMap.height}`}
                      className="w-full aspect-[860/732] min-h-[280px]"
                      role="img"
                      aria-label="Carte du dernier déploiement"
                    >
                      <defs>
                        <pattern id="deploy-grid" width="60" height="60" patternUnits="userSpaceOnUse">
                          <path d="M 60 0 L 0 0 0 60" fill="none" stroke="currentColor" strokeWidth="1" className="text-zinc-800" />
                        </pattern>
                      </defs>
                      <rect x="0" y="0" width={replayMap.width} height={replayMap.height} fill="url(#deploy-grid)" />
                      <line x1={replayMap.width / 2} y1="0" x2={replayMap.width / 2} y2={replayMap.height} stroke="currentColor" strokeDasharray="10 10" className="text-zinc-800" />
                      <line x1="0" y1={replayMap.height / 2} x2={replayMap.width} y2={replayMap.height / 2} stroke="currentColor" strokeDasharray="10 10" className="text-zinc-800" />

                      {latestAttack?.red_zone_valid && latestAttack.red_zone_x2 > latestAttack.red_zone_x1 && latestAttack.red_zone_y2 > latestAttack.red_zone_y1 && (
                        <rect
                          x={latestAttack.red_zone_x1}
                          y={latestAttack.red_zone_y1}
                          width={latestAttack.red_zone_x2 - latestAttack.red_zone_x1}
                          height={latestAttack.red_zone_y2 - latestAttack.red_zone_y1}
                          fill="currentColor"
                          stroke="currentColor"
                          strokeWidth="4"
                          strokeDasharray="12 8"
                          opacity="0.10"
                          className="text-rose-500"
                        />
                      )}

                      {latestAttack?.corridor_verified && (
                        <line
                          x1={latestAttack.deploy_line_x1}
                          y1={latestAttack.deploy_line_y1}
                          x2={latestAttack.deploy_line_x2}
                          y2={latestAttack.deploy_line_y2}
                          stroke="currentColor"
                          strokeWidth="9"
                          strokeLinecap="round"
                          opacity="0.92"
                          className="text-emerald-400"
                        />
                      )}
                      {replayMap.events.slice(-28).map((ev, index) => {
                        const x1 = ev.p1?.x || 0;
                        const y1 = ev.p1?.y || 0;
                        const x2 = ev.p2?.x || x1;
                        const y2 = ev.p2?.y || y1;
                        const pointMode = x1 === x2 && y1 === y2;
                        const cls = replayMap.categoryClass(ev.category);
                        const labelX = pointMode ? x1 : (x1 + x2) / 2;
                        const labelY = pointMode ? y1 : (y1 + y2) / 2;
                        return (
                          <g key={`map-${ev.offset_ms}-${index}`} className={cls}>
                            {!pointMode && (
                              <line
                                x1={x1}
                                y1={y1}
                                x2={x2}
                                y2={y2}
                                stroke="currentColor"
                                strokeWidth="7"
                                strokeLinecap="round"
                                opacity="0.75"
                              />
                            )}
                            <circle cx={labelX} cy={labelY} r="13" fill="currentColor" opacity="0.92" />
                            <text x={labelX} y={labelY + 4} textAnchor="middle" fontSize="11" fontWeight="900" fill="white">
                              {index + 1}
                            </text>
                          </g>
                        );
                      })}
                    </svg>
                  </div>
                  <div className="grid grid-cols-2 xl:grid-cols-1 gap-2 min-w-[160px]">
                    {[
                      ['Troupe', 'bg-sky-500'],
                      ['Héros', 'bg-amber-500'],
                      ['Siège / CDC', 'bg-rose-500'],
                      ['Sort', 'bg-violet-500'],
                      ['Ligne sûre', 'bg-emerald-400'],
                      ['Zone rouge', 'bg-rose-400'],
                    ].map(([label, cls]) => (
                      <div key={label} className="rounded-xl border border-zinc-100 dark:border-zinc-800 px-3 py-2 flex items-center gap-2">
                        <span className={`size-2.5 rounded-full ${cls}`} />
                        <span className="text-[9px] font-black uppercase tracking-wider text-zinc-500">{label}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>

              <div className="px-6 py-5 overflow-x-auto">
              <div className="flex gap-3 min-w-max">
                {(replay.events ?? []).slice(-14).map((ev, index) => {
                  const pointMode = ev.p1?.x === ev.p2?.x && ev.p1?.y === ev.p2?.y;
                  const title = ev.name || ev.category || ev.kind;
                  return (
                    <div key={`${ev.offset_ms}-${index}`} className="w-52 rounded-2xl border border-zinc-100 dark:border-zinc-800 bg-zinc-50/60 dark:bg-zinc-950/30 p-4">
                      <div className="flex items-center justify-between gap-3">
                        <span className="text-[9px] font-black uppercase tracking-widest text-zinc-400">
                          T+{((ev.offset_ms || 0) / 1000).toFixed(2)}s
                        </span>
                        <span className="text-[9px] font-black uppercase tracking-widest text-zinc-400">{ev.category || ev.kind}</span>
                      </div>
                      <div className="mt-2 text-sm font-black text-zinc-950 dark:text-white truncate">{title}</div>
                      <div className="mt-1 text-[10px] font-bold text-zinc-500">
                        {ev.count ? `×${ev.count}` : 'événement'}{ev.deploy_side ? ` · ${sideLabel(ev.deploy_side)}` : ''}
                      </div>
                      <div className="mt-3 text-[9px] font-mono text-zinc-400 leading-relaxed">
                        {ev.kind === 'deploy' ? (
                          pointMode
                            ? `point (${ev.p1?.x ?? 0}, ${ev.p1?.y ?? 0})`
                            : `ligne (${ev.p1?.x ?? 0}, ${ev.p1?.y ?? 0}) → (${ev.p2?.x ?? 0}, ${ev.p2?.y ?? 0})`
                        ) : ev.kind}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
            </>
          )}
        </section>
      )}

      {/* Metrics Row */}
      <div className={(activityPage === 'summary' ? '' : 'hidden ') + "grid grid-cols-1 md:grid-cols-3 gap-8"}>
        {[
          { label: 'Or récupéré', value: stats.total_gold, color: 'text-amber-500', bg: 'bg-amber-500/10', icon: 'monetization_on', rate: getRate(stats.total_gold) },
          { label: 'Élixir récupéré', value: stats.total_elixir, color: 'text-fuchsia-500', bg: 'bg-fuchsia-500/10', icon: 'water_drop', rate: getRate(stats.total_elixir) },
          { label: 'Élixir noir', value: stats.total_de, color: 'text-zinc-950 dark:text-zinc-100', bg: 'bg-zinc-100 dark:bg-zinc-800', icon: 'water_drop', rate: getRate(stats.total_de) }
        ].map((item, idx) => (
          <div key={idx} className="bg-white dark:bg-zinc-900 p-6 rounded-[2.5rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none hover:shadow-premium-hover dark:hover:bg-zinc-800/50 transition-all duration-300 group">
            <div className="flex items-center justify-between mb-5">
              <div className={`w-14 h-14 rounded-2xl ${item.bg} flex items-center justify-center transition-transform group-hover:scale-110 duration-300 shadow-sm`}>
                 <span className={`material-symbols-outlined ${item.color} text-2xl`}>{item.icon}</span>
              </div>
              <div className="px-4 py-1.5 bg-zinc-50 dark:bg-zinc-800 rounded-full border border-zinc-100 dark:border-zinc-700 flex items-center justify-center min-w-[80px]">
                <span className="text-[10px] font-black text-zinc-500 dark:text-zinc-500 uppercase tracking-widest leading-none">{item.rate}</span>
              </div>
            </div>
            <div className="space-y-1">
              <span className="text-[11px] font-black text-zinc-500 dark:text-zinc-500 uppercase tracking-[0.2em]">{item.label}</span>
              <div className="text-4xl font-bold tracking-tight text-zinc-950 dark:text-white">{item.value.toLocaleString()}</div>
            </div>
          </div>
        ))}
      </div>

      {/* Persistent Attack History */}
      <section className={(activityPage === 'history' ? '' : 'hidden ') + "bg-white dark:bg-zinc-900 rounded-[2.5rem] border border-zinc-100/70 dark:border-zinc-800/70 shadow-premium dark:shadow-none overflow-hidden"}>
        <div className="px-6 py-5 border-b border-zinc-100 dark:border-zinc-800/70 bg-gradient-to-r from-zinc-50/80 to-white dark:from-zinc-900 dark:to-zinc-900">
          <div className="flex flex-wrap items-center justify-between gap-4">
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 rounded-2xl bg-zinc-950 dark:bg-white text-white dark:text-zinc-950 flex items-center justify-center shadow-sm">
                <span className="material-symbols-outlined">swords</span>
              </div>
              <div>
                <div className="flex items-center gap-2">
                  <h3 className="text-xl font-bold text-zinc-950 dark:text-white tracking-tight">Historique des attaques</h3>
                  <span className="px-2.5 py-1 rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 text-[9px] font-black uppercase tracking-widest border border-emerald-500/20">
                    Saved
                  </span>
                </div>
                <p className="text-sm text-zinc-500 font-medium">
                  {history.length.toLocaleString()} persistent battle{history.length === 1 ? '' : 's'} stored locally.
                </p>
              </div>
            </div>

            <div className="flex items-center gap-2">
              {([
                ['all', 'Tout'],
                ['complete', 'Déploiement complet'],
                ['partial', 'Partiel'],
              ] as const).map(([id, label]) => (
                <button
                  key={id}
                  onClick={() => { setHistoryFilter(id); setHistoryLimit(10); }}
                  className={`h-9 px-3 rounded-xl text-[10px] font-black uppercase tracking-widest border transition-all ${
                    historyFilter === id
                      ? 'bg-zinc-950 text-white border-zinc-950 dark:bg-white dark:text-zinc-950 dark:border-white'
                      : 'bg-white dark:bg-zinc-900 text-zinc-500 border-zinc-200 dark:border-zinc-700 hover:border-zinc-400 dark:hover:border-zinc-500'
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>

          {latestAttack && (
            <div className="mt-5 grid grid-cols-1 xl:grid-cols-[1.5fr_1fr] gap-3">
              <div className="rounded-2xl bg-zinc-950 text-white dark:bg-zinc-800 p-4 flex flex-wrap items-center justify-between gap-4">
                <div>
                  <div className="text-[9px] uppercase tracking-[0.25em] font-black text-zinc-400 mb-1">Dernière attaque</div>
                  <div className="flex items-center gap-3 flex-wrap">
                    <span className="text-lg font-bold">{latestAttack.strategy || 'Stratégie inconnue'}</span>
                    <span className="text-[10px] font-black uppercase tracking-widest text-zinc-400">
                      {sideLabel(latestAttack.target_edge || 'auto')}{latestAttack.deploy_side && latestAttack.deploy_side !== 'Inconnu' ? ` → ${sideLabel(latestAttack.deploy_side)}` : ''}
                    </span>
                    <span className={`px-2 py-1 rounded-lg text-[9px] font-black uppercase tracking-wider ${
                      latestAttack.deploy_success
                        ? 'bg-emerald-500/15 text-emerald-400'
                        : 'bg-amber-500/15 text-amber-300'
                    }`}>
                      {latestAttack.deploy_success ? 'Déploiement complet' : `${latestAttack.undeployed_slots} emplacement(s) restant(s)`}
                    </span>
                    <span className="px-2 py-1 rounded-lg bg-white/10 text-zinc-300 text-[9px] font-black uppercase tracking-wider">
                      {(latestAttack.search_duration_ms / 1000 || 0).toFixed(1)} s recherche · {latestAttack.search_skips || 0} ignorés
                    </span>
                    <span className="px-2 py-1 rounded-lg bg-white/10 text-zinc-300 text-[9px] font-black uppercase tracking-wider">
                      score {latestAttack.target_score || 0}/100 · {latestAttack.runtime_mode || 'Inconnu'}
                    </span>
                    <span className="px-2 py-1 rounded-lg bg-white/10 text-zinc-300 text-[9px] font-black uppercase tracking-wider">
                      {latestAttack.destruction_pct || 0}% · {(latestAttack.battle_end_reason || 'inconnu').split('_').join(' ')}
                    </span>
                    <span className="px-2 py-1 rounded-lg bg-white/10 text-zinc-300 text-[9px] font-black uppercase tracking-wider">
                      {latestAttack.full_routine_duration_ms > 0 ? `${(latestAttack.full_routine_duration_ms / 1000).toFixed(0)}s cycle réel` : `${(latestAttack.cycle_duration_ms / 1000).toFixed(0)} s cycle`}
                      {latestAttack.return_home_duration_ms > 0 ? ` · ${(latestAttack.return_home_duration_ms / 1000).toFixed(1)}s home` : ''}
                    </span>
                    {latestAttack.red_zone_valid && (
                      <span className="px-2 py-1 rounded-lg bg-white/10 text-zinc-300 text-[9px] font-black uppercase tracking-wider">
                        RZ {latestAttack.red_zone_x1},{latestAttack.red_zone_y1}→{latestAttack.red_zone_x2},{latestAttack.red_zone_y2}
                        {' · '}line {latestAttack.deploy_line_x1},{latestAttack.deploy_line_y1}→{latestAttack.deploy_line_x2},{latestAttack.deploy_line_y2}
                        {' · '}free {latestAttack.deploy_free_space || 0}px
                      </span>
                    )}
                    {latestAttack.safety_mode && (
                      <span className={`px-2 py-1 rounded-lg text-[9px] font-black uppercase tracking-wider ${
                        latestAttack.corridor_verified && latestAttack.hud_safe
                          ? 'bg-emerald-500/15 text-emerald-300'
                          : 'bg-amber-500/15 text-amber-300'
                      }`}>
                        safety {latestAttack.safety_mode.split('_').join(' ')}
                      </span>
                    )}
                    {(latestAttack.result_confidence || latestAttack.stars_source || latestAttack.loot_source) && (
                      <span className={`px-2 py-1 rounded-lg text-[9px] font-black uppercase tracking-wider ${
                        latestAttack.result_confidence === 'high'
                          ? 'bg-emerald-500/15 text-emerald-300'
                          : latestAttack.result_confidence === 'low'
                            ? 'bg-amber-500/15 text-amber-300'
                            : 'bg-white/10 text-zinc-300'
                      }`}>
                        result {latestAttack.result_confidence || 'inconnu'} · ★ {(latestAttack.stars_source || 'inconnu').split('_').join(' ')} · loot {(latestAttack.loot_source || 'inconnu').split('_').join(' ')}
                      </span>
                    )}
                  </div>
                </div>
                <div className="flex items-center gap-1">
                  {[0, 1, 2].map((star) => (
                    <span
                      key={star}
                      className={`material-symbols-outlined text-2xl ${
                        star < latestAttack.stars ? 'text-amber-400' : 'text-zinc-700'
                      }`}
                      style={{ fontVariationSettings: star < latestAttack.stars ? "'FILL' 1" : "'FILL' 0" }}
                    >
                      star
                    </span>
                  ))}
                </div>
              </div>

              <div className="rounded-2xl border border-zinc-200 dark:border-zinc-700 p-4 grid grid-cols-3 gap-3 bg-white/70 dark:bg-zinc-900/60">
                <div>
                  <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">Or</div>
                  <div className="text-sm font-black text-amber-500 tabular-nums">{totalHistoryLoot.gold.toLocaleString()}</div>
                </div>
                <div>
                  <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">Élixir</div>
                  <div className="text-sm font-black text-fuchsia-500 tabular-nums">{totalHistoryLoot.elixir.toLocaleString()}</div>
                </div>
                <div>
                  <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">Élixir noir</div>
                  <div className="text-sm font-black text-zinc-700 dark:text-zinc-200 tabular-nums">{totalHistoryLoot.de.toLocaleString()}</div>
                </div>
              </div>
            </div>
          )}
        </div>

        {filteredHistory.length === 0 ? (
          <div className="px-6 py-20 text-center">
            <div className="w-14 h-14 mx-auto mb-4 rounded-2xl bg-zinc-100 dark:bg-zinc-800 flex items-center justify-center">
              <span className="material-symbols-outlined text-zinc-400 text-2xl">history</span>
            </div>
            <div className="text-sm font-bold text-zinc-600 dark:text-zinc-300">Aucune attaque sauvegardée pour ce filtre</div>
            <div className="text-xs text-zinc-400 mt-1">Les combats terminés apparaîtront ici automatiquement et resteront disponibles après redémarrage.</div>
          </div>
        ) : (
          <>
            <div className="overflow-x-auto">
              <table className="w-full text-left border-collapse min-w-[1080px]">
                <thead>
                  <tr className="bg-zinc-50/70 dark:bg-zinc-800/30">
                    <th className="px-6 py-3 text-[10px] font-black text-zinc-400 uppercase tracking-[0.2em]">Combat</th>
                    <th className="px-4 py-3 text-[10px] font-black text-zinc-400 uppercase tracking-[0.2em]">Stratégie</th>
                    <th className="px-4 py-3 text-[10px] font-black text-zinc-400 uppercase tracking-[0.2em]">Butin</th>
                    <th className="px-4 py-3 text-[10px] font-black text-zinc-400 uppercase tracking-[0.2em]">Déploiement</th>
                    <th className="px-4 py-3 text-[10px] font-black text-zinc-400 uppercase tracking-[0.2em]">Recherche</th>
                    <th className="px-6 py-3 text-[10px] font-black text-zinc-400 uppercase tracking-[0.2em] text-right">Date</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-100 dark:divide-zinc-800/70">
                  {visibleHistory.map((rep, i) => {
                    const totalLoot = rep.gold_stolen + rep.bonus_gold + rep.elixir_stolen + rep.bonus_elixir;
                    const date = new Date(rep.timestamp);
                    return (
                      <tr key={`${rep.timestamp}-${i}`} className="group hover:bg-zinc-50/70 dark:hover:bg-zinc-800/30 transition-colors">
                        <td className="px-6 py-4">
                          <div className="flex items-center gap-3">
                            <div className={`w-10 h-10 rounded-xl flex items-center justify-center ${
                              rep.stars >= 2
                                ? 'bg-emerald-500/10 text-emerald-500'
                                : rep.stars === 1
                                  ? 'bg-amber-500/10 text-amber-500'
                                  : 'bg-rose-500/10 text-rose-500'
                            }`}>
                              <span className="material-symbols-outlined text-xl">military_tech</span>
                            </div>
                            <div>
                              <div className="flex items-center gap-1">
                                {[0, 1, 2].map((star) => (
                                  <span
                                    key={star}
                                    className={`material-symbols-outlined text-base ${
                                      star < rep.stars ? 'text-amber-400' : 'text-zinc-200 dark:text-zinc-700'
                                    }`}
                                    style={{ fontVariationSettings: star < rep.stars ? "'FILL' 1" : "'FILL' 0" }}
                                  >
                                    star
                                  </span>
                                ))}
                              </div>
                              <div className="text-[9px] font-black text-zinc-400 uppercase tracking-wider mt-0.5">
                                Attack #{rep.total_attacks_session || history.length - i}
                              </div>
                              {rep.stars_source && (
                                <div className="text-[8px] font-bold text-zinc-400 uppercase tracking-wider mt-0.5">
                                  stars: {rep.stars_source.split('_').join(' ')}
                                </div>
                              )}
                            </div>
                          </div>
                        </td>

                        <td className="px-4 py-4">
                          <div className="text-sm font-bold text-zinc-800 dark:text-zinc-200">{rep.strategy || 'Inconnu'}</div>
                          <div className="text-[10px] font-black text-zinc-400 uppercase tracking-widest mt-1">
                            {sideLabel(rep.target_edge || 'auto')}{rep.deploy_side && rep.deploy_side !== 'Inconnu' ? ` → ${sideLabel(rep.deploy_side)}` : ''}
                          </div>
                          <div className="text-[9px] font-bold text-zinc-400 uppercase tracking-wider mt-1">
                            score {rep.target_score || 0}/100 · {runtimeModeLabel(rep.runtime_mode)}
                          </div>
                        </td>

                        <td className="px-4 py-4">
                          <div className="flex items-center gap-4 text-xs font-bold tabular-nums">
                            <span className="text-amber-500">{(rep.gold_stolen + rep.bonus_gold).toLocaleString()} G</span>
                            <span className="text-fuchsia-500">{(rep.elixir_stolen + rep.bonus_elixir).toLocaleString()} E</span>
                            <span className="text-zinc-600 dark:text-zinc-300">{(rep.dark_elixir_stolen + rep.bonus_de).toLocaleString()} DE</span>
                          </div>
                          <div className="text-[9px] text-zinc-400 font-black uppercase tracking-wider mt-1">
                            {(totalLoot / 1e6).toFixed(2)}M combiné
                          </div>
                          {(rep.loot_source || rep.result_confidence) && (
                            <div className="text-[9px] text-zinc-400 font-bold uppercase tracking-wider mt-1">
                              {(rep.loot_source || 'inconnu').split('_').join(' ')} · {rep.result_confidence || 'inconnu'} confiance
                            </div>
                          )}
                        </td>

                        <td className="px-4 py-4">
                          <div className={`inline-flex items-center gap-2 px-3 py-1.5 rounded-xl text-[10px] font-black uppercase tracking-wider border ${
                            rep.deploy_success
                              ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/20'
                              : 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20'
                          }`}>
                            <span className="material-symbols-outlined text-sm">{rep.deploy_success ? 'check_circle' : 'warning'}</span>
                            {rep.deploy_success ? 'Complet' : `${rep.undeployed_slots} restant(s)`}
                          </div>
                          <div className="text-[9px] text-zinc-400 font-black uppercase tracking-wider mt-1.5 tabular-nums">
                            {((rep.deploy_duration_ms || 0) / 1000).toFixed(1)}s déploiement
                          </div>
                          {!rep.parsed_results && (
                            <div className="text-[9px] text-rose-500 font-black uppercase tracking-wider mt-1.5">Lecture OCR du résultat incomplète</div>
                          )}
                        </td>

                        <td className="px-4 py-4">
                          <div className="text-sm font-black text-zinc-700 dark:text-zinc-200 tabular-nums">
                            {((rep.search_duration_ms || 0) / 1000).toFixed(1)}s
                          </div>
                          <div className="text-[9px] font-black text-zinc-400 uppercase tracking-wider mt-1">
                            {rep.search_skips || 0} ignorés · {((rep.full_routine_duration_ms || rep.cycle_duration_ms || 0) / 1000).toFixed(0)}s cycle réel
                          </div>
                          <div className="text-[9px] font-bold text-zinc-400 uppercase tracking-wider mt-1">
                            {rep.destruction_pct || 0}% · {(rep.battle_end_reason || 'inconnu').split('_').join(' ')}
                          </div>
                        </td>

                        <td className="px-6 py-4 text-right">
                          <div className="text-xs font-black text-zinc-600 dark:text-zinc-300 tabular-nums">
                            {date.toLocaleDateString([], { day: '2-digit', month: '2-digit' })}
                          </div>
                          <div className="text-[10px] font-bold text-zinc-400 tabular-nums mt-1">
                            {date.toLocaleTimeString([], { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>

            <div className="px-6 py-4 border-t border-zinc-100 dark:border-zinc-800/70 flex items-center justify-between gap-4 bg-zinc-50/40 dark:bg-zinc-800/10">
              <div className="text-[10px] font-black uppercase tracking-widest text-zinc-400">
                Affichage {Math.min(historyLimit, filteredHistory.length)} sur {filteredHistory.length} attaques sauvegardées
              </div>
              <div className="flex items-center gap-2">
                {historyLimit > 10 && (
                  <button
                    onClick={() => setHistoryLimit(10)}
                    className="h-9 px-4 rounded-xl border border-zinc-200 dark:border-zinc-700 text-[10px] font-black uppercase tracking-widest text-zinc-500 hover:text-zinc-900 dark:hover:text-white transition-colors"
                  >
                    Réduire
                  </button>
                )}
                {historyLimit < filteredHistory.length && (
                  <button
                    onClick={() => setHistoryLimit((n) => Math.min(n + 15, filteredHistory.length))}
                    className="h-9 px-4 rounded-xl bg-zinc-950 dark:bg-white text-white dark:text-zinc-950 text-[10px] font-black uppercase tracking-widest transition-transform active:scale-95"
                  >
                    Show more
                  </button>
                )}
              </div>
            </div>
          </>
        )}
      </section>

      {/* Summary Row */}
      <div className={(activityPage === 'summary' ? '' : 'hidden ') + "grid grid-cols-2 lg:grid-cols-4 gap-8"}>
        {[
          { label: 'Villages analysés', value: stats.search_skips + stats.attacks_completed, icon: 'search', detail: `${stats.search_skips} ignorés` },
          { label: 'Attaques', value: stats.attacks_completed, icon: 'bolt' },
          { label: 'Butin total', value: `${((stats.total_gold + stats.total_elixir) / 1e6).toFixed(1)}M`, icon: 'trending_up' },
          { label: 'Temps actif', value: formatUptime(stats.uptime), icon: 'timer' }
        ].map((item, idx) => (
          <div key={idx} className="group bg-white dark:bg-zinc-900 p-5 rounded-[2rem] border border-zinc-100/50 dark:border-zinc-800/50 flex items-center gap-6 shadow-premium dark:shadow-none transition-all duration-500 hover:bg-zinc-50 dark:hover:bg-zinc-800/40">
             <div className="w-14 h-14 rounded-2xl bg-zinc-50 dark:bg-zinc-800 flex items-center justify-center border border-zinc-100 dark:border-zinc-700 shadow-sm transition-transform group-hover:scale-110">
                <span className="material-symbols-outlined text-zinc-500 dark:text-zinc-500 text-2xl">{item.icon}</span>
             </div>
             <div className="flex flex-col min-w-0">
                <div className="text-[10px] font-black text-zinc-500 dark:text-zinc-500 uppercase tracking-[0.2em] mb-0.5">{item.label}</div>
                <div className="flex items-baseline gap-2">
                  <div className="text-2xl font-bold text-zinc-950 dark:text-white tracking-tight">{item.value}</div>
                  {item.detail && (
                    <div className="text-[9px] font-black text-zinc-400 dark:text-zinc-700 uppercase tracking-widest tabular-nums">{item.detail}</div>
                  )}
                </div>
             </div>
          </div>
        ))}
      </div>

      {/* Logs Terminal */}
      <section className={(activityPage === 'console' ? '' : 'hidden ') + "bg-white dark:bg-black rounded-[3rem] p-3 shadow-premium-lg border border-zinc-200/60 dark:border-zinc-900/80 transition-all duration-500"}>
        <div className="px-8 py-3 flex flex-wrap items-center gap-4 justify-between border-b border-zinc-100 dark:border-zinc-900/50">
          <div className="flex items-center gap-4">
            <div className="flex gap-2">
              <div className="w-3 h-3 rounded-full bg-rose-500/20 border border-rose-500/40"></div>
              <div className="w-3 h-3 rounded-full bg-amber-500/20 border border-amber-500/40"></div>
              <div className="w-3 h-3 rounded-full bg-emerald-500/20 border border-emerald-500/40"></div>
            </div>
            <span className="text-[10px] font-black text-zinc-600 dark:text-zinc-500 uppercase tracking-[0.3em]">Console système</span>
          </div>
          <div className="flex items-center gap-3">
            <button
              onClick={() => setTerminalAutoScroll(!terminalAutoScroll)}
              className={`flex items-center gap-2 h-9 px-4 rounded-xl transition-all uppercase tracking-[0.2em] text-[10px] font-black border ${
                terminalAutoScroll
                  ? 'bg-emerald-500/10 text-emerald-500 border-emerald-500/30 hover:bg-emerald-500/20'
                  : 'bg-zinc-100 dark:bg-zinc-900/50 text-zinc-500 border-zinc-200 dark:border-zinc-800/50 hover:text-zinc-700 dark:hover:text-white hover:bg-zinc-200 dark:hover:bg-zinc-800'
              }`}
              title={terminalAutoScroll ? 'Défilement auto activé (cliquer pour désactiver)' : 'Défilement auto désactivé (cliquer pour activer)'}
            >
              <span className="material-symbols-outlined text-sm">
                {terminalAutoScroll ? 'keyboard_arrow_down' : 'keyboard_arrow_up'}
              </span>
              Auto
            </button>
            <button
              onClick={() => void copyText(exportText)}
              className="h-9 px-5 rounded-xl bg-zinc-100 dark:bg-zinc-900/50 text-[10px] font-black text-zinc-500 hover:text-zinc-700 dark:hover:text-white hover:bg-zinc-200 dark:hover:bg-zinc-800 transition-all uppercase tracking-[0.2em] border border-zinc-200 dark:border-zinc-800/50 active:scale-95"
              title="Copier toute la console dans le presse-papiers (sans ANSI)"
            >
              Export Logs
            </button>
          </div>
        </div>

        {/* Filter bar: text search + severity chips. */}
        <div className="px-8 py-3 flex flex-wrap items-center gap-3 border-b border-zinc-100 dark:border-zinc-900/50">
          <div className="relative flex-1 min-w-[180px] max-w-sm">
            <span className="absolute left-3 top-1/2 -translate-y-1/2 material-symbols-outlined text-sm text-zinc-600">search</span>
            <input
              value={logFilter}
              onChange={(e) => setLogFilter(e.target.value)}
              placeholder="Filtrer les logs…"
              aria-label="Filtrer les logs par texte"
              className="w-full h-9 pl-9 pr-8 rounded-xl bg-white dark:bg-zinc-900/50 border border-zinc-200 dark:border-zinc-800/60 text-xs font-bold text-zinc-800 dark:text-zinc-200 placeholder:text-zinc-400 dark:placeholder:text-zinc-600 focus:outline-none focus:border-emerald-500/40 focus:ring-2 focus:ring-emerald-500/10 transition-all"
            />
            {logFilter && (
              <button
                onClick={() => setLogFilter('')}
                className="absolute right-2.5 top-1/2 -translate-y-1/2 text-zinc-400 dark:text-zinc-600 hover:text-zinc-700 dark:hover:text-zinc-300 transition-colors"
                aria-label="Effacer le filtre"
              >
                <span className="material-symbols-outlined text-sm">close</span>
              </button>
            )}
          </div>
          <div className="flex items-center gap-2 text-[10px] font-black text-zinc-400 dark:text-zinc-600 uppercase tracking-widest tabular-nums whitespace-nowrap" aria-live="polite">
            {filteredLogs.length}<span className="text-zinc-300 dark:text-zinc-700">/</span>{parsedLogs.length} lines
          </div>
          <div className="flex items-center gap-1.5 flex-wrap">
            {severityChips.map((chip) => {
              const count = chip.id === 'all' ? parsedLogs.length : severityCounts[chip.id];
              const isActive = severityFilter === chip.id;
              return (
                <button
                  key={chip.id}
                  onClick={() => setSeverityFilter(chip.id)}
                  aria-pressed={isActive}
                  className={`h-8 px-3 rounded-lg text-[10px] font-black uppercase tracking-widest border transition-all flex items-center gap-1.5 ${
                    isActive
                      ? chip.active
                      : 'text-zinc-500 border-zinc-200 dark:border-zinc-800/50 hover:text-zinc-700 dark:hover:text-zinc-300 hover:border-zinc-300 dark:hover:border-zinc-700'
                  }`}
                >
                  <span className={`w-1.5 h-1.5 rounded-full ${chip.dot} ${isActive ? '' : 'opacity-40'}`}></span>
                  {chip.label}
                  <span className={`tabular-nums ${isActive ? '' : 'opacity-40'}`}>{count}</span>
                </button>
              );
            })}
          </div>
        </div>

        <div
          ref={containerRef}
          role="log"
          aria-label="Console système — sortie du bot en direct"
          onMouseEnter={() => setTerminalHovered(true)}
          onMouseLeave={() => setTerminalHovered(false)}
          className="p-5 h-80 terminal-scroll overflow-y-auto font-mono text-[13px] leading-relaxed text-zinc-600 dark:text-zinc-400 selection:bg-emerald-500/20"
        >
          <div className="space-y-1">
            {parsedLogs.length === 0 ? (
              <div className="flex items-center gap-4 text-zinc-400 dark:text-zinc-700 py-2">
                <div className="w-2 h-2 bg-zinc-300 dark:bg-zinc-700 rounded-full animate-pulse"></div>
                <span className="italic uppercase tracking-[0.3em] font-black text-[10px]">Initialisation de la connexion…</span>
              </div>
            ) : filteredLogs.length === 0 ? (
              <div className="text-zinc-400 dark:text-zinc-700 py-2 text-[11px] font-bold uppercase tracking-[0.25em]">
                Aucun journal ne correspond au filtre actuel
              </div>
            ) : (
              filteredLogs.map((line, i) => (
                <div key={i} className="flex items-start gap-3 group/log hover:bg-zinc-100 dark:hover:bg-zinc-900/50 rounded-lg px-2 -mx-2 py-1 transition-colors">
                  <span className="text-zinc-400 dark:text-zinc-700 shrink-0 font-bold tabular-nums text-xs pt-px">
                    {line.timestamp || '--:--:--'}
                  </span>
                  <span className="shrink-0 text-[10px] font-black w-12 pt-px text-center uppercase tracking-wider tabular-nums">
                    <span className={line.level === 'error' ? 'text-rose-500' : line.level === 'warn' ? 'text-amber-500' : line.level === 'debug' ? 'text-violet-500' : line.level === 'success' ? 'text-emerald-500' : 'text-zinc-600'}>
                      {line.level === 'success' ? 'OK' : line.level}
                    </span>
                  </span>
                  <span
                    className={`flex-1 break-words ${
                      line.level === 'error' ? 'text-rose-600 dark:text-rose-400 font-semibold' :
                      line.level === 'warn' ? 'text-amber-600 dark:text-amber-300/90' :
                      line.level === 'debug' ? 'text-violet-600 dark:text-violet-300/70' :
                      line.level === 'success' ? 'text-emerald-600 dark:text-emerald-400' :
                      'text-zinc-600 dark:text-zinc-300'
                    }`}
                  >
                    {highlightMatch(line.message)}
                  </span>
                  <button
                    onClick={() => copyLine(i, line.message)}
                    className={`opacity-0 group-hover/log:opacity-100 transition-all p-0.5 -mr-1 ${
                      copiedIdx === i
                        ? 'text-emerald-500 opacity-100'
                        : 'text-zinc-400 dark:text-zinc-600 hover:text-zinc-700 dark:hover:text-white'
                    }`}
                    title="Copier la ligne"
                    aria-label="Copier la ligne de log"
                  >
                    <span className="material-symbols-outlined text-sm">
                      {copiedIdx === i ? 'check' : 'content_copy'}
                    </span>
                  </button>
                </div>
              ))
            )}
          </div>
        </div>
      </section>
    </div>
  );
});

export default Dashboard;
