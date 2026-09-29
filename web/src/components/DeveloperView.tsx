import React from 'react';
import { GetDeveloperIncidents, GetDeveloperLicenses } from '../../wailsjs/go/main/App';

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
  hint?: string;
  role?: string;
  active?: boolean;
  machine_id?: string;
  created_at?: string;
  last_seen_at?: string;
  app_version?: string;
};

const DeveloperView: React.FC = () => {
  const [incidents, setIncidents] = React.useState<Incident[]>([]);
  const [licenses, setLicenses] = React.useState<LicenseRow[]>([]);
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState('');
  const [tab, setTab] = React.useState<'incidents' | 'licenses'>('incidents');

  const refresh = React.useCallback(async () => {
    setBusy(true);
    setError('');
    try {
      const [i, l] = await Promise.all([
        GetDeveloperIncidents(),
        GetDeveloperLicenses(),
      ]);
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
    return () => window.clearInterval(id);
  }, [refresh]);

  const recentErrors = incidents.filter((x) => x.level === 'error' || x.level === 'fatal' || x.level === 'panic');

  return (
    <div className="space-y-6 max-w-6xl mx-auto">
      <section className="rounded-[2.25rem] bg-zinc-950 dark:bg-white text-white dark:text-zinc-950 p-7 shadow-premium-lg">
        <div className="flex flex-col lg:flex-row lg:items-end lg:justify-between gap-5">
          <div>
            <div className="text-[10px] font-black uppercase tracking-[0.25em] text-zinc-400 dark:text-zinc-500">Developer support</div>
            <h2 className="mt-2 text-3xl font-black tracking-tight">Remote diagnostics</h2>
            <p className="mt-2 max-w-2xl text-sm font-semibold text-zinc-400 dark:text-zinc-600">
              Automatic ClashGO incidents grouped by license, machine and app version.
            </p>
          </div>
          <button
            type="button"
            onClick={() => void refresh()}
            disabled={busy}
            className="h-11 px-5 rounded-xl bg-white dark:bg-zinc-950 text-zinc-950 dark:text-white text-[10px] font-black uppercase tracking-widest disabled:opacity-40"
          >
            {busy ? 'Refreshing…' : 'Refresh'}
          </button>
        </div>

        <div className="mt-6 grid grid-cols-3 gap-3">
          {[
            ['Licenses', licenses.length],
            ['Incidents', incidents.length],
            ['Errors', recentErrors.length],
          ].map(([label, value]) => (
            <div key={String(label)} className="rounded-2xl bg-white/5 dark:bg-zinc-100 p-4">
              <div className="text-2xl font-black">{value}</div>
              <div className="mt-1 text-[9px] font-black uppercase tracking-widest text-zinc-400 dark:text-zinc-500">{label}</div>
            </div>
          ))}
        </div>
      </section>

      <div className="flex items-center gap-2">
        {(['incidents', 'licenses'] as const).map((item) => (
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
            {item}
          </button>
        ))}
      </div>

      {error && (
        <div className="rounded-2xl bg-rose-500/10 border border-rose-500/20 px-4 py-3 text-sm font-bold text-rose-500">
          {error}
        </div>
      )}

      {tab === 'incidents' ? (
        <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-premium dark:shadow-none overflow-hidden">
          <div className="p-6 border-b border-zinc-100 dark:border-zinc-800">
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Automatic error feed</div>
            <h3 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">Latest incidents</h3>
          </div>
          <div className="divide-y divide-zinc-100 dark:divide-zinc-800">
            {incidents.slice().reverse().map((item, index) => (
              <div key={item.id || String(index)} className="p-5 flex flex-col lg:flex-row lg:items-start gap-4">
                <div className="lg:w-40 shrink-0">
                  <div className="text-xs font-black text-zinc-900 dark:text-white">{item.license_hint || 'Unknown license'}</div>
                  <div className="mt-1 text-[10px] font-mono text-zinc-400">{item.app_version || 'unknown version'}</div>
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
                    <span className="text-[10px] font-semibold text-zinc-400">{item.received_at || item.at}</span>
                  </div>
                  <div className="mt-2 text-sm font-bold text-zinc-800 dark:text-zinc-100 break-words">{item.message || 'No message'}</div>
                  {item.machine_id && (
                    <div className="mt-2 text-[10px] font-mono text-zinc-400 truncate">machine {item.machine_id}</div>
                  )}
                </div>
              </div>
            ))}
            {!busy && incidents.length === 0 && (
              <div className="p-8 text-center text-sm font-semibold text-zinc-400">No incidents received yet.</div>
            )}
          </div>
        </section>
      ) : (
        <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-premium dark:shadow-none overflow-hidden">
          <div className="p-6 border-b border-zinc-100 dark:border-zinc-800">
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Licensed installations</div>
            <h3 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">Users & machines</h3>
          </div>
          <div className="divide-y divide-zinc-100 dark:divide-zinc-800">
            {licenses.map((item, index) => (
              <div key={(item.hint || 'license') + index} className="p-5 flex flex-col md:flex-row md:items-center gap-4">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-black text-zinc-900 dark:text-white">{item.hint || 'Hidden license'}</span>
                    <span className="px-2 py-1 rounded-lg bg-zinc-100 dark:bg-zinc-800 text-[9px] font-black uppercase tracking-widest text-zinc-500">{item.role || 'member'}</span>
                  </div>
                  <div className="mt-1 text-[10px] font-mono text-zinc-400 truncate">
                    {item.machine_id ? 'machine ' + item.machine_id : 'not activated'}
                  </div>
                </div>
                <div className="md:text-right">
                  <div className={'text-[10px] font-black uppercase tracking-widest ' + (item.active ? 'text-emerald-500' : 'text-rose-500')}>
                    {item.active ? 'Active' : 'Revoked'}
                  </div>
                  <div className="mt-1 text-[10px] text-zinc-400">{item.app_version || 'never connected'}</div>
                </div>
              </div>
            ))}
            {!busy && licenses.length === 0 && (
              <div className="p-8 text-center text-sm font-semibold text-zinc-400">No licenses found.</div>
            )}
          </div>
        </section>
      )}
    </div>
  );
};

export default DeveloperView;
