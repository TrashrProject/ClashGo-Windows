import React from 'react';
import { CreateAdminLicense, GetDeveloperIncidents, GetDeveloperLicenses, GetLicenseState } from '../../wailsjs/go/main/App';

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
  expires_at?: string;
  customer_name?: string;
};

type LicenseState = {
  activated?: boolean;
  role?: string;
};

type AdminResult = {
  role?: string;
  plan?: string;
  duration_days?: number;
  licenses?: string[];
};

const planLabel = (plan?: string) => {
  if (plan === 'free_2d') return 'FREE · 2 jours';
  if (plan === 'week_1') return '1 semaine';
  if (plan === 'month_1') return '1 mois';
  if (plan === 'lifetime') return 'À vie';
  return 'Non défini';
};

const compactMachine = (value?: string) => {
  if (!value) return 'Non activée';
  if (value.length < 18) return value;
  return value.slice(0, 10) + '…' + value.slice(-6);
};

const formatDate = (value?: string) => {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isFinite(date.getTime()) ? date.toLocaleString('fr-FR') : value;
};

const DeveloperView: React.FC = () => {
  const [incidents, setIncidents] = React.useState<Incident[]>([]);
  const [licenses, setLicenses] = React.useState<LicenseRow[]>([]);
  const [licenseState, setLicenseState] = React.useState<LicenseState | null>(null);
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState('');
  const [tab, setTab] = React.useState<'incidents' | 'licenses' | 'admin'>('incidents');

  const [customerName, setCustomerName] = React.useState('');
  const [customerContact, setCustomerContact] = React.useState('');
  const [newRole, setNewRole] = React.useState<'member' | 'developer' | 'admin'>('member');
  const [newPlan, setNewPlan] = React.useState<'free_2d' | 'week_1' | 'month_1' | 'lifetime'>('month_1');
  const [creating, setCreating] = React.useState(false);
  const [createdKey, setCreatedKey] = React.useState('');
  const [copyState, setCopyState] = React.useState('');

  const refresh = React.useCallback(async () => {
    setBusy(true);
    setError('');
    try {
      const [i, l, state] = await Promise.all([
        GetDeveloperIncidents(),
        GetDeveloperLicenses(),
        GetLicenseState(),
      ]);
      setIncidents((i || []) as Incident[]);
      setLicenses((l || []) as LicenseRow[]);
      setLicenseState((state || null) as LicenseState | null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }, []);

  React.useEffect(() => {
    void refresh();
    const id = window.setInterval(() => void refresh(), 30000);
    return () => window.clearInterval(id);
  }, [refresh]);

  React.useEffect(() => {
    if (licenseState?.role !== 'admin' && tab === 'admin') {
      setTab('incidents');
    }
  }, [licenseState?.role, tab]);

  const recentErrors = incidents.filter((x) => x.level === 'error' || x.level === 'fatal' || x.level === 'panic');
  const isAdmin = licenseState?.role === 'admin';

  const createLicense = async () => {
    if (!isAdmin || creating) return;
    setCreating(true);
    setError('');
    setCreatedKey('');
    setCopyState('');
    try {
      const result = await CreateAdminLicense({
        role: newRole,
        plan: newPlan,
        customer_name: customerName.trim(),
        customer_contact: customerContact.trim(),
      } as any) as AdminResult;
      const key = result?.licenses?.[0] || '';
      if (!key) throw new Error('Aucune clé reçue du serveur.');
      setCreatedKey(key);
      setCustomerName('');
      setCustomerContact('');
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setCreating(false);
    }
  };

  const copyKey = async () => {
    if (!createdKey) return;
    try {
      await navigator.clipboard.writeText(createdKey);
      setCopyState('Clé copiée');
    } catch {
      setCopyState('Copie impossible · sélectionne la clé');
    }
  };

  const tabs: Array<['incidents' | 'licenses' | 'admin', string]> = [
    ['incidents', 'Incidents'],
    ['licenses', 'Licences'],
    ...(isAdmin ? [['admin', 'Créer une licence'] as ['admin', string]] : []),
  ];

  return (
    <div className="space-y-6 max-w-6xl mx-auto">
      <section className="rounded-[2.25rem] bg-zinc-950 dark:bg-white text-white dark:text-zinc-950 p-7 shadow-premium-lg">
        <div className="flex flex-col lg:flex-row lg:items-end lg:justify-between gap-5">
          <div>
            <div className="text-[10px] font-black uppercase tracking-[0.25em] text-zinc-400 dark:text-zinc-500">CLASHGO SUPPORT</div>
            <h2 className="mt-2 text-3xl font-black tracking-tight">{isAdmin ? 'Administration & diagnostic' : 'Diagnostic distant'}</h2>
            <p className="mt-2 max-w-2xl text-sm font-semibold text-zinc-400 dark:text-zinc-600">
              Incidents automatiques, installations actives et outils réservés aux licences Developer/Admin.
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
            ['Incidents', incidents.length],
            ['Erreurs', recentErrors.length],
          ].map(([label, value]) => (
            <div key={String(label)} className="rounded-2xl bg-white/5 dark:bg-zinc-100 p-4">
              <div className="text-2xl font-black">{value}</div>
              <div className="mt-1 text-[9px] font-black uppercase tracking-widest text-zinc-400 dark:text-zinc-500">{label}</div>
            </div>
          ))}
        </div>
      </section>

      <div className="flex flex-wrap items-center gap-2">
        {tabs.map(([id, label]) => (
          <button
            key={id}
            type="button"
            onClick={() => setTab(id)}
            className={
              'px-4 py-2.5 rounded-xl text-[10px] font-black uppercase tracking-widest transition-all ' +
              (tab === id
                ? 'bg-zinc-950 dark:bg-white text-white dark:text-zinc-950'
                : 'bg-white dark:bg-zinc-900 text-zinc-500 border border-zinc-100 dark:border-zinc-800')
            }
          >
            {label}
          </button>
        ))}
      </div>

      {error && (
        <div className="rounded-2xl bg-rose-500/10 border border-rose-500/20 px-4 py-3 text-sm font-bold text-rose-500">
          {error}
        </div>
      )}

      {tab === 'incidents' && (
        <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-premium dark:shadow-none overflow-hidden">
          <div className="p-6 border-b border-zinc-100 dark:border-zinc-800">
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">REMONTÉE AUTOMATIQUE</div>
            <h3 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">Derniers incidents</h3>
          </div>
          <div className="divide-y divide-zinc-100 dark:divide-zinc-800">
            {incidents.slice().reverse().map((item, index) => (
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
                      {item.level || 'error'}
                    </span>
                    <span className="text-[10px] font-semibold text-zinc-400">{formatDate(item.received_at || item.at)}</span>
                  </div>
                  <div className="mt-2 text-sm font-bold text-zinc-800 dark:text-zinc-100 break-words">{item.message || 'Aucun message'}</div>
                  {item.machine_id && (
                    <div className="mt-2 text-[10px] font-mono text-zinc-400">PC {compactMachine(item.machine_id)}</div>
                  )}
                </div>
              </div>
            ))}
            {!busy && incidents.length === 0 && (
              <div className="p-8 text-center text-sm font-semibold text-zinc-400">Aucun incident reçu.</div>
            )}
          </div>
        </section>
      )}

      {tab === 'licenses' && (
        <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-premium dark:shadow-none overflow-hidden">
          <div className="p-6 border-b border-zinc-100 dark:border-zinc-800">
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">INSTALLATIONS AUTORISÉES</div>
            <h3 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">Utilisateurs & machines</h3>
          </div>
          <div className="divide-y divide-zinc-100 dark:divide-zinc-800">
            {licenses.map((item, index) => (
              <div key={(item.id || item.hint || 'license') + index} className="p-5 flex flex-col lg:flex-row lg:items-center gap-4">
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-black text-zinc-900 dark:text-white">{item.customer_name || item.hint || 'Licence masquée'}</span>
                    <span className="px-2 py-1 rounded-lg bg-zinc-100 dark:bg-zinc-800 text-[9px] font-black uppercase tracking-widest text-zinc-500">{item.role || 'member'}</span>
                    <span className="px-2 py-1 rounded-lg bg-zinc-100 dark:bg-zinc-800 text-[9px] font-black uppercase tracking-widest text-zinc-500">{planLabel(item.plan)}</span>
                  </div>
                  <div className="mt-1 text-[10px] font-mono text-zinc-400">
                    {item.hint || '••••'} · {compactMachine(item.machine_id)}
                  </div>
                  {item.expires_at && (
                    <div className="mt-1 text-[10px] font-semibold text-zinc-400">Expire le {formatDate(item.expires_at)}</div>
                  )}
                </div>
                <div className="lg:text-right">
                  <div className={'text-[10px] font-black uppercase tracking-widest ' + (item.active ? 'text-emerald-500' : 'text-rose-500')}>
                    {item.active ? 'Active' : 'Révoquée'}
                  </div>
                  <div className="mt-1 text-[10px] text-zinc-400">{item.app_version || 'Jamais connectée'}</div>
                </div>
              </div>
            ))}
            {!busy && licenses.length === 0 && (
              <div className="p-8 text-center text-sm font-semibold text-zinc-400">Aucune licence trouvée.</div>
            )}
          </div>
        </section>
      )}

      {tab === 'admin' && isAdmin && (
        <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6 shadow-premium dark:shadow-none">
          <div className="max-w-3xl">
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">ADMINISTRATION</div>
            <h3 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">Créer une licence</h3>
            <p className="mt-2 text-sm font-semibold text-zinc-500">
              La durée commence à la première activation sur le PC du membre. La clé complète n’est affichée qu’au moment de sa création.
            </p>

            <div className="mt-6 grid grid-cols-1 md:grid-cols-2 gap-3">
              <label className="space-y-2">
                <span className="text-[10px] font-black uppercase tracking-widest text-zinc-400">Nom / pseudo client</span>
                <input
                  value={customerName}
                  onChange={(e) => setCustomerName(e.target.value)}
                  placeholder="Ex. Kevin"
                  maxLength={120}
                  className="w-full h-12 rounded-xl border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 px-4 text-sm font-bold outline-none"
                />
              </label>
              <label className="space-y-2">
                <span className="text-[10px] font-black uppercase tracking-widest text-zinc-400">Discord / contact</span>
                <input
                  value={customerContact}
                  onChange={(e) => setCustomerContact(e.target.value)}
                  placeholder="Optionnel"
                  maxLength={180}
                  className="w-full h-12 rounded-xl border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 px-4 text-sm font-bold outline-none"
                />
              </label>
              <label className="space-y-2">
                <span className="text-[10px] font-black uppercase tracking-widest text-zinc-400">Rôle</span>
                <select
                  value={newRole}
                  onChange={(e) => setNewRole(e.target.value as typeof newRole)}
                  className="w-full h-12 rounded-xl border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 px-4 text-sm font-black"
                >
                  <option value="member">Membre</option>
                  <option value="developer">Développeur</option>
                  <option value="admin">Admin</option>
                </select>
              </label>
              <label className="space-y-2">
                <span className="text-[10px] font-black uppercase tracking-widest text-zinc-400">Durée</span>
                <select
                  value={newPlan}
                  onChange={(e) => setNewPlan(e.target.value as typeof newPlan)}
                  className="w-full h-12 rounded-xl border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 px-4 text-sm font-black"
                >
                  <option value="free_2d">FREE · 2 jours</option>
                  <option value="week_1">1 semaine</option>
                  <option value="month_1">1 mois</option>
                  <option value="lifetime">À vie</option>
                </select>
              </label>
            </div>

            <button
              type="button"
              onClick={() => void createLicense()}
              disabled={creating}
              className="mt-5 h-12 rounded-xl bg-zinc-950 dark:bg-white px-6 text-[10px] font-black uppercase tracking-widest text-white dark:text-zinc-950 disabled:opacity-40"
            >
              {creating ? 'Création…' : 'Générer la licence'}
            </button>

            {createdKey && (
              <div className="mt-5 rounded-2xl border border-emerald-500/20 bg-emerald-500/10 p-4">
                <div className="text-[9px] font-black uppercase tracking-[0.2em] text-emerald-500">CLÉ CRÉÉE · À COPIER MAINTENANT</div>
                <div className="mt-3 rounded-xl bg-zinc-950 dark:bg-white px-4 py-3 font-mono text-sm font-black text-white dark:text-zinc-950 break-all select-all">
                  {createdKey}
                </div>
                <div className="mt-3 flex flex-wrap items-center gap-3">
                  <button
                    type="button"
                    onClick={() => void copyKey()}
                    className="h-10 rounded-xl bg-emerald-500 px-4 text-[10px] font-black uppercase tracking-widest text-white"
                  >
                    Copier la clé
                  </button>
                  {copyState && <span className="text-[10px] font-bold text-emerald-600 dark:text-emerald-400">{copyState}</span>}
                </div>
              </div>
            )}
          </div>
        </section>
      )}
    </div>
  );
};

export default DeveloperView;
