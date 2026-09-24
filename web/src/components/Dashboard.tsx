import React from 'react';
import { BotStats } from '../types';
import { formatUptime } from '../utils';
import AutomationOverview from './AutomationOverview';

interface DashboardProps {
  stats: BotStats;
}

const Dashboard: React.FC<DashboardProps> = React.memo(({ stats }) => {
  const nextActionIn = (stats.village_next_unix ?? 0) > Math.floor(Date.now() / 1000)
    ? Math.max(1, Math.ceil(stats.village_next_unix - Date.now() / 1000))
    : 0;

  return (
    <div className="space-y-6 max-w-6xl mx-auto">
      <AutomationOverview />

      <section className="bg-white dark:bg-zinc-900 rounded-[2.25rem] border border-zinc-100/70 dark:border-zinc-800/70 shadow-premium dark:shadow-none p-6">
        <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-5">
          <div className="flex items-start gap-4">
            <div className="w-12 h-12 rounded-2xl bg-emerald-500/10 text-emerald-500 flex items-center justify-center shrink-0">
              <span className="material-symbols-outlined">psychology</span>
            </div>
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Automation brain</div>
              <div className="text-2xl font-black text-zinc-950 dark:text-white capitalize mt-1">
                {stats.village_action || 'idle'}
              </div>
              <div className="text-sm text-zinc-500 mt-1 max-w-2xl">
                {stats.village_reason || 'ClashGO is waiting for the next safe action.'}
              </div>
            </div>
          </div>

          <div className="flex items-center gap-3">
            {nextActionIn > 0 && (
              <div className="px-4 py-3 rounded-2xl bg-zinc-50 dark:bg-zinc-800/60 border border-zinc-100 dark:border-zinc-700">
                <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">Next check</div>
                <div className="text-lg font-black text-zinc-950 dark:text-white tabular-nums">{nextActionIn}s</div>
              </div>
            )}
            <div className="px-4 py-3 rounded-2xl bg-zinc-50 dark:bg-zinc-800/60 border border-zinc-100 dark:border-zinc-700">
              <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">Session</div>
              <div className="text-lg font-black text-zinc-950 dark:text-white">{formatUptime(stats.uptime)}</div>
            </div>
          </div>
        </div>
      </section>

      {(stats.training_items_pending ?? 0) > 0 && (
        <section className="rounded-[2rem] border border-amber-500/20 bg-amber-500/[0.05] dark:bg-amber-500/[0.04] p-5">
          <div className="flex items-center gap-4">
            <div className="w-11 h-11 rounded-2xl bg-amber-500/10 text-amber-500 flex items-center justify-center">
              <span className="material-symbols-outlined">military_tech</span>
            </div>
            <div className="min-w-0 flex-1">
              <div className="text-[10px] font-black uppercase tracking-widest text-amber-600 dark:text-amber-400">Army check</div>
              <div className="text-sm font-black text-zinc-950 dark:text-white mt-1">
                {stats.training_items_pending} correction{stats.training_items_pending === 1 ? '' : 's'} detected in the selected recipe
              </div>
              <div className="text-xs text-zinc-500 mt-1">
                ClashGO will only continue once the army state is visually verified.
              </div>
            </div>
          </div>
        </section>
      )}

      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        {[
          { label: 'Attacks', value: stats.attacks_completed.toLocaleString(), icon: 'swords' },
          { label: 'Donations', value: (stats.donations_sent ?? 0).toLocaleString(), icon: 'volunteer_activism' },
          { label: 'Army repairs', value: (stats.army_repair_successes ?? 0).toLocaleString(), icon: 'autorenew' },
          { label: 'Connection', value: stats.adb_health.consecutive_fails === 0 ? 'Ready' : 'Check', icon: 'hub' },
        ].map((item) => (
          <div key={item.label} className="rounded-2xl border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 px-5 py-4 shadow-sm">
            <div className="flex items-center justify-between gap-3">
              <div>
                <div className="text-[9px] font-black uppercase tracking-[0.18em] text-zinc-400">{item.label}</div>
                <div className="text-xl font-black text-zinc-950 dark:text-white mt-1">{item.value}</div>
              </div>
              <span className="material-symbols-outlined text-zinc-400">{item.icon}</span>
            </div>
          </div>
        ))}
      </div>

      <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white/70 dark:bg-zinc-900/70 px-5 py-4">
        <div className="flex items-center gap-3 text-sm text-zinc-500">
          <span className="material-symbols-outlined text-zinc-400">insights</span>
          <span>
            Detailed attacks, loot, strategies, resources and technical history are grouped in <strong className="text-zinc-800 dark:text-zinc-200">Analytics</strong>.
          </span>
        </div>
      </section>
    </div>
  );
});

export default Dashboard;
