
import React from 'react';
import { InterfaceLevel } from '../types';
import { ActivateLicense, ClearAccount, DeactivateLicense, GetAccountConfig, GetCachedPlayerProfile, GetConfig, GetCurrentArmée, GetLicensePolicy, GetLicenseState, GetMemberSettings, GetPlayerProfile, GetVillageResources, SaveMemberSettings } from '../../wailsjs/go/main/App';

type Unit = { name: string; level: number; maxLevel: number; village: string };
type CurrentArméeUnit = {
  name: string;
  category: string;
  count: number;
  confidence: number;
  slot_x: number;
};

type CurrentArmée = {
  timestamp: string;
  units: CurrentArméeUnit[];
  target_town_hall?: number;
  target_label?: string;
  ready: boolean;
  uncertain: boolean;
  warnings?: string[];
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

type LicenseState = {
  activated: boolean;
  role: 'member' | 'developer' | 'admin' | '';
  member_name?: string;
  license_hint?: string;
  machine_id?: string;
  last_validated?: string;
  offline_until?: string;
  plan?: string;
  expires_at?: string;
  error?: string;
};

type MemberSettings = {
  speed_profile: 'cautious' | 'normal' | 'fast';
  max_attacks_per_hour: number;
  break_every_attacks: number;
  break_minutes: number;
  adaptive_search: boolean;
  auto_profile_sync: boolean;
  auto_army_guard: boolean;
  auto_resource_tracking: boolean;
};

const applySpeedPreset = (settings: MemberSettings, profile: MemberSettings['speed_profile']): MemberSettings => {
  switch (profile) {
    case 'cautious':
      return { ...settings, speed_profile: profile, max_attacks_per_hour: 8, break_every_attacks: 4, break_minutes: 4 };
    case 'fast':
      return { ...settings, speed_profile: profile, max_attacks_per_hour: 16, break_every_attacks: 6, break_minutes: 2 };
    default:
      return { ...settings, speed_profile: 'normal', max_attacks_per_hour: 12, break_every_attacks: 5, break_minutes: 3 };
  }
};

type LicensePolicy = {
  enforced: boolean;
  service_configured: boolean;
  service_url?: string;
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
  trophies: number; bestTrophées: number; warStars: number;
  attackWins: number; defenseWins: number; donations: number; donationsReceived: number;
  clan?: { tag: string; name: string; clanLevel: number };
  league?: { id: number; name: string };
  troops: Unit[]; heroes: Unit[]; spells: Unit[]; heroEquipment: Unit[];
};

interface AccountViewProps {
  playerTag: string;
  interfaceLevel: InterfaceLevel;
  onInterfaceLevelChange: (level: InterfaceLevel) => void;
  onAccountChanged: (tag: string) => void;
}

const AccountView: React.FC<AccountViewProps> = React.memo(({
  playerTag,
  interfaceLevel,
  onInterfaceLevelChange,
  onAccountChanged,
}) => {
  const [profile, setProfile] = React.useState<PlayerProfile | null>(null);
  const [resources, setResources] = React.useState<VillageResources | null>(null);
  const [farmProfile, setFarmProfile] = React.useState<FarmProfile | null>(null);
  const [currentArmée, setCurrentArmée] = React.useState<CurrentArmée | null>(null);
  const [serviceConfigured, setServiceConfigured] = React.useState(false);
  const [busy, setBusy] = React.useState(false);
  const [message, setMessage] = React.useState('');
  const [error, setError] = React.useState('');
  const [licenseState, setLicenseState] = React.useState<LicenseState | null>(null);
  const [licensePolicy, setLicensePolicy] = React.useState<LicensePolicy | null>(null);
  const [licenseKey, setLicenseKey] = React.useState('');
  const [licenseBusy, setLicenseBusy] = React.useState(false);
  const [licenseError, setLicenseError] = React.useState('');
  const [memberSettings, setMemberSettings] = React.useState<MemberSettings | null>(null);
  const [memberSaving, setMemberSaving] = React.useState(false);
  const [memberMessage, setMemberMessage] = React.useState('');
  const [memberSaveError, setMemberSaveError] = React.useState('');
  const [memberPage, setMemberPage] = React.useState<'account' | 'settings' | 'village'>('account');

  const refreshLicense = React.useCallback(async () => {
    try {
      const [state, policy] = await Promise.all([GetLicenseState(), GetLicensePolicy()]);
      setLicenseState(state as LicenseState);
      setLicensePolicy(policy as LicensePolicy);
      if (state?.activated && (state.role === 'developer' || state.role === 'admin')) {
        onInterfaceLevelChange('developer');
      }
    } catch {
      // Licensing UI remains usable in local beta mode while the control
      // service is not configured.
    }
  }, [onInterfaceLevelChange]);

  const refreshMemberSettings = React.useCallback(async () => {
    try {
      const settings = await GetMemberSettings();
      setMemberSettings(settings as MemberSettings);
    } catch {
      // Member preferences are best-effort while the Wails bridge initializes.
    }
  }, []);

  const saveMemberSettings = async (next: MemberSettings) => {
    if (memberSaving) return;
    setMemberSaving(true);
    setMemberMessage('');
    setMemberSaveError('');
    try {
      const saved = await SaveMemberSettings(next as any);
      setMemberSettings(saved as MemberSettings);
      setMemberMessage('Réglages appliqués au bot.');
    } catch (e) {
      setMemberSaveError(e instanceof Error ? e.message : String(e));
    } finally {
      setMemberSaving(false);
    }
  };

  const activateLicense = async () => {
    const key = licenseKey.trim();
    if (!key || licenseBusy) return;
    setLicenseBusy(true);
    setLicenseError('');
    try {
      const state = await ActivateLicense(key);
      const typed = state as LicenseState;
      setLicenseState(typed);
      setLicenseKey('');
      if (typed.role === 'developer' || typed.role === 'admin') {
        onInterfaceLevelChange('developer');
      } else {
        onInterfaceLevelChange('simple');
      }
    } catch (e) {
      setLicenseError(e instanceof Error ? e.message : String(e));
    } finally {
      setLicenseBusy(false);
    }
  };

  const deactivateLicense = async () => {
    if (licenseBusy) return;
    setLicenseBusy(true);
    setLicenseError('');
    try {
      await DeactivateLicense();
      await refreshLicense();
      onInterfaceLevelChange('simple');
    } catch (e) {
      setLicenseError(e instanceof Error ? e.message : String(e));
    } finally {
      setLicenseBusy(false);
    }
  };

  const refresh = React.useCallback(async () => {
    setBusy(true); setError('');
    try {
      const account = await GetAccountConfig();
      setServiceConfigured(account.service_configured);

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

  React.useEffect(() => {
    void refresh();
    void refreshLicense();
    void refreshMemberSettings();
  }, [refresh, refreshLicense, refreshMemberSettings, playerTag]);

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
        const [snap, army] = await Promise.all([GetVillageResources(), GetCurrentArmée()]);
        if (active && snap) setResources(snap as VillageResources);
        if (active && army) setCurrentArmée(army as CurrentArmée);
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
  const homeSorts = (profile?.spells ?? []).filter(u => u.village === 'home');

  return (
    <div className="space-y-6 max-w-6xl mx-auto">
      <div className="sticky top-0 z-20 -mx-1 px-1 py-2 bg-zinc-50/90 dark:bg-zinc-950/90 backdrop-blur-xl">
        <div className="inline-flex rounded-2xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-1.5 shadow-sm">
          {([
            ['account', 'Compte', 'badge'],
            ['settings', 'Réglages bot', 'tune'],
            ['village', 'Village', 'castle'],
          ] as const).map(([id, label, icon]) => (
            <button
              key={id}
              type="button"
              onClick={() => setMemberPage(id)}
              className={
                'flex items-center gap-2 rounded-xl px-4 py-2.5 text-[10px] font-black uppercase tracking-[0.16em] transition ' +
                (memberPage === id
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
      <section className={(memberPage === 'account' ? '' : 'hidden ') + "rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-zinc-950 dark:bg-white p-6 md:p-7 text-white dark:text-zinc-950 shadow-premium-lg"}>
        <div className="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-6">
          <div className="min-w-0">
            <div className="text-[10px] font-black uppercase tracking-[0.24em] text-zinc-400 dark:text-zinc-500">Licence ClashGO</div>
            <div className="mt-2 flex flex-wrap items-center gap-3">
              <h3 className="text-2xl font-black">
                {licenseState?.activated
                  ? (licenseState.member_name ? 'Bonjour ' + licenseState.member_name : 'Licence active')
                  : 'Activer ClashGO'}
              </h3>
              {licenseState?.activated && (
                <span className="px-3 py-1 rounded-full bg-emerald-400/15 text-emerald-400 dark:text-emerald-600 text-[10px] font-black uppercase tracking-widest">
                  {licenseState.role || 'member'}
                </span>
              )}
            </div>
            <p className="mt-2 text-sm font-semibold text-zinc-400 dark:text-zinc-600">
              {licenseState?.activated
                ? (licenseState.license_hint || 'Licence') + ' · liée à ce PC'
                : 'Entre la clé reçue pour activer ton espace membre sur ce PC.'}
            </p>
            {licenseState?.activated && (
              <div className="mt-2 flex flex-wrap gap-2 text-[10px] font-bold uppercase tracking-widest text-zinc-500">
                <span>
                  {licenseState.plan === 'free_2d'
                    ? 'FREE · 2 jours'
                    : licenseState.plan === 'week_1'
                      ? '1 semaine'
                      : licenseState.plan === 'month_1'
                        ? '1 mois'
                        : 'À vie'}
                </span>
                {licenseState.expires_at && (
                  <span>· Expire le {new Date(licenseState.expires_at).toLocaleString()}</span>
                )}
              </div>
            )}
            {licenseState?.offline_until && (
              <p className="mt-2 text-[10px] font-bold uppercase tracking-widest text-zinc-500">
                Accès hors ligne jusqu’au {new Date(licenseState.offline_until).toLocaleString()}
              </p>
            )}

            {licenseState?.activated && (
              <div className="mt-5 grid grid-cols-2 md:grid-cols-4 gap-2">
                <div className="rounded-xl border border-white/10 dark:border-zinc-200/70 bg-white/5 dark:bg-zinc-100 px-3 py-2.5">
                  <div className="text-[8px] font-black uppercase tracking-[0.18em] text-zinc-500">Plan</div>
                  <div className="mt-1 text-xs font-black">
                    {licenseState.plan === 'free_2d'
                      ? 'FREE 2J'
                      : licenseState.plan === 'week_1'
                        ? '1 SEMAINE'
                        : licenseState.plan === 'month_1'
                          ? '1 MOIS'
                          : 'À VIE'}
                  </div>
                </div>
                <div className="rounded-xl border border-white/10 dark:border-zinc-200/70 bg-white/5 dark:bg-zinc-100 px-3 py-2.5">
                  <div className="text-[8px] font-black uppercase tracking-[0.18em] text-zinc-500">Rôle</div>
                  <div className="mt-1 text-xs font-black uppercase">{licenseState.role || 'member'}</div>
                </div>
                <div className="rounded-xl border border-white/10 dark:border-zinc-200/70 bg-white/5 dark:bg-zinc-100 px-3 py-2.5">
                  <div className="text-[8px] font-black uppercase tracking-[0.18em] text-zinc-500">Appareil</div>
                  <div className="mt-1 text-xs font-black font-mono">
                    {licenseState.machine_id ? licenseState.machine_id.slice(0, 8) + '…' : '—'}
                  </div>
                </div>
                <div className="rounded-xl border border-white/10 dark:border-zinc-200/70 bg-white/5 dark:bg-zinc-100 px-3 py-2.5">
                  <div className="text-[8px] font-black uppercase tracking-[0.18em] text-zinc-500">Expiration</div>
                  <div className="mt-1 text-xs font-black">
                    {licenseState.expires_at ? new Date(licenseState.expires_at).toLocaleDateString() : 'Jamais'}
                  </div>
                </div>
              </div>
            )}
          </div>

          {licenseState?.activated ? (
            <button
              type="button"
              onClick={() => void deactivateLicense()}
              disabled={licenseBusy}
              className="shrink-0 px-5 py-3 rounded-xl border border-white/10 dark:border-zinc-200 text-[10px] font-black uppercase tracking-widest text-zinc-300 dark:text-zinc-600 hover:text-white dark:hover:text-zinc-950 disabled:opacity-40"
            >
              Désactiver sur ce PC
            </button>
          ) : (
            <div className="w-full xl:w-auto flex flex-col sm:flex-row gap-2">
              <input
                value={licenseKey}
                onChange={(e) => setLicenseKey(e.target.value.toUpperCase())}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') void activateLicense();
                }}
                placeholder="CGO-XXXXXX-XXXXXX-XXXXXX-XXXXXX"
                autoComplete="off"
                spellCheck={false}
                className="w-full sm:w-[330px] h-12 rounded-xl border border-white/10 dark:border-zinc-200 bg-white/5 dark:bg-zinc-100 px-4 font-mono text-xs outline-none focus:ring-2 focus:ring-emerald-500/40"
              />
              <button
                type="button"
                onClick={() => void activateLicense()}
                disabled={licenseBusy || !licenseKey.trim()}
                className="h-12 px-5 rounded-xl bg-white dark:bg-zinc-950 text-zinc-950 dark:text-white text-[10px] font-black uppercase tracking-widest disabled:opacity-30"
              >
                {licenseBusy ? 'Vérification…' : 'Activer'}
              </button>
            </div>
          )}
        </div>
        {(licenseError || licenseState?.error) && (
          <div className="mt-4 rounded-xl bg-rose-500/10 px-4 py-3 text-xs font-bold text-rose-400 dark:text-rose-600">
            {licenseError || licenseState?.error}
          </div>
        )}
      </section>
      {memberPage === 'settings' && (licenseState?.activated || licensePolicy?.enforced === false) && memberSettings && (
        <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6 shadow-premium dark:shadow-none">
          <div className="flex flex-col gap-6">
            <div>
              <div className="flex flex-wrap items-center gap-2">
                <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Mon ClashGO</div>
                {!licenseState?.activated && licensePolicy?.enforced === false && (
                  <span className="rounded-full bg-amber-500/10 px-2 py-1 text-[8px] font-black uppercase tracking-widest text-amber-500">
                    Mode bêta local
                  </span>
                )}
              </div>
              <h3 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">Réglages membre</h3>
              <p className="mt-2 text-sm font-semibold text-zinc-500 max-w-2xl">
                Ces réglages agissent réellement sur le bot et sont appliqués sans redémarrage. Les contrôles de sécurité restent actifs, même en mode Rapide.
              </p>
            </div>

            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.18em] text-zinc-400 mb-3">Vitesse du bot</div>
              <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                {([
                  ['cautious', 'Prudente', 'Plus lente et conservatrice'],
                  ['normal', 'Normale', 'Équilibre par défaut'],
                  ['fast', 'Rapide', 'Actions et enchaînements accélérés'],
                ] as const).map(([value, label, description]) => (
                  <button
                    key={value}
                    type="button"
                    disabled={memberSaving}
                    onClick={() => {
                      const next = applySpeedPreset(memberSettings, value);
                      setMemberSettings(next);
                      void saveMemberSettings(next);
                    }}
                    className={
                      'text-left rounded-2xl border p-4 transition ' +
                      (memberSettings.speed_profile === value
                        ? 'border-zinc-950 bg-zinc-950 text-white dark:border-white dark:bg-white dark:text-zinc-950'
                        : 'border-zinc-200 bg-zinc-50 text-zinc-950 hover:border-zinc-400 dark:border-zinc-800 dark:bg-zinc-950 dark:text-white')
                    }
                  >
                    <div className="text-sm font-black">{label}</div>
                    <div className={'mt-1 text-[11px] font-semibold ' + (memberSettings.speed_profile === value ? 'opacity-70' : 'text-zinc-500')}>
                      {description}
                    </div>
                  </button>
                ))}
              </div>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <label className="rounded-2xl border border-zinc-100 dark:border-zinc-800 p-4">
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <div className="text-sm font-black text-zinc-950 dark:text-white">Attaques / heure</div>
                    <div className="text-[11px] font-semibold text-zinc-500">Limite de sécurité du cycle automatique</div>
                  </div>
                  <input
                    type="number"
                    min={1}
                    max={24}
                    value={memberSettings.max_attacks_per_hour}
                    onChange={(e) => setMemberSettings({ ...memberSettings, max_attacks_per_hour: Math.max(1, Math.min(24, Number(e.target.value) || 1)) })}
                    onBlur={() => void saveMemberSettings(memberSettings)}
                    className="w-20 rounded-xl border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 px-3 py-2 text-center text-sm font-black"
                  />
                </div>
              </label>

              <label className="rounded-2xl border border-zinc-100 dark:border-zinc-800 p-4">
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <div className="text-sm font-black text-zinc-950 dark:text-white">Pause automatique</div>
                    <div className="text-[11px] font-semibold text-zinc-500">Toutes les X attaques · 0 pour désactiver</div>
                  </div>
                  <input
                    type="number"
                    min={0}
                    max={20}
                    value={memberSettings.break_every_attacks}
                    onChange={(e) => setMemberSettings({ ...memberSettings, break_every_attacks: Math.max(0, Math.min(20, Number(e.target.value) || 0)) })}
                    onBlur={() => void saveMemberSettings(memberSettings)}
                    className="w-20 rounded-xl border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 px-3 py-2 text-center text-sm font-black"
                  />
                </div>
              </label>

              <label className="rounded-2xl border border-zinc-100 dark:border-zinc-800 p-4">
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <div className="text-sm font-black text-zinc-950 dark:text-white">Durée de pause</div>
                    <div className="text-[11px] font-semibold text-zinc-500">Minutes de repos automatique</div>
                  </div>
                  <input
                    type="number"
                    min={0}
                    max={30}
                    value={memberSettings.break_minutes}
                    onChange={(e) => setMemberSettings({ ...memberSettings, break_minutes: Math.max(0, Math.min(30, Number(e.target.value) || 0)) })}
                    onBlur={() => void saveMemberSettings(memberSettings)}
                    className="w-20 rounded-xl border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 px-3 py-2 text-center text-sm font-black"
                  />
                </div>
              </label>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              {([
                ['adaptive_search', 'Recherche adaptative', 'Relâche progressivement les seuils après plusieurs villages ignorés.'],
                ['auto_profile_sync', 'Profil automatique', 'Synchronise automatiquement le profil Clash lié.'],
                ['auto_army_guard', 'Contrôle armée', 'Vérifie que l’armée correspond au plan avant une attaque.'],
                ['auto_resource_tracking', 'Suivi ressources', 'Suit automatiquement les ressources du village.'],
              ] as const).map(([key, label, description]) => (
                <button
                  key={key}
                  type="button"
                  disabled={memberSaving}
                  onClick={() => {
                    const next = { ...memberSettings, [key]: !memberSettings[key] } as MemberSettings;
                    setMemberSettings(next);
                    void saveMemberSettings(next);
                  }}
                  className="flex items-center justify-between gap-4 rounded-2xl border border-zinc-100 dark:border-zinc-800 p-4 text-left"
                >
                  <div>
                    <div className="text-sm font-black text-zinc-950 dark:text-white">{label}</div>
                    <div className="mt-1 text-[11px] font-semibold text-zinc-500">{description}</div>
                  </div>
                  <span className={
                    'shrink-0 w-10 h-6 rounded-full p-1 transition ' +
                    (memberSettings[key] ? 'bg-emerald-500' : 'bg-zinc-300 dark:bg-zinc-700')
                  }>
                    <span className={
                      'block w-4 h-4 rounded-full bg-white transition-transform ' +
                      (memberSettings[key] ? 'translate-x-4' : '')
                    } />
                  </span>
                </button>
              ))}
            </div>

            {memberMessage && (
              <div className="rounded-xl bg-emerald-500/10 px-4 py-3 text-[11px] font-bold text-emerald-500">{memberMessage}</div>
            )}
            {memberSaveError && (
              <div className="rounded-xl bg-rose-500/10 px-4 py-3 text-[11px] font-bold text-rose-500">{memberSaveError}</div>
            )}
          </div>
        </section>
      )}

      <section className={(memberPage === 'account' ? '' : 'hidden ') + "rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6 shadow-premium dark:shadow-none"}>
        <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-5">
          <div>
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Niveau d’interface</div>
            <h3 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">Choisis le niveau de détails</h3>
            <p className="mt-2 text-sm font-semibold text-zinc-500 max-w-2xl">
              Simple garde l’essentiel. Avancé affiche l’activité détaillée, les statistiques et les réglages système.
            </p>
          </div>
          <div className="flex gap-2 rounded-2xl bg-zinc-50 dark:bg-zinc-800/60 p-1.5">
            {(['simple', 'advanced'] as InterfaceLevel[]).map((level) => (
              <button
                key={level}
                type="button"
                onClick={() => onInterfaceLevelChange(level)}
                className={
                  'px-4 py-2.5 rounded-xl text-[10px] font-black uppercase tracking-[0.18em] transition-all ' +
                  (interfaceLevel === level
                    ? 'bg-zinc-950 dark:bg-white text-white dark:text-zinc-950 shadow-sm'
                    : 'text-zinc-500 hover:text-zinc-900 dark:hover:text-white')
                }
              >
                {level}
              </button>
            ))}
          </div>
        </div>
        <div className="mt-4 flex items-center gap-2 text-[10px] font-bold uppercase tracking-widest text-zinc-400">
          <span className="material-symbols-outlined text-sm">shield_person</span>
          Les outils développeur sont accessibles uniquement avec une licence Developer ou Admin.
        </div>
      </section>
      <section className={(memberPage === 'village' ? '' : 'hidden ') + "rounded-[2.5rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-8 shadow-premium dark:shadow-none"}>
        <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-6">
          <div className="min-w-0">
            <div className="text-[10px] font-black uppercase tracking-[0.25em] text-zinc-400">Compte Clash lié</div>
            <div className="mt-2 flex flex-wrap items-baseline gap-3">
              <h3 className="text-3xl font-black text-zinc-950 dark:text-white">{profile?.name || playerTag || 'Aucun compte'}</h3>
              <span className="text-sm font-mono font-bold text-zinc-500">{profile?.tag || playerTag}</span>
            </div>
            <p className="mt-2 text-sm font-medium text-zinc-500">
              {profile ? 'Compte synchronisé automatiquement par ClashGO. Aucune clé API développeur n’est nécessaire.' : 'Tag joueur enregistré. ClashGO synchronisera ce compte automatiquement.'}
            </p>
          </div>
          <details className="relative">
            <summary className="list-none cursor-pointer px-4 py-3 rounded-xl border border-zinc-200 dark:border-zinc-700 text-xs font-black text-zinc-500 select-none">
              Options du compte
            </summary>
            <div className="absolute right-0 mt-2 z-20 min-w-[190px] rounded-xl border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-900 p-2 shadow-xl">
              <button type="button" onClick={() => void unlink()} disabled={busy} className="w-full px-3 py-2.5 rounded-lg text-left text-xs font-black text-rose-500 hover:bg-rose-500/5 disabled:opacity-40">
                Délier le compte
              </button>
            </div>
          </details>
        </div>

        <div className="mt-5 flex flex-wrap items-center gap-3">
          <div className={`flex items-center gap-2 text-[10px] font-black uppercase tracking-widest ${
            error ? 'text-amber-500' : profile ? 'text-emerald-500' : busy ? 'text-amber-500' : serviceConfigured ? 'text-zinc-500' : 'text-amber-500'
          }`}>
            <span className="material-symbols-outlined text-base">
              {error ? 'cloud_off' : profile ? 'cloud_done' : busy ? 'sync' : serviceConfigured ? 'cloud_queue' : 'cloud_off'}
            </span>
            {error
              ? profile
                ? 'Profil hors ligne disponible · nouvelle tentative automatique'
                : 'Service de compte indisponible · nouvelle tentative automatique'
              : profile
                ? 'Compte ClashGO synchronisé'
                : busy
                  ? 'Connexion au compte ClashGO'
                  : serviceConfigured
                    ? 'Service de compte configuré'
                    : 'Service de compte non configuré'}
          </div>
        </div>

        {(message || error) && (
          <div
            title={error || undefined}
            className={'mt-4 rounded-xl px-4 py-3 text-xs font-bold ' + (error ? 'bg-amber-500/10 text-amber-600 dark:text-amber-400' : 'bg-emerald-500/10 text-emerald-500')}
          >
            {error ? 'La synchronisation est temporairement indisponible. ClashGO réessaiera automatiquement.' : message}
          </div>
        )}
      </section>

      {memberPage === 'village' && profile && (
        <>
          <section className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-6 gap-3">
            {[
              ['HDV', 'TH ' + profile.townHallLevel],
              ['XP', String(profile.expLevel)],
              ['Trophées', profile.trophies.toLocaleString()],
              ['Record', profile.bestTrophées.toLocaleString()],
              ['Étoiles de guerre', profile.warStars.toLocaleString()],
              ['Ligue', profile.league?.name || '—'],
            ].map(([label, value]) => (
              <div key={label} className="rounded-2xl border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-4">
                <div className="text-[9px] uppercase tracking-[0.18em] font-black text-zinc-400">{label}</div>
                <div className="mt-1 text-lg font-black text-zinc-950 dark:text-white">{value}</div>
              </div>
            ))}
          </section>

          {currentArmée && currentArmée.units.length > 0 && (
            <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6">
              <div className="flex items-center justify-between gap-4 mb-5">
                <div>
                  <div className="text-[10px] font-black uppercase tracking-[0.2em] text-zinc-400">Dernière armée détectée</div>
                  <div className="mt-1 flex flex-wrap items-center gap-3">
                    <h4 className="text-xl font-black text-zinc-950 dark:text-white">Composition détectée</h4>
                    <span className={`px-2.5 py-1 rounded-full text-[9px] font-black uppercase tracking-wider border ${
                      currentArmée.uncertain
                        ? 'bg-amber-500/10 border-amber-500/20 text-amber-600 dark:text-amber-400'
                        : currentArmée.ready
                          ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-600 dark:text-emerald-400'
                          : 'bg-rose-500/10 border-rose-500/20 text-rose-600 dark:text-rose-400'
                    }`}>
                      {currentArmée.uncertain ? 'Vérification vision' : currentArmée.ready ? 'Composition conforme' : 'Composition différente'}
                    </span>
                  </div>
                  <p className="mt-1 text-xs text-zinc-500">
                    Détectée automatiquement depuis la barre de troupes avant le déploiement.
                    {currentArmée.target_label ? ` Target: ${currentArmée.target_label}.` : ''}
                  </p>
                </div>
                <div className="text-[10px] font-bold text-zinc-400">
                  {new Date(currentArmée.timestamp).toLocaleTimeString()}
                </div>
              </div>
              <div className="grid grid-cols-2 md:grid-cols-3 xl:grid-cols-5 gap-2">
                {currentArmée.units.map((unit, idx) => (
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
              {!!currentArmée.warnings?.length && (
                <details className="mt-4 rounded-xl border border-amber-500/20 bg-amber-500/5 p-4">
                  <summary className="cursor-pointer text-[10px] font-black uppercase tracking-wider text-amber-600 dark:text-amber-400">
                    Notes de détection ({currentArmée.warnings.length})
                  </summary>
                  <div className="mt-3 space-y-1">
                    {currentArmée.warnings.map((warning, idx) => (
                      <div key={idx} className="text-[11px] font-medium text-zinc-500">{warning}</div>
                    ))}
                  </div>
                </details>
              )}
            </section>
          )}

          {farmProfile && (
            <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6">
              <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-4">
                <div>
                  <div className="text-[10px] font-black uppercase tracking-[0.2em] text-zinc-400">Plan de farm automatique</div>
                  <h4 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">{farmProfile.label}</h4>
                  <p className="mt-1 text-xs font-medium text-zinc-500">Selected automatically from your linked HDV. No manual setup required.</p>
                </div>
                <div className="flex gap-3 text-center">
                  <div className="rounded-xl bg-zinc-50 dark:bg-zinc-950/40 px-4 py-3">
                    <div className="text-[9px] font-black uppercase tracking-wider text-zinc-400">Armée</div>
                    <div className="text-lg font-black text-zinc-950 dark:text-white">{farmProfile.troop_capacity}</div>
                  </div>
                  <div className="rounded-xl bg-zinc-50 dark:bg-zinc-950/40 px-4 py-3">
                    <div className="text-[9px] font-black uppercase tracking-wider text-zinc-400">Sorts</div>
                    <div className="text-lg font-black text-zinc-950 dark:text-white">{farmProfile.spell_capacity}</div>
                  </div>
                </div>
              </div>

              <div className="mt-5 grid grid-cols-1 md:grid-cols-4 gap-3">
                <PlanGroup title="Troops" items={farmProfile.troops.map(u => u.count + '× ' + u.name)} />
                <PlanGroup title="Sorts" items={farmProfile.spells.map(u => u.count + '× ' + u.name)} />
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
                <UnitGrid title="Sorts" units={homeSorts} />
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
