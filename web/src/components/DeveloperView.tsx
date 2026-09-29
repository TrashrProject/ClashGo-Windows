import React from 'react';
import {
  AdminRenewLicense,
  AdminResetLicenseMachine,
  AdminSetLicenseActive,
  AdminSetLicenseRole,
  AdminUpdateLicenseCustomer,
  CreateAdminLicense,
  GetAdminLicenseHistory,
  GetDeveloperIncidents,
  GetDeveloperLicenses,
  GetLicenseState,
} from '../../wailsjs/go/main/App';

type Incident = {
  id?: string;
  at?: string;
  received_at?: string;
  license_hint?: string;
  role?: string;
  machine_id?: string;
  app_version?: string;
  level?: string;
  message?: string;
  customer_name?: string;
  customer_contact?: string;
  customer_notes?: string;
  payment_status?: string;
  total_paid_cents?: number;
  next_due_at?: string;
};

type LicenseRow = {
  id?: string;
  hint?: string;
  role?: string;
  active?: boolean;
  machine_id?: string;
  created_at?: string;
  last_seen_at?: string;
  app_version?: string;
  plan?: string;
  duration_days?: number;
  activated_at?: string;
  expires_at?: string;
  customer_name?: string;
  customer_contact?: string;
  customer_notes?: string;
  payment_status?: string;
  total_paid_cents?: number;
  next_due_at?: string;
};

type LicenseHistoryEvent = {
  id?: string;
  license_id?: string;
  license_hint?: string;
  customer_name?: string;
  customer_contact?: string;
  event_type?: string;
  plan?: string;
  amount_cents?: number;
  payment_status?: string;
  note?: string;
  created_at?: string;
  expires_at?: string;
};

type LicenseState = {
  activated?: boolean;
  role?: 'member' | 'developer' | 'admin' | '';
  license_hint?: string;
  machine_id?: string;
};

const copyText = async (value: string): Promise<void> => {
  try {
    await navigator.clipboard.writeText(value);
  } catch {
    const area = document.createElement('textarea');
    area.value = value;
    area.style.position = 'fixed';
    area.style.opacity = '0';
    document.body.appendChild(area);
    area.select();
    document.execCommand('copy');
    document.body.removeChild(area);
  }
};

const planLabel = (value?: string): string => {
  if (value === 'free_2d') return 'FREE · 2 jours';
  if (value === 'week_1') return '1 semaine';
  if (value === 'month_1') return '1 mois';
  return 'À vie';
};

const paymentLabel = (value?: string): string => {
  if (value === 'paid') return 'Payé';
  if (value === 'pending') return 'En attente';
  if (value === 'offered') return 'Offert';
  if (value === 'free') return 'Free';
  return 'Non renseigné';
};

const euroLabel = (cents?: number): string => {
  const value = Number(cents || 0) / 100;
  return value.toLocaleString('fr-FR', { style: 'currency', currency: 'EUR' });
};

const dateLabel = (value?: string): string => {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isFinite(date.getTime()) ? date.toLocaleString('fr-FR') : '—';
};

const shortMachine = (value?: string): string => {
  if (!value) return 'Non activée';
  if (value.length <= 18) return value;
  return value.slice(0, 10) + '…' + value.slice(-6);
};

const expiryLabel = (value?: string, now = Date.now()): string => {
  if (!value) return 'À vie';
  const expiry = new Date(value).getTime();
  if (!Number.isFinite(expiry)) return 'Expiration inconnue';
  const diff = expiry - now;
  const day = 24 * 60 * 60 * 1000;
  if (diff <= 0) {
    const days = Math.max(0, Math.floor(Math.abs(diff) / day));
    if (days === 0) return 'Expirée aujourd’hui';
    return days === 1 ? 'Expirée depuis 1 jour' : `Expirée depuis ${days} jours`;
  }
  const days = Math.ceil(diff / day);
  if (days === 1) return 'Expire demain';
  if (days <= 7) return `Expire dans ${days} jours`;
  return 'Expire le ' + dateLabel(value);
};

const isLicenseExpired = (item: LicenseRow, now = Date.now()): boolean => {
  if (!item.expires_at) return false;
  const expiry = new Date(item.expires_at).getTime();
  return Number.isFinite(expiry) && expiry <= now;
};

const isLicenseUsable = (item: LicenseRow, now = Date.now()): boolean =>
  item.active !== false && !isLicenseExpired(item, now);

const licenseStatusLabel = (item: LicenseRow, now = Date.now()): string => {
  if (item.active === false) return 'Révoquée';
  if (isLicenseExpired(item, now)) return 'Expirée';
  if (!item.machine_id && !item.activated_at) return 'Jamais activée';
  return 'Active';
};

