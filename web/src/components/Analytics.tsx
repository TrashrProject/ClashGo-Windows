import React from 'react';
import { BotStats, VillageResourceSnapshot } from '../types';

interface AnalyticsProps {
  stats: BotStats;
  resourceHistory: VillageResourceSnapshot[];
}

const Analytics: React.FC<AnalyticsProps> = React.memo(({ stats, resourceHistory }) => {
  // `color` drives Tailwind bar classes; `hex` feeds the conic-gradient
  // (Tailwind class names are NOT valid CSS color values — using them
  // inside the gradient string would silently drop the donut).
  const starData = [
    { label: '3 Stars', count: stats.stars_3, color: 'bg-emerald-500', hex: '#10b981', bg: 'bg-emerald-500/10' },
    { label: '2 Stars', count: stats.stars_2, color: 'bg-zinc-800', hex: '#27272a', bg: 'bg-zinc-800/10' },
    { label: '1 Star', count: stats.stars_1, color: 'bg-zinc-400', hex: '#a1a1aa', bg: 'bg-zinc-400/10' },
    { label: '0 Stars', count: stats.stars_0, color: 'bg-rose-500', hex: '#f43f5e', bg: 'bg-rose-500/10' },
  ];

  const validResourceHistory = resourceHistory.filter((s) => s.valid);
  const firstResource = validResourceHistory[0];
  const lastResource = validResourceHistory[validResourceHistory.length - 1];
  const resourceDelta = {
    gold: firstResource && lastResource ? lastResource.gold - firstResource.gold : 0,
    elixir: firstResource && lastResource ? lastResource.elixir - firstResource.elixir : 0,
    dark: firstResource && lastResource ? lastResource.dark_elixir - firstResource.dark_elixir : 0,
  };
  const formatSigned = (v: number) => (v > 0 ? '+' : '') + v.toLocaleString();

  const totalAttacks = stats.stars_3 + stats.stars_2 + stats.stars_1 + stats.stars_0;
  const getPercent = (count: number) => totalAttacks > 0 ? Math.round((count / totalAttacks) * 100) : 0;
  const threeStarRate = totalAttacks > 0 ? Math.round((stats.stars_3 / totalAttacks) * 100) : 0;

  // CSS-only donut (conic-gradient — no chart dependency). Each
  // segment's sweep is the star-rate percentage mapped to degrees;
  // zero-count segments collapse to a 0deg stop and stay invisible.
  let sweep = 0;
  const gradientStops = starData.map((s) => {
    const from = sweep;
    sweep += getPercent(s.count) * 3.6;
    return `${s.hex} ${from}deg ${Math.max(sweep, from)}deg`;
  });
  // Donut is only rendered when totalAttacks > 0; zero-count segments
  // collapse to a 0deg stop and stay invisible.
  const donutBg = `conic-gradient(${gradientStops.join(', ')})`;

  return (
    <div className="grid grid-cols-1 xl:grid-cols-2 gap-8 max-w-6xl mx-auto">

      <div className="xl:col-span-2 bg-white dark:bg-zinc-900 p-8 rounded-[3rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none">
        <div className="flex flex-col md:flex-row md:items-end md:justify-between gap-4 mb-6">
          <div>
            <h3 className="text-2xl font-bold text-zinc-950 dark:text-white tracking-tight">Village Resource Tracking</h3>
            <p className="text-sm text-zinc-500 mt-1">Automatic BlueStacks snapshots. No manual entry required.</p>
          </div>
          <div className="text-[10px] font-black uppercase tracking-widest text-zinc-400">
            {validResourceHistory.length} snapshots
          </div>
        </div>

        {lastResource ? (
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            {[
              { label: 'Gold', current: lastResource.gold, delta: resourceDelta.gold, icon: 'monetization_on' },
              { label: 'Elixir', current: lastResource.elixir, delta: resourceDelta.elixir, icon: 'water_drop' },
              { label: 'Dark Elixir', current: lastResource.dark_elixir, delta: resourceDelta.dark, icon: 'opacity' },
            ].map((item) => (
              <div key={item.label} className="rounded-2xl border border-zinc-100 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-950/30 p-5">
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <div className="text-[10px] font-black uppercase tracking-[0.18em] text-zinc-400">{item.label}</div>
                    <div className="mt-2 text-2xl font-black text-zinc-950 dark:text-white tabular-nums">{item.current.toLocaleString()}</div>
                  </div>
                  <span className="material-symbols-outlined text-zinc-400">{item.icon}</span>
                </div>
                <div className={`mt-3 text-xs font-black tabular-nums ${item.delta >= 0 ? 'text-emerald-500' : 'text-rose-500'}`}>
                  {formatSigned(item.delta)} since first snapshot
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="py-10 text-center text-zinc-400 text-xs font-black uppercase tracking-widest">
            Waiting for ClashGO to scan the home village
          </div>
        )}
      </div>

      <div className="bg-white dark:bg-zinc-900 p-8 rounded-[3rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none group transition-all duration-500">
        <div className="flex justify-between items-center mb-8">
          <h3 className="text-2xl font-bold text-zinc-950 dark:text-white tracking-tight">Attack Success</h3>
          <div className="w-12 h-12 rounded-2xl bg-zinc-50 dark:bg-zinc-800 flex items-center justify-center border border-zinc-100 dark:border-zinc-700 shadow-sm transition-transform group-hover:scale-110">
            <span className="material-symbols-outlined text-zinc-500 dark:text-zinc-500">equalizer</span>
          </div>
        </div>
        {totalAttacks === 0 ? (
          <div className="py-12 text-center text-zinc-400 dark:text-zinc-700 text-[11px] font-black uppercase tracking-[0.3em] italic">
            No attacks recorded yet // Run the bot to populate analytics
          </div>
        ) : (
        <div className="grid grid-cols-1 sm:grid-cols-[auto_1fr] gap-10 items-center">
          {/* Star-distribution donut — center hole carries the 3★ rate. */}
          <div className="relative w-36 h-36 mx-auto rounded-full transition-transform duration-500 group-hover:scale-[1.03]" style={{ background: donutBg }}>
            <div className="absolute inset-[16px] bg-white dark:bg-zinc-900 rounded-full flex flex-col items-center justify-center border border-zinc-100 dark:border-zinc-800/60 shadow-sm">
              <span className="text-3xl font-bold text-zinc-950 dark:text-white tabular-nums tracking-tight leading-none">{threeStarRate}%</span>
              <span className="mt-1.5 text-[9px] font-black text-zinc-400 dark:text-zinc-600 uppercase tracking-[0.2em]">3★ Rate</span>
            </div>
          </div>
          <div className="space-y-6">
            {starData.map((item, idx) => {
              const percent = getPercent(item.count);
              return (
                <div key={idx} className="space-y-4">
                  <div className="flex justify-between text-[11px] font-black text-zinc-500 dark:text-zinc-500 uppercase tracking-[0.2em]">
                    <span>{item.label}</span>
                    <span className="text-zinc-950 dark:text-white tabular-nums">{item.count} <span className="text-zinc-400 dark:text-zinc-700 ml-2 font-bold">({percent}%)</span></span>
                  </div>
                  <div className="w-full bg-zinc-50 dark:bg-zinc-800/50 h-3.5 rounded-full overflow-hidden p-0.5 border border-zinc-100/50 dark:border-zinc-800/50">
                    <div 
                      className={`h-full transition-all duration-1000 rounded-full ${item.color} shadow-sm`} 
                      style={{ width: `${percent}%` }}
                    ></div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
        )}
      </div>

      <div className="bg-white dark:bg-zinc-900 p-8 rounded-[3rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none transition-all duration-500">
        <div className="flex justify-between items-center mb-8">
          <h3 className="text-2xl font-bold text-zinc-950 dark:text-white tracking-tight">Financial Performance</h3>
          <div className="w-12 h-12 rounded-2xl bg-zinc-50 dark:bg-zinc-800 flex items-center justify-center border border-zinc-100 dark:border-zinc-700 shadow-sm">
            <span className="material-symbols-outlined text-zinc-500 dark:text-zinc-500">insights</span>
          </div>
        </div>
        {stats.attacks_completed === 0 ? (
          <div className="py-12 text-center text-zinc-400 dark:text-zinc-700 text-[11px] font-black uppercase tracking-[0.3em] italic">
            No revenue recorded yet // Run the bot to populate metrics
          </div>
        ) : (
        <div className="space-y-8">
          {[
            { label: 'Total Revenue', value: (stats.total_gold + stats.total_elixir).toLocaleString(), icon: 'account_balance_wallet', color: 'text-zinc-950 dark:text-zinc-100' },
            { label: 'Avg Gold / Attack', value: stats.attacks_completed > 0 ? Math.round(stats.total_gold / stats.attacks_completed).toLocaleString() : '0', icon: 'monetization_on', color: 'text-amber-500' },
            { label: 'Avg Elixir / Attack', value: stats.attacks_completed > 0 ? Math.round(stats.total_elixir / stats.attacks_completed).toLocaleString() : '0', icon: 'water_drop', color: 'text-fuchsia-500' },
            { label: 'Avg Dark Elixir / Attack', value: stats.attacks_completed > 0 ? Math.round(stats.total_de / stats.attacks_completed).toLocaleString() : '0', icon: 'water_drop', color: 'text-zinc-950 dark:text-zinc-100' }
          ].map((m, i) => (
            <div key={i} className="flex justify-between items-center group">
              <div className="flex items-center gap-6">
                <div className="w-14 h-14 rounded-2xl bg-zinc-50 dark:bg-zinc-800 flex items-center justify-center border border-zinc-100 dark:border-zinc-700 shadow-sm group-hover:scale-110 transition-transform duration-300">
                  <span className={`material-symbols-outlined ${m.color} text-2xl`}>{m.icon}</span>
                </div>
                <div>
                  <span className="block text-[10px] font-black text-zinc-500 dark:text-zinc-500 uppercase tracking-[0.2em] mb-1">{m.label}</span>
                  <div className="text-xs text-zinc-400 dark:text-zinc-700 font-bold uppercase tracking-widest">Calculated Average</div>
                </div>
              </div>
              <span className="text-3xl font-bold text-zinc-950 dark:text-white tracking-tight tabular-nums">{m.value}</span>
            </div>
          ))}
        </div>
        )}
      </div>

    </div>
  );
});

export default Analytics;
