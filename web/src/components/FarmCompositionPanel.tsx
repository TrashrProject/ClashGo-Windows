import React from 'react';
import { GetConfig, SaveFarmComposition } from '../../wailsjs/go/main/App';

type FarmUnit = {
  name: string;
  count: number;
  housing: number;
};

type FarmProfile = {
  town_hall: number;
  label: string;
  troop_capacity: number;
  spell_capacity: number;
  clan_castle_troop_capacity: number;
  clan_castle_spell_capacity: number;
  clan_castle_siege_capacity: number;
  troops: FarmUnit[];
  spells: FarmUnit[];
  heroes: string[];
  siege: string;
};

type FarmConfig = {
  enabled: boolean;
  town_hall: number;
  profiles: Record<string, FarmProfile>;
};

const HEROES = [
  { name: 'Barbarian King', unlock: 4 },
  { name: 'Archer Queen', unlock: 8 },
  { name: 'Minion Prince', unlock: 9 },
  { name: 'Grand Warden', unlock: 11 },
  { name: 'Royal Champion', unlock: 13 },
  { name: 'Dragon Duke', unlock: 15 },
];

const THS = Array.from({ length: 11 }, (_, i) => i + 8);

const FarmCompositionPanel: React.FC = React.memo(() => {
  const [farm, setFarm] = React.useState<FarmConfig | null>(null);
  const [selectedTH, setSelectedTH] = React.useState(18);
  const [draft, setDraft] = React.useState<FarmProfile | null>(null);
  const [status, setStatus] = React.useState<'idle' | 'saving' | 'saved' | 'error'>('idle');
  const [error, setError] = React.useState('');

  const load = React.useCallback(async () => {
    const cfg = await GetConfig();
    const raw = (cfg as any)?.attack?.farm as FarmConfig | undefined;
    if (!raw?.profiles) return;
    setFarm(raw);
    const th = raw.town_hall || 18;
    setSelectedTH(th);
    setDraft(JSON.parse(JSON.stringify(raw.profiles[String(th)])));
  }, []);

  React.useEffect(() => {
    load().catch((e) => setError(String(e)));
  }, [load]);

  const pickTH = (th: number) => {
    if (!farm) return;
    setSelectedTH(th);
    setDraft(JSON.parse(JSON.stringify(farm.profiles[String(th)])));
    setStatus('idle');
    setError('');
  };

  const setUnit = (kind: 'troops' | 'spells', idx: number, patch: Partial<FarmUnit>) => {
    if (!draft) return;
    const next = { ...draft, [kind]: draft[kind].map((u, i) => i === idx ? { ...u, ...patch } : u) };
    setDraft(next);
  };

  const troopUsed = draft?.troops.reduce((sum, u) => sum + u.count * u.housing, 0) ?? 0;
  const spellUsed = draft?.spells.reduce((sum, u) => sum + u.count * u.housing, 0) ?? 0;

  const toggleHero = (hero: string) => {
    if (!draft) return;
    const has = draft.heroes.includes(hero);
    let heroes = has ? draft.heroes.filter(h => h !== hero) : [...draft.heroes, hero];
    if (!has && heroes.length > 4) return;
    setDraft({ ...draft, heroes });
  };

  const save = async () => {
    if (!farm || !draft) return;
    setStatus('saving');
    setError('');
    try {
      await SaveFarmComposition(farm.enabled, selectedTH, JSON.stringify(draft));
      const updated: FarmConfig = {
        ...farm,
        town_hall: selectedTH,
        profiles: { ...farm.profiles, [String(selectedTH)]: draft },
      };
      setFarm(updated);
      setStatus('saved');
      window.setTimeout(() => setStatus('idle'), 1600);
    } catch (e) {
      setStatus('error');
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  const toggleEnabled = async () => {
    if (!farm || !draft) return;
    const enabled = !farm.enabled;
    setFarm({ ...farm, enabled });
    try {
      await SaveFarmComposition(enabled, selectedTH, JSON.stringify(draft));
    } catch (e) {
      setFarm({ ...farm, enabled: !enabled });
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  if (!farm || !draft) {
    return (
      <div className="bg-white dark:bg-zinc-900 p-8 rounded-[3rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none">
        <div className="text-sm font-bold text-zinc-500">Loading farm compositions…</div>
      </div>
    );
  }

  const availableHeroes = HEROES.filter(h => selectedTH >= h.unlock);
  const troopOverflow = troopUsed > draft.troop_capacity;
  const spellOverflow = spellUsed > draft.spell_capacity;

  return (
    <div className="bg-white dark:bg-zinc-900 p-8 rounded-[3rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none space-y-8">
      <div className="flex items-start justify-between gap-6">
        <div>
          <div className="flex items-center gap-3 mb-2">
            <div className="w-11 h-11 rounded-2xl bg-sky-500/10 text-sky-500 flex items-center justify-center">
              <span className="material-symbols-outlined">shield</span>
            </div>
            <div>
              <h3 className="text-2xl font-bold text-zinc-950 dark:text-white tracking-tight">Farm Composition by TH</h3>
              <p className="text-sm text-zinc-500 font-medium">The bot uses this profile as the target amount for troops, spells, heroes and siege.</p>
            </div>
          </div>
        </div>
        <button type="button" role="switch" aria-checked={farm.enabled} onClick={toggleEnabled}
          className={`w-14 h-7 rounded-full transition-all relative shrink-0 ${farm.enabled ? 'bg-emerald-500/80' : 'bg-zinc-200 dark:bg-zinc-800'}`}>
          <div className={`absolute top-1 w-5 h-5 rounded-full bg-white shadow-lg transition-all ${farm.enabled ? 'left-8' : 'left-1'}`} />
        </button>
      </div>

      <div className="flex gap-2 overflow-x-auto pb-2">
        {THS.map(th => (
          <button key={th} type="button" onClick={() => pickTH(th)}
            className={`px-4 py-2.5 rounded-xl text-xs font-black whitespace-nowrap transition-all ${selectedTH === th ? 'bg-zinc-950 text-white dark:bg-white dark:text-zinc-950' : 'bg-zinc-50 dark:bg-zinc-800 text-zinc-500 hover:text-zinc-950 dark:hover:text-white'}`}>
            HDV {th}
          </button>
        ))}
      </div>

      <div className="grid grid-cols-2 lg:grid-cols-5 gap-3">
        {[
          ['Army', `${troopUsed}/${draft.troop_capacity}`],
          ['Spells', `${spellUsed}/${draft.spell_capacity}`],
          ['CC Troops', String(draft.clan_castle_troop_capacity)],
          ['CC Spells', String(draft.clan_castle_spell_capacity)],
          ['CC Siege', String(draft.clan_castle_siege_capacity)],
        ].map(([label, value]) => (
          <div key={label} className="rounded-2xl border border-zinc-100 dark:border-zinc-800 bg-zinc-50/60 dark:bg-zinc-950/30 p-4">
            <div className="text-[10px] uppercase tracking-[0.18em] font-black text-zinc-400">{label}</div>
            <div className="text-xl font-black text-zinc-950 dark:text-white mt-1">{value}</div>
          </div>
        ))}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h4 className="font-black text-zinc-950 dark:text-white">Troops</h4>
            <span className={`text-xs font-black ${troopOverflow ? 'text-rose-500' : 'text-emerald-500'}`}>{troopUsed}/{draft.troop_capacity}</span>
          </div>
          {draft.troops.map((unit, idx) => (
            <div key={idx} className="grid grid-cols-[1fr_84px_72px] gap-2">
              <input value={unit.name} onChange={e => setUnit('troops', idx, { name: e.target.value })}
                className="bg-zinc-50 dark:bg-zinc-950/50 border border-zinc-100 dark:border-zinc-800 rounded-xl px-3 py-2.5 text-sm font-bold" />
              <input type="number" min={0} value={unit.count} onChange={e => setUnit('troops', idx, { count: Math.max(0, Number(e.target.value) || 0) })}
                className="bg-zinc-50 dark:bg-zinc-950/50 border border-zinc-100 dark:border-zinc-800 rounded-xl px-3 py-2.5 text-sm font-black text-center" />
              <div className="rounded-xl bg-zinc-50 dark:bg-zinc-950/50 border border-zinc-100 dark:border-zinc-800 px-2 py-2.5 text-xs font-black text-center text-zinc-500">×{unit.housing}</div>
            </div>
          ))}
          <p className="text-[11px] text-zinc-400 font-medium">Count is the exact target the deployer will try to place when the card is identified.</p>
        </div>

        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h4 className="font-black text-zinc-950 dark:text-white">Spells</h4>
            <span className={`text-xs font-black ${spellOverflow ? 'text-rose-500' : 'text-emerald-500'}`}>{spellUsed}/{draft.spell_capacity}</span>
          </div>
          {draft.spells.map((unit, idx) => (
            <div key={idx} className="grid grid-cols-[1fr_84px_72px] gap-2">
              <input value={unit.name} onChange={e => setUnit('spells', idx, { name: e.target.value })}
                className="bg-zinc-50 dark:bg-zinc-950/50 border border-zinc-100 dark:border-zinc-800 rounded-xl px-3 py-2.5 text-sm font-bold" />
              <input type="number" min={0} value={unit.count} onChange={e => setUnit('spells', idx, { count: Math.max(0, Number(e.target.value) || 0) })}
                className="bg-zinc-50 dark:bg-zinc-950/50 border border-zinc-100 dark:border-zinc-800 rounded-xl px-3 py-2.5 text-sm font-black text-center" />
              <div className="rounded-xl bg-zinc-50 dark:bg-zinc-950/50 border border-zinc-100 dark:border-zinc-800 px-2 py-2.5 text-xs font-black text-center text-zinc-500">×{unit.housing}</div>
            </div>
          ))}

          <div className="pt-3">
            <h4 className="font-black text-zinc-950 dark:text-white mb-3">Heroes <span className="text-xs text-zinc-400">({draft.heroes.length}/4 active)</span></h4>
            <div className="flex flex-wrap gap-2">
              {availableHeroes.map(hero => {
                const on = draft.heroes.includes(hero.name);
                return (
                  <button key={hero.name} type="button" onClick={() => toggleHero(hero.name)}
                    className={`px-3 py-2 rounded-xl border text-xs font-black transition-all ${on ? 'bg-violet-500/10 border-violet-500/30 text-violet-600 dark:text-violet-300' : 'border-zinc-100 dark:border-zinc-800 text-zinc-400'}`}>
                    {hero.name}
                  </button>
                );
              })}
            </div>
          </div>

          <div className="pt-3">
            <label className="text-[10px] uppercase tracking-[0.18em] font-black text-zinc-400">Siege machine</label>
            <input value={draft.siege} disabled={draft.clan_castle_siege_capacity === 0}
              onChange={e => setDraft({ ...draft, siege: e.target.value })}
              placeholder={draft.clan_castle_siege_capacity === 0 ? 'Not available at this TH' : 'Stone Slammer'}
              className="mt-2 w-full bg-zinc-50 dark:bg-zinc-950/50 border border-zinc-100 dark:border-zinc-800 rounded-xl px-3 py-2.5 text-sm font-bold disabled:opacity-40" />
          </div>
        </div>
      </div>

      {(troopOverflow || spellOverflow || error) && (
        <div className="rounded-2xl bg-rose-500/10 border border-rose-500/20 px-4 py-3 text-sm font-bold text-rose-500">
          {error || (troopOverflow ? 'Troop housing exceeds the TH capacity.' : 'Spell housing exceeds the TH capacity.')}
        </div>
      )}

      <div className="flex items-center justify-between gap-4 pt-2">
        <p className="text-xs text-zinc-400 font-medium">Selected profile: HDV {selectedTH}. Changes are persisted in config.json and hot-applied to the running bot.</p>
        <button type="button" onClick={save} disabled={status === 'saving' || troopOverflow || spellOverflow}
          className={`px-6 py-3 rounded-2xl text-sm font-black transition-all disabled:opacity-40 ${status === 'saved' ? 'bg-emerald-500 text-white' : status === 'error' ? 'bg-rose-500 text-white' : 'bg-zinc-950 dark:bg-white text-white dark:text-zinc-950'}`}>
          {status === 'saving' ? 'Saving…' : status === 'saved' ? 'Saved ✓' : status === 'error' ? 'Failed' : 'Save Composition'}
        </button>
      </div>
    </div>
  );
});

export default FarmCompositionPanel;
