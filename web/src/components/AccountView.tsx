
import React from 'react';
import { ClearAccount, GetAccountConfig, GetCachedPlayerProfile, GetConfig, GetCurrentArmy, GetPlayerProfile, GetVillageResources } from '../../wailsjs/go/main/App';

type Unit = { name: string; level: number; maxLevel: number; village: string };
type CurrentArmyUnit = {
  name: string;
  category: string;
  count: number;
  confidence: number;
  slot_x: number;
};

type CurrentArmy = {
  timestamp: string;
  units: CurrentArmyUnit[];
};

type FarmUnit = { name: string; count: number; housing: number };
type FarmProfile = {
  town_hall: number;
  label: string;
  troop_capacity: number;
  spell_capacity: number;
  troops: FarmUnit[];
  spells: FarmUnit[];
  heroes: string[];
  siege: string;
};

type VillageResources = {
  timestamp: string;
  gold: number;
  elixir: number;
  dark_elixir: number;
  gold_valid: boolean;
  elixir_valid: boolean;
  dark_valid: boolean;
  valid: boolean;
};

type PlayerProfile = {
  tag: string; name: string; townHallLevel: number; expLevel: number;
  trophies: number; bestTrophies: number; warStars: number;
  attackWins: number; defenseWins: number; donations: number; donationsReceived: number;
  clan?: { tag: string; name: string; clanLevel: number };
  league?: { id: number; name: string };
  troops: Unit[]; heroes: Unit[]; spells: Unit[]; heroEquipment: Unit[];
};

interface AccountViewProps {
  playerTag: string;
  onAccountChanged: (tag: string) => void;
}

