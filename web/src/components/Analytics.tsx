import React from 'react';
import { AttackReport, BotStats, VillageResourceSnapshot } from '../types';
import { formatUptime, parseLogLine, LogSeverity } from '../utils';

interface AnalyticsProps {
  stats: BotStats;
  resourceHistory: VillageResourceSnapshot[];
  history: AttackReport[];
  logs: string[];
}

const formatCompact = (value: number): string => {
  if (Math.abs(value) >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (Math.abs(value) >= 1_000) return `${(value / 1_000).toFixed(0)}K`;
  return value.toLocaleString();
};

const Analytics: React.FC<AnalyticsProps> = React.memo(({ stats, resourceHistory, history, logs }) => {
  const [attackLimit, setAttackLimit] = React.useState(15);
  const [logFilter, setLogFilter] = React.useState<LogSeverity | 'all'>('all');

  const validResourceHistory = React.useMemo(
    () => (resourceHistory ?? []).filter((s) => s.valid),
    [resourceHistory],
  );
  const firstResource = validResourceHistory[0];
  const lastResource = validResourceHistory[validResourceHistory.length - 1];
  const resourceDelta = {
    gold: firstResource && lastResource ? lastResource.gold - firstResource.gold : 0,
    elixir: firstResource && lastResource ? lastResource.elixir - firstResource.elixir : 0,
    dark: firstResource && lastResource ? lastResource.dark_elixir - firstResource.dark_elixir : 0,
  };

  const totalAttacks = stats.stars_3 + stats.stars_2 + stats.stars_1 + stats.stars_0;
  const threeStarRate = totalAttacks > 0 ? Math.round((stats.stars_3 / totalAttacks) * 100) : 0;
  const avgStars = totalAttacks > 0
    ? ((stats.stars_3 * 3 + stats.stars_2 * 2 + stats.stars_1) / totalAttacks).toFixed(2)
    : '0.00';

  const strategyStats = React.useMemo(() => {
    const map = new Map<string, {
      name: string;
      attacks: number;
      stars: number;
      gold: number;
      elixir: number;
      dark: number;
      completeDeploys: number;
    }>();

    for (const rep of history ?? []) {
      const name = rep.strategy || 'Unknown';
      const row = map.get(name) ?? {
        name, attacks: 0, stars: 0, gold: 0, elixir: 0, dark: 0, completeDeploys: 0,
      };
      row.attacks++;
      row.stars += rep.stars || 0;
      row.gold += (rep.gold_stolen || 0) + (rep.bonus_gold || 0);
      row.elixir += (rep.elixir_stolen || 0) + (rep.bonus_elixir || 0);
      row.dark += (rep.dark_elixir_stolen || 0) + (rep.bonus_de || 0);
      if (rep.deploy_success) row.completeDeploys++;
      map.set(name, row);
    }

    return Array.from(map.values()).sort((a, b) => b.attacks - a.attacks);
  }, [history]);

  const parsedLogs = React.useMemo(() => (logs ?? []).map(parseLogLine), [logs]);
  const logCounts = React.useMemo(() => {
    const counts: Record<LogSeverity, number> = { debug: 0, info: 0, success: 0, warn: 0, error: 0 };
    for (const item of parsedLogs) counts[item.level]++;
    return counts;
  }, [parsedLogs]);
  const visibleLogs = React.useMemo(
    () => parsedLogs.filter((item) => logFilter === 'all' || item.level === logFilter).slice(-120),
    [parsedLogs, logFilter],
  );

  const starData = [
    { label: '3 stars', count: stats.stars_3, className: 'bg-emerald-500' },
    { label: '2 stars', count: stats.stars_2, className: 'bg-zinc-700' },
    { label: '1 star', count: stats.stars_1, className: 'bg-zinc-400' },
    { label: '0 stars', count: stats.stars_0, className: 'bg-rose-500' },
  ];

  return (
    <div className="space-y-8 max-w-[1400px] mx-auto">
      <section>
        <div className="mb-5">
          <h3 className="text-2xl font-black text-zinc-950 dark:text-white tracking-tight">Session overview</h3>
          <p className="text-sm text-zinc-500 mt-1">All detailed activity is grouped here so the home page stays clean.</p>
        </div>
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
          {[
            { label: 'Attacks', value: stats.attacks_completed.toLocaleString(), detail: `${stats.search_skips.toLocaleString()} skipped`, icon: 'swords' },
            { label: '3★ rate', value: `${threeStarRate}%`, detail: `${avgStars} avg stars`, icon: 'star' },
            { label: 'Total loot', value: formatCompact(stats.total_gold + stats.total_elixir), detail: `${formatCompact(stats.total_de)} dark`, icon: 'account_balance_wallet' },
            { label: 'Uptime', value: formatUptime(stats.uptime), detail: `${stats.recovery_successes}/${stats.recovery_attempts} recoveries`, icon: 'timer' },
          ].map((item) => (
            <div key={item.label} className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 shadow-sm">
              <div className="flex items-center justify-between">
                <div>
                  <div className="text-[9px] font-black uppercase tracking-[0.2em] text-zinc-400">{item.label}</div>
                  <div className="text-2xl font-black text-zinc-950 dark:text-white mt-2">{item.value}</div>
                  <div className="text-[10px] text-zinc-500 mt-1">{item.detail}</div>
                </div>
                <span className="material-symbols-outlined text-zinc-400">{item.icon}</span>
              </div>
            </div>
          ))}
        </div>
      </section>

      <section className="bg-white dark:bg-zinc-900 p-6 md:p-8 rounded-[2.5rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none">
        <div className="flex flex-col md:flex-row md:items-end md:justify-between gap-4 mb-6">
          <div>
            <h3 className="text-xl font-black text-zinc-950 dark:text-white">Village resources</h3>
            <p className="text-sm text-zinc-500 mt-1">Automatic snapshots collected while ClashGO is at the village.</p>
          </div>
          <div className="text-[10px] font-black uppercase tracking-widest text-zinc-400">{validResourceHistory.length} snapshots</div>
        </div>

        {lastResource ? (
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            {[
              { label: 'Gold', current: lastResource.gold, delta: resourceDelta.gold, icon: 'monetization_on' },
              { label: 'Elixir', current: lastResource.elixir, delta: resourceDelta.elixir, icon: 'water_drop' },
              { label: 'Dark Elixir', current: lastResource.dark_elixir, delta: resourceDelta.dark, icon: 'opacity' },
            ].map((item) => (
              <div key={item.label} className="rounded-2xl border border-zinc-100 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-950/30 p-5">
                <div className="flex items-center justify-between">
                  <div>
                    <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">{item.label}</div>
                    <div className="text-2xl font-black text-zinc-950 dark:text-white mt-2 tabular-nums">{item.current.toLocaleString()}</div>
                  </div>
                  <span className="material-symbols-outlined text-zinc-400">{item.icon}</span>
                </div>
                <div className={`text-xs font-black mt-3 tabular-nums ${item.delta >= 0 ? 'text-emerald-500' : 'text-rose-500'}`}>
                  {item.delta > 0 ? '+' : ''}{item.delta.toLocaleString()} since first snapshot
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="py-10 text-center text-xs font-black uppercase tracking-widest text-zinc-400">Waiting for village scans</div>
        )}
      </section>

      <section className="grid grid-cols-1 xl:grid-cols-[0.9fr_1.1fr] gap-6">
        <div className="bg-white dark:bg-zinc-900 p-6 rounded-[2.5rem] border border-zinc-100 dark:border-zinc-800 shadow-sm">
          <div className="flex items-center justify-between mb-6">
            <div>
              <h3 className="text-xl font-black text-zinc-950 dark:text-white">Battle results</h3>
              <p className="text-xs text-zinc-500 mt-1">Star distribution for this session.</p>
            </div>
            <div className="text-right">
              <div className="text-3xl font-black text-zinc-950 dark:text-white">{avgStars}</div>
              <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">avg stars</div>
            </div>
          </div>
          <div className="space-y-4">
            {starData.map((item) => {
              const pct = totalAttacks > 0 ? Math.round((item.count / totalAttacks) * 100) : 0;
              return (
                <div key={item.label}>
                  <div className="flex items-center justify-between text-xs mb-2">
                    <span className="font-black text-zinc-600 dark:text-zinc-300">{item.label}</span>
                    <span className="font-black text-zinc-950 dark:text-white">{item.count} · {pct}%</span>
                  </div>
                  <div className="h-2.5 rounded-full bg-zinc-100 dark:bg-zinc-800 overflow-hidden">
                    <div className={`h-full rounded-full ${item.className}`} style={{ width: `${pct}%` }} />
                  </div>
                </div>
              );
            })}
          </div>
        </div>

        <div className="bg-white dark:bg-zinc-900 p-6 rounded-[2.5rem] border border-zinc-100 dark:border-zinc-800 shadow-sm">
          <div className="mb-6">
            <h3 className="text-xl font-black text-zinc-950 dark:text-white">Automation operations</h3>
            <p className="text-xs text-zinc-500 mt-1">Donations, army corrections and recovery activity.</p>
          </div>
          <div className="mb-4 rounded-2xl border border-zinc-100 dark:border-zinc-800 bg-zinc-50/70 dark:bg-zinc-800/30 p-4">
            <div className="flex items-center justify-between gap-4">
              <div>
                <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">Exclusive scheduler</div>
                <div className="mt-1 text-sm font-black text-zinc-950 dark:text-white capitalize">
                  {stats.automation_busy ? (stats.automation_task || 'working') : 'Ready for next task'}
                </div>
                <div className="mt-1 text-[10px] text-zinc-500">
                  {stats.wall_upgrade_pending ? 'Wall maintenance is queued. ' : ''}
                  {stats.automation_last_task ? `Last completed: ${stats.automation_last_task}.` : 'No completed task yet.'}
                  {stats.automation_busy && (stats.automation_task_age_sec ?? 0) > 0 ? ` Running for ${stats.automation_task_age_sec}s.` : ''}
                </div>
              </div>
              <div className="text-right">
                <div className="text-2xl font-black text-zinc-950 dark:text-white">{stats.automation_tasks_completed ?? 0}</div>
                <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">tasks completed</div>
              </div>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            {[
              { label: 'Scheduler starts', value: stats.automation_tasks_started ?? 0, detail: stats.automation_busy ? `Running ${stats.automation_task || 'task'}` : 'No overlapping UI tasks' },
              { label: 'Recovered task panics', value: stats.automation_task_panics ?? 0, detail: (stats.automation_task_panics ?? 0) === 0 ? 'No scheduler panics recovered' : 'See activity log for details' },
              { label: 'Donation checks', value: stats.donation_checks ?? 0, detail: stats.last_donation_result || 'No check yet' },
              { label: 'Verified donations', value: stats.donations_sent ?? 0, detail: stats.last_donation_unix ? new Date(stats.last_donation_unix * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : 'None yet' },
              { label: 'Army preflight', value: stats.army_check_pending ? 'Queued' : 'Ready', detail: stats.army_check_pending ? 'Will run before matchmaking' : 'Latest required check completed' },
              { label: 'Army repairs', value: stats.army_repair_successes ?? 0, detail: `${stats.army_repair_attempts ?? 0} attempts` },
              { label: 'Recoveries', value: stats.recovery_successes ?? 0, detail: `${stats.recovery_attempts ?? 0} attempts` },
              { label: 'BlueStacks restarts', value: stats.bluestacks_restarts ?? 0, detail: `${stats.adb_health.errors_total ?? 0} connection errors` },
            ].map((item) => (
              <div key={item.label} className="rounded-2xl bg-zinc-50 dark:bg-zinc-800/40 border border-zinc-100 dark:border-zinc-800 p-4">
                <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">{item.label}</div>
                <div className="text-2xl font-black text-zinc-950 dark:text-white mt-2">{item.value}</div>
                <div className="text-[10px] text-zinc-500 mt-1 truncate" title={item.detail}>{item.detail}</div>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="bg-white dark:bg-zinc-900 rounded-[2.5rem] border border-zinc-100 dark:border-zinc-800 shadow-sm overflow-hidden">
        <div className="p-6 border-b border-zinc-100 dark:border-zinc-800 flex flex-col md:flex-row md:items-center md:justify-between gap-4">
          <div>
            <h3 className="text-xl font-black text-zinc-950 dark:text-white">Attack history</h3>
            <p className="text-xs text-zinc-500 mt-1">Saved locally and kept across restarts.</p>
          </div>
          <div className="text-[10px] font-black uppercase tracking-widest text-zinc-400">{history?.length ?? 0} battles</div>
        </div>

        {(history?.length ?? 0) === 0 ? (
          <div className="py-12 text-center text-xs font-black uppercase tracking-widest text-zinc-400">No attacks recorded yet</div>
        ) : (
          <>
            <div className="overflow-x-auto">
              <table className="w-full min-w-[940px] text-left">
                <thead className="bg-zinc-50/70 dark:bg-zinc-800/30">
                  <tr className="text-[9px] font-black uppercase tracking-widest text-zinc-400">
                    <th className="px-6 py-3">Time</th>
                    <th className="px-4 py-3">Strategy</th>
                    <th className="px-4 py-3">Stars</th>
                    <th className="px-4 py-3">Deploy</th>
                    <th className="px-4 py-3">Gold</th>
                    <th className="px-4 py-3">Elixir</th>
                    <th className="px-4 py-3">Dark</th>
                    <th className="px-4 py-3">Edge</th>
                  </tr>
                </thead>
                <tbody>
                  {(history ?? []).slice(0, attackLimit).map((rep, idx) => (
                    <tr key={`${rep.timestamp}-${idx}`} className="border-t border-zinc-100 dark:border-zinc-800 text-sm">
                      <td className="px-6 py-4 font-bold text-zinc-500">{rep.timestamp ? new Date(rep.timestamp).toLocaleString() : '—'}</td>
                      <td className="px-4 py-4 font-black text-zinc-950 dark:text-white">{rep.strategy || 'Unknown'}</td>
                      <td className="px-4 py-4 font-black text-amber-500">{rep.stars} ★</td>
                      <td className="px-4 py-4">
                        <span className={rep.deploy_success ? 'text-emerald-500 font-black' : 'text-amber-500 font-black'}>
                          {rep.deploy_success ? 'Complete' : `${rep.undeployed_slots} left`}
                        </span>
                      </td>
                      <td className="px-4 py-4 font-bold text-amber-500 tabular-nums">{((rep.gold_stolen || 0) + (rep.bonus_gold || 0)).toLocaleString()}</td>
                      <td className="px-4 py-4 font-bold text-fuchsia-500 tabular-nums">{((rep.elixir_stolen || 0) + (rep.bonus_elixir || 0)).toLocaleString()}</td>
                      <td className="px-4 py-4 font-bold text-zinc-500 tabular-nums">{((rep.dark_elixir_stolen || 0) + (rep.bonus_de || 0)).toLocaleString()}</td>
                      <td className="px-4 py-4 font-bold text-zinc-500">{rep.target_edge || 'Auto'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {attackLimit < (history?.length ?? 0) && (
              <div className="p-4 border-t border-zinc-100 dark:border-zinc-800 text-center">
                <button
                  type="button"
                  onClick={() => setAttackLimit((value) => value + 15)}
                  className="px-4 py-2 rounded-xl bg-zinc-100 dark:bg-zinc-800 text-[10px] font-black uppercase tracking-widest text-zinc-600 dark:text-zinc-300"
                >
                  Show more
                </button>
              </div>
            )}
          </>
        )}
      </section>

      <section className="bg-white dark:bg-zinc-900 p-6 md:p-8 rounded-[2.5rem] border border-zinc-100 dark:border-zinc-800 shadow-sm">
        <div className="flex items-center justify-between gap-4 mb-6">
          <div>
            <h3 className="text-xl font-black text-zinc-950 dark:text-white">Strategy performance</h3>
            <p className="text-xs text-zinc-500 mt-1">Average results grouped by strategy.</p>
          </div>
          <div className="text-[10px] font-black uppercase tracking-widest text-zinc-400">{strategyStats.length} strategies</div>
        </div>

        {strategyStats.length === 0 ? (
          <div className="py-10 text-center text-xs font-black uppercase tracking-widest text-zinc-400">Waiting for attack history</div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[760px] text-left">
              <thead>
                <tr className="text-[9px] font-black uppercase tracking-widest text-zinc-400 border-b border-zinc-100 dark:border-zinc-800">
                  <th className="pb-3 pr-4">Strategy</th>
                  <th className="pb-3 px-3">Attacks</th>
                  <th className="pb-3 px-3">Avg stars</th>
                  <th className="pb-3 px-3">Full deploy</th>
                  <th className="pb-3 px-3">Avg gold</th>
                  <th className="pb-3 px-3">Avg elixir</th>
                  <th className="pb-3 pl-3">Avg dark</th>
                </tr>
              </thead>
              <tbody>
                {strategyStats.map((row) => (
                  <tr key={row.name} className="border-b border-zinc-50 dark:border-zinc-800/60 last:border-0">
                    <td className="py-4 pr-4 text-sm font-black text-zinc-950 dark:text-white">{row.name}</td>
                    <td className="py-4 px-3 text-sm font-bold text-zinc-500">{row.attacks}</td>
                    <td className="py-4 px-3 text-sm font-bold text-zinc-500">{(row.stars / Math.max(1, row.attacks)).toFixed(2)}</td>
                    <td className="py-4 px-3 text-sm font-bold text-zinc-500">{Math.round((row.completeDeploys / Math.max(1, row.attacks)) * 100)}%</td>
                    <td className="py-4 px-3 text-sm font-bold text-amber-500">{Math.round(row.gold / Math.max(1, row.attacks)).toLocaleString()}</td>
                    <td className="py-4 px-3 text-sm font-bold text-fuchsia-500">{Math.round(row.elixir / Math.max(1, row.attacks)).toLocaleString()}</td>
                    <td className="py-4 pl-3 text-sm font-bold text-zinc-500">{Math.round(row.dark / Math.max(1, row.attacks)).toLocaleString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section className="bg-zinc-950 rounded-[2.5rem] border border-zinc-800 overflow-hidden shadow-xl">
        <div className="p-6 border-b border-zinc-800 flex flex-col lg:flex-row lg:items-center lg:justify-between gap-4">
          <div>
            <h3 className="text-xl font-black text-white">Activity log</h3>
            <p className="text-xs text-zinc-500 mt-1">Technical events are kept here instead of cluttering the home page.</p>
          </div>
          <div className="flex flex-wrap gap-2">
            {([
              ['all', 'All', parsedLogs.length],
              ['success', 'Success', logCounts.success],
              ['warn', 'Warnings', logCounts.warn],
              ['error', 'Errors', logCounts.error],
            ] as const).map(([id, label, count]) => (
              <button
                key={id}
                type="button"
                onClick={() => setLogFilter(id)}
                className={`px-3 py-2 rounded-xl border text-[9px] font-black uppercase tracking-widest ${
                  logFilter === id
                    ? 'bg-white text-zinc-950 border-white'
                    : 'bg-zinc-900 text-zinc-400 border-zinc-800 hover:text-white'
                }`}
              >
                {label} · {count}
              </button>
            ))}
          </div>
        </div>

        <div className="max-h-[420px] overflow-auto p-4 font-mono text-[11px] leading-relaxed">
          {visibleLogs.length === 0 ? (
            <div className="py-10 text-center text-zinc-600 uppercase tracking-widest font-black">No log entries for this filter</div>
          ) : (
            visibleLogs.map((item, idx) => (
              <div key={idx} className="grid grid-cols-[72px_72px_1fr] gap-3 px-3 py-1.5 rounded-lg hover:bg-white/[0.03]">
                <span className="text-zinc-600">{item.timestamp || '—'}</span>
                <span className={
                  item.level === 'error' ? 'text-rose-400 font-black uppercase'
                  : item.level === 'warn' ? 'text-amber-400 font-black uppercase'
                  : item.level === 'success' ? 'text-emerald-400 font-black uppercase'
                  : 'text-zinc-500 font-black uppercase'
                }>{item.level}</span>
                <span className="text-zinc-300 break-words">{item.message}</span>
              </div>
            ))
          )}
        </div>
      </section>
    </div>
  );
});

export default Analytics;
