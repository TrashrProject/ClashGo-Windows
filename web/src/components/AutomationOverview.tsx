import React from 'react';
import { GetAccountConfig, GetCachedPlayerProfile, GetConfig, GetVillageResources } from '../../wailsjs/go/main/App';

type OverviewState = {
  tag: string;
  name: string;
  th: number;
  simpleMode: boolean;
  farmEnabled: boolean;
  farmTH: number;
  farmLabel: string;
  gold: number | null;
  elixir: number | null;
  dark: number | null;
  resourceTime: string;
};

const AutomationOverview: React.FC = React.memo(() => {
  const [state, setState] = React.useState<OverviewState>({
    tag: '', name: '', th: 0,
    simpleMode: true,
    farmEnabled: false,
    farmTH: 0,
    farmLabel: '',
    gold: null, elixir: null, dark: null,
    resourceTime: '',
  });

  React.useEffect(() => {
    let active = true;
    const load = async () => {
      try {
        const [account, profile, cfg, resources] = await Promise.all([
          GetAccountConfig(),
          GetCachedPlayerProfile(),
          GetConfig(),
          GetVillageResources(),
        ]);
        if (!active) return;

        const farm = (cfg as any)?.attack?.farm;
        const selected = farm?.profiles?.[String(farm?.town_hall)];
        setState({
          tag: account?.player_tag || '',
          name: (profile as any)?.name || '',
          th: Number((profile as any)?.townHallLevel || 0),
          simpleMode: (cfg as any)?.automation?.simple_mode ?? true,
          farmEnabled: !!farm?.enabled,
          farmTH: Number(farm?.town_hall || 0),
          farmLabel: selected?.label || '',
          gold: resources?.gold_valid ? Number(resources.gold) : null,
          elixir: resources?.elixir_valid ? Number(resources.elixir) : null,
          dark: resources?.dark_valid ? Number(resources.dark_elixir) : null,
          resourceTime: resources?.timestamp || '',
        });
      } catch {
        // Dashboard stays usable while the backend is starting.
      }
    };
    void load();
    const id = window.setInterval(load, 5000);
    return () => {
      active = false;
      window.clearInterval(id);
    };
  }, []);

  const accountReady = !!state.tag;
  const farmReady = state.farmEnabled && state.farmTH > 0;
  const autoReady = state.simpleMode && accountReady && farmReady;

  return (
    <section className="bg-zinc-950 dark:bg-black text-white rounded-[2.5rem] border border-zinc-800 p-6 md:p-7 shadow-xl">
      <div className="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-6">
        <div className="min-w-0">
          <div className="flex items-center gap-3">
            <div className={`w-3 h-3 rounded-full ${autoReady ? 'bg-emerald-400' : 'bg-amber-400'}`} />
            <div className="text-[10px] font-black uppercase tracking-[0.25em] text-zinc-500">Automatic setup</div>
          </div>
          <div className="mt-2 text-2xl font-black tracking-tight">
            {autoReady ? 'ClashGO is configured automatically' : 'ClashGO is finishing setup'}
          </div>
          <div className="mt-2 text-sm font-medium text-zinc-400">
            {state.name || state.tag || 'Link your Clash account'}{state.th > 0 ? ` · HDV ${state.th}` : ''}
            {state.farmLabel ? ` · ${state.farmLabel}` : ''}
          </div>
        </div>

        <div className="grid grid-cols-3 gap-2 min-w-0 xl:min-w-[440px]">
          {[
            ['Gold', state.gold],
            ['Elixir', state.elixir],
            ['Dark', state.dark],
          ].map(([label, value]) => (
            <div key={String(label)} className="rounded-2xl border border-zinc-800 bg-zinc-900/80 p-4 min-w-0">
              <div className="text-[9px] font-black uppercase tracking-[0.18em] text-zinc-500">{label}</div>
              <div className="mt-1 text-lg font-black truncate tabular-nums">
                {typeof value === 'number' ? value.toLocaleString() : '—'}
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="mt-5 pt-5 border-t border-zinc-800 flex flex-wrap items-center gap-2">
        {[
          ['Account', accountReady],
          ['Automatic mode', state.simpleMode],
          ['Farm profile', farmReady],
          ['Resources', state.gold !== null || state.elixir !== null],
        ].map(([label, ok]) => (
          <div key={String(label)} className={`px-3 py-2 rounded-xl border text-[10px] font-black uppercase tracking-wider ${
            ok
              ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-400'
              : 'border-amber-500/20 bg-amber-500/10 text-amber-400'
          }`}>
            {ok ? '✓' : '…'} {label}
          </div>
        ))}
        {state.resourceTime && (
          <div className="ml-auto text-[10px] font-bold text-zinc-600">
            Last village scan {new Date(state.resourceTime).toLocaleTimeString()}
          </div>
        )}
      </div>
    </section>
  );
});

export default AutomationOverview;
