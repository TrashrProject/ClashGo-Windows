import React from 'react';
import { ActivityEvent, AttackReport, BotStats } from '../types';
import { formatUptime } from '../utils';

interface HomeViewProps {
  stats: BotStats;
  history: AttackReport[];
  activity: ActivityEvent[];
  running: boolean;
  starting: boolean;
  onStart: () => void;
  onStop: () => void;
  onOpenAutomation: () => void;
  onOpenAccount: () => void;
}

const formatLoot = (value: number): string => {
  if (!Number.isFinite(value) || value <= 0) return '0';
  if (value >= 1_000_000) return (value / 1_000_000).toFixed(value >= 10_000_000 ? 0 : 1) + 'M';
  if (value >= 1_000) return Math.round(value / 1_000) + 'K';
  return Math.round(value).toLocaleString();
};

const HomeView: React.FC<HomeViewProps> = React.memo((props) => {
  const {
    stats, history, activity, running, starting,
    onStart, onStop, onOpenAutomation, onOpenAccount,
  } = props;

  const lastAttack = history && history.length > 0 ? history[0] : undefined;
  const recentActivity = React.useMemo(
    () => (activity || [])
      .filter((event) => event.type !== 'state_changed' && event.type !== 'target_skipped')
      .slice(0, 3),
    [activity],
  );

  const botLabel = starting ? 'Starting…' : running ? 'Bot running' : 'Bot stopped';
  const botSub = starting
    ? 'ClashGO is preparing BlueStacks and the automation runtime.'
    : running
      ? 'Automation is active. You can leave ClashGO running.'
      : 'Everything is ready when you are.';

  const statusDot = running
    ? 'bg-emerald-400 animate-pulse'
    : starting
      ? 'bg-amber-400 animate-pulse'
      : 'bg-zinc-600';

  const actionClass = running
    ? 'bg-rose-500 text-white hover:bg-rose-400'
    : starting
      ? 'bg-amber-400/20 text-amber-300 dark:text-amber-700'
      : 'bg-white dark:bg-zinc-950 text-zinc-950 dark:text-white hover:scale-[1.01]';

  return (
    <div className="space-y-6 max-w-6xl mx-auto">
      <section className="rounded-[2.25rem] bg-zinc-950 dark:bg-white text-white dark:text-zinc-950 p-7 md:p-9 shadow-premium-lg overflow-hidden">
        <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-8">
          <div className="min-w-0">
            <div className="flex items-center gap-2 text-[10px] font-black uppercase tracking-[0.25em] text-zinc-400 dark:text-zinc-500">
              <span className={'w-2 h-2 rounded-full ' + statusDot} />
              ClashGO
            </div>
            <h2 className="mt-3 text-3xl md:text-4xl font-black tracking-tight">{botLabel}</h2>
            <p className="mt-2 max-w-xl text-sm font-semibold text-zinc-400 dark:text-zinc-600">{botSub}</p>
          </div>
          <button
            type="button"
            onClick={starting ? undefined : (running ? onStop : onStart)}
            disabled={starting}
            className={'h-14 px-7 rounded-2xl font-black text-xs uppercase tracking-[0.2em] transition-all active:scale-[0.98] disabled:cursor-wait ' + actionClass}
          >
            {starting ? 'Starting…' : running ? 'Stop bot' : 'Start bot'}
          </button>
        </div>
      </section>

      <section className="grid grid-cols-2 xl:grid-cols-4 gap-4">
        {[
          { label: 'Attacks', value: String(stats.attacks_completed || 0), icon: 'swords' },
          { label: 'Gold', value: formatLoot(stats.total_gold || 0), icon: 'paid' },
          { label: 'Elixir', value: formatLoot(stats.total_elixir || 0), icon: 'water_drop' },
          { label: 'Uptime', value: formatUptime(stats.uptime || 0), icon: 'schedule' },
        ].map((item) => (
          <div key={item.label} className="rounded-[1.75rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 shadow-premium dark:shadow-none">
            <span className="material-symbols-outlined text-lg text-zinc-400">{item.icon}</span>
            <div className="mt-4 text-2xl font-black text-zinc-950 dark:text-white tabular-nums">{item.value}</div>
            <div className="mt-1 text-[10px] font-black uppercase tracking-[0.2em] text-zinc-400">{item.label}</div>
          </div>
        ))}
      </section>

      <section className="grid lg:grid-cols-2 gap-6">
        <div className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6 shadow-premium dark:shadow-none">
          <div className="flex items-center justify-between gap-4">
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Last attack</div>
              <h3 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">
                {lastAttack ? String(lastAttack.stars) + '★ · ' + (lastAttack.strategy || 'Strategy') : 'No attack yet'}
              </h3>
            </div>
            {lastAttack && (
              <span className={'px-3 py-1.5 rounded-xl text-[10px] font-black uppercase tracking-widest ' + (lastAttack.deploy_success ? 'bg-emerald-500/10 text-emerald-500' : 'bg-amber-500/10 text-amber-500')}>
                {lastAttack.deploy_success ? 'Complete' : 'Partial'}
              </span>
            )}
          </div>

          {lastAttack ? (
            <div className="mt-5 grid grid-cols-3 gap-3">
              {[
                ['Gold', lastAttack.gold_stolen],
                ['Elixir', lastAttack.elixir_stolen],
                ['Dark', lastAttack.dark_elixir_stolen],
              ].map(([label, value]) => (
                <div key={String(label)} className="rounded-2xl bg-zinc-50 dark:bg-zinc-800/60 p-4">
                  <div className="text-lg font-black">{formatLoot(Number(value))}</div>
                  <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">{label}</div>
                </div>
              ))}
            </div>
          ) : (
            <p className="mt-4 text-sm font-semibold text-zinc-500">Your first farming result will appear here automatically.</p>
          )}
        </div>

        <div className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6 shadow-premium dark:shadow-none">
          <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Quick actions</div>
          <h3 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">Everything useful in two clicks</h3>
          <div className="mt-5 grid gap-3">
            <button type="button" onClick={onOpenAutomation} className="w-full flex items-center justify-between rounded-2xl bg-zinc-50 dark:bg-zinc-800/60 px-4 py-4 text-left hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors">
              <span>
                <span className="block text-sm font-black text-zinc-950 dark:text-white">Automation</span>
                <span className="block mt-0.5 text-xs font-semibold text-zinc-500">Loot targets, army and attack behavior</span>
              </span>
              <span className="material-symbols-outlined text-zinc-400">chevron_right</span>
            </button>
            <button type="button" onClick={onOpenAccount} className="w-full flex items-center justify-between rounded-2xl bg-zinc-50 dark:bg-zinc-800/60 px-4 py-4 text-left hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors">
              <span>
                <span className="block text-sm font-black text-zinc-950 dark:text-white">Account</span>
                <span className="block mt-0.5 text-xs font-semibold text-zinc-500">Village profile and ClashGO access</span>
              </span>
              <span className="material-symbols-outlined text-zinc-400">chevron_right</span>
            </button>
          </div>
        </div>
      </section>

      {recentActivity.length > 0 && (
        <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6 shadow-premium dark:shadow-none">
          <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Recent activity</div>
          <div className="mt-4 grid md:grid-cols-3 gap-3">
            {recentActivity.map((event, index) => (
              <div key={event.at + '-' + index} className="rounded-2xl bg-zinc-50 dark:bg-zinc-800/60 px-4 py-3">
                <div className="text-xs font-black text-zinc-800 dark:text-zinc-100">{event.type.split('_').join(' ')}</div>
                <div className="mt-1 text-[10px] font-semibold text-zinc-400">{event.at}</div>
              </div>
            ))}
          </div>
        </section>
      )}
    </div>
  );
});

export default HomeView;
