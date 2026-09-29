import React from 'react';
import {
  AdminRenewLicense,
  AdminResetLicenseMachine,
  AdminSetLicenseActive,
  AdminSetLicenseRole,
  CreateAdminLicense,
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
};

type LicenseState = {
  activated?: boolean;
  role?: 'member' | 'developer' | 'admin' | '';
  license_hint?: string;
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

const DeveloperView: React.FC = () => {
  const [incidents, setIncidents] = React.useState<Incident[]>([]);
  const [licenses, setLicenses] = React.useState<LicenseRow[]>([]);
  const [role, setRole] = React.useState<LicenseState['role']>('');
  const [currentLicenseHint, setCurrentLicenseHint] = React.useState('');
  const [busy, setBusy] = React.useState(false);
  const [creating, setCreating] = React.useState(false);
  const [actionID, setActionID] = React.useState('');
  const [error, setError] = React.useState('');
  const [notice, setNotice] = React.useState('');
  const [tab, setTab] = React.useState<'incidents' | 'licenses'>('licenses');

  const [newRole, setNewRole] = React.useState<'member' | 'developer' | 'admin'>('member');
  const [newPlan, setNewPlan] = React.useState<'free_2d' | 'week_1' | 'month_1' | 'lifetime'>('month_1');
  const [customerName, setCustomerName] = React.useState('');
  const [customerContact, setCustomerContact] = React.useState('');
  const [generatedKey, setGeneratedKey] = React.useState('');
  const [copied, setCopied] = React.useState(false);
  const [search, setSearch] = React.useState('');
  const [licenseFilter, setLicenseFilter] = React.useState<'all' | 'active' | 'revoked'>('all');
  const [renewPlans, setRenewPlans] = React.useState<Record<string, 'free_2d' | 'week_1' | 'month_1' | 'lifetime'>>({});
  const generatedKeyTimerRef = React.useRef<number | null>(null);

  const isAdmin = role === 'admin';

  const refresh = React.useCallback(async () => {
    setBusy(true);
    setError('');
    try {
      const [state, i, l] = await Promise.all([
        GetLicenseState(),
        GetDeveloperIncidents(),
        GetDeveloperLicenses(),
      ]);
      setRole((state as LicenseState)?.role || '');
      setCurrentLicenseHint((state as LicenseState)?.license_hint || '');
      setIncidents((i || []) as Incident[]);
      setLicenses((l || []) as LicenseRow[]);
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
      const result = await CreateAdminLicense({
        role: newRole,
        plan: newPlan,
        customer_name: customerName.trim(),
        customer_contact: customerContact.trim(),
      } as any);
      const keys = (result as { licenses?: string[] })?.licenses || [];
      if (keys.length === 0) throw new Error('Aucune clé retournée par le serveur.');
      setGeneratedKey(keys[0]);
      setNotice('Licence créée. Copie la clé maintenant : elle sera masquée automatiquement dans 2 minutes.');
      if (generatedKeyTimerRef.current !== null) {
        window.clearTimeout(generatedKeyTimerRef.current);
      }
      generatedKeyTimerRef.current = window.setTimeout(() => {
        setGeneratedKey('');
        setCopied(false);
        generatedKeyTimerRef.current = null;
      }, 120000);
      setCustomerName('');
      setCustomerContact('');
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setCreating(false);
    }
  };

  const runLicenseAction = async (id: string, action: () => Promise<unknown>, success: string) => {
    if (!isAdmin || !id || actionID) return;
    setActionID(id);
    setError('');
    setNotice('');
    try {
      await action();
      setNotice(success);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
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
    await runLicenseAction(
      id,
      () => AdminRenewLicense(id, plan),
      'Licence renouvelée en ' + planLabel(plan) + ' · même clé conservée.'
    );
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

  const copyGeneratedKey = async () => {
    if (!generatedKey) return;
    await copyText(generatedKey);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  };

  const recentErrors = incidents.filter((x) => x.level === 'error' || x.level === 'fatal' || x.level === 'panic');
  const activeLicenses = licenses.filter((x) => x.active !== false).length;
  const normalizedSearch = search.trim().toLowerCase();
  const filteredLicenses = licenses.filter((item) => {
    if (licenseFilter === 'active' && item.active === false) return false;
    if (licenseFilter === 'revoked' && item.active !== false) return false;
    if (!normalizedSearch) return true;
    return [
      item.customer_name,
      item.customer_contact,
      item.hint,
      item.id,
      item.machine_id,
      item.app_version,
      item.role,
      planLabel(item.plan),
    ].some((value) => String(value || '').toLowerCase().includes(normalizedSearch));
  });
  const filteredIncidents = incidents.filter((item) => {
    if (!normalizedSearch) return true;
    return [
      item.license_hint,
      item.machine_id,
      item.app_version,
      item.level,
      item.message,
    ].some((value) => String(value || '').toLowerCase().includes(normalizedSearch));
  });

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

        <div className="mt-6 grid grid-cols-3 gap-3">
          {[
            ['Licences', licenses.length],
            ['Actives', activeLicenses],
            ['Erreurs', recentErrors.length],
          ].map(([label, value]) => (
            <div key={String(label)} className="rounded-2xl bg-white/5 dark:bg-zinc-100 p-4">
              <div className="text-2xl font-black">{value}</div>
              <div className="mt-1 text-[9px] font-black uppercase tracking-widest text-zinc-400 dark:text-zinc-500">{label}</div>
            </div>
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
                  {copied ? 'Copiée ✓' : 'Copier'}
                </button>
                <button
                  type="button"
                  onClick={() => {
                    if (generatedKeyTimerRef.current !== null) {
                      window.clearTimeout(generatedKeyTimerRef.current);
                      generatedKeyTimerRef.current = null;
                    }
                    setGeneratedKey('');
                    setCopied(false);
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
              placeholder={tab === 'licenses' ? 'Client, contact, licence, PC…' : 'Licence, version, erreur…'}
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
          </div>
          <div className="divide-y divide-zinc-100 dark:divide-zinc-800">
            {filteredIncidents.slice().reverse().map((item, index) => (
              <div key={item.id || String(index)} className="p-5 flex flex-col lg:flex-row lg:items-start gap-4">
                <div className="lg:w-44 shrink-0">
                  <div className="text-xs font-black text-zinc-900 dark:text-white">{item.license_hint || 'Licence inconnue'}</div>
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
                    {Boolean(currentLicenseHint && item.hint === currentLicenseHint) && (
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
                </div>
                <div className="lg:text-right">
                  <div className={'text-[10px] font-black uppercase tracking-widest ' + (item.active !== false ? 'text-emerald-500' : 'text-rose-500')}>
                    {item.active !== false ? 'Active' : 'Révoquée'}
                  </div>
                  <div className="mt-1 text-[10px] text-zinc-400">
                    {item.expires_at ? 'Expire ' + dateLabel(item.expires_at) : 'Sans expiration'}
                  </div>
                  <div className="mt-1 text-[10px] text-zinc-400">{item.app_version || 'Jamais connectée'}</div>
                </div>

                {isAdmin && item.id && (
                  <div className="lg:w-full xl:w-auto xl:min-w-[390px] flex flex-wrap items-center gap-2 lg:justify-end">
                    <select
                      value={(item.role === 'developer' || item.role === 'admin') ? item.role : 'member'}
                      disabled={actionID === item.id || Boolean(currentLicenseHint && item.hint === currentLicenseHint)}
                      onChange={(e) => void setLicenseRole(item, e.target.value as 'member' | 'developer' | 'admin')}
                      className="h-9 rounded-lg border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 px-2 text-[9px] font-black uppercase tracking-wider outline-none disabled:opacity-40"
                    >
                      <option value="member">Membre</option>
                      <option value="developer">Développeur</option>
                      <option value="admin">Admin</option>
                    </select>

                    <select
                      value={renewPlans[String(item.id)] || ((item.plan === 'free_2d' || item.plan === 'week_1' || item.plan === 'month_1' || item.plan === 'lifetime') ? item.plan : 'month_1')}
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
                      <option value="lifetime">À vie</option>
                    </select>

                    <button
                      type="button"
                      disabled={actionID === item.id}
                      onClick={() => void renewLicense(item)}
                      className="h-9 rounded-lg border border-zinc-200 dark:border-zinc-700 px-3 text-[9px] font-black uppercase tracking-wider text-zinc-600 dark:text-zinc-300 disabled:opacity-40"
                    >
                      Renouveler
                    </button>

                    <button
                      type="button"
                      disabled={actionID === item.id || !item.machine_id || Boolean(currentLicenseHint && item.hint === currentLicenseHint)}
                      onClick={() => void resetMachine(item)}
                      className="h-9 rounded-lg border border-zinc-200 dark:border-zinc-700 px-3 text-[9px] font-black uppercase tracking-wider text-zinc-600 dark:text-zinc-300 disabled:opacity-30"
                    >
                      Reset PC
                    </button>

                    <button
                      type="button"
                      disabled={actionID === item.id || Boolean(currentLicenseHint && item.hint === currentLicenseHint)}
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
