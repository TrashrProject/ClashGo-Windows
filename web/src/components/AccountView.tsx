
import React from 'react';
import { InterfaceLevel } from '../types';
import { ActivateLicense, ApplyMemberPreset, ApplySavedMemberPreset, CheckForUpdate, ClearAccount, DeactivateLicense, DeleteMemberPreset, GetAccountConfig, GetAppVersion, GetCachedPlayerProfile, GetControlServiceConfig, GetConfig, GetCurrentArmy, GetLicensePolicy, GetLicenseState, GetMemberInterfaceLevel, GetMemberPresets, GetMemberSettings, GetMultiAccountConfig, GetMultiAccountStatus, GetMultiAccountSwitchCalibration, GetPlayerProfile, HasPreviousMemberSettings, GetUpdateStatus, GetVillageResources, InstallAndRestart, RefreshLicense, ResolveMultiAccountRecovery, SaveAccountConfig, SaveMemberPreset, SaveMemberSettings, SaveMultiAccountConfig, SetBetaControlServiceURL, UndoMemberSettings } from '../../wailsjs/go/main/App';
import { EventsOn } from '../../wailsjs/runtime';
import MultiAccountCalibration from './MultiAccountCalibration';

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
  target_town_hall?: number;
  target_label?: string;
  ready: boolean;
  uncertain: boolean;
  warnings?: string[];
};

type ManagedAccount = {
  id: string;
  label?: string;
  player_tag: string;
  enabled: boolean;
  switch_slot?: number;
  max_attacks_per_turn?: number;
  town_hall?: number;
  strategy_file?: string;
};

type MultiAccountConfigView = {
  enabled: boolean;
  active_account_id?: string;
  default_attacks_per_turn: number;
  accounts: ManagedAccount[];
};

type MultiAccountRuntimeStatusView = {
  enabled: boolean;
  active_account_id?: string;
  active_account_label?: string;
  attacks_this_turn: number;
  next_account_id?: string;
  next_account_label?: string;
  rotation_due: boolean;
  total_switches: number;
  last_switch_at?: string;
  last_error?: string;
  recovery_required?: boolean;
  recovery_target_account_id?: string;
  switch_in_flight?: boolean;
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
  machine_name?: string;
  last_validated?: string;
  offline_until?: string;
  plan?: string;
  expires_at?: string;
  error?: string;
};

type MemberUpdateStatus = {
  state?: string;
  available?: boolean;
  current_version?: string;
  latest_version?: string;
  progress?: number;
  error?: string;
};

type MemberSettings = {
  speed_profile: 'cautious' | 'normal' | 'fast';
  max_attacks_per_hour: number;
  max_attacks_per_session: number;
  break_every_attacks: number;
  break_minutes: number;
  adaptive_search: boolean;
  auto_profile_sync: boolean;
  auto_army_guard: boolean;
  auto_resource_tracking: boolean;
};

type MemberPresetSlot = {
  slot: number;
  name: string;
  updated_at?: string;
  settings: MemberSettings;
};

const formatLicenseRemaining = (expiresAt?: string): string => {
  if (!expiresAt) return 'À vie';
  const expiry = new Date(expiresAt).getTime();
  if (!Number.isFinite(expiry)) return 'Expiration inconnue';
  const remaining = expiry - Date.now();
  if (remaining <= 0) return 'Expirée';
  const hours = Math.ceil(remaining / (60 * 60 * 1000));
  if (hours <= 24) return 'Expire aujourd’hui';
  const days = Math.ceil(hours / 24);
  return days === 1 ? 'Reste 1 jour' : `Reste ${days} jours`;
};

const licenseExpiryState = (expiresAt?: string): 'none' | 'soon' | 'expired' => {
  if (!expiresAt) return 'none';
  const expiry = new Date(expiresAt).getTime();
  if (!Number.isFinite(expiry)) return 'none';
  const remaining = expiry - Date.now();
  if (remaining <= 0) return 'expired';
  return remaining <= 3 * 24 * 60 * 60 * 1000 ? 'soon' : 'none';
};

const isOfflineGrace = (state: LicenseState | null): boolean =>
  Boolean(state?.error && state.error.toLowerCase().includes('offline'));

const snapshotAgeLabel = (timestamp?: string): string => {
  if (!timestamp) return 'En attente';
  const value = new Date(timestamp).getTime();
  if (!Number.isFinite(value)) return 'Date inconnue';
  const ageMs = Math.max(0, Date.now() - value);
  const minutes = Math.floor(ageMs / 60000);
  if (minutes < 1) return 'À l’instant';
  if (minutes < 60) return `Il y a ${minutes} min`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `Il y a ${hours} h`;
  const days = Math.floor(hours / 24);
  return `Il y a ${days} j`;
};

const friendlyAccountActionError = (value: unknown): string => {
  const raw = value instanceof Error ? value.message : String(value || '');
  const text = raw.toLowerCase();
  if (text.includes('stop clashgo before changing')) {
    return 'Arrête ClashGO avant de changer le compte joueur.';
  }
  if (text.includes('stop clashgo before unlinking')) {
    return 'Arrête ClashGO avant de délier le compte joueur.';
  }
  return raw || 'Impossible de modifier le compte Clash.';
};

const friendlyLicenseError = (value?: string): string => {
  const raw = String(value || '');
  const text = raw.toLowerCase();
  if (text.includes('already activated on another machine') || text.includes('machine mismatch')) {
    return 'Licence déjà liée à un autre PC · réinitialisation nécessaire.';
  }
  if (text.includes('free trial already used')) {
    return 'L’essai gratuit de 2 jours a déjà été utilisé sur ce PC.';
  }
  if (text.includes('expired')) return 'Licence expirée · renouvellement nécessaire.';
  if (text.includes('invalid') || text.includes('revoked')) return 'Licence invalide ou désactivée.';
  if (text.includes('not configured')) return 'Le service de licence ClashGO n’est pas encore configuré.';
  if (text.includes('deadline exceeded') || text.includes('timeout') || text.includes('timed out')) {
    return 'Le serveur de licence met trop de temps à répondre. Réessaie dans quelques secondes.';
  }
  if (text.includes('connection refused') || text.includes('network') || text.includes('no such host') || text.includes('unavailable')) {
    return 'Impossible de joindre le serveur de licence. Vérifie Internet puis réessaie.';
  }
  if (text.includes('http 5') || text.includes('internal server error')) {
    return 'Le service de licence rencontre un problème temporaire. Réessaie dans quelques instants.';
  }
  if (text.includes('decode license response')) {
    return 'Réponse du service de licence invalide. Réessaie dans quelques instants.';
  }
  return raw || 'Impossible de vérifier la licence pour le moment.';
};