const DeveloperView: React.FC = () => {
  const [incidents, setIncidents] = React.useState<Incident[]>([]);
  const [licenses, setLicenses] = React.useState<LicenseRow[]>([]);
  const [history, setHistory] = React.useState<LicenseHistoryEvent[]>([]);
  const [role, setRole] = React.useState<LicenseState['role']>('');
  const [currentLicenseHint, setCurrentLicenseHint] = React.useState('');
  const [currentMachineID, setCurrentMachineID] = React.useState('');
  const [busy, setBusy] = React.useState(false);
  const [creating, setCreating] = React.useState(false);
  const [actionID, setActionID] = React.useState('');
  const [error, setError] = React.useState('');
  const [notice, setNotice] = React.useState('');
  const [tab, setTab] = React.useState<'incidents' | 'licenses' | 'history'>('licenses');

  const [newRole, setNewRole] = React.useState<'member' | 'developer' | 'admin'>('member');
  const [newPlan, setNewPlan] = React.useState<'free_2d' | 'week_1' | 'month_1' | 'lifetime'>('month_1');
  const [customerName, setCustomerName] = React.useState('');
  const [customerContact, setCustomerContact] = React.useState('');
  const [customerNotes, setCustomerNotes] = React.useState('');
  const [creationAmount, setCreationAmount] = React.useState('');
  const [creationPaymentStatus, setCreationPaymentStatus] = React.useState<'paid' | 'pending' | 'offered' | 'free'>('paid');
  const [creationPaymentNote, setCreationPaymentNote] = React.useState('');
  const [generatedKey, setGeneratedKey] = React.useState('');
  const [generatedMeta, setGeneratedMeta] = React.useState<{ plan: string; role: string; customerName: string } | null>(null);
  const [copied, setCopied] = React.useState(false);
  const [clientMessageCopied, setClientMessageCopied] = React.useState(false);
  const [search, setSearch] = React.useState('');
  const [licenseFilter, setLicenseFilter] = React.useState<'all' | 'active' | 'revoked' | 'expired' | 'expiring' | 'unactivated'>('all');
  const [renewPlans, setRenewPlans] = React.useState<Record<string, 'free_2d' | 'week_1' | 'month_1' | 'lifetime'>>({});
  const [renewingID, setRenewingID] = React.useState('');
  const [renewAmount, setRenewAmount] = React.useState('');
  const [renewPaymentStatus, setRenewPaymentStatus] = React.useState<'paid' | 'pending' | 'offered' | 'free'>('paid');
  const [renewNote, setRenewNote] = React.useState('');
  const [editingCustomerID, setEditingCustomerID] = React.useState('');
  const [editCustomerName, setEditCustomerName] = React.useState('');
  const [editCustomerContact, setEditCustomerContact] = React.useState('');
  const [editCustomerNotes, setEditCustomerNotes] = React.useState('');
  const generatedKeyTimerRef = React.useRef<number | null>(null);

  const isAdmin = role === 'admin';

  React.useEffect(() => {
    if (!isAdmin && tab === 'history') setTab('licenses');
  }, [isAdmin, tab]);

  const refresh = React.useCallback(async () => {
    setBusy(true);
    setError('');
    try {
      const [state, i, l] = await Promise.all([
        GetLicenseState(),
        GetDeveloperIncidents(),
        GetDeveloperLicenses(),
      ]);
      const typedState = (state || {}) as LicenseState;
      setRole(typedState.role || '');
      setCurrentLicenseHint(typedState.license_hint || '');
      setCurrentMachineID(typedState.machine_id || '');
      setIncidents((i || []) as Incident[]);
      setLicenses((l || []) as LicenseRow[]);
      if (typedState.role === 'admin') {
        const rows = await GetAdminLicenseHistory();
        setHistory((rows || []) as LicenseHistoryEvent[]);
      } else {
        setHistory([]);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }, []);

  React.useEffect(() => {
    void refresh();
    const id = window.setInterval(() => void refresh(), 30000);
    return () => {
      window.clearInterval(id);
      if (generatedKeyTimerRef.current !== null) {
        window.clearTimeout(generatedKeyTimerRef.current);
        generatedKeyTimerRef.current = null;
      }
    };
  }, [refresh]);

  const createLicense = async () => {
    if (!isAdmin || creating) return;
    if (newRole === 'admin' && !window.confirm('Créer une nouvelle licence ADMIN ? Elle pourra gérer toutes les licences ClashGO.')) return;
    setCreating(true);
    setError('');
    setNotice('');
    setGeneratedKey('');
    setCopied(false);
    try {
      const normalizedAmount = creationAmount.trim().replace(',', '.');
      const parsedAmount = normalizedAmount === '' ? 0 : Number(normalizedAmount);
      if (!Number.isFinite(parsedAmount) || parsedAmount < 0) {
        throw new Error('Montant invalide.');
      }
      const result = await CreateAdminLicense({
        role: newRole,
        plan: newPlan,
        customer_name: customerName.trim(),
        customer_contact: customerContact.trim(),
        customer_notes: customerNotes.trim(),
        amount_cents: Math.round(parsedAmount * 100),
        payment_status: creationPaymentStatus,
        note: creationPaymentNote.trim(),
      } as any);
      const keys = (result as { licenses?: string[] })?.licenses || [];
      if (keys.length === 0) throw new Error('Aucune clé retournée par le serveur.');
      setGeneratedKey(keys[0]);
      setGeneratedMeta({ plan: newPlan, role: newRole, customerName: customerName.trim() });
      setNotice('Licence créée. Copie la clé maintenant : elle sera masquée automatiquement dans 2 minutes.');
      if (generatedKeyTimerRef.current !== null) {
        window.clearTimeout(generatedKeyTimerRef.current);
      }
      generatedKeyTimerRef.current = window.setTimeout(() => {
        setGeneratedKey('');
        setGeneratedMeta(null);
        setCopied(false);
        setClientMessageCopied(false);
        generatedKeyTimerRef.current = null;
      }, 120000);
      setCustomerName('');
      setCustomerContact('');
      setCustomerNotes('');
      setCreationAmount('');
      setCreationPaymentStatus('paid');
      setCreationPaymentNote('');
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setCreating(false);
    }
  };

  const runLicenseAction = async (id: string, action: () => Promise<unknown>, success: string): Promise<boolean> => {
    if (!isAdmin || !id || actionID) return false;
    setActionID(id);
    setError('');
    setNotice('');
    try {
      await action();
      setNotice(success);
      await refresh();
      return true;
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      return false;
    } finally {
      setActionID('');
    }
  };

  const renewLicense = async (item: LicenseRow) => {
    const id = String(item.id || '');
    if (!id) return;
    const currentPlan = (item.plan === 'free_2d' || item.plan === 'week_1' || item.plan === 'month_1' || item.plan === 'lifetime')
      ? item.plan
      : 'month_1';
    const plan = renewPlans[id] || currentPlan;
    const normalizedAmount = renewAmount.trim().replace(',', '.');
    const parsedAmount = normalizedAmount === '' ? 0 : Number(normalizedAmount);
    if (!Number.isFinite(parsedAmount) || parsedAmount < 0) {
      setError('Montant invalide.');
      return;
    }
    const amountCents = Math.round(parsedAmount * 100);
    const ok = await runLicenseAction(
      id,
      () => AdminRenewLicense(id, plan, amountCents, renewPaymentStatus, renewNote.trim()),
      'Licence renouvelée en ' + planLabel(plan) + ' · même clé conservée.'
    );
    if (ok) {
      setRenewingID('');
      setRenewAmount('');
      setRenewPaymentStatus('paid');
      setRenewNote('');
    }
  };

  const openRenewal = (item: LicenseRow) => {
    const id = String(item.id || '');
    if (!id) return;
    setRenewingID(id);
    setRenewAmount('');
    setRenewPaymentStatus('paid');
    setRenewNote('');
    setError('');
    setNotice('');
  };

  const cancelRenewal = () => {
    setRenewingID('');
    setRenewAmount('');
    setRenewPaymentStatus('paid');
    setRenewNote('');
  };

  const resetMachine = async (item: LicenseRow) => {
    const id = String(item.id || '');
    if (!id || !window.confirm('Délier cette licence de son PC actuel ?')) return;
    await runLicenseAction(id, () => AdminResetLicenseMachine(id), 'PC réinitialisé · la licence peut être activée sur une nouvelle machine.');
  };

  const setActive = async (item: LicenseRow, active: boolean) => {
    const id = String(item.id || '');
    if (!id) return;
    const label = active ? 'Réactiver cette licence ?' : 'Révoquer cette licence ?';
    if (!window.confirm(label)) return;
    await runLicenseAction(
      id,
      () => AdminSetLicenseActive(id, active),
      active ? 'Licence réactivée.' : 'Licence révoquée.'
    );
  };

  const setLicenseRole = async (item: LicenseRow, nextRole: 'member' | 'developer' | 'admin') => {
    const id = String(item.id || '');
    if (!id || nextRole === item.role) return;
    await runLicenseAction(id, () => AdminSetLicenseRole(id, nextRole), 'Rôle de la licence mis à jour.');
  };

  const startCustomerEdit = (item: LicenseRow) => {
    const id = String(item.id || '');
    if (!id) return;
    setEditingCustomerID(id);
    setEditCustomerName(item.customer_name || '');
    setEditCustomerContact(item.customer_contact || '');
    setEditCustomerNotes(item.customer_notes || '');
    setError('');
    setNotice('');
  };

  const cancelCustomerEdit = () => {
    setEditingCustomerID('');
    setEditCustomerName('');
    setEditCustomerContact('');
    setEditCustomerNotes('');
  };

  const saveCustomerEdit = async (item: LicenseRow) => {
    const id = String(item.id || '');
    const name = editCustomerName.trim();
    if (!id || !name) {
      setError('Le nom ou pseudo du client est obligatoire.');
      return;
    }
    await runLicenseAction(
      id,
      () => AdminUpdateLicenseCustomer(id, name, editCustomerContact.trim(), editCustomerNotes.trim()),
      'Informations client mises à jour.'
    );
    cancelCustomerEdit();
  };

  const copyGeneratedKey = async () => {
    if (!generatedKey) return;
    await copyText(generatedKey);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  };

  const copyClientMessage = async () => {
    if (!generatedKey || !generatedMeta) return;
    const hello = generatedMeta.customerName ? `Bonjour ${generatedMeta.customerName},` : 'Bonjour,';
    const roleLabel = generatedMeta.role === 'developer'
      ? 'Developer'
      : generatedMeta.role === 'admin'
        ? 'Admin'
        : 'Membre';
    const message = [
      hello,
      '',
      `Voici ta licence ClashGO · ${planLabel(generatedMeta.plan)} · ${roleLabel}`,
      generatedKey,
      '',
      'Activation :',
      '1. Ouvre ClashGO sur ton PC.',
      '2. Entre cette clé sur l’écran d’activation.',
      '3. La licence se lie automatiquement à ce PC.',
      '',
      'La durée commence à la première activation.',
      'Une licence est liée à un seul PC à la fois. Si tu changes de PC, contacte-moi pour la réinitialiser.',
    ].join('\n');
    await copyText(message);
    setClientMessageCopied(true);
    window.setTimeout(() => setClientMessageCopied(false), 1800);
  };

  const recentErrors = incidents.filter((x) => x.level === 'error' || x.level === 'fatal' || x.level === 'panic');
  const now = Date.now();
  const activeLicenses = licenses.filter((item) => isLicenseUsable(item, now)).length;
  const expiredLicenses = licenses.filter((item) => item.active !== false && isLicenseExpired(item, now)).length;
  const sevenDaysMs = 7 * 24 * 60 * 60 * 1000;
  const expiringSoon = licenses.filter((item) => {
    if (item.active === false || !item.expires_at) return false;
    const expiry = new Date(item.expires_at).getTime();
    return Number.isFinite(expiry) && expiry > now && expiry - now <= sevenDaysMs;
  }).length;
  const neverActivated = licenses.filter((item) =>
    item.active !== false && !item.machine_id && !item.activated_at
  ).length;
  const isCurrentAdminLicense = (item: LicenseRow): boolean =>
    isAdmin &&
    Boolean(currentLicenseHint) &&
    Boolean(currentMachineID) &&
    item.hint === currentLicenseHint &&
    item.machine_id === currentMachineID;
  const normalizedSearch = search.trim().toLowerCase();
  const licensePriority = (item: LicenseRow): number => {
    if (item.active === false) return 50;
    if (isLicenseExpired(item, now)) return 0;
    if (item.expires_at) {
      const expiry = new Date(item.expires_at).getTime();
      if (Number.isFinite(expiry) && expiry > now && expiry - now <= sevenDaysMs) return 10;
    }
    if (!item.machine_id && !item.activated_at) return 20;
    return 30;
  };

  const filteredLicenses = licenses.filter((item) => {
    if (licenseFilter === 'active' && !isLicenseUsable(item, now)) return false;
    if (licenseFilter === 'revoked' && item.active !== false) return false;
    if (licenseFilter === 'expired' && !(item.active !== false && isLicenseExpired(item, now))) return false;
    if (licenseFilter === 'unactivated' && (item.active === false || isLicenseExpired(item, now) || Boolean(item.machine_id) || Boolean(item.activated_at))) return false;
    if (licenseFilter === 'expiring') {
      const expiry = item.expires_at ? new Date(item.expires_at).getTime() : Number.NaN;
      if (item.active === false || !Number.isFinite(expiry) || expiry <= now || expiry - now > sevenDaysMs) return false;
    }
    if (!normalizedSearch) return true;
    return [
      item.customer_name,
      item.customer_contact,
      item.customer_notes,
      item.payment_status,
      item.total_paid_cents,
      item.hint,
      item.id,
      item.machine_id,
      item.app_version,
      item.role,
      planLabel(item.plan),
    ].some((value) => String(value || '').toLowerCase().includes(normalizedSearch));
  }).sort((a, b) => {
    const priority = licensePriority(a) - licensePriority(b);
    if (priority !== 0) return priority;
    const aExpiry = a.expires_at ? new Date(a.expires_at).getTime() : Number.POSITIVE_INFINITY;
    const bExpiry = b.expires_at ? new Date(b.expires_at).getTime() : Number.POSITIVE_INFINITY;
    if (aExpiry !== bExpiry) return aExpiry - bExpiry;
    return String(a.customer_name || a.hint || '').localeCompare(String(b.customer_name || b.hint || ''), 'fr');
  });
  const filteredIncidents = incidents.filter((item) => {
    if (!normalizedSearch) return true;
    return [
      item.customer_name,
      item.customer_contact,
      item.license_hint,
      item.machine_id,
      item.app_version,
      item.level,
      item.message,
    ].some((value) => String(value || '').toLowerCase().includes(normalizedSearch));
  });

  const filteredHistory = history.filter((item) => {
    if (!normalizedSearch) return true;
    return [
      item.customer_name,
      item.customer_contact,
      item.license_hint,
      item.event_type,
      item.plan,
      item.payment_status,
      item.note,
    ].some((value) => String(value || '').toLowerCase().includes(normalizedSearch));
  });

  const incidentGroups = (() => {
    const groups = new Map<string, {
      key: string;
      level: string;
      version: string;
      message: string;
      count: number;
      clients: Set<string>;
      machines: Set<string>;
    }>();

    for (const item of filteredIncidents) {
      const level = item.level || 'error';
      const version = item.app_version || 'Version inconnue';
      const message = item.message || 'Aucun message';
      const key = [level, version, message].join('|');
      let group = groups.get(key);
      if (!group) {
        group = {
          key,
          level,
          version,
          message,
          count: 0,
          clients: new Set<string>(),
          machines: new Set<string>(),
        };
        groups.set(key, group);
      }
      group.count += 1;
      if (item.customer_name || item.license_hint) group.clients.add(item.customer_name || item.license_hint || '');
      if (item.machine_id) group.machines.add(item.machine_id);
    }

    return Array.from(groups.values())
      .sort((a, b) => b.count - a.count)
      .slice(0, 3);
  })();

  return (
    <div className="space-y-6 max-w-6xl mx-auto">
      <section className="rounded-[2.25rem] bg-zinc-950 dark:bg-white text-white dark:text-zinc-950 p-7 shadow-premium-lg">
        <div className="flex flex-col lg:flex-row lg:items-end lg:justify-between gap-5">
          <div>
            <div className="text-[10px] font-black uppercase tracking-[0.25em] text-zinc-400 dark:text-zinc-500">
              {isAdmin ? 'Administration ClashGO' : 'Support développeur'}
            </div>
            <h2 className="mt-2 text-3xl font-black tracking-tight">
              {isAdmin ? 'Licences & support' : 'Diagnostic distant'}
            </h2>
            <p className="mt-2 max-w-2xl text-sm font-semibold text-zinc-400 dark:text-zinc-600">
              {isAdmin
                ? 'Crée et surveille les licences depuis ClashGO. Aucun secret administrateur n’est stocké dans l’application.'
                : 'Consulte les installations et les erreurs automatiques remontées par ClashGO.'}
            </p>
          </div>
          <button
            type="button"
            onClick={() => void refresh()}
            disabled={busy}
            className="h-11 px-5 rounded-xl bg-white dark:bg-zinc-950 text-zinc-950 dark:text-white text-[10px] font-black uppercase tracking-widest disabled:opacity-40"
          >
            {busy ? 'Actualisation…' : 'Actualiser'}
          </button>
        </div>

        <div className="mt-6 grid grid-cols-2 md:grid-cols-3 xl:grid-cols-6 gap-3">
          {[
            { label: 'Licences', value: licenses.length, tab: 'licenses' as const, filter: 'all' as const },
            { label: 'Utilisables', value: activeLicenses, tab: 'licenses' as const, filter: 'active' as const },
            { label: 'Expire < 7j', value: expiringSoon, tab: 'licenses' as const, filter: 'expiring' as const },
            { label: 'Expirées', value: expiredLicenses, tab: 'licenses' as const, filter: 'expired' as const },
            { label: 'Jamais activées', value: neverActivated, tab: 'licenses' as const, filter: 'unactivated' as const },
            { label: 'Erreurs', value: recentErrors.length, tab: 'incidents' as const, filter: null },
          ].map((item) => (
            <button
              key={item.label}
              type="button"
              onClick={() => {
                setTab(item.tab);
                if (item.filter) setLicenseFilter(item.filter);
              }}
              className="rounded-2xl bg-white/5 dark:bg-zinc-100 p-4 text-left transition hover:bg-white/10 dark:hover:bg-zinc-200 active:scale-[0.98]"
              title={'Afficher ' + item.label.toLowerCase()}
            >
              <div className="text-2xl font-black">{item.value}</div>
              <div className="mt-1 text-[9px] font-black uppercase tracking-widest text-zinc-400 dark:text-zinc-500">{item.label}</div>
            </button>
          ))}
        </div>
      </section>

      {isAdmin && (
        <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6 shadow-premium dark:shadow-none">
          <div className="flex flex-col lg:flex-row lg:items-start lg:justify-between gap-5">
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Nouvelle licence</div>
              <h3 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">Créer une clé</h3>
              <p className="mt-2 text-xs font-semibold text-zinc-500">
                La durée démarre à la première activation. La clé complète n’est affichée qu’au moment de sa création.
              </p>
            </div>
            <span className="rounded-full bg-emerald-500/10 px-3 py-1.5 text-[9px] font-black uppercase tracking-widest text-emerald-500">
              ADMIN
            </span>
          </div>

          <div className="mt-5">
            <div className="mb-2 text-[9px] font-black uppercase tracking-widest text-zinc-400">Durée rapide</div>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-2">
              {([
                ['free_2d', 'FREE · 2J', 'Test rapide'],
                ['week_1', '1 SEMAINE', '7 jours'],
                ['month_1', '1 MOIS', '30 jours'],
                ['lifetime', 'À VIE', 'Sans expiration'],
              ] as const).map(([value, label, description]) => (
                <button
                  key={value}
                  type="button"
                  onClick={() => setNewPlan(value)}
                  className={
                    'rounded-xl border px-3 py-3 text-left transition ' +
                    (newPlan === value
                      ? 'border-zinc-950 bg-zinc-950 text-white dark:border-white dark:bg-white dark:text-zinc-950'
                      : 'border-zinc-200 bg-zinc-50 text-zinc-950 hover:border-zinc-400 dark:border-zinc-800 dark:bg-zinc-950 dark:text-white')
                  }
                >
                  <div className="text-xs font-black">{label}</div>
                  <div className={'mt-1 text-[9px] font-bold ' + (newPlan === value ? 'opacity-60' : 'text-zinc-400')}>{description}</div>
                </button>
              ))}
            </div>
          </div>

          <div className="mt-5 grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-3">
            <label>
              <div className="mb-2 text-[9px] font-black uppercase tracking-widest text-zinc-400">Client / pseudo</div>
              <input
                value={customerName}
                onChange={(e) => setCustomerName(e.target.value)}
                placeholder="Ex. Nathan"
                className="h-12 w-full rounded-xl border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 px-4 text-sm font-semibold outline-none focus:border-zinc-400"
              />
            </label>
            <label>
              <div className="mb-2 text-[9px] font-black uppercase tracking-widest text-zinc-400">Contact</div>
              <input
                value={customerContact}
                onChange={(e) => setCustomerContact(e.target.value)}
                placeholder="Discord / contact optionnel"
                className="h-12 w-full rounded-xl border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 px-4 text-sm font-semibold outline-none focus:border-zinc-400"
              />
            </label>
            <label>
              <div className="mb-2 text-[9px] font-black uppercase tracking-widest text-zinc-400">Durée</div>
              <select
                value={newPlan}
                onChange={(e) => setNewPlan(e.target.value as typeof newPlan)}
                className="h-12 w-full rounded-xl border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 px-4 text-sm font-bold outline-none"
              >
                <option value="free_2d">FREE · 2 jours</option>
                <option value="week_1">1 semaine</option>
                <option value="month_1">1 mois</option>
                <option value="lifetime">À vie</option>
              </select>
            </label>
            <label>
              <div className="mb-2 text-[9px] font-black uppercase tracking-widest text-zinc-400">Rôle</div>
              <select
                value={newRole}
                onChange={(e) => setNewRole(e.target.value as typeof newRole)}
                className="h-12 w-full rounded-xl border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 px-4 text-sm font-bold outline-none"
              >
                <option value="member">Membre</option>
                <option value="developer">Développeur</option>
                <option value="admin">Admin</option>
              </select>
            </label>
          </div>

          <label className="mt-3 block">
            <div className="mb-2 text-[9px] font-black uppercase tracking-widest text-zinc-400">Note interne</div>
            <textarea
              value={customerNotes}
              onChange={(e) => setCustomerNotes(e.target.value)}
              placeholder="Optionnel · ex. testeur, ami de…, à rappeler…"
              maxLength={1000}
              rows={2}
              className="w-full resize-none rounded-xl border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 px-4 py-3 text-sm font-semibold outline-none focus:border-zinc-400"
            />
          </label>

          <details className="mt-3 rounded-xl border border-zinc-200 dark:border-zinc-700 overflow-hidden">
            <summary className="cursor-pointer list-none flex items-center justify-between gap-3 bg-zinc-50 dark:bg-zinc-950/50 px-4 py-3">
              <div>
                <div className="text-[9px] font-black uppercase tracking-widest text-zinc-500">Suivi paiement manuel</div>
                <div className="mt-1 text-[10px] font-semibold text-zinc-400">Optionnel · aucun paiement n’est traité par ClashGO</div>
              </div>
              <span className="material-symbols-outlined text-base text-zinc-400">expand_more</span>
            </summary>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-3 p-4">
              <label>
                <div className="mb-1 text-[8px] font-black uppercase tracking-wider text-zinc-400">Montant encaissé (€)</div>
                <input
                  value={creationAmount}
                  onChange={(e) => setCreationAmount(e.target.value)}
                  inputMode="decimal"
                  placeholder="Ex. 9,99"
                  className="h-10 w-full rounded-lg border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 text-xs font-bold outline-none"
                />
              </label>
              <label>
                <div className="mb-1 text-[8px] font-black uppercase tracking-wider text-zinc-400">Statut</div>
                <select
                  value={creationPaymentStatus}
                  onChange={(e) => {
                    const next = e.target.value as typeof creationPaymentStatus;
                    setCreationPaymentStatus(next);
                    if (next === 'offered' || next === 'free') setCreationAmount('');
                  }}
                  className="h-10 w-full rounded-lg border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-2 text-[9px] font-black uppercase outline-none"
                >
                  <option value="paid">Payé</option>
                  <option value="pending">En attente</option>
                  <option value="offered">Offert</option>
                  <option value="free">Free</option>
                </select>
              </label>
              <label>
                <div className="mb-1 text-[8px] font-black uppercase tracking-wider text-zinc-400">Note paiement</div>
                <input
                  value={creationPaymentNote}
                  onChange={(e) => setCreationPaymentNote(e.target.value)}
                  maxLength={1000}
                  placeholder="Optionnel"
                  className="h-10 w-full rounded-lg border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 text-xs font-semibold outline-none"
                />
              </label>
            </div>
          </details>

          <button
            type="button"
            onClick={() => void createLicense()}
            disabled={creating}
            className="mt-4 h-12 rounded-xl bg-zinc-950 dark:bg-white px-6 text-[10px] font-black uppercase tracking-[0.18em] text-white dark:text-zinc-950 disabled:opacity-40"
          >
            {creating ? 'Création…' : 'Générer la licence'}
          </button>

          {generatedKey && (
            <div className="mt-5 rounded-2xl border border-emerald-500/30 bg-emerald-500/10 p-4">
              <div className="text-[9px] font-black uppercase tracking-[0.2em] text-emerald-600 dark:text-emerald-400">Clé générée · à copier maintenant</div>
              <div className="mt-3 flex flex-col md:flex-row md:items-center gap-3">
                <code className="min-w-0 flex-1 break-all rounded-xl bg-zinc-950 px-4 py-3 text-sm font-bold text-white">{generatedKey}</code>
                <button
                  type="button"
                  onClick={() => void copyGeneratedKey()}
                  className="h-11 shrink-0 rounded-xl bg-emerald-500 px-5 text-[10px] font-black uppercase tracking-widest text-white"
                >
                  {copied ? 'Copiée ✓' : 'Copier la clé'}
                </button>
                <button
                  type="button"
                  onClick={() => void copyClientMessage()}
                  className="h-11 shrink-0 rounded-xl border border-emerald-500/40 bg-white/60 dark:bg-zinc-950/30 px-5 text-[10px] font-black uppercase tracking-widest text-emerald-700 dark:text-emerald-300"
                >
                  {clientMessageCopied ? 'Message copié ✓' : 'Copier le message client'}
                </button>
                <button
                  type="button"
                  onClick={() => {
                    if (generatedKeyTimerRef.current !== null) {
                      window.clearTimeout(generatedKeyTimerRef.current);
                      generatedKeyTimerRef.current = null;
                    }
                    setGeneratedKey('');
                    setGeneratedMeta(null);
                    setCopied(false);
                    setClientMessageCopied(false);
                  }}
                  className="h-11 shrink-0 rounded-xl border border-emerald-500/30 px-4 text-[10px] font-black uppercase tracking-widest text-emerald-600 dark:text-emerald-400"
                >
                  Masquer
                </button>
              </div>
              {notice && <div className="mt-2 text-xs font-semibold text-emerald-700 dark:text-emerald-300">{notice}</div>}
            </div>
          )}
        </section>
      )}

      <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-3">
        <div className="flex items-center gap-2">
        {([
          ['licenses', 'Licences'],
          ...(isAdmin ? [['history', 'Historique'] as const] : []),
          ['incidents', 'Incidents'],
        ] as const).map(([item, label]) => (
          <button
            key={item}
            type="button"
            onClick={() => setTab(item)}
            className={
              'px-4 py-2.5 rounded-xl text-[10px] font-black uppercase tracking-widest transition-all ' +
              (tab === item
                ? 'bg-zinc-950 dark:bg-white text-white dark:text-zinc-950'
                : 'bg-white dark:bg-zinc-900 text-zinc-500 border border-zinc-100 dark:border-zinc-800')
            }
          >
            {label}
          </button>
        ))}
        </div>

        <div className="flex flex-col sm:flex-row gap-2">
          <div className="relative">
            <span className="material-symbols-outlined absolute left-3 top-1/2 -translate-y-1/2 text-base text-zinc-400">search</span>
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={
                tab === 'licenses'
                  ? 'Client, contact, licence, PC…'
                  : tab === 'history'
                    ? 'Client, licence, note, formule…'
                    : 'Licence, version, erreur…'
              }
              className="h-10 w-full sm:w-72 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 pl-9 pr-3 text-xs font-semibold outline-none focus:border-zinc-400"
            />
          </div>
          {tab === 'licenses' && (
            <select
              value={licenseFilter}
              onChange={(e) => setLicenseFilter(e.target.value as typeof licenseFilter)}
              className="h-10 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 px-3 text-[10px] font-black uppercase tracking-wider text-zinc-500 outline-none"
            >
              <option value="all">Toutes</option>
              <option value="active">Actives</option>
              <option value="expiring">Expire &lt; 7j</option>
              <option value="expired">Expirées</option>
              <option value="unactivated">Jamais activées</option>
              <option value="revoked">Révoquées</option>
            </select>
          )}
        </div>
      </div>

      {notice && !generatedKey && (
        <div className="rounded-2xl bg-emerald-500/10 border border-emerald-500/20 px-4 py-3 text-sm font-bold text-emerald-600 dark:text-emerald-400">
          {notice}
        </div>
      )}

      {error && (
        <div className="rounded-2xl bg-rose-500/10 border border-rose-500/20 px-4 py-3 text-sm font-bold text-rose-500">
          {error}
        </div>
      )}

      {tab === 'incidents' ? (
        <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-premium dark:shadow-none overflow-hidden">
          <div className="p-6 border-b border-zinc-100 dark:border-zinc-800">
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Erreurs automatiques</div>
            <h3 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">Derniers incidents</h3>

            {incidentGroups.length > 0 && (
              <div className="mt-5 grid grid-cols-1 lg:grid-cols-3 gap-3">
                {incidentGroups.map((group) => (
                  <div key={group.key} className="rounded-2xl border border-zinc-100 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950/40 p-4">
                    <div className="flex items-center justify-between gap-3">
                      <span className={
                        'rounded-lg px-2 py-1 text-[9px] font-black uppercase tracking-widest ' +
                        ((group.level === 'fatal' || group.level === 'panic')
                          ? 'bg-rose-500/10 text-rose-500'
                          : 'bg-amber-500/10 text-amber-500')
                      }>
                        {group.level}
                      </span>
                      <span className="text-lg font-black text-zinc-950 dark:text-white">{group.count}×</span>
                    </div>
                    <div className="mt-3 text-xs font-black text-zinc-800 dark:text-zinc-100 line-clamp-2" title={group.message}>
                      {group.message}
                    </div>
                    <div className="mt-3 flex flex-wrap gap-x-3 gap-y-1 text-[9px] font-bold uppercase tracking-wider text-zinc-400">
                      <span>{group.version}</span>
                      <span>{group.clients.size} client{group.clients.size > 1 ? 's' : ''}</span>
                      <span>{group.machines.size} PC</span>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
          <div className="divide-y divide-zinc-100 dark:divide-zinc-800">
            {filteredIncidents.slice().reverse().map((item, index) => (
              <div key={item.id || String(index)} className="p-5 flex flex-col lg:flex-row lg:items-start gap-4">
                <div className="lg:w-44 shrink-0">
                  <div className="text-xs font-black text-zinc-900 dark:text-white">{item.customer_name || item.license_hint || 'Licence inconnue'}</div>
                  <div className="mt-1 text-[10px] font-semibold text-zinc-400">{item.customer_contact || item.license_hint || ''}</div>
                  <div className="mt-1 text-[10px] font-mono text-zinc-400">{item.app_version || 'Version inconnue'}</div>
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className={
                      'px-2 py-1 rounded-lg text-[9px] font-black uppercase tracking-widest ' +
                      ((item.level === 'fatal' || item.level === 'panic')
                        ? 'bg-rose-500/10 text-rose-500'
                        : 'bg-amber-500/10 text-amber-500')
                    }>
                      {item.level || 'erreur'}
                    </span>
                    <span className="text-[10px] font-semibold text-zinc-400">{dateLabel(item.received_at || item.at)}</span>
                  </div>
                  <div className="mt-2 text-sm font-bold text-zinc-800 dark:text-zinc-100 break-words">{item.message || 'Aucun message'}</div>
                  {item.machine_id && (
                    <div className="mt-2 text-[10px] font-mono text-zinc-400 truncate">PC {shortMachine(item.machine_id)}</div>
                  )}
                </div>
              </div>
            ))}
            {!busy && filteredIncidents.length === 0 && (
              <div className="p-8 text-center text-sm font-semibold text-zinc-400">Aucun incident reçu.</div>
            )}
          </div>
        </section>
      ) : tab === 'history' && isAdmin ? (
        <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-premium dark:shadow-none overflow-hidden">
          <div className="p-6 border-b border-zinc-100 dark:border-zinc-800">
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Suivi manuel</div>
            <h3 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">Historique licences & renouvellements</h3>
            <p className="mt-2 text-xs font-semibold text-zinc-500">
              Les montants sont uniquement des informations saisies manuellement. Aucun paiement n’est traité par ClashGO.
            </p>
          </div>
          <div className="divide-y divide-zinc-100 dark:divide-zinc-800">
            {filteredHistory.map((item, index) => (
              <div key={item.id || String(index)} className="p-5 grid grid-cols-1 lg:grid-cols-[1.2fr_.8fr_.8fr_1.5fr] gap-4 lg:items-center">
                <div>
                  <div className="text-sm font-black text-zinc-900 dark:text-white">
                    {item.customer_name || item.license_hint || 'Licence'}
                  </div>
                  <div className="mt-1 text-[10px] font-semibold text-zinc-400">
                    {item.customer_contact || item.license_hint || ''}
                  </div>
                  <div className="mt-1 text-[10px] font-mono text-zinc-400">{dateLabel(item.created_at)}</div>
                </div>
                <div>
                  <span className="rounded-lg bg-zinc-100 dark:bg-zinc-800 px-2 py-1 text-[9px] font-black uppercase tracking-widest text-zinc-500">
                    {item.event_type === 'renewal' ? 'Renouvellement' : item.event_type === 'created' ? 'Création' : item.event_type || 'Événement'}
                  </span>
                  <div className="mt-2 text-xs font-black text-zinc-700 dark:text-zinc-200">{planLabel(item.plan)}</div>
                </div>
                <div>
                  <div className="text-sm font-black text-zinc-900 dark:text-white">{euroLabel(item.amount_cents)}</div>
                  <div className="mt-1 text-[10px] font-bold text-zinc-400">{paymentLabel(item.payment_status)}</div>
                  {item.expires_at && <div className="mt-1 text-[10px] text-zinc-400">{expiryLabel(item.expires_at)}</div>}
                </div>
                <div className="text-xs font-semibold text-zinc-500 break-words">
                  {item.note || 'Aucune note'}
                </div>
              </div>
            ))}
            {!busy && filteredHistory.length === 0 && (
              <div className="p-8 text-center text-sm font-semibold text-zinc-400">Aucun événement trouvé.</div>
            )}
          </div>
        </section>
      ) : (
        <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-premium dark:shadow-none overflow-hidden">
          <div className="p-6 border-b border-zinc-100 dark:border-zinc-800">
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Installations ClashGO</div>
            <h3 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">Licences & machines</h3>
          </div>
          <div className="divide-y divide-zinc-100 dark:divide-zinc-800">
            {filteredLicenses.map((item, index) => (
              <div key={(item.id || item.hint || 'license') + index} className="p-5 flex flex-col lg:flex-row lg:items-center gap-4">
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-black text-zinc-900 dark:text-white">
                      {item.customer_name || item.hint || 'Licence'}
                    </span>
                    <span className="px-2 py-1 rounded-lg bg-zinc-100 dark:bg-zinc-800 text-[9px] font-black uppercase tracking-widest text-zinc-500">
                      {item.role || 'member'}
                    </span>
                    <span className="px-2 py-1 rounded-lg bg-zinc-100 dark:bg-zinc-800 text-[9px] font-black uppercase tracking-widest text-zinc-500">
                      {planLabel(item.plan)}
                    </span>
                    {isCurrentAdminLicense(item) && (
                      <span className="px-2 py-1 rounded-lg bg-sky-500/10 text-[9px] font-black uppercase tracking-widest text-sky-500">
                        Cette licence
                      </span>
                    )}
                  </div>
                  <div className="mt-1 text-[10px] font-mono text-zinc-400">
                    {item.hint || '••••'} · {shortMachine(item.machine_id)}
                  </div>
                  {item.customer_contact && (
                    <div className="mt-1 text-[10px] font-semibold text-zinc-400">{item.customer_contact}</div>
                  )}
                  {item.customer_notes && (
                    <div className="mt-2 max-w-2xl text-[10px] font-semibold text-zinc-500 line-clamp-2" title={item.customer_notes}>
                      Note · {item.customer_notes}
                    </div>
                  )}
                  {isAdmin && (item.payment_status || Number(item.total_paid_cents || 0) > 0) && (
                    <div className="mt-2 flex flex-wrap gap-2 text-[9px] font-black uppercase tracking-wider">
                      <span className="rounded-full bg-emerald-500/10 px-2.5 py-1 text-emerald-600 dark:text-emerald-400">
                        Total {euroLabel(item.total_paid_cents)}
                      </span>
                      <span className="rounded-full bg-zinc-100 dark:bg-zinc-800 px-2.5 py-1 text-zinc-500">
                        {paymentLabel(item.payment_status)}
                      </span>
                    </div>
                  )}

                  {isAdmin && item.id && editingCustomerID === String(item.id) && (
                    <div className="mt-3 grid grid-cols-1 md:grid-cols-2 gap-2 max-w-2xl">
                      <input
                        value={editCustomerName}
                        onChange={(e) => setEditCustomerName(e.target.value)}
                        placeholder="Nom / pseudo"
                        maxLength={120}
                        autoFocus
                        className="h-9 rounded-lg border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 px-3 text-xs font-semibold outline-none focus:border-zinc-400"
                      />
                      <input
                        value={editCustomerContact}
                        onChange={(e) => setEditCustomerContact(e.target.value)}
                        placeholder="Discord / contact"
                        maxLength={180}
                        className="h-9 rounded-lg border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 px-3 text-xs font-semibold outline-none focus:border-zinc-400"
                      />
                      <textarea
                        value={editCustomerNotes}
                        onChange={(e) => setEditCustomerNotes(e.target.value)}
                        placeholder="Note interne"
                        maxLength={1000}
                        rows={2}
                        className="md:col-span-2 resize-none rounded-lg border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 px-3 py-2 text-xs font-semibold outline-none focus:border-zinc-400"
                      />
                      <div className="md:col-span-2 flex gap-2">
                        <button
                          type="button"
                          disabled={actionID === item.id || !editCustomerName.trim()}
                          onClick={() => void saveCustomerEdit(item)}
                          className="h-9 rounded-lg bg-zinc-950 dark:bg-white px-3 text-[9px] font-black uppercase tracking-wider text-white dark:text-zinc-950 disabled:opacity-40"
                        >
                          Enregistrer
                        </button>
                        <button
                          type="button"
                          disabled={actionID === item.id}
                          onClick={cancelCustomerEdit}
                          className="h-9 rounded-lg border border-zinc-200 dark:border-zinc-700 px-3 text-[9px] font-black uppercase tracking-wider text-zinc-500 disabled:opacity-40"
                        >
                          Annuler
                        </button>
                      </div>
                    </div>
                  )}
                </div>
                <div className="lg:text-right">
                  <div className={
                    'text-[10px] font-black uppercase tracking-widest ' +
                    (item.active === false || isLicenseExpired(item, now)
                      ? 'text-rose-500'
                      : (!item.machine_id && !item.activated_at)
                        ? 'text-amber-500'
                        : 'text-emerald-500')
                  }>
                    {licenseStatusLabel(item, now)}
                  </div>
                  <div className="mt-1 text-[10px] text-zinc-400">
                    {expiryLabel(item.expires_at, now)}
                  </div>
                  <div className="mt-1 text-[10px] text-zinc-400">{item.app_version || 'Jamais connectée'}</div>
                </div>

                {isAdmin && item.id && (
                  <div className="lg:w-full xl:w-auto xl:min-w-[390px] flex flex-wrap items-center gap-2 lg:justify-end">
                    <button
                      type="button"
                      disabled={actionID === item.id || editingCustomerID === String(item.id)}
                      onClick={() => startCustomerEdit(item)}
                      className="h-9 rounded-lg border border-zinc-200 dark:border-zinc-700 px-3 text-[9px] font-black uppercase tracking-wider text-zinc-600 dark:text-zinc-300 disabled:opacity-40"
                    >
                      Client
                    </button>

                    <select
                      value={(item.role === 'developer' || item.role === 'admin') ? item.role : 'member'}
                      disabled={actionID === item.id || isCurrentAdminLicense(item)}
                      onChange={(e) => void setLicenseRole(item, e.target.value as 'member' | 'developer' | 'admin')}
                      className="h-9 rounded-lg border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 px-2 text-[9px] font-black uppercase tracking-wider outline-none disabled:opacity-40"
                    >
                      <option value="member">Membre</option>
                      <option value="developer">Développeur</option>
                      <option value="admin">Admin</option>
                    </select>

                    {item.plan !== 'lifetime' ? (
                      <>
                        <select
                          value={renewPlans[String(item.id)] || ((item.plan === 'free_2d' || item.plan === 'week_1' || item.plan === 'month_1') ? item.plan : 'month_1')}
                          disabled={actionID === item.id}
                          onChange={(e) => setRenewPlans((current) => ({
                            ...current,
                            [String(item.id)]: e.target.value as 'free_2d' | 'week_1' | 'month_1' | 'lifetime',
                          }))}
                          className="h-9 rounded-lg border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 px-2 text-[9px] font-black uppercase tracking-wider outline-none disabled:opacity-40"
                          title="Formule appliquée au prochain renouvellement"
                        >
                          <option value="free_2d">+2 jours</option>
                          <option value="week_1">+1 semaine</option>
                          <option value="month_1">+1 mois</option>
                          <option value="lifetime">Passer à vie</option>
                        </select>

                        <button
                          type="button"
                          disabled={actionID === item.id}
                          onClick={() => openRenewal(item)}
                          className="h-9 rounded-lg border border-zinc-200 dark:border-zinc-700 px-3 text-[9px] font-black uppercase tracking-wider text-zinc-600 dark:text-zinc-300 disabled:opacity-40"
                        >
                          Renouveler
                        </button>
                      </>
                    ) : (
                      <span className="h-9 inline-flex items-center rounded-lg bg-emerald-500/10 px-3 text-[9px] font-black uppercase tracking-wider text-emerald-500">
                        À vie
                      </span>
                    )}

                    {renewingID === String(item.id) && item.plan !== 'lifetime' && (
                      <div className="w-full rounded-xl border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950/70 p-3">
                        <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-2">
                          <label>
                            <div className="mb-1 text-[8px] font-black uppercase tracking-wider text-zinc-400">Montant encaissé (€)</div>
                            <input
                              value={renewAmount}
                              onChange={(e) => setRenewAmount(e.target.value)}
                              inputMode="decimal"
                              placeholder="Ex. 9,99"
                              className="h-9 w-full rounded-lg border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 text-xs font-bold outline-none"
                            />
                          </label>
                          <label>
                            <div className="mb-1 text-[8px] font-black uppercase tracking-wider text-zinc-400">Statut</div>
                            <select
                              value={renewPaymentStatus}
                              onChange={(e) => setRenewPaymentStatus(e.target.value as typeof renewPaymentStatus)}
                              className="h-9 w-full rounded-lg border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-2 text-[9px] font-black uppercase outline-none"
                            >
                              <option value="paid">Payé</option>
                              <option value="pending">En attente</option>
                              <option value="offered">Offert</option>
                              <option value="free">Free</option>
                            </select>
                          </label>
                          <label className="sm:col-span-2">
                            <div className="mb-1 text-[8px] font-black uppercase tracking-wider text-zinc-400">Note renouvellement</div>
                            <input
                              value={renewNote}
                              onChange={(e) => setRenewNote(e.target.value)}
                              maxLength={1000}
                              placeholder="Optionnel"
                              className="h-9 w-full rounded-lg border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 text-xs font-semibold outline-none"
                            />
                          </label>
                        </div>
                        <div className="mt-3 flex gap-2">
                          <button
                            type="button"
                            disabled={actionID === item.id}
                            onClick={() => void renewLicense(item)}
                            className="h-9 rounded-lg bg-zinc-950 dark:bg-white px-4 text-[9px] font-black uppercase tracking-wider text-white dark:text-zinc-950 disabled:opacity-40"
                          >
                            Confirmer le renouvellement
                          </button>
                          <button
                            type="button"
                            disabled={actionID === item.id}
                            onClick={cancelRenewal}
                            className="h-9 rounded-lg border border-zinc-200 dark:border-zinc-700 px-3 text-[9px] font-black uppercase tracking-wider text-zinc-500 disabled:opacity-40"
                          >
                            Annuler
                          </button>
                        </div>
                      </div>
                    )}

                    <button
                      type="button"
                      disabled={actionID === item.id || !item.machine_id || isCurrentAdminLicense(item)}
                      onClick={() => void resetMachine(item)}
                      className="h-9 rounded-lg border border-zinc-200 dark:border-zinc-700 px-3 text-[9px] font-black uppercase tracking-wider text-zinc-600 dark:text-zinc-300 disabled:opacity-30"
                    >
                      Reset PC
                    </button>

                    <button
                      type="button"
                      disabled={actionID === item.id || isCurrentAdminLicense(item)}
                      onClick={() => void setActive(item, item.active === false)}
                      className={
                        'h-9 rounded-lg border px-3 text-[9px] font-black uppercase tracking-wider disabled:opacity-40 ' +
                        (item.active === false
                          ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-500'
                          : 'border-rose-500/30 bg-rose-500/10 text-rose-500')
                      }
                    >
                      {actionID === item.id ? '…' : item.active === false ? 'Réactiver' : 'Révoquer'}
                    </button>
                  </div>
                )}
              </div>
            ))}
            {!busy && filteredLicenses.length === 0 && (
              <div className="p-8 text-center text-sm font-semibold text-zinc-400">Aucune licence trouvée.</div>
            )}
          </div>
        </section>
      )}
    </div>
  );
};

export default DeveloperView;