const AccountView: React.FC<AccountViewProps> = React.memo(({ playerTag, onAccountChanged }) => {
  const [profile, setProfile] = React.useState<PlayerProfile | null>(null);
  const [resources, setResources] = React.useState<VillageResources | null>(null);
  const [farmProfile, setFarmProfile] = React.useState<FarmProfile | null>(null);
  const [currentArmy, setCurrentArmy] = React.useState<CurrentArmy | null>(null);
  const [serviceConfigured, setServiceConfigured] = React.useState(false);
  const [serviceURL, setServiceURL] = React.useState('');
  const [busy, setBusy] = React.useState(false);
  const [message, setMessage] = React.useState('');
  const [error, setError] = React.useState('');

  const refresh = React.useCallback(async () => {
    setBusy(true); setError('');
    try {
      const account = await GetAccountConfig();
      setServiceConfigured(account.service_configured);
      setServiceURL(account.service_url || '');

      const cfg = await GetConfig();
      const farm = (cfg as any)?.attack?.farm;
      if (farm?.enabled && farm?.profiles) {
        setFarmProfile(farm.profiles[String(farm.town_hall)] || null);
      } else {
        setFarmProfile(null);
      }

      const cached = await GetCachedPlayerProfile();
      if (cached) setProfile(cached as PlayerProfile);

      const p = await GetPlayerProfile();
      setProfile(p as PlayerProfile);

      // Account sync may auto-switch the farm profile to the player's HDV.
      // Re-read config once so the visible plan updates immediately.
      const refreshedCfg = await GetConfig();
      const refreshedFarm = (refreshedCfg as any)?.attack?.farm;
      if (refreshedFarm?.enabled && refreshedFarm?.profiles) {
        setFarmProfile(refreshedFarm.profiles[String(refreshedFarm.town_hall)] || null);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }, []);

  React.useEffect(() => { void refresh(); }, [refresh, playerTag]);

  // The local/proxied account service may start a few seconds after ClashGO.
  // Retry automatically while no profile is available so users never have to
  // hammer "Sync profile" after launching the service.
  React.useEffect(() => {
    // Keep public progression fresh without asking the user to press Sync.
    // Fifteen minutes is intentionally conservative for a mostly-static
    // profile and keeps pressure off the ClashGO account service.
    const id = window.setInterval(() => {
      void refresh();
    }, 15 * 60 * 1000);
    return () => window.clearInterval(id);
  }, [refresh]);

  React.useEffect(() => {
    let active = true;
    const loadResources = async () => {
      try {
        const [snap, army] = await Promise.all([GetVillageResources(), GetCurrentArmy()]);
        if (active && snap) setResources(snap as VillageResources);
        if (active && army) setCurrentArmy(army as CurrentArmy);
      } catch {
        // Live tracking is best-effort while BlueStacks is unavailable.
      }
    };
    void loadResources();
    const id = window.setInterval(loadResources, 5000);
    return () => {
      active = false;
      window.clearInterval(id);
    };
  }, []);

  React.useEffect(() => {
    if (profile || busy || !playerTag) return;
    const id = window.setInterval(() => {
      void refresh();
    }, 4000);
    return () => window.clearInterval(id);
  }, [profile, busy, playerTag, refresh]);

  const unlink = async () => {
    setBusy(true);
    try {
      await ClearAccount();
      setProfile(null);
      onAccountChanged('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const homeTroops = (profile?.troops ?? []).filter(u => u.village === 'home');
  const homeHeroes = (profile?.heroes ?? []).filter(u => u.village === 'home');
  const homeSpells = (profile?.spells ?? []).filter(u => u.village === 'home');

  return (
    <div className="space-y-6 max-w-6xl mx-auto">
      <section className="rounded-[2.5rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-8 shadow-premium dark:shadow-none">
        <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-6">
          <div className="min-w-0">
            <div className="text-[10px] font-black uppercase tracking-[0.25em] text-zinc-400">Linked Clash account</div>
            <div className="mt-2 flex flex-wrap items-baseline gap-3">
              <h3 className="text-3xl font-black text-zinc-950 dark:text-white">{profile?.name || playerTag || 'No account'}</h3>
              <span className="text-sm font-mono font-bold text-zinc-500">{profile?.tag || playerTag}</span>
            </div>
            <p className="mt-2 text-sm font-medium text-zinc-500">
              {profile ? 'Account synchronized automatically through ClashGO. No developer API key is required.' : 'Player tag saved. ClashGO will synchronize this account automatically.'}
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <button type="button" onClick={() => void refresh()} disabled={busy} className="px-4 py-3 rounded-xl border border-zinc-200 dark:border-zinc-700 text-xs font-black disabled:opacity-40">
              {busy ? 'Syncing...' : 'Sync profile'}
            </button>
            <button type="button" onClick={() => void unlink()} disabled={busy} className="px-4 py-3 rounded-xl border border-rose-500/20 bg-rose-500/5 text-xs font-black text-rose-500 disabled:opacity-40">
              Unlink account
            </button>
          </div>
        </div>

        <div className="mt-5 flex flex-wrap items-center gap-3">
          <div className={`flex items-center gap-2 text-[10px] font-black uppercase tracking-widest ${
            profile ? 'text-emerald-500' : error ? 'text-rose-500' : busy ? 'text-amber-500' : serviceConfigured ? 'text-zinc-500' : 'text-amber-500'
          }`}>
            <span className="material-symbols-outlined text-base">
              {profile ? 'cloud_done' : error ? 'cloud_off' : busy ? 'sync' : serviceConfigured ? 'cloud_queue' : 'cloud_off'}
            </span>
            {profile
              ? 'ClashGO account service connected'
              : error
                ? 'Account service unavailable — automatic retry enabled'
                : busy
                  ? 'Connecting to ClashGO account service'
                  : serviceConfigured
                    ? 'Account service configured'
                    : 'Account service not configured'}
          </div>
        </div>

        {(message || error) && (
          <div className={'mt-4 rounded-xl px-4 py-3 text-xs font-bold ' + (error ? 'bg-rose-500/10 text-rose-500' : 'bg-emerald-500/10 text-emerald-500')}>
            {error || message}
          </div>
        )}
      </section>

      {profile && (
        <>
          <section className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-6 gap-3">
            {[
              ['Town Hall', 'TH ' + profile.townHallLevel],
              ['XP', String(profile.expLevel)],
              ['Trophies', profile.trophies.toLocaleString()],
              ['Best', profile.bestTrophies.toLocaleString()],
              ['War stars', profile.warStars.toLocaleString()],
              ['League', profile.league?.name || '—'],
            ].map(([label, value]) => (
              <div key={label} className="rounded-2xl border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-4">
                <div className="text-[9px] uppercase tracking-[0.18em] font-black text-zinc-400">{label}</div>
                <div className="mt-1 text-lg font-black text-zinc-950 dark:text-white">{value}</div>
              </div>
            ))}
          </section>

          {currentArmy && currentArmy.units.length > 0 && (
            <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6">
              <div className="flex items-center justify-between gap-4 mb-5">
                <div>
                  <div className="text-[10px] font-black uppercase tracking-[0.2em] text-zinc-400">Last detected army</div>
                  <h4 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">Live battle composition</h4>
                  <p className="mt-1 text-xs text-zinc-500">
                    Captured automatically from the troop bar before deployment.
                  </p>
                </div>
                <div className="text-[10px] font-bold text-zinc-400">
                  {new Date(currentArmy.timestamp).toLocaleTimeString()}
                </div>
              </div>
              <div className="grid grid-cols-2 md:grid-cols-3 xl:grid-cols-5 gap-2">
                {currentArmy.units.map((unit, idx) => (
                  <div key={idx} className="rounded-xl bg-zinc-50 dark:bg-zinc-950/40 border border-zinc-100 dark:border-zinc-800 p-3 min-w-0">
                    <div className="truncate text-xs font-black text-zinc-900 dark:text-white" title={unit.name || 'Unknown card'}>
                      {unit.name || 'Unknown card'}
                    </div>
                    <div className="mt-1 flex items-center justify-between gap-2 text-[10px] font-bold text-zinc-400">
                      <span>{unit.category}</span>
                      <span className="tabular-nums">{unit.count > 0 ? '×' + unit.count : 'detected'}</span>
                    </div>
                  </div>
                ))}
              </div>
            </section>
          )}

          {farmProfile && (
            <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6">
              <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-4">
                <div>
                  <div className="text-[10px] font-black uppercase tracking-[0.2em] text-zinc-400">Automatic farm plan</div>
                  <h4 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">{farmProfile.label}</h4>
                  <p className="mt-1 text-xs font-medium text-zinc-500">Selected automatically from your linked Town Hall. No manual setup required.</p>
                </div>
                <div className="flex gap-3 text-center">
                  <div className="rounded-xl bg-zinc-50 dark:bg-zinc-950/40 px-4 py-3">
                    <div className="text-[9px] font-black uppercase tracking-wider text-zinc-400">Army</div>
                    <div className="text-lg font-black text-zinc-950 dark:text-white">{farmProfile.troop_capacity}</div>
                  </div>
                  <div className="rounded-xl bg-zinc-50 dark:bg-zinc-950/40 px-4 py-3">
                    <div className="text-[9px] font-black uppercase tracking-wider text-zinc-400">Spells</div>
                    <div className="text-lg font-black text-zinc-950 dark:text-white">{farmProfile.spell_capacity}</div>
                  </div>
                </div>
              </div>

              <div className="mt-5 grid grid-cols-1 md:grid-cols-4 gap-3">
                <PlanGroup title="Troops" items={farmProfile.troops.map(u => u.count + '× ' + u.name)} />
                <PlanGroup title="Spells" items={farmProfile.spells.map(u => u.count + '× ' + u.name)} />
                <PlanGroup title="Heroes" items={farmProfile.heroes} />
                <PlanGroup title="Siege" items={farmProfile.siege ? [farmProfile.siege] : ['None']} />
              </div>
            </section>
          )}

          <section className="grid grid-cols-1 lg:grid-cols-3 gap-5">
            <div className="lg:col-span-2 rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6">
              <div className="flex items-center justify-between mb-5">
                <div>
                  <h4 className="text-xl font-black text-zinc-950 dark:text-white">Account progression</h4>
                  <p className="text-xs text-zinc-500 mt-1">Unlocked levels from the official player profile.</p>
                </div>
                <span className="text-[10px] font-black text-zinc-400 uppercase tracking-widest">{homeTroops.length} troops</span>
              </div>
              <div className="space-y-6">
                <UnitGrid title="Heroes" units={homeHeroes} />
                <UnitGrid title="Troops" units={homeTroops} />
                <UnitGrid title="Spells" units={homeSpells} />
              </div>
            </div>

            <div className="space-y-5">
              <div className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6">
                <h4 className="text-lg font-black text-zinc-950 dark:text-white">Clan</h4>
                <div className="mt-4 text-2xl font-black text-zinc-950 dark:text-white">{profile.clan?.name || 'No clan'}</div>
                {profile.clan && <div className="mt-1 text-xs font-mono text-zinc-500">{profile.clan.tag} · Level {profile.clan.clanLevel}</div>}
              </div>

              <div className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6">
                <div className="flex items-center gap-2">
                  <span className="material-symbols-outlined text-amber-500">database</span>
                  <h4 className="text-lg font-black text-zinc-950 dark:text-white">Live resources</h4>
                </div>
                <p className="mt-2 text-xs font-medium text-zinc-500">
                  Read automatically from the BlueStacks village HUD. No extra setup is required.
                </p>
                <div className="mt-4 space-y-2">
                  {[
                    ['Gold', resources?.gold_valid ? resources.gold : null],
                    ['Elixir', resources?.elixir_valid ? resources.elixir : null],
                    ['Dark Elixir', resources?.dark_valid ? resources.dark_elixir : null],
                  ].map(([name, value]) => (
                    <div key={String(name)} className="flex justify-between items-center rounded-xl bg-zinc-50 dark:bg-zinc-950/40 px-4 py-3">
                      <span className="text-xs font-black text-zinc-500">{name}</span>
                      <span className="text-sm font-black text-zinc-950 dark:text-white tabular-nums">
                        {typeof value === 'number' ? value.toLocaleString() : 'Waiting for village scan'}
                      </span>
                    </div>
                  ))}
                </div>
                {resources?.timestamp && (
                  <div className="mt-3 text-[10px] font-bold text-zinc-400">
                    Last scan: {new Date(resources.timestamp).toLocaleTimeString()}
                  </div>
                )}
              </div>
            </div>
          </section>
        </>
      )}
    </div>
  );
});

const PlanGroup: React.FC<{ title: string; items: string[] }> = ({ title, items }) => (
  <div className="rounded-xl border border-zinc-100 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-950/30 p-4">
    <div className="text-[9px] font-black uppercase tracking-[0.18em] text-zinc-400">{title}</div>
    <div className="mt-2 space-y-1">
      {items.map((item, i) => (
        <div key={i} className="text-xs font-black text-zinc-800 dark:text-zinc-200">{item}</div>
      ))}
    </div>
  </div>
);

const UnitGrid: React.FC<{ title: string; units: Unit[] }> = ({ title, units }) => (
  <div>
    <div className="mb-3 text-[10px] font-black uppercase tracking-[0.2em] text-zinc-400">{title}</div>
    {units.length === 0 ? <div className="text-xs text-zinc-400">No data</div> : (
      <div className="grid grid-cols-2 md:grid-cols-3 xl:grid-cols-4 gap-2">
        {units.map(unit => (
          <div key={unit.name} className="rounded-xl border border-zinc-100 dark:border-zinc-800 bg-zinc-50/60 dark:bg-zinc-950/30 px-3 py-3 min-w-0">
            <div className="truncate text-xs font-black text-zinc-800 dark:text-zinc-200" title={unit.name}>{unit.name}</div>
            <div className="mt-1 text-[10px] font-bold text-zinc-400">Lv {unit.level}/{unit.maxLevel}</div>
          </div>
        ))}
      </div>
    )}
  </div>
);

export default AccountView;