const friendlyControlServiceError = (value: unknown): string => {
  const raw = value instanceof Error ? value.message : String(value || '');
  const text = raw.toLowerCase();
  if (text.includes('unreachable') || text.includes('connection refused') || text.includes('no such host')) {
    return 'Serveur introuvable. Vérifie que le serveur local est lancé puis réessaie.';
  }
  if (text.includes('health check returned')) {
    return 'Le serveur répond, mais son service ClashGO n’est pas prêt.';
  }
  if (text.includes('health response') || text.includes('health check failed')) {
    return 'Le serveur ne répond pas comme un serveur de licences ClashGO valide.';
  }
  if (text.includes('must use https')) {
    return 'Une adresse distante doit utiliser HTTPS. HTTP est autorisé uniquement en local.';
  }
  if (text.includes('invalid control service url') || text.includes('control service url is required')) {
    return 'Adresse du serveur invalide.';
  }
  if (text.includes('stop clashgo')) {
    return 'Arrête le bot avant de changer le serveur de licences.';
  }
  return raw || 'Impossible de connecter le serveur de licences.';
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

const isCustomPacing = (settings: MemberSettings): boolean => {
  const preset = applySpeedPreset(settings, settings.speed_profile);
  return preset.max_attacks_per_hour !== settings.max_attacks_per_hour
    || preset.break_every_attacks !== settings.break_every_attacks
    || preset.break_minutes !== settings.break_minutes;
};

type UsagePreset = 'short' | 'balanced' | 'fast';

const activeUsagePreset = (settings: MemberSettings): UsagePreset | null => {
  const matches = (
    speed: MemberSettings['speed_profile'],
    perHour: number,
    perSession: number,
    breakEvery: number,
    breakMinutes: number,
  ) =>
    settings.speed_profile === speed
    && settings.max_attacks_per_hour === perHour
    && settings.max_attacks_per_session === perSession
    && settings.break_every_attacks === breakEvery
    && settings.break_minutes === breakMinutes
    && settings.adaptive_search;

  if (matches('normal', 12, 10, 5, 3)) return 'short';
  if (matches('normal', 12, 50, 5, 3)) return 'balanced';
  if (matches('fast', 16, 100, 6, 2)) return 'fast';
  return null;
};

type LicensePolicy = {
  enforced: boolean;
  service_configured: boolean;
  service_url?: string;
};

type ControlServiceView = {
  service_url?: string;
  configured?: boolean;
  embedded?: boolean;
  override_allowed?: boolean;
  requires_restart?: boolean;
};

const safeLicenseEventsOn = (
  eventName: string,
  callback: (payload: LicenseState) => void,
): (() => void) => {
  try {
    return EventsOn(eventName, callback);
  } catch {
    return () => {};
  }
};

const safeEventsOn = <T,>(
  eventName: string,
  callback: (payload: T) => void,
): (() => void) => {
  try {
    return EventsOn(eventName, callback);
  } catch {
    return () => {};
  }
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
  interfaceLevel: InterfaceLevel;
  initialPage?: 'account' | 'settings' | 'village';
  testSessionActive?: boolean;
  automationActive?: boolean;
  onInterfaceLevelChange: (level: InterfaceLevel) => void;
  onAccountChanged: (tag: string) => void;
  onReadinessChanged?: () => void;
  onPageChange?: (page: 'account' | 'settings' | 'village') => void;
}

const AccountView: React.FC<AccountViewProps> = React.memo(({
  playerTag,
  interfaceLevel,
  initialPage = 'account',
  testSessionActive = false,
  automationActive = false,
  onInterfaceLevelChange,
  onAccountChanged,
  onReadinessChanged,
  onPageChange,
}) => {
  const [profile, setProfile] = React.useState<PlayerProfile | null>(null);
  const [resources, setResources] = React.useState<VillageResources | null>(null);
  const [farmProfile, setFarmProfile] = React.useState<FarmProfile | null>(null);
  const [currentArmy, setCurrentArmy] = React.useState<CurrentArmy | null>(null);
  const [serviceConfigured, setServiceConfigured] = React.useState(false);
  const [busy, setBusy] = React.useState(false);
  const [message, setMessage] = React.useState('');
  const [error, setError] = React.useState('');
  const [licenseState, setLicenseState] = React.useState<LicenseState | null>(null);
  const [licensePolicy, setLicensePolicy] = React.useState<LicensePolicy | null>(null);
  const [licenseKey, setLicenseKey] = React.useState('');
  const [licenseBusy, setLicenseBusy] = React.useState(false);
  const [licenseError, setLicenseError] = React.useState('');
  const [licenseRefreshing, setLicenseRefreshing] = React.useState(false);
  const [controlService, setControlService] = React.useState<ControlServiceView | null>(null);
  const [controlURLInput, setControlURLInput] = React.useState('http://127.0.0.1:8787');
  const [controlURLBusy, setControlURLBusy] = React.useState(false);
  const [controlURLMessage, setControlURLMessage] = React.useState('');
  const [memberSettings, setMemberSettings] = React.useState<MemberSettings | null>(null);
  const [memberSaving, setMemberSaving] = React.useState(false);
  const [memberMessage, setMemberMessage] = React.useState('');
  const [memberSaveError, setMemberSaveError] = React.useState('');
  const [memberUndoAvailable, setMemberUndoAvailable] = React.useState(false);
  const [memberUndoBusy, setMemberUndoBusy] = React.useState(false);
  const [memberPresets, setMemberPresets] = React.useState<MemberPresetSlot[]>([]);
  const [memberPresetBusy, setMemberPresetBusy] = React.useState<number | null>(null);
  const [memberPresetNames, setMemberPresetNames] = React.useState<Record<number, string>>({});
  const [memberPresetDeleteConfirm, setMemberPresetDeleteConfirm] = React.useState<number | null>(null);
  const memberSaveLockRef = React.useRef(false);
  const memberPresetLockRef = React.useRef(false);
  const licenseActionLockRef = React.useRef(false);
  const accountActionLockRef = React.useRef(false);
  const activeLicenseHintRef = React.useRef<string>('');
  const [memberPage, setMemberPage] = React.useState<'account' | 'settings' | 'village'>(initialPage);

  React.useEffect(() => {
    setMemberPage(initialPage);
  }, [initialPage]);

  React.useEffect(() => {
    setAccountTagInput(playerTag || '');
  }, [playerTag]);

  React.useEffect(() => {
    if (memberPage !== 'account') return;
    let active = true;
    void Promise.all([
      GetMultiAccountConfig(),
      GetMultiAccountStatus().catch(() => null),
      GetMultiAccountSwitchCalibration().catch(() => ''),
    ]).then(([cfg, status, calibration]) => {
      if (!active) return;
      const value = (cfg || {}) as MultiAccountConfigView;
      setMultiAccount({
        enabled: Boolean(value.enabled),
        active_account_id: value.active_account_id || '',
        default_attacks_per_turn: Math.max(1, Number(value.default_attacks_per_turn || 10)),
        accounts: Array.isArray(value.accounts) ? value.accounts : [],
      });
      setMultiAccountStatus(status ? status as MultiAccountRuntimeStatusView : null);
      setMultiAccountCalibrated(Boolean(String(calibration || '').trim()));
    }).catch(() => {
      // Multi-account is optional; leave the single-account UI unaffected.
    });
    return () => { active = false; };
  }, [memberPage]);

  const [confirmDeactivate, setConfirmDeactivate] = React.useState(false);
  const [supportCodeCopied, setSupportCodeCopied] = React.useState(false);
  const [supportSummaryCopied, setSupportSummaryCopied] = React.useState(false);
  const [confirmUnlink, setConfirmUnlink] = React.useState(false);
  const [appVersion, setAppVersion] = React.useState('');
  const [memberUpdate, setMemberUpdate] = React.useState<MemberUpdateStatus | null>(null);
  const [checkingUpdate, setCheckingUpdate] = React.useState(false);
  const [installingUpdate, setInstallingUpdate] = React.useState(false);
  const [accountTagInput, setAccountTagInput] = React.useState(playerTag || '');
  const [accountLinkBusy, setAccountLinkBusy] = React.useState(false);
  const [accountLinkMessage, setAccountLinkMessage] = React.useState('');
  const [multiAccount, setMultiAccount] = React.useState<MultiAccountConfigView>({
    enabled: false,
    active_account_id: '',
    default_attacks_per_turn: 10,
    accounts: [],
  });
  const [multiAccountBusy, setMultiAccountBusy] = React.useState(false);
  const [multiAccountMessage, setMultiAccountMessage] = React.useState('');
  const [multiAccountError, setMultiAccountError] = React.useState('');
  const [multiAccountCalibrated, setMultiAccountCalibrated] = React.useState(false);
  const [multiAccountStatus, setMultiAccountStatus] = React.useState<MultiAccountRuntimeStatusView | null>(null);
  const [multiAccountRecoveryBusy, setMultiAccountRecoveryBusy] = React.useState(false);

  React.useEffect(() => {
    if (memberPage !== 'account') return;
    let cancelled = false;

    const refresh = async () => {
      try {
        const status = await GetMultiAccountStatus();
        if (!cancelled) setMultiAccountStatus(status as MultiAccountRuntimeStatusView);
      } catch {
        // Runtime status is best-effort while the Wails bridge starts.
      }
    };

    void refresh();
    const interval = window.setInterval(() => void refresh(), automationActive ? 2000 : 5000);
    return () => {
      cancelled = true;
      window.clearInterval(interval);
    };
  }, [memberPage, automationActive]);

  const refreshLicense = React.useCallback(async () => {
    try {
      const [state, policy, control] = await Promise.all([
        GetLicenseState(),
        GetLicensePolicy(),
        GetControlServiceConfig().catch(() => null),
      ]);
      setLicenseState(state as LicenseState);
      activeLicenseHintRef.current = state?.license_hint || '';
      setLicensePolicy(policy as LicensePolicy);
      if (control) {
        const typedControl = control as ControlServiceView;
        setControlService(typedControl);
        if (typedControl.service_url) setControlURLInput(typedControl.service_url);
      }
      if (state?.activated && (state.role === 'developer' || state.role === 'admin')) {
        onInterfaceLevelChange('developer');
      }
    } catch {
      // Licensing UI remains usable in local beta mode while the control
      // service is not configured.
    }
  }, [onInterfaceLevelChange]);

  const saveBetaControlService = async () => {
    if (controlURLBusy || automationActive || !controlService?.override_allowed) return;
    setControlURLBusy(true);
    setControlURLMessage('');
    try {
      const value = await SetBetaControlServiceURL(controlURLInput.trim());
      const typed = (value || {}) as ControlServiceView;
      setControlService(typed);
      if (typed.service_url) setControlURLInput(typed.service_url);
      setControlURLMessage('Serveur de licences connecté. Tu peux activer ta clé immédiatement.');
      await refreshLicense();
      onReadinessChanged?.();
    } catch (e) {
      setControlURLMessage(friendlyControlServiceError(e));
    } finally {
      setControlURLBusy(false);
    }
  };

  const refreshMemberUpdate = React.useCallback(async () => {
    try {
      const [version, status] = await Promise.all([GetAppVersion(), GetUpdateStatus()]);
      setAppVersion(String(version || ''));
      setMemberUpdate(status as MemberUpdateStatus);
    } catch {
      // Update status is secondary information; the member area remains usable.
    }
  }, [playerTag]);

  const checkMemberUpdate = async () => {
    if (checkingUpdate) return;
    setCheckingUpdate(true);
    try {
      const status = await CheckForUpdate();
      setMemberUpdate(status as MemberUpdateStatus);
    } catch (e) {
      setMemberUpdate((current) => ({
        ...(current || {}),
        state: 'error',
        error: e instanceof Error ? e.message : String(e),
      }));
    } finally {
      setCheckingUpdate(false);
    }
  };

  const installMemberUpdate = async () => {
    if (installingUpdate || !memberUpdate?.available) return;
    setInstallingUpdate(true);
    setMemberUpdate((current) => ({ ...(current || {}), state: 'downloading', error: '' }));
    try {
      await InstallAndRestart();
    } catch (e) {
      setMemberUpdate((current) => ({
        ...(current || {}),
        state: 'error',
        error: e instanceof Error ? e.message : String(e),
      }));
      setInstallingUpdate(false);
    }
  };

  const refreshMemberSettings = React.useCallback(async () => {
    try {
      const [settings, canUndo, presets] = await Promise.all([
        GetMemberSettings(),
        HasPreviousMemberSettings().catch(() => false),
        GetMemberPresets().catch(() => []),
      ]);
      setMemberSettings(settings as MemberSettings);
      setMemberUndoAvailable(Boolean(canUndo));
      const loadedPresets = (presets || []) as MemberPresetSlot[];
      setMemberPresets(loadedPresets);
      // Rebuild slot names from the active licence only. Merging with the
      // previous React state could leak an unsaved/profile label from the
      // member that was active before a licence switch.
      const nextNames: Record<number, string> = {};
      for (const preset of loadedPresets) {
        nextNames[preset.slot] = preset.name || `Profil ${preset.slot}`;
      }
      setMemberPresetNames(nextNames);
    } catch {
      // Member preferences are best-effort while the Wails bridge initializes.
    }
  }, []);

  const applyUsagePreset = async (preset: 'short' | 'balanced' | 'fast') => {
    if (memberSaveLockRef.current || memberSaving || testSessionActive) return;
    memberSaveLockRef.current = true;
    setMemberSaving(true);
    setMemberMessage('');
    setMemberSaveError('');
    try {
      const saved = await ApplyMemberPreset(preset);
      setMemberSettings(saved as MemberSettings);
      setMemberUndoAvailable(true);
      setMemberMessage(
        preset === 'short'
          ? 'Session courte appliquée.'
          : preset === 'fast'
            ? 'Farm rapide appliqué.'
            : 'Farm équilibré appliqué.'
      );
      onReadinessChanged?.();
    } catch (e) {
      setMemberSaveError(e instanceof Error ? e.message : String(e));
      try {
        const current = await GetMemberSettings();
        setMemberSettings(current as MemberSettings);
      } catch {
        // Keep the last known state if the bridge is temporarily unavailable.
      }
    } finally {
      memberSaveLockRef.current = false;
      setMemberSaving(false);
    }
  };

  const saveMemberSettings = async (next: MemberSettings) => {
    if (memberSaveLockRef.current || memberSaving || testSessionActive) return;
    memberSaveLockRef.current = true;
    setMemberSaving(true);
    setMemberMessage('');
    setMemberSaveError('');
    try {
      const saved = await SaveMemberSettings(next as any);
      setMemberSettings(saved as MemberSettings);
      setMemberUndoAvailable(true);
      setMemberMessage('Réglages appliqués au bot.');
      onReadinessChanged?.();
    } catch (e) {
      setMemberSaveError(e instanceof Error ? e.message : String(e));
      try {
        const current = await GetMemberSettings();
        setMemberSettings(current as MemberSettings);
      } catch {
        // Keep the visible error if the local settings cannot be reloaded.
      }
    } finally {
      memberSaveLockRef.current = false;
      setMemberSaving(false);
    }
  };

  const undoMemberSettings = async () => {
    if (memberUndoBusy || memberSaving || testSessionActive || !memberUndoAvailable) return;
    setMemberUndoBusy(true);
    setMemberMessage('');
    setMemberSaveError('');
    try {
      const restored = await UndoMemberSettings();
      setMemberSettings(restored as MemberSettings);
      setMemberUndoAvailable(true);
      setMemberMessage('Dernière modification annulée. Tu peux recliquer pour rétablir l’état précédent.');
      onReadinessChanged?.();
    } catch (e) {
      setMemberSaveError(e instanceof Error ? e.message : String(e));
      setMemberUndoAvailable(false);
    } finally {
      setMemberUndoBusy(false);
    }
  };

  const savePersonalPreset = async (slot: number) => {
    if (memberPresetLockRef.current || memberPresetBusy !== null || memberSaving || testSessionActive) return;
    memberPresetLockRef.current = true;
    setMemberPresetBusy(slot);
    setMemberMessage('');
    setMemberSaveError('');
    try {
      const presets = await SaveMemberPreset(slot, memberPresetNames[slot] || `Profil ${slot}`);
      setMemberPresets((presets || []) as MemberPresetSlot[]);
      setMemberMessage(`Profil personnel ${slot} enregistré.`);
    } catch (e) {
      setMemberSaveError(e instanceof Error ? e.message : String(e));
    } finally {
      memberPresetLockRef.current = false;
      setMemberPresetBusy(null);
    }
  };

  const applyPersonalPreset = async (slot: number) => {
    if (memberPresetLockRef.current || memberPresetBusy !== null || memberSaving || testSessionActive) return;
    memberPresetLockRef.current = true;
    setMemberPresetBusy(slot);
    setMemberMessage('');
    setMemberSaveError('');
    try {
      const saved = await ApplySavedMemberPreset(slot);
      setMemberSettings(saved as MemberSettings);
      setMemberUndoAvailable(true);
      setMemberMessage(`Profil personnel ${slot} appliqué.`);
      onReadinessChanged?.();
    } catch (e) {
      setMemberSaveError(e instanceof Error ? e.message : String(e));
    } finally {
      memberPresetLockRef.current = false;
      setMemberPresetBusy(null);
    }
  };

  const deletePersonalPreset = async (slot: number) => {
    if (memberPresetLockRef.current || memberPresetBusy !== null || memberSaving || testSessionActive) return;
    memberPresetLockRef.current = true;
    setMemberPresetBusy(slot);
    setMemberMessage('');
    setMemberSaveError('');
    try {
      const presets = await DeleteMemberPreset(slot);
      setMemberPresets((presets || []) as MemberPresetSlot[]);
      setMemberPresetDeleteConfirm(null);
      setMemberMessage(`Profil personnel ${slot} supprimé.`);
    } catch (e) {
      setMemberSaveError(e instanceof Error ? e.message : String(e));
    } finally {
      memberPresetLockRef.current = false;
      setMemberPresetBusy(null);
    }
  };

  const activateLicense = async () => {
    const key = licenseKey.trim();
    if (!key || licenseActionLockRef.current || licenseBusy) return;
    licenseActionLockRef.current = true;
    setLicenseBusy(true);
    setLicenseError('');
    try {
      const state = await ActivateLicense(key);
      const typed = state as LicenseState;
      setLicenseState(typed);
      setLicenseKey('');
      await refreshMemberSettings();
      if (typed.role === 'developer' || typed.role === 'admin') {
        onInterfaceLevelChange('developer');
      } else {
        try {
          const savedLevel = await GetMemberInterfaceLevel();
          onInterfaceLevelChange(savedLevel === 'advanced' ? 'advanced' : 'simple');
        } catch {
          onInterfaceLevelChange('simple');
        }
      }
    } catch (e) {
      setLicenseError(e instanceof Error ? e.message : String(e));
    } finally {
      licenseActionLockRef.current = false;
      setLicenseBusy(false);
    }
  };

  const refreshLicenseNow = async () => {
    if (licenseActionLockRef.current || licenseRefreshing) return;
    licenseActionLockRef.current = true;
    setLicenseRefreshing(true);
    setLicenseError('');
    try {
      const state = await RefreshLicense();
      const typed = state as LicenseState;
      setLicenseState(typed);
      if (typed.activated && (typed.role === 'developer' || typed.role === 'admin')) {
        onInterfaceLevelChange('developer');
      } else if (typed.activated) {
        try {
          const savedLevel = await GetMemberInterfaceLevel();
          onInterfaceLevelChange(savedLevel === 'advanced' ? 'advanced' : 'simple');
        } catch {
          // App-level license handling already clamps privileged UI safely.
        }
      }
    } catch (e) {
      setLicenseError(e instanceof Error ? e.message : String(e));
    } finally {
      licenseActionLockRef.current = false;
      setLicenseRefreshing(false);
    }
  };

  const deactivateLicense = async () => {
    if (licenseActionLockRef.current || licenseBusy) return;
    licenseActionLockRef.current = true;
    setLicenseBusy(true);
    setLicenseError('');
    try {
      await DeactivateLicense();
      setConfirmDeactivate(false);
      setMemberSettings(null);
      setMemberUndoAvailable(false);
      await refreshLicense();
      onInterfaceLevelChange('simple');
    } catch (e) {
      setLicenseError(e instanceof Error ? e.message : String(e));
    } finally {
      licenseActionLockRef.current = false;
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

      if (playerTag) {
        const cached = await GetCachedPlayerProfile();
        if (cached) setProfile(cached as PlayerProfile);

        if (account.service_configured) {
          try {
            const p = await GetPlayerProfile();
            setProfile(p as PlayerProfile);

            // Account sync may auto-switch the farm profile to the player's HDV.
            // Re-read config once so the visible plan updates immediately.
            const refreshedCfg = await GetConfig();
            const refreshedFarm = (refreshedCfg as any)?.attack?.farm;
            if (refreshedFarm?.enabled && refreshedFarm?.profiles) {
              setFarmProfile(refreshedFarm.profiles[String(refreshedFarm.town_hall)] || null);
            }
          } catch (syncErr) {
            // The tag is still useful locally even when the optional account
            // service is offline. Surface a soft warning without treating the
            // member space as broken.
            setError(syncErr instanceof Error ? syncErr.message : String(syncErr));
          }
        }
      } else {
        setProfile(null);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }, [playerTag]);

  React.useEffect(() => {
    try { localStorage.setItem('clashgo_member_page', memberPage); } catch {}
  }, [memberPage]);

  React.useEffect(() => {
    void refresh();
    void refreshLicense();
    void refreshMemberSettings();
    void refreshMemberUpdate();
  }, [refresh, refreshLicense, refreshMemberSettings, refreshMemberUpdate, playerTag]);

  React.useEffect(() => {
    const off = safeLicenseEventsOn('license_state', (payload) => {
      if (!payload || typeof payload !== 'object') return;

      const nextHint = payload.license_hint || '';
      const previousHint = activeLicenseHintRef.current;
      const identityChanged = Boolean(previousHint && nextHint && previousHint !== nextHint);
      const deactivated = !payload.activated;

      if (identityChanged || deactivated) {
        // Clear every member-facing transient immediately so a licence switch
        // cannot show the previous member's village, presets or success/error
        // messages while the new scoped state is loading.
        setProfile(null);
        setResources(null);
        setFarmProfile(null);
        setCurrentArmy(null);
        setMemberPresets([]);
        setMemberPresetNames({});
        setMemberUndoAvailable(false);
        setMemberMessage('');
        setMemberSaveError('');
        setMessage('');
        setError('');
        setAccountLinkMessage('');
        setConfirmDeactivate(false);
        setConfirmUnlink(false);
      }

      activeLicenseHintRef.current = nextHint;
      setLicenseState(payload);

      if (payload.activated) {
        void refreshMemberSettings();
      } else {
        setMemberSettings(null);
      }

      if (payload.activated && (payload.role === 'developer' || payload.role === 'admin')) {
        onInterfaceLevelChange('developer');
      } else if (payload.activated) {
        // App.tsx performs the immediate non-persistent security clamp.
        // Here we only restore the persisted per-license preference so this
        // component can never overwrite "advanced" during a role transition.
        void GetMemberInterfaceLevel()
          .then((saved: unknown) => onInterfaceLevelChange(saved === 'advanced' ? 'advanced' : 'simple'))
          .catch(() => {});
      }
    });
    return off;
  }, [interfaceLevel, onInterfaceLevelChange, refreshMemberSettings]);

  React.useEffect(() => {
    const off = safeEventsOn<MemberUpdateStatus>('updater_status', (payload) => {
      if (!payload || typeof payload !== 'object') return;
      setMemberUpdate(payload);
      if (payload.state === 'error') {
        setInstallingUpdate(false);
      }
    });
    return off;
  }, []);

  // The local/proxied account service may start a few seconds after ClashGO.
  // Retry automatically while no profile is available so users never have to
  // hammer "Sync profile" after launching the service.
  React.useEffect(() => {
    if (memberPage !== 'village') return;

    // Keep public progression fresh only while the Village surface is visible.
    // The initial member load still hydrates the cached profile immediately.
    const id = window.setInterval(() => {
      void refresh();
    }, 15 * 60 * 1000);
    return () => window.clearInterval(id);
  }, [memberPage, refresh]);

  React.useEffect(() => {
    if (memberPage !== 'village') return;

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
  }, [memberPage]);

  React.useEffect(() => {
    if (memberPage !== 'village' || profile || busy || !playerTag || !serviceConfigured) return;
    const id = window.setInterval(() => {
      void refresh();
    }, 4000);
    return () => window.clearInterval(id);
  }, [memberPage, profile, busy, playerTag, serviceConfigured, refresh]);

  const linkClashAccount = async () => {
    if (accountActionLockRef.current || accountLinkBusy || automationActive) return;
    let tag = accountTagInput.trim().toUpperCase().replace(/\s+/g, '');
    if (!tag) {
      setAccountLinkMessage('Entre ton tag joueur Clash of Clans.');
      return;
    }
    if (!tag.startsWith('#')) tag = '#' + tag;
    if (tag.length < 4 || tag.length > 20 || !/^[#][A-Z0-9]+$/.test(tag)) {
      setAccountLinkMessage('Tag invalide. Exemple : #2ABC123XY');
      return;
    }

    accountActionLockRef.current = true;
    setAccountLinkBusy(true);
    setAccountLinkMessage('');
    setError('');
    try {
      await SaveAccountConfig(tag);
      setAccountTagInput(tag);
      onAccountChanged(tag);

      if (serviceConfigured) {
        try {
          const synced = await GetPlayerProfile();
          if (synced) {
            setProfile(synced as PlayerProfile);
            setAccountLinkMessage('Compte lié et profil synchronisé.');
          } else {
            setAccountLinkMessage('Tag enregistré. La synchronisation se fera automatiquement.');
          }
        } catch {
          setAccountLinkMessage('Tag enregistré. Le profil se synchronisera plus tard ; le bot reste utilisable.');
        }
      } else {
        setAccountLinkMessage('Tag enregistré localement. Le bot reste utilisable ; la synchronisation du profil est optionnelle.');
      }
    } catch (e) {
      setAccountLinkMessage(friendlyAccountActionError(e));
    } finally {
      accountActionLockRef.current = false;
      setAccountLinkBusy(false);
    }
  };

  const addMultiAccount = () => {
    setMultiAccount(current => {
      const used = new Set(current.accounts.map(a => a.id));
      let n = current.accounts.length + 1;
      while (used.has(`account-${n}`)) n += 1;
      const next: ManagedAccount = {
        id: `account-${n}`,
        label: `Compte ${n}`,
        player_tag: '',
        enabled: true,
        switch_slot: n,
        max_attacks_per_turn: 0,
        town_hall: 0,
        strategy_file: '',
      };
      return {
        ...current,
        active_account_id: current.active_account_id || next.id,
        accounts: [...current.accounts, next],
      };
    });
    setMultiAccountMessage('');
    setMultiAccountError('');
  };

  const updateMultiAccount = (index: number, patch: Partial<ManagedAccount>) => {
    setMultiAccount(current => ({
      ...current,
      accounts: current.accounts.map((account, i) => i === index ? { ...account, ...patch } : account),
    }));
    setMultiAccountMessage('');
    setMultiAccountError('');
  };

  const removeMultiAccount = (index: number) => {
    setMultiAccount(current => {
      const removed = current.accounts[index];
      const accounts = current.accounts.filter((_, i) => i !== index);
      let active = current.active_account_id || '';
      if (removed?.id === active) {
        active = accounts.find(a => a.enabled)?.id || accounts[0]?.id || '';
      }
      return { ...current, active_account_id: active, accounts };
    });
    setMultiAccountMessage('');
    setMultiAccountError('');
  };

  const saveMultiAccounts = async () => {
    if (multiAccountBusy || automationActive) return;
    setMultiAccountBusy(true);
    setMultiAccountMessage('');
    setMultiAccountError('');
    try {
      const payload: MultiAccountConfigView = {
        ...multiAccount,
        default_attacks_per_turn: Math.max(1, Math.min(100, Number(multiAccount.default_attacks_per_turn || 10))),
        accounts: multiAccount.accounts.map((account, index) => ({
          ...account,
          id: account.id || `account-${index + 1}`,
          label: String(account.label || '').trim(),
          player_tag: String(account.player_tag || '').trim().toUpperCase(),
          switch_slot: Math.max(0, Number(account.switch_slot || 0)),
          max_attacks_per_turn: Math.max(0, Math.min(100, Number(account.max_attacks_per_turn || 0))),
          town_hall: Math.max(0, Number(account.town_hall || 0)),
          strategy_file: String(account.strategy_file || '').trim(),
        })),
      };
      await SaveMultiAccountConfig(JSON.stringify(payload));
      const refreshed = await GetMultiAccountConfig();
      const value = (refreshed || payload) as MultiAccountConfigView;
      setMultiAccount({
        enabled: Boolean(value.enabled),
        active_account_id: value.active_account_id || '',
        default_attacks_per_turn: Number(value.default_attacks_per_turn || 10),
        accounts: Array.isArray(value.accounts) ? value.accounts : [],
      });
      const active = value.accounts?.find(a => a.id === value.active_account_id);
      if (active?.player_tag) onAccountChanged(active.player_tag);
      setMultiAccountMessage(
        value.enabled
          ? (multiAccountCalibrated
              ? 'Multi-comptes enregistré. La rotation automatique est prête.'
              : 'Multi-comptes enregistré. Il reste à calibrer le sélecteur Supercell ID avant la rotation automatique.')
          : 'Configuration multi-comptes enregistrée.'
      );
    } catch (e) {
      setMultiAccountError(friendlyAccountActionError(e));
    } finally {
      setMultiAccountBusy(false);
    }
  };

  const confirmMultiAccountRecovery = async (account: ManagedAccount) => {
    if (multiAccountRecoveryBusy || automationActive || !multiAccountStatus?.recovery_required) return;
    setMultiAccountRecoveryBusy(true);
    setMultiAccountError('');
    setMultiAccountMessage('');
    try {
      await ResolveMultiAccountRecovery(account.id);
      const [status, cfg] = await Promise.all([
        GetMultiAccountStatus(),
        GetMultiAccountConfig(),
      ]);
      const nextStatus = status as MultiAccountRuntimeStatusView;
      const nextConfig = (cfg || {}) as MultiAccountConfigView;
      setMultiAccountStatus(nextStatus);
      setMultiAccount({
        enabled: Boolean(nextConfig.enabled),
        active_account_id: nextConfig.active_account_id || account.id,
        default_attacks_per_turn: Math.max(1, Number(nextConfig.default_attacks_per_turn || 10)),
        accounts: Array.isArray(nextConfig.accounts) ? nextConfig.accounts : multiAccount.accounts,
      });
      onAccountChanged(account.player_tag);
      setMultiAccountMessage(
        `Compte confirmé : ${account.label || account.player_tag}. ClashGO peut maintenant être démarré avec l’IA isolée de ce compte.`
      );
    } catch (e) {
      setMultiAccountError(friendlyAccountActionError(e));
    } finally {
      setMultiAccountRecoveryBusy(false);
    }
  };

  const unlink = async () => {
    if (accountActionLockRef.current || automationActive) return;
    accountActionLockRef.current = true;
    setBusy(true);
    try {
      await ClearAccount();
      setConfirmUnlink(false);
      setProfile(null);
      setResources(null);
      setFarmProfile(null);
      setCurrentArmy(null);
      setAccountTagInput('');
      setAccountLinkMessage('');
      setError('');
      onAccountChanged('');
    } catch (e) {
      setError(friendlyAccountActionError(e));
    } finally {
      accountActionLockRef.current = false;
      setBusy(false);
    }
  };

  const homeTroops = (profile?.troops ?? []).filter(u => u.village === 'home');
  const homeHeroes = (profile?.heroes ?? []).filter(u => u.village === 'home');
  const homeSpells = (profile?.spells ?? []).filter(u => u.village === 'home');

  return (
    <div className="space-y-6 max-w-6xl mx-auto">
      <div className="sticky top-0 z-20 -mx-1 px-1 py-2 bg-zinc-50/90 dark:bg-zinc-950/90 backdrop-blur-xl overflow-x-auto">
        <div className="flex w-full min-w-max sm:w-auto sm:inline-flex rounded-2xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-1.5 shadow-sm">
          {([
            ['account', 'Compte', 'badge'],
            ['settings', 'Réglages bot', 'tune'],
            ['village', 'Village', 'castle'],
          ] as const).map(([id, label, icon]) => (
            <button
              key={id}
              type="button"
              onClick={() => {
                setMemberPage(id);
                onPageChange?.(id);
              }}
              className={
                'flex flex-1 sm:flex-none items-center justify-center gap-2 rounded-xl px-3 sm:px-4 py-2.5 text-[9px] sm:text-[10px] font-black uppercase tracking-[0.12em] sm:tracking-[0.16em] transition whitespace-nowrap ' +
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

      {memberPage === 'account' && !licensePolicy?.enforced && controlService?.override_allowed && (
        <section className="rounded-[2rem] border border-sky-200 dark:border-sky-900/50 bg-sky-50 dark:bg-sky-950/20 p-5 md:p-6">
          <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-4">
            <div className="min-w-0">
              <div className="text-[9px] font-black uppercase tracking-[0.2em] text-sky-500">Mode test bêta</div>
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="mt-1 text-lg font-black text-zinc-950 dark:text-white">Connecter le serveur de licences local</h3>
                {controlService?.configured && (
                  <span className="rounded-full bg-emerald-500/10 px-2.5 py-1 text-[8px] font-black uppercase tracking-widest text-emerald-600">
                    Serveur connecté
                  </span>
                )}
              </div>
              <p className="mt-1 text-xs font-semibold text-zinc-500">
                Pour tester les licences sans serveur public, lance START-CLASHGO-LICENSE-TEST.cmd puis garde l’adresse locale ci-dessous.
              </p>
            </div>
            <div className="flex flex-col sm:flex-row gap-2 lg:min-w-[420px]">
              <input
                value={controlURLInput}
                onChange={(e) => setControlURLInput(e.target.value)}
                disabled={controlURLBusy || automationActive}
                spellCheck={false}
                className="h-11 flex-1 rounded-xl border border-sky-200 dark:border-sky-900/60 bg-white dark:bg-zinc-950 px-3 text-xs font-mono outline-none focus:border-sky-400 disabled:opacity-50"
                aria-label="Adresse du serveur local ClashGO"
              />
              <button
                type="button"
                onClick={() => void saveBetaControlService()}
                disabled={controlURLBusy || automationActive || !controlURLInput.trim()}
                className="h-11 rounded-xl bg-sky-500 px-4 text-[10px] font-black uppercase tracking-widest text-white disabled:opacity-40"
              >
                {controlURLBusy ? 'Connexion…' : 'Connecter'}
              </button>
            </div>
          </div>
          {automationActive && (
            <div className="mt-3 text-[10px] font-bold text-amber-600 dark:text-amber-400">Arrête le bot avant de changer de serveur de licences.</div>
          )}
          {controlURLMessage && (
            <div className="mt-3 text-[10px] font-bold text-sky-700 dark:text-sky-300">{controlURLMessage}</div>
          )}
        </section>
      )}

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
                  <span>· Expire le {new Date(licenseState.expires_at).toLocaleString('fr-FR')}</span>
                )}
              </div>
            )}
            {licenseState?.last_validated && (
              <p className="mt-2 text-[10px] font-bold uppercase tracking-widest text-zinc-500">
                Dernière validation · {snapshotAgeLabel(licenseState.last_validated)}
              </p>
            )}
            {licenseState?.offline_until && isOfflineGrace(licenseState) && (
              <p className="mt-2 text-[10px] font-bold uppercase tracking-widest text-amber-500">
                Accès hors ligne jusqu’au {new Date(licenseState.offline_until).toLocaleString('fr-FR')}
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
                  <div className="mt-1 text-xs font-black">
                    {licenseState.machine_name || (licenseState.machine_id ? 'PC lié' : '—')}
                  </div>
                  {licenseState.machine_id && (
                    <div className="mt-0.5 text-[9px] font-mono font-bold text-zinc-500">
                      {licenseState.machine_id.slice(0, 8)}…
                    </div>
                  )}
                </div>
                <div className="rounded-xl border border-white/10 dark:border-zinc-200/70 bg-white/5 dark:bg-zinc-100 px-3 py-2.5">
                  <div className="text-[8px] font-black uppercase tracking-[0.18em] text-zinc-500">Expiration</div>
                  <div className="mt-1 text-xs font-black">
                    {formatLicenseRemaining(licenseState.expires_at)}
                  </div>
                  {licenseState.expires_at && (
                    <div className="mt-0.5 text-[9px] font-bold text-zinc-500">
                      {new Date(licenseState.expires_at).toLocaleDateString('fr-FR')}
                    </div>
                  )}
                </div>
              </div>
            )}
            {licenseState?.activated && licenseState.license_hint && licenseState.machine_id && (
              <div className="mt-3 flex flex-wrap items-center gap-2">
                <span className="text-[9px] font-black uppercase tracking-[0.18em] text-zinc-500">Code support</span>
                <code className="rounded-lg border border-white/10 dark:border-zinc-200/70 bg-white/5 dark:bg-zinc-100 px-2.5 py-1.5 text-[10px] font-black tracking-wider">
                  {licenseState.license_hint.replace(/[^A-Z0-9]/gi, '').slice(-4)}-{licenseState.machine_id.slice(0, 8).toUpperCase()}
                </code>
                <button
                  type="button"
                  onClick={async () => {
                    const code = `${licenseState.license_hint?.replace(/[^A-Z0-9]/gi, '').slice(-4) || 'LIC'}-${licenseState.machine_id?.slice(0, 8).toUpperCase() || 'DEVICE'}`;
                    try {
                      await navigator.clipboard.writeText(code);
                    } catch {
                      const ta = document.createElement('textarea');
                      ta.value = code;
                      ta.style.position = 'fixed';
                      ta.style.opacity = '0';
                      document.body.appendChild(ta);
                      ta.select();
                      document.execCommand('copy');
                      document.body.removeChild(ta);
                    }
                    setSupportCodeCopied(true);
                    window.setTimeout(() => setSupportCodeCopied(false), 1200);
                  }}
                  className="rounded-lg border border-white/10 dark:border-zinc-200/70 px-2.5 py-1.5 text-[9px] font-black uppercase tracking-wider text-zinc-400 dark:text-zinc-600 hover:text-white dark:hover:text-zinc-950"
                >
                  {supportCodeCopied ? 'Copié' : 'Copier'}
                </button>
                <button
                  type="button"
                  onClick={async () => {
                    const code = `${licenseState.license_hint?.replace(/[^A-Z0-9]/gi, '').slice(-4) || 'LIC'}-${licenseState.machine_id?.slice(0, 8).toUpperCase() || 'DEVICE'}`;
                    const plan = licenseState.plan === 'free_2d'
                      ? 'FREE 2J'
                      : licenseState.plan === 'week_1'
                        ? '1 semaine'
                        : licenseState.plan === 'month_1'
                          ? '1 mois'
                          : 'À vie';
                    const lines = [
                      'ClashGO · Infos support',
                      'Code support: ' + code,
                      'Version: ' + (appVersion ? 'v' + appVersion : 'inconnue'),
                      'Rôle: ' + (licenseState.role || 'member'),
                      'Plan: ' + plan,
                      'Expiration: ' + (licenseState.expires_at ? new Date(licenseState.expires_at).toLocaleString('fr-FR') : 'Jamais'),
                      'Dernière validation: ' + (licenseState.last_validated ? new Date(licenseState.last_validated).toLocaleString('fr-FR') : 'Inconnue'),
                    ];
                    const value = lines.join('\n');
                    try {
                      await navigator.clipboard.writeText(value);
                    } catch {
                      const ta = document.createElement('textarea');
                      ta.value = value;
                      ta.style.position = 'fixed';
                      ta.style.opacity = '0';
                      document.body.appendChild(ta);
                      ta.select();
                      document.execCommand('copy');
                      document.body.removeChild(ta);
                    }
                    setSupportSummaryCopied(true);
                    window.setTimeout(() => setSupportSummaryCopied(false), 1800);
                  }}
                  className="rounded-lg border border-white/10 dark:border-zinc-200/70 px-2.5 py-1.5 text-[9px] font-black uppercase tracking-wider text-zinc-400 dark:text-zinc-600 hover:text-white dark:hover:text-zinc-950"
                >
                  {supportSummaryCopied ? 'Infos copiées' : 'Copier infos support'}
                </button>
              </div>
            )}
          </div>

          {licenseState?.activated ? (
            <div className="shrink-0 flex flex-col items-end gap-2">
              {!confirmDeactivate && (
                <button
                  type="button"
                  onClick={() => void refreshLicenseNow()}
                  disabled={licenseBusy || licenseRefreshing}
                  className="px-5 py-3 rounded-xl bg-white dark:bg-zinc-950 text-zinc-950 dark:text-white text-[10px] font-black uppercase tracking-widest disabled:opacity-40"
                >
                  {licenseRefreshing ? 'Actualisation…' : 'Actualiser la licence'}
                </button>
              )}
              {!confirmDeactivate ? (
                <button
                  type="button"
                  onClick={() => setConfirmDeactivate(true)}
                  disabled={licenseBusy || licenseRefreshing}
                  className="px-5 py-3 rounded-xl border border-white/10 dark:border-zinc-200 text-[10px] font-black uppercase tracking-widest text-zinc-300 dark:text-zinc-600 hover:text-white dark:hover:text-zinc-950 disabled:opacity-40"
                >
                  Désactiver sur ce PC
                </button>
              ) : (
                <div className="max-w-[320px] rounded-2xl border border-rose-500/20 bg-rose-500/10 p-3">
                  <div className="text-[11px] font-bold text-rose-300 dark:text-rose-700">
                    Tu devras ressaisir ta clé pour réactiver ClashGO sur ce PC.
                  </div>
                  <div className="mt-3 flex gap-2">
                    <button
                      type="button"
                      onClick={() => void deactivateLicense()}
                      disabled={licenseBusy}
                      className="rounded-xl bg-rose-500 px-3 py-2 text-[9px] font-black uppercase tracking-widest text-white disabled:opacity-40"
                    >
                      {licenseBusy ? 'Désactivation…' : 'Confirmer'}
                    </button>
                    <button
                      type="button"
                      onClick={() => setConfirmDeactivate(false)}
                      disabled={licenseBusy}
                      className="rounded-xl border border-white/10 dark:border-zinc-300 px-3 py-2 text-[9px] font-black uppercase tracking-widest text-zinc-300 dark:text-zinc-600 disabled:opacity-40"
                    >
                      Annuler
                    </button>
                  </div>
                </div>
              )}
            </div>
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
        {(licenseError || (licenseState?.error && !isOfflineGrace(licenseState))) && (
          <div className="mt-4 rounded-xl bg-rose-500/10 px-4 py-3 text-xs font-bold text-rose-400 dark:text-rose-600">
            {friendlyLicenseError(licenseError || licenseState?.error)}
          </div>
        )}
        {isOfflineGrace(licenseState) && (
          <div className="mt-4 flex items-center gap-3 rounded-xl bg-amber-500/10 px-4 py-3 text-xs font-bold text-amber-500">
            <span className="material-symbols-outlined text-base">cloud_off</span>
            Connexion au serveur de licence indisponible · accès temporaire hors ligne actif.
          </div>
        )}
        {licenseState?.activated && licenseExpiryState(licenseState.expires_at) === 'soon' && (
          <div className="mt-4 flex items-center gap-3 rounded-xl border border-amber-500/20 bg-amber-500/10 px-4 py-3 text-xs font-bold text-amber-500">
            <span className="material-symbols-outlined text-base">schedule</span>
            Ta licence expire bientôt. Tu peux la renouveler avec la même clé, sans refaire l’activation.
          </div>
        )}
        {licenseExpiryState(licenseState?.expires_at) === 'expired' && (
          <div className="mt-4 flex items-center gap-3 rounded-xl border border-rose-500/20 bg-rose-500/10 px-4 py-3 text-xs font-bold text-rose-500">
            <span className="material-symbols-outlined text-base">event_busy</span>
            Cette licence est expirée. Après renouvellement, clique sur « Actualiser la licence ».
          </div>
        )}
      </section>

      {memberPage === 'account' && licenseState?.activated && (
        <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6 shadow-premium dark:shadow-none">
          <div className="flex items-start gap-4">
            <div className="grid h-11 w-11 shrink-0 place-items-center rounded-2xl bg-emerald-500/10 text-emerald-500">
              <span className="material-symbols-outlined">folder_managed</span>
            </div>
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Données membre</div>
              <h3 className="mt-1 text-lg font-black text-zinc-950 dark:text-white">Tes données suivent cette licence</h3>
              <p className="mt-2 max-w-3xl text-sm font-semibold leading-6 text-zinc-500">
                Tes préférences, ton compte Clash lié, tes statistiques et ton historique local restent associés à cette licence sur ce PC.
                Si une autre licence est utilisée plus tard, ClashGO charge son propre espace sans mélanger les données.
              </p>
              <div className="mt-4 flex flex-wrap gap-2">
                {['Préférences', 'Compte Clash', 'Statistiques', 'Historique'].map((label) => (
                  <span key={label} className="rounded-full bg-zinc-100 dark:bg-zinc-800 px-3 py-1.5 text-[9px] font-black uppercase tracking-wider text-zinc-500">
                    {label}
                  </span>
                ))}
              </div>
            </div>
          </div>
        </section>
      )}

      {memberPage === 'account' && (
        <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6 shadow-premium dark:shadow-none">
          <div className="flex flex-col lg:flex-row lg:items-start lg:justify-between gap-5">
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Multi-comptes</div>
              <h3 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">Rotation automatique Supercell ID</h3>
              <p className="mt-2 text-sm font-semibold text-zinc-500 max-w-2xl">
                Chaque compte conserve sa propre mémoire IA. ClashGO change de compte uniquement depuis le village et seulement après une calibration valide.
              </p>
            </div>
            <label className="flex items-center gap-3 rounded-2xl border border-zinc-200 dark:border-zinc-700 px-4 py-3">
              <input
                type="checkbox"
                checked={multiAccount.enabled}
                disabled={automationActive || multiAccountBusy}
                onChange={(e) => setMultiAccount(current => ({ ...current, enabled: e.target.checked }))}
                className="h-4 w-4"
              />
              <span className="text-[10px] font-black uppercase tracking-widest text-zinc-600 dark:text-zinc-300">Activer</span>
            </label>
          </div>

          <div className="mt-5 grid gap-3 md:grid-cols-[220px_1fr]">
            <label className="rounded-2xl border border-zinc-100 dark:border-zinc-800 p-4">
              <span className="text-[9px] font-black uppercase tracking-widest text-zinc-400">Attaques / tour par défaut</span>
              <input
                type="number"
                min={1}
                max={100}
                value={multiAccount.default_attacks_per_turn}
                disabled={automationActive || multiAccountBusy}
                onChange={(e) => setMultiAccount(current => ({ ...current, default_attacks_per_turn: Number(e.target.value || 1) }))}
                className="mt-2 h-10 w-full rounded-xl border border-zinc-200 dark:border-zinc-700 bg-transparent px-3 text-sm font-black outline-none"
              />
            </label>
            <div className={
              'rounded-2xl border p-4 ' +
              (multiAccountCalibrated
                ? 'border-emerald-500/20 bg-emerald-500/5'
                : 'border-amber-500/20 bg-amber-500/5')
            }>
              <div className="flex items-center gap-2">
                <span className="material-symbols-outlined text-base">
                  {multiAccountCalibrated ? 'verified' : 'warning'}
                </span>
                <span className={
                  'text-[10px] font-black uppercase tracking-widest ' +
                  (multiAccountCalibrated ? 'text-emerald-500' : 'text-amber-500')
                }>
                  {multiAccountCalibrated ? 'Sélecteur calibré' : 'Calibration Supercell ID requise'}
                </span>
              </div>
              <p className="mt-1 text-xs font-semibold text-zinc-500">
                Sans calibration, ClashGO n’effectuera aucun clic de changement de compte : la rotation sera simplement différée.
              </p>
              {multiAccountStatus && (
                <div className="mt-3 flex flex-wrap gap-2 text-[9px] font-black uppercase tracking-wider">
                  <span className="rounded-full bg-zinc-100 dark:bg-zinc-800 px-2.5 py-1 text-zinc-500">
                    Actif · {multiAccountStatus.active_account_label || multiAccountStatus.active_account_id || '—'}
                  </span>
                  <span className="rounded-full bg-zinc-100 dark:bg-zinc-800 px-2.5 py-1 text-zinc-500">
                    Tour · {multiAccountStatus.attacks_this_turn || 0} attaques
                  </span>
                  <span className="rounded-full bg-zinc-100 dark:bg-zinc-800 px-2.5 py-1 text-zinc-500">
                    Switchs · {multiAccountStatus.total_switches || 0}
                  </span>
                  {multiAccountStatus.switch_in_flight && (
                    <span className="rounded-full bg-violet-500/10 px-2.5 py-1 text-violet-500">
                      Changement en cours
                    </span>
                  )}
                  {multiAccountStatus.rotation_due && !multiAccountStatus.recovery_required && (
                    <span className="rounded-full bg-sky-500/10 px-2.5 py-1 text-sky-500">
                      Prochain · {multiAccountStatus.next_account_label || multiAccountStatus.next_account_id || '—'}
                    </span>
                  )}
                </div>
              )}

              {multiAccountStatus?.recovery_required && (
                <div className="mt-3 rounded-2xl border border-rose-500/20 bg-rose-500/10 p-3">
                  <div className="flex items-center gap-2 text-rose-600 dark:text-rose-400">
                    <span className="material-symbols-outlined text-base">emergency_home</span>
                    <span className="text-[10px] font-black uppercase tracking-widest">
                      Vérification du compte requise
                    </span>
                  </div>
                  <p className="mt-2 text-[11px] font-semibold leading-relaxed text-zinc-600 dark:text-zinc-300">
                    Un changement de compte a été interrompu avant confirmation. ClashGO refuse maintenant de démarrer pour éviter d’utiliser l’IA, les remparts ou l’historique du mauvais compte. Regarde quel village est réellement affiché dans BlueStacks puis confirme-le ci-dessous, bot arrêté.
                  </p>
                  <div className="mt-3 flex flex-wrap gap-2">
                    {multiAccount.accounts.filter(account => account.enabled).map(account => (
                      <button
                        key={account.id}
                        type="button"
                        disabled={multiAccountRecoveryBusy}
                        onClick={() => void confirmMultiAccountRecovery(account)}
                        className="rounded-xl border border-rose-500/20 bg-white/70 dark:bg-zinc-950/50 px-3 py-2 text-[9px] font-black uppercase tracking-wider text-rose-600 dark:text-rose-400 disabled:opacity-40"
                      >
                        Confirmer · {account.label || account.player_tag}
                      </button>
                    ))}
                  </div>
                </div>
              )}

              {multiAccountStatus?.last_error && !multiAccountStatus.recovery_required && (
                <div className="mt-3 rounded-xl bg-amber-500/10 px-3 py-2 text-[10px] font-bold text-amber-600 dark:text-amber-400">
                  Dernier switch différé : {multiAccountStatus.last_error}
                </div>
              )}
            </div>
          </div>

          <MultiAccountCalibration
            accounts={multiAccount.accounts}
            disabled={automationActive || multiAccountBusy}
            calibrated={multiAccountCalibrated}
            onSaved={() => {
              setMultiAccountCalibrated(true);
              setMultiAccountMessage('Calibration Supercell ID enregistrée. La rotation peut maintenant effectuer des changements de compte vérifiés.');
            }}
          />

          <div className="mt-5 space-y-3">
            {multiAccount.accounts.map((account, index) => (
              <div
                key={account.id || index}
                className={
                  'rounded-2xl border p-4 ' +
                  (multiAccount.active_account_id === account.id
                    ? 'border-sky-400/40 bg-sky-500/5'
                    : 'border-zinc-100 dark:border-zinc-800')
                }
              >
                <div className="grid gap-3 lg:grid-cols-[44px_1.2fr_1.3fr_90px_90px_120px_auto] lg:items-end">
                  <label className="flex h-10 items-center justify-center rounded-xl border border-zinc-200 dark:border-zinc-700">
                    <input
                      type="checkbox"
                      checked={account.enabled}
                      disabled={automationActive || multiAccountBusy}
                      onChange={(e) => updateMultiAccount(index, { enabled: e.target.checked })}
                      title="Compte actif dans la rotation"
                    />
                  </label>
                  <label>
                    <span className="text-[8px] font-black uppercase tracking-widest text-zinc-400">Nom</span>
                    <input
                      value={account.label || ''}
                      disabled={automationActive || multiAccountBusy}
                      onChange={(e) => updateMultiAccount(index, { label: e.target.value })}
                      className="mt-1 h-10 w-full rounded-xl border border-zinc-200 dark:border-zinc-700 bg-transparent px-3 text-xs font-bold outline-none"
                      placeholder={`Compte ${index + 1}`}
                    />
                  </label>
                  <label>
                    <span className="text-[8px] font-black uppercase tracking-widest text-zinc-400">PlayerTag</span>
                    <input
                      value={account.player_tag || ''}
                      disabled={automationActive || multiAccountBusy}
                      onChange={(e) => updateMultiAccount(index, { player_tag: e.target.value.toUpperCase() })}
                      className="mt-1 h-10 w-full rounded-xl border border-zinc-200 dark:border-zinc-700 bg-transparent px-3 text-xs font-mono font-bold outline-none"
                      placeholder="#2ABC123"
                    />
                  </label>
                  <label>
                    <span className="text-[8px] font-black uppercase tracking-widest text-zinc-400">Slot</span>
                    <input
                      type="number"
                      min={1}
                      max={20}
                      value={account.switch_slot || ''}
                      disabled={automationActive || multiAccountBusy}
                      onChange={(e) => updateMultiAccount(index, { switch_slot: Number(e.target.value || 0) })}
                      className="mt-1 h-10 w-full rounded-xl border border-zinc-200 dark:border-zinc-700 bg-transparent px-3 text-xs font-bold outline-none"
                    />
                  </label>
                  <label>
                    <span className="text-[8px] font-black uppercase tracking-widest text-zinc-400">HDV</span>
                    <input
                      type="number"
                      min={8}
                      max={18}
                      value={account.town_hall || ''}
                      disabled={automationActive || multiAccountBusy}
                      onChange={(e) => updateMultiAccount(index, { town_hall: Number(e.target.value || 0) })}
                      className="mt-1 h-10 w-full rounded-xl border border-zinc-200 dark:border-zinc-700 bg-transparent px-3 text-xs font-bold outline-none"
                    />
                  </label>
                  <label>
                    <span className="text-[8px] font-black uppercase tracking-widest text-zinc-400">Attaques / tour</span>
                    <input
                      type="number"
                      min={0}
                      max={100}
                      value={account.max_attacks_per_turn || 0}
                      disabled={automationActive || multiAccountBusy}
                      onChange={(e) => updateMultiAccount(index, { max_attacks_per_turn: Number(e.target.value || 0) })}
                      className="mt-1 h-10 w-full rounded-xl border border-zinc-200 dark:border-zinc-700 bg-transparent px-3 text-xs font-bold outline-none"
                      title="0 = valeur par défaut"
                    />
                  </label>
                  <div className="flex gap-2">
                    <div
                      className={
                        'flex h-10 items-center rounded-xl px-3 text-[9px] font-black uppercase tracking-wider ' +
                        (multiAccount.active_account_id === account.id
                          ? 'bg-sky-500 text-white'
                          : 'border border-zinc-200 dark:border-zinc-700 text-zinc-400')
                      }
                      title={multiAccount.active_account_id === account.id ? 'Compte actuellement chargé' : 'Compte de la rotation'}
                    >
                      {multiAccount.active_account_id === account.id ? 'Actuel' : 'Rotation'}
                    </div>
                    <button
                      type="button"
                      disabled={automationActive || multiAccountBusy}
                      onClick={() => removeMultiAccount(index)}
                      className="h-10 rounded-xl border border-rose-500/20 px-3 text-rose-500"
                      title="Supprimer"
                    >
                      <span className="material-symbols-outlined text-base">delete</span>
                    </button>
                  </div>
                </div>
              </div>
            ))}
            {multiAccount.accounts.length === 0 && (
              <div className="rounded-2xl border border-dashed border-zinc-200 dark:border-zinc-700 p-6 text-center text-xs font-semibold text-zinc-500">
                Ajoute au moins deux comptes pour activer la rotation.
              </div>
            )}
          </div>

          <div className="mt-5 flex flex-wrap items-center justify-between gap-3">
            <button
              type="button"
              onClick={addMultiAccount}
              disabled={automationActive || multiAccountBusy || multiAccount.accounts.length >= 20}
              className="rounded-xl border border-zinc-200 dark:border-zinc-700 px-4 py-2.5 text-[10px] font-black uppercase tracking-widest text-zinc-600 dark:text-zinc-300 disabled:opacity-40"
            >
              + Ajouter un compte
            </button>
            <button
              type="button"
              onClick={() => void saveMultiAccounts()}
              disabled={automationActive || multiAccountBusy}
              className="rounded-xl bg-zinc-950 dark:bg-white px-5 py-2.5 text-[10px] font-black uppercase tracking-widest text-white dark:text-zinc-950 disabled:opacity-40"
            >
              {multiAccountBusy ? 'Enregistrement…' : 'Enregistrer le multi-compte'}
            </button>
          </div>
          {automationActive && (
            <div className="mt-3 text-[10px] font-bold text-amber-500">Arrête ClashGO avant de modifier la rotation des comptes.</div>
          )}
          {multiAccountMessage && (
            <div className="mt-3 rounded-xl bg-emerald-500/10 px-4 py-3 text-[11px] font-bold text-emerald-500">{multiAccountMessage}</div>
          )}
          {multiAccountError && (
            <div className="mt-3 rounded-xl bg-rose-500/10 px-4 py-3 text-[11px] font-bold text-rose-500">{multiAccountError}</div>
          )}
        </section>
      )}

      {memberPage === 'account' && (
        <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6 shadow-premium dark:shadow-none">
          <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-5">
            <div className="min-w-0">
              <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Version ClashGO</div>
              <div className="mt-2 flex flex-wrap items-center gap-2">
                <h3 className="text-xl font-black text-zinc-950 dark:text-white">
                  {appVersion ? 'v' + appVersion : 'Version actuelle'}
                </h3>
                <span className={
                  'rounded-full px-2.5 py-1 text-[8px] font-black uppercase tracking-widest ' +
                  (memberUpdate?.state === 'error'
                    ? 'bg-rose-500/10 text-rose-500'
                    : memberUpdate?.state === 'downloading' || memberUpdate?.state === 'restarting'
                      ? 'bg-sky-500/10 text-sky-500'
                      : memberUpdate?.available
                        ? 'bg-amber-500/10 text-amber-500'
                        : 'bg-emerald-500/10 text-emerald-500')
                }>
                  {memberUpdate?.state === 'downloading'
                    ? `Téléchargement ${Math.round((memberUpdate.progress || 0) * 100)}%`
                    : memberUpdate?.state === 'ready'
                      ? 'Prête à installer'
                      : memberUpdate?.state === 'restarting'
                        ? 'Redémarrage…'
                        : memberUpdate?.available
                          ? 'Mise à jour disponible'
                          : memberUpdate?.state === 'error'
                            ? 'Vérification impossible'
                            : 'À jour'}
                </span>
              </div>
              <p className="mt-2 text-xs font-semibold text-zinc-500">
                {memberUpdate?.available && memberUpdate.latest_version
                  ? 'Nouvelle version : v' + memberUpdate.latest_version + '. Elle pourra être installée avec le système de mise à jour ClashGO.'
                  : memberUpdate?.error
                    ? 'ClashGO réessaiera automatiquement plus tard.'
                    : 'Les mises à jour beta sont vérifiées automatiquement.'}
              </p>
            </div>
            <div className="flex shrink-0 flex-wrap gap-2">
              {(memberUpdate?.available || memberUpdate?.state === 'ready') && (
                <button
                  type="button"
                  disabled={installingUpdate || checkingUpdate || memberUpdate?.state === 'downloading' || memberUpdate?.state === 'restarting'}
                  onClick={() => void installMemberUpdate()}
                  className="rounded-xl bg-zinc-950 dark:bg-white px-4 py-3 text-[10px] font-black uppercase tracking-widest text-white dark:text-zinc-950 disabled:opacity-40"
                >
                  {installingUpdate ? 'Préparation…' : 'Installer et redémarrer'}
                </button>
              )}
              <button
                type="button"
                disabled={checkingUpdate || installingUpdate}
                onClick={() => void checkMemberUpdate()}
                className="rounded-xl border border-zinc-200 dark:border-zinc-700 px-4 py-3 text-[10px] font-black uppercase tracking-widest text-zinc-500 hover:text-zinc-950 dark:hover:text-white disabled:opacity-40"
              >
                {checkingUpdate ? 'Vérification…' : 'Vérifier maintenant'}
              </button>
            </div>
          </div>
        </section>
      )}

      {memberPage === 'settings' && (licenseState?.activated || licensePolicy?.enforced === false) && memberSettings && (
        <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6 shadow-premium dark:shadow-none">
          <div className="flex flex-col gap-6">
            {testSessionActive && (
              <div className="rounded-2xl border border-sky-400/30 bg-sky-400/10 px-4 py-3 text-xs font-bold text-sky-600 dark:text-sky-300">
                Session test en cours · les réglages personnels sont verrouillés et seront restaurés automatiquement à la fin.
              </div>
            )}
            <div>
              <div className="flex flex-wrap items-center gap-2">
                <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Mon ClashGO</div>
                {!licenseState?.activated && licensePolicy?.enforced === false && (
                  <span className="rounded-full bg-amber-500/10 px-2 py-1 text-[8px] font-black uppercase tracking-widest text-amber-500">
                    Mode bêta local
                  </span>
                )}
              </div>
              <div className="mt-1 flex flex-wrap items-center gap-3">
                <h3 className="text-xl font-black text-zinc-950 dark:text-white">Réglages membre</h3>
                <span className={
                  'inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[8px] font-black uppercase tracking-widest ' +
                  (memberSaving
                    ? 'bg-amber-500/10 text-amber-500'
                    : 'bg-emerald-500/10 text-emerald-500')
                }>
                  <span className="material-symbols-outlined text-[12px]">
                    {memberSaving ? 'sync' : 'check_circle'}
                  </span>
                  {memberSaving ? 'Enregistrement…' : 'Enregistré'}
                </span>
              </div>
              <p className="mt-2 text-sm font-semibold text-zinc-500 max-w-2xl">
                Ces réglages agissent réellement sur le bot et sont appliqués sans redémarrage. Les contrôles de sécurité restent actifs, même en mode Rapide.
              </p>
            </div>
            <div className="flex flex-wrap justify-end gap-2">
              <button
                type="button"
                disabled={!memberUndoAvailable || memberUndoBusy || memberSaving || testSessionActive}
                onClick={() => void undoMemberSettings()}
                className="rounded-xl border border-zinc-200 dark:border-zinc-700 px-4 py-2.5 text-[10px] font-black uppercase tracking-widest text-zinc-500 hover:text-zinc-950 dark:hover:text-white disabled:opacity-30"
                title={memberUndoAvailable ? 'Revenir à la configuration précédente' : 'Aucune modification précédente disponible'}
              >
                {memberUndoBusy ? 'Restauration…' : 'Annuler la dernière modification'}
              </button>
              <button
                type="button"
                disabled={memberSaving || testSessionActive}
                onClick={() => {
                  const next = {
                    ...applySpeedPreset(memberSettings, 'normal'),
                    adaptive_search: true,
                    auto_profile_sync: true,
                    auto_army_guard: true,
                    auto_resource_tracking: true,
                    max_attacks_per_session: 100,
                  };
                  setMemberSettings(next);
                  void saveMemberSettings(next);
                }}
                className="rounded-xl border border-zinc-200 dark:border-zinc-700 px-4 py-2.5 text-[10px] font-black uppercase tracking-widest text-zinc-500 hover:text-zinc-950 dark:hover:text-white disabled:opacity-40"
              >
                Restaurer la cadence recommandée
              </button>
            </div>

            <div>
              <div className="flex items-end justify-between gap-4 mb-3">
                <div>
                  <div className="text-[10px] font-black uppercase tracking-[0.18em] text-zinc-400">Profils rapides</div>
                  <div className="mt-1 text-xs font-semibold text-zinc-500">
                    {activeUsagePreset(memberSettings)
                      ? 'Le profil actif est mis en évidence.'
                      : 'Configuration personnalisée · choisis un profil pour revenir à un preset cohérent.'}
                  </div>
                </div>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                {([
                  ['short', 'Session courte', '10 attaques · rythme normal', 'timer'],
                  ['balanced', 'Farm équilibré', '50 attaques · recommandé', 'balance'],
                  ['fast', 'Farm rapide', '100 attaques · rythme rapide', 'bolt'],
                ] as const).map(([preset, label, description, icon]) => (
                  <button
                    key={preset}
                    type="button"
                    disabled={memberSaving || testSessionActive}
                    onClick={() => void applyUsagePreset(preset)}
                    className={
                      'group rounded-2xl border p-4 text-left transition hover:-translate-y-0.5 disabled:opacity-40 ' +
                      (activeUsagePreset(memberSettings) === preset
                        ? 'border-zinc-950 bg-zinc-950 text-white dark:border-white dark:bg-white dark:text-zinc-950'
                        : 'border-zinc-200 bg-zinc-50 text-zinc-950 hover:border-zinc-400 dark:border-zinc-800 dark:bg-zinc-950 dark:text-white dark:hover:border-zinc-600')
                    }
                  >
                    <div className="flex items-center justify-between gap-3">
                      <span className={
                        'material-symbols-outlined text-lg ' +
                        (activeUsagePreset(memberSettings) === preset ? 'opacity-80' : 'text-zinc-400 group-hover:text-zinc-950 dark:group-hover:text-white')
                      }>{icon}</span>
                      <span className={
                        'text-[8px] font-black uppercase tracking-[0.16em] ' +
                        (activeUsagePreset(memberSettings) === preset ? 'opacity-70' : 'text-zinc-400')
                      }>
                        {activeUsagePreset(memberSettings) === preset ? 'Actif' : '1 clic'}
                      </span>
                    </div>
                    <div className="mt-4 text-sm font-black text-zinc-950 dark:text-white">{label}</div>
                    <div className="mt-1 text-[11px] font-semibold text-zinc-500">{description}</div>
                  </button>
                ))}
              </div>
            </div>

            <div>
              <div className="flex items-end justify-between gap-4 mb-3">
                <div>
                  <div className="text-[10px] font-black uppercase tracking-[0.18em] text-zinc-400">Mes profils</div>
                  <div className="mt-1 text-xs font-semibold text-zinc-500">3 emplacements personnels liés à ta licence.</div>
                </div>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                {[1, 2, 3].map((slot) => {
                  const preset = memberPresets.find((item) => item.slot === slot);
                  return (
                    <div key={slot} className="rounded-2xl border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950 p-4">
                      <div className="flex items-center justify-between gap-2">
                        <span className="text-[9px] font-black uppercase tracking-[0.18em] text-zinc-400">Emplacement {slot}</span>
                        {preset && <span className="material-symbols-outlined text-base text-emerald-500">bookmark_added</span>}
                      </div>
                      <input
                        value={memberPresetNames[slot] ?? preset?.name ?? `Profil ${slot}`}
                        onChange={(e) => setMemberPresetNames((current) => ({ ...current, [slot]: e.target.value }))}
                        maxLength={32}
                        disabled={testSessionActive}
                        className="mt-3 w-full rounded-xl border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm font-black text-zinc-950 dark:text-white outline-none focus:border-zinc-400"
                      />
                      {preset && (
                        <div className="mt-3 rounded-xl border border-zinc-200/70 dark:border-zinc-800 bg-white/70 dark:bg-zinc-900/70 px-3 py-2 text-[10px] font-bold text-zinc-500">
                          {preset.settings.speed_profile === 'fast'
                            ? 'Rapide'
                            : preset.settings.speed_profile === 'cautious'
                              ? 'Prudente'
                              : 'Normale'}
                          {' · '}{preset.settings.max_attacks_per_session} / session
                          {' · '}{preset.settings.max_attacks_per_hour} / h
                        </div>
                      )}
                      <div className="mt-3 flex flex-wrap gap-2">
                        <button
                          type="button"
                          disabled={memberPresetBusy !== null || memberSaving || testSessionActive}
                          onClick={() => void savePersonalPreset(slot)}
                          className="rounded-lg bg-zinc-950 dark:bg-white px-3 py-2 text-[9px] font-black uppercase tracking-wider text-white dark:text-zinc-950 disabled:opacity-30"
                        >
                          {memberPresetBusy === slot ? '…' : preset ? 'Mettre à jour' : 'Enregistrer'}
                        </button>
                        {preset && (
                          <>
                            <button
                              type="button"
                              disabled={memberPresetBusy !== null || memberSaving || testSessionActive}
                              onClick={() => void applyPersonalPreset(slot)}
                              className="rounded-lg border border-zinc-200 dark:border-zinc-700 px-3 py-2 text-[9px] font-black uppercase tracking-wider text-zinc-500 hover:text-zinc-950 dark:hover:text-white disabled:opacity-30"
                            >
                              Appliquer
                            </button>
                            <button
                              type="button"
                              disabled={memberPresetBusy !== null || memberSaving || testSessionActive}
                              onClick={() => {
                                if (memberPresetDeleteConfirm === slot) {
                                  void deletePersonalPreset(slot);
                                } else {
                                  setMemberPresetDeleteConfirm(slot);
                                }
                              }}
                              className={
                                'rounded-lg border px-3 py-2 text-[9px] font-black uppercase tracking-wider disabled:opacity-30 ' +
                                (memberPresetDeleteConfirm === slot
                                  ? 'border-rose-500 bg-rose-500 text-white'
                                  : 'border-rose-200 dark:border-rose-900/50 text-rose-500')
                              }
                            >
                              {memberPresetDeleteConfirm === slot ? 'Confirmer' : 'Supprimer'}
                            </button>
                          </>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>

            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.18em] text-zinc-400 mb-3">Vitesse du bot</div>
              <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                {([
                  ['cautious', 'Prudente', '8 attaques/h · 45 s minimum entre attaques'],
                  ['normal', 'Normale', '12 attaques/h · 30 s minimum entre attaques'],
                  ['fast', 'Rapide', '16 attaques/h · 20 s minimum entre attaques'],
                ] as const).map(([value, label, description]) => (
                  <button
                    key={value}
                    type="button"
                    disabled={memberSaving || testSessionActive}
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

            <div className="rounded-2xl border border-zinc-100 dark:border-zinc-800 bg-zinc-50/60 dark:bg-zinc-950/30 px-4 py-3">
              <div className="flex flex-wrap items-center gap-x-5 gap-y-2 text-[10px] font-black uppercase tracking-widest text-zinc-500">
                <span>
                  Profil {memberSettings.speed_profile === 'cautious' ? 'Prudent' : memberSettings.speed_profile === 'fast' ? 'Rapide' : 'Normal'}
                  {isCustomPacing(memberSettings) ? ' · personnalisé' : ''}
                </span>
                <span>{memberSettings.max_attacks_per_hour} attaques/h max</span>
                <span>{memberSettings.max_attacks_per_session} attaques / session</span>
                <span>
                  {memberSettings.break_every_attacks > 0
                    ? `Pause ${memberSettings.break_minutes} min / ${memberSettings.break_every_attacks} attaques`
                    : 'Pauses planifiées désactivées'}
                </span>
              </div>
            </div>

            <details className="group rounded-2xl border border-zinc-100 dark:border-zinc-800 overflow-hidden">
              <summary className="cursor-pointer list-none flex items-center justify-between gap-4 px-5 py-4 bg-zinc-50/50 dark:bg-zinc-950/30">
                <div>
                  <div className="text-sm font-black text-zinc-950 dark:text-white">Réglages avancés de cadence</div>
                  <div className="mt-1 text-[11px] font-semibold text-zinc-500">À modifier seulement si tu veux affiner le preset choisi.</div>
                </div>
                <span className="material-symbols-outlined text-zinc-400 transition-transform group-open:rotate-180">expand_more</span>
              </summary>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4 p-4">
                <label className="rounded-2xl border border-zinc-100 dark:border-zinc-800 p-4">
                  <div className="flex items-center justify-between gap-4">
                    <div>
                      <div className="text-sm font-black text-zinc-950 dark:text-white">Attaques / heure</div>
                      <div className="text-[11px] font-semibold text-zinc-500">Limite de sécurité du cycle automatique</div>
                    </div>
                    <input
                      type="number"
                      disabled={memberSaving || testSessionActive}
                      min={1}
                      max={24}
                      value={memberSettings.max_attacks_per_hour}
                      onChange={(e) => setMemberSettings({ ...memberSettings, max_attacks_per_hour: Math.max(1, Math.min(24, Number(e.target.value) || 1)) })}
                      onBlur={() => void saveMemberSettings(memberSettings)}
                      className="w-20 rounded-xl border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-3 py-2 text-center text-sm font-black"
                    />
                  </div>
                </label>

                <label className="rounded-2xl border border-zinc-100 dark:border-zinc-800 p-4">
                  <div className="flex items-center justify-between gap-4">
                    <div>
                      <div className="text-sm font-black text-zinc-950 dark:text-white">Attaques / session</div>
                      <div className="text-[11px] font-semibold text-zinc-500">Arrêt propre du bot après cette limite</div>
                    </div>
                    <input
                      type="number"
                      disabled={memberSaving || testSessionActive}
                      min={1}
                      max={500}
                      value={memberSettings.max_attacks_per_session}
                      onChange={(e) => setMemberSettings({
                        ...memberSettings,
                        max_attacks_per_session: Math.max(1, Math.min(500, Number(e.target.value) || 1)),
                      })}
                      onBlur={() => void saveMemberSettings(memberSettings)}
                      className="w-20 rounded-xl border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-3 py-2 text-center text-sm font-black"
                    />
                  </div>
                  <div className="mt-3 flex flex-wrap gap-2">
                    {[10, 25, 50, 100].map((limit) => (
                      <button
                        key={limit}
                        type="button"
                        disabled={memberSaving || testSessionActive}
                        onClick={() => {
                          const next = { ...memberSettings, max_attacks_per_session: limit };
                          setMemberSettings(next);
                          void saveMemberSettings(next);
                        }}
                        className={
                          'rounded-lg px-2.5 py-1.5 text-[9px] font-black uppercase tracking-wider transition ' +
                          (memberSettings.max_attacks_per_session === limit
                            ? 'bg-zinc-950 text-white dark:bg-white dark:text-zinc-950'
                            : 'bg-zinc-100 text-zinc-500 hover:text-zinc-950 dark:bg-zinc-800 dark:hover:text-white')
                        }
                      >
                        {limit}
                      </button>
                    ))}
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
                      disabled={memberSaving || testSessionActive}
                      min={0}
                      max={20}
                      value={memberSettings.break_every_attacks}
                      onChange={(e) => setMemberSettings({ ...memberSettings, break_every_attacks: Math.max(0, Math.min(20, Number(e.target.value) || 0)) })}
                      onBlur={() => void saveMemberSettings(memberSettings)}
                      className="w-20 rounded-xl border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-3 py-2 text-center text-sm font-black"
                    />
                  </div>
                </label>

                <label className="rounded-2xl border border-zinc-100 dark:border-zinc-800 p-4 md:col-span-2">
                  <div className="flex items-center justify-between gap-4">
                    <div>
                      <div className="text-sm font-black text-zinc-950 dark:text-white">Durée de pause</div>
                      <div className="text-[11px] font-semibold text-zinc-500">Minutes de repos automatique</div>
                    </div>
                    <input
                      type="number"
                      disabled={memberSaving || testSessionActive}
                      min={0}
                      max={30}
                      value={memberSettings.break_minutes}
                      onChange={(e) => setMemberSettings({ ...memberSettings, break_minutes: Math.max(0, Math.min(30, Number(e.target.value) || 0)) })}
                      onBlur={() => void saveMemberSettings(memberSettings)}
                      className="w-20 rounded-xl border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-3 py-2 text-center text-sm font-black"
                    />
                  </div>
                </label>
              </div>
            </details>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <button
                type="button"
                disabled={memberSaving || testSessionActive}
                onClick={() => {
                  const next = { ...memberSettings, adaptive_search: !memberSettings.adaptive_search };
                  setMemberSettings(next);
                  void saveMemberSettings(next);
                }}
                className="flex items-center justify-between gap-4 rounded-2xl border border-zinc-100 dark:border-zinc-800 p-4 text-left"
              >
                <div>
                  <div className="text-sm font-black text-zinc-950 dark:text-white">Recherche adaptative</div>
                  <div className="mt-1 text-[11px] font-semibold text-zinc-500">
                    Relâche progressivement les seuils après plusieurs villages ignorés.
                  </div>
                </div>
                <span className={
                  'shrink-0 w-10 h-6 rounded-full p-1 transition ' +
                  (memberSettings.adaptive_search ? 'bg-emerald-500' : 'bg-zinc-300 dark:bg-zinc-700')
                }>
                  <span className={
                    'block w-4 h-4 rounded-full bg-white transition-transform ' +
                    (memberSettings.adaptive_search ? 'translate-x-4' : '')
                  } />
                </span>
              </button>

              <div className="rounded-2xl border border-zinc-100 dark:border-zinc-800 p-4">
                <div className="text-sm font-black text-zinc-950 dark:text-white">Automatismes actifs</div>
                <div className="mt-3 flex flex-wrap gap-2">
                  {[
                    ['Profil', memberSettings.auto_profile_sync],
                    ['Armée', memberSettings.auto_army_guard],
                    ['Ressources', memberSettings.auto_resource_tracking],
                  ].map(([label, enabled]) => (
                    <span
                      key={String(label)}
                      className={
                        'rounded-full px-2.5 py-1 text-[9px] font-black uppercase tracking-wider ' +
                        (enabled
                          ? 'bg-emerald-500/10 text-emerald-500'
                          : 'bg-zinc-100 text-zinc-400 dark:bg-zinc-800')
                      }
                    >
                      {label} · {enabled ? 'AUTO' : 'OFF'}
                    </span>
                  ))}
                </div>
                <div className="mt-2 text-[11px] font-semibold text-zinc-500">
                  Ces automatismes suivent le mode automatique ClashGO et ne nécessitent pas de réglage manuel ici.
                </div>
              </div>
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
                {level === 'simple' ? 'Simple' : 'Avancé'}
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
            <div className="text-[10px] font-black uppercase tracking-[0.25em] text-zinc-400">{playerTag ? 'Compte Clash lié' : 'Compte Clash optionnel'}</div>
            <div className="mt-2 flex flex-wrap items-baseline gap-3">
              <h3 className="text-3xl font-black text-zinc-950 dark:text-white">{profile?.name || playerTag || 'Aucun compte'}</h3>
              <span className="text-sm font-mono font-bold text-zinc-500">{profile?.tag || playerTag}</span>
            </div>
            <p className="mt-2 text-sm font-medium text-zinc-500">
              {profile
                ? 'Compte synchronisé automatiquement par ClashGO. Aucune clé API développeur n’est nécessaire.'
                : playerTag
                  ? 'Tag joueur enregistré. ClashGO synchronisera ce compte automatiquement dès que le service sera disponible.'
                  : 'Tu peux lier ton tag joueur pour sélectionner automatiquement le profil HDV. ClashGO reste utilisable sans cette liaison.'}
            </p>
          </div>
          {playerTag && (
          <details className="relative">
            <summary className="list-none cursor-pointer px-4 py-3 rounded-xl border border-zinc-200 dark:border-zinc-700 text-xs font-black text-zinc-500 select-none">
              Options du compte
            </summary>
            <div className="absolute right-0 mt-2 z-20 min-w-[190px] rounded-xl border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-900 p-2 shadow-xl">
              {!confirmUnlink ? (
                <button
                  type="button"
                  onClick={() => setConfirmUnlink(true)}
                  disabled={busy || automationActive}
                  className="w-full px-3 py-2.5 rounded-lg text-left text-xs font-black text-rose-500 hover:bg-rose-500/5 disabled:opacity-40"
                >
                  Délier le compte
                </button>
              ) : (
                <div className="space-y-2 p-1">
                  <div className="text-[10px] font-bold leading-relaxed text-zinc-500">
                    Le profil Clash devra être lié à nouveau avec son tag.
                  </div>
                  <div className="flex gap-2">
                    <button
                      type="button"
                      onClick={() => void unlink()}
                      disabled={busy || automationActive}
                      className="flex-1 rounded-lg bg-rose-500 px-2 py-2 text-[9px] font-black uppercase tracking-wider text-white disabled:opacity-40"
                    >
                      {busy ? 'Déliage…' : 'Confirmer'}
                    </button>
                    <button
                      type="button"
                      onClick={() => setConfirmUnlink(false)}
                      disabled={busy || automationActive}
                      className="rounded-lg border border-zinc-200 dark:border-zinc-700 px-2 py-2 text-[9px] font-black uppercase tracking-wider text-zinc-500 disabled:opacity-40"
                    >
                      Annuler
                    </button>
                  </div>
                </div>
              )}
            </div>
          </details>
          )}
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
                    : 'Service de compte non configuré · optionnel'}
          </div>
        </div>

        {(message || error) && (
          <div
            title={error || undefined}
            className={'mt-4 rounded-xl px-4 py-3 text-xs font-bold ' + (error ? 'bg-amber-500/10 text-amber-600 dark:text-amber-400' : 'bg-emerald-500/10 text-emerald-500')}
          >
            {error
              ? 'La synchronisation du profil est temporairement indisponible. Le bot reste utilisable avec la configuration locale.'
              : message}
          </div>
        )}
      </section>

      {memberPage === 'village' && !profile && (
        <section className="rounded-[2rem] border border-dashed border-zinc-200 dark:border-zinc-800 bg-white/60 dark:bg-zinc-900/60 p-8 text-center">
          <div className="mx-auto grid h-12 w-12 place-items-center rounded-2xl bg-zinc-100 dark:bg-zinc-800 text-zinc-400">
            <span className="material-symbols-outlined">sync</span>
          </div>
          <h4 className="mt-4 text-lg font-black text-zinc-950 dark:text-white">{playerTag ? 'Profil en attente de synchronisation' : 'Compte Clash non lié'}</h4>
          <p className="mx-auto mt-2 max-w-lg text-sm font-semibold text-zinc-500">
            {playerTag
              ? 'ClashGO récupérera automatiquement les informations du village dès que le service de compte sera disponible.'
              : 'Cette liaison est facultative. Le bot peut utiliser la configuration locale et tu pourras ajouter ton tag plus tard.'}
          </p>
          {!playerTag && (
            <div className="mx-auto mt-6 max-w-xl text-left">
              <label className="text-[9px] font-black uppercase tracking-[0.18em] text-zinc-400">Tag joueur</label>
              <div className="mt-2 flex flex-col sm:flex-row gap-2">
                <input
                  value={accountTagInput}
                  onChange={(e) => {
                    setAccountTagInput(e.target.value.toUpperCase());
                    setAccountLinkMessage('');
                  }}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') void linkClashAccount();
                  }}
                  placeholder="#2ABC123XY"
                  autoComplete="off"
                  spellCheck={false}
                  className="min-w-0 flex-1 rounded-xl border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-950 px-4 py-3 font-mono text-sm font-black uppercase outline-none focus:border-zinc-400 dark:focus:border-zinc-500"
                />
                <button
                  type="button"
                  onClick={() => void linkClashAccount()}
                  disabled={accountLinkBusy || automationActive || !accountTagInput.trim()}
                  className="rounded-xl bg-zinc-950 dark:bg-white px-5 py-3 text-[10px] font-black uppercase tracking-widest text-white dark:text-zinc-950 disabled:opacity-40"
                >
                  {accountLinkBusy ? 'Enregistrement…' : 'Lier ce compte'}
                </button>
              </div>
              <div className="mt-2 text-[10px] font-semibold text-zinc-400">
                Le tag est visible dans ton profil Clash of Clans. Aucun mot de passe Supercell n’est demandé.
              </div>
              {automationActive && (
                <div className="mt-3 rounded-xl border border-amber-200 dark:border-amber-900/50 bg-amber-50 dark:bg-amber-950/20 px-4 py-3 text-[11px] font-bold text-amber-700 dark:text-amber-300">
                  Arrête ClashGO avant de changer ou délier le compte joueur.
                </div>
              )}
              {accountLinkMessage && (
                <div className="mt-3 rounded-xl bg-zinc-100 dark:bg-zinc-800 px-4 py-3 text-xs font-bold text-zinc-600 dark:text-zinc-300">
                  {accountLinkMessage}
                </div>
              )}
            </div>
          )}
        </section>
      )}

      {memberPage === 'village' && profile && (
        <>
          <section className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-6 gap-3">
            {[
              ['HDV', 'TH ' + profile.townHallLevel],
              ['XP', String(profile.expLevel)],
              ['Trophées', profile.trophies.toLocaleString()],
              ['Record', profile.bestTrophies.toLocaleString()],
              ['Étoiles de guerre', profile.warStars.toLocaleString()],
              ['Ligue', profile.league?.name || '—'],
            ].map(([label, value]) => (
              <div key={label} className="rounded-2xl border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-4">
                <div className="text-[9px] uppercase tracking-[0.18em] font-black text-zinc-400">{label}</div>
                <div className="mt-1 text-lg font-black text-zinc-950 dark:text-white">{value}</div>
              </div>
            ))}
          </section>

          {(!currentArmy || currentArmy.units.length === 0) && (
            <section className="rounded-[2rem] border border-dashed border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6">
              <div className="flex items-center gap-4">
                <div className="grid h-11 w-11 shrink-0 place-items-center rounded-2xl bg-zinc-100 dark:bg-zinc-800 text-zinc-400">
                  <span className="material-symbols-outlined">groups</span>
                </div>
                <div>
                  <h4 className="text-sm font-black text-zinc-950 dark:text-white">Armée pas encore détectée</h4>
                  <p className="mt-1 text-xs font-semibold text-zinc-500">
                    La composition apparaîtra ici automatiquement lors d’un prochain cycle de farm.
                  </p>
                </div>
              </div>
            </section>
          )}

          {currentArmy && currentArmy.units.length > 0 && (
            <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6">
              <div className="flex items-center justify-between gap-4 mb-5">
                <div>
                  <div className="text-[10px] font-black uppercase tracking-[0.2em] text-zinc-400">Dernière armée détectée</div>
                  <div className="mt-1 flex flex-wrap items-center gap-3">
                    <h4 className="text-xl font-black text-zinc-950 dark:text-white">Composition détectée</h4>
                    <span className={`px-2.5 py-1 rounded-full text-[9px] font-black uppercase tracking-wider border ${
                      currentArmy.uncertain
                        ? 'bg-amber-500/10 border-amber-500/20 text-amber-600 dark:text-amber-400'
                        : currentArmy.ready
                          ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-600 dark:text-emerald-400'
                          : 'bg-rose-500/10 border-rose-500/20 text-rose-600 dark:text-rose-400'
                    }`}>
                      {currentArmy.uncertain ? 'Lecture incertaine' : currentArmy.ready ? 'Armée prête' : 'Armée incomplète'}
                    </span>
                  </div>
                  <p className="mt-1 text-xs text-zinc-500">
                    Détectée automatiquement depuis la barre de troupes avant le déploiement.
                    {currentArmy.target_label ? ` Cible : ${currentArmy.target_label}.` : ''}
                    {!currentArmy.ready && !currentArmy.uncertain ? ' ClashGO passera la base avant tout déploiement.' : ''}
                    {currentArmy.uncertain ? ' En cas de doute OCR, ClashGO laisse l’attaque continuer.' : ''}
                  </p>
                </div>
                <div className="text-[10px] font-bold text-zinc-400">
                  Dernière détection · {snapshotAgeLabel(currentArmy.timestamp)}
                </div>
              </div>
              <div className="grid grid-cols-2 md:grid-cols-3 xl:grid-cols-5 gap-2">
                {currentArmy.units.map((unit, idx) => (
                  <div key={idx} className="rounded-xl bg-zinc-50 dark:bg-zinc-950/40 border border-zinc-100 dark:border-zinc-800 p-3 min-w-0">
                    <div className="truncate text-xs font-black text-zinc-900 dark:text-white" title={unit.name || 'Carte inconnue'}>
                      {unit.name || 'Carte inconnue'}
                    </div>
                    <div className="mt-1 flex items-center justify-between gap-2 text-[10px] font-bold text-zinc-400">
                      <span>{unit.category}</span>
                      <span className="tabular-nums">{unit.count > 0 ? '×' + unit.count : 'détecté'}</span>
                    </div>
                  </div>
                ))}
              </div>
              {!!currentArmy.warnings?.length && (
                <details className="mt-4 rounded-xl border border-amber-500/20 bg-amber-500/5 p-4">
                  <summary className="cursor-pointer text-[10px] font-black uppercase tracking-wider text-amber-600 dark:text-amber-400">
                    Notes de détection ({currentArmy.warnings.length})
                  </summary>
                  <div className="mt-3 space-y-1">
                    {currentArmy.warnings.map((warning, idx) => (
                      <div key={idx} className="text-[11px] font-medium text-zinc-500">{warning}</div>
                    ))}
                  </div>
                </details>
              )}
            </section>
          )}

          {!farmProfile && (
            <section className="rounded-[2rem] border border-dashed border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6">
              <div className="flex items-center gap-4">
                <div className="grid h-11 w-11 shrink-0 place-items-center rounded-2xl bg-zinc-100 dark:bg-zinc-800 text-zinc-400">
                  <span className="material-symbols-outlined">auto_awesome</span>
                </div>
                <div>
                  <h4 className="text-sm font-black text-zinc-950 dark:text-white">Plan de farm en préparation</h4>
                  <p className="mt-1 text-xs font-semibold text-zinc-500">
                    ClashGO sélectionnera automatiquement le profil adapté à ton HDV dès que la synchronisation sera complète.
                  </p>
                </div>
              </div>
            </section>
          )}

          {farmProfile && (
            <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6">
              <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-4">
                <div>
                  <div className="text-[10px] font-black uppercase tracking-[0.2em] text-zinc-400">Plan de farm automatique</div>
                  <h4 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">{farmProfile.label}</h4>
                  <p className="mt-1 text-xs font-medium text-zinc-500">Sélectionné automatiquement selon ton HDV lié. Aucun réglage manuel nécessaire.</p>
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
                <PlanGroup title="Troupes" items={farmProfile.troops.map(u => u.count + '× ' + u.name)} />
                <PlanGroup title="Sorts" items={farmProfile.spells.map(u => u.count + '× ' + u.name)} />
                <PlanGroup title="Héros" items={farmProfile.heroes} />
                <PlanGroup title="Siège" items={farmProfile.siege ? [farmProfile.siege] : ['Aucun']} />
              </div>
            </section>
          )}

          <section className="grid grid-cols-1 lg:grid-cols-3 gap-5">
            <div className="lg:col-span-2 rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6">
              <div className="flex items-center justify-between mb-5">
                <div>
                  <h4 className="text-xl font-black text-zinc-950 dark:text-white">Progression du compte</h4>
                  <p className="text-xs text-zinc-500 mt-1">Niveaux débloqués depuis le profil officiel du joueur.</p>
                </div>
                <span className="text-[10px] font-black text-zinc-400 uppercase tracking-widest">{homeTroops.length} troupes</span>
              </div>
              <div className="space-y-6">
                <UnitGrid title="Héros" units={homeHeroes} />
                <UnitGrid title="Troupes" units={homeTroops} />
                <UnitGrid title="Sorts" units={homeSpells} />
              </div>
            </div>

            <div className="space-y-5">
              <div className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6">
                <h4 className="text-lg font-black text-zinc-950 dark:text-white">Clan</h4>
                <div className="mt-4 text-2xl font-black text-zinc-950 dark:text-white">{profile.clan?.name || 'Aucun clan'}</div>
                {profile.clan && <div className="mt-1 text-xs font-mono text-zinc-500">{profile.clan.tag} · Niveau {profile.clan.clanLevel}</div>}
              </div>

              <div className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6">
                <div className="flex items-center gap-2">
                  <span className="material-symbols-outlined text-amber-500">database</span>
                  <h4 className="text-lg font-black text-zinc-950 dark:text-white">Ressources en direct</h4>
                </div>
                <p className="mt-2 text-xs font-medium text-zinc-500">
                  Lues automatiquement depuis le village dans BlueStacks. Aucun réglage supplémentaire n’est nécessaire.
                </p>
                <div className="mt-4 space-y-2">
                  {[
                    ['Gold', resources?.gold_valid ? resources.gold : null],
                    ['Elixir', resources?.elixir_valid ? resources.elixir : null],
                    ['Élixir noir', resources?.dark_valid ? resources.dark_elixir : null],
                  ].map(([name, value]) => (
                    <div key={String(name)} className="flex justify-between items-center rounded-xl bg-zinc-50 dark:bg-zinc-950/40 px-4 py-3">
                      <span className="text-xs font-black text-zinc-500">{name}</span>
                      <span className="text-sm font-black text-zinc-950 dark:text-white tabular-nums">
                        {typeof value === 'number' ? value.toLocaleString() : 'En attente du scan du village'}
                      </span>
                    </div>
                  ))}
                </div>
                {resources?.timestamp && (
                  <div className="mt-3 text-[10px] font-bold text-zinc-400">
                    Dernier scan : {snapshotAgeLabel(resources.timestamp)}
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
    {units.length === 0 ? <div className="text-xs text-zinc-400">Aucune donnée</div> : (
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
