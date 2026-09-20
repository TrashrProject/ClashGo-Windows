
import React from 'react';
import { GetPlayerProfile, SaveAccountConfig } from '../../wailsjs/go/main/App';

interface Props { onLinked: (tag: string) => void }

const AccountOnboarding: React.FC<Props> = ({ onLinked }) => {
  const [tag, setTag] = React.useState('');
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState('');

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const raw = tag.trim();
    if (!raw) return;
    setBusy(true); setError('');
    try {
      await SaveAccountConfig(raw);

      // Best-effort first sync before closing onboarding. A successful sync
      // lets the backend select the correct HDV farm profile immediately, so
      // the user's next action can simply be START BOT. Linking still works
      // offline: the profile call may fail and background retry will handle it.
      try {
        await GetPlayerProfile();
      } catch (syncErr) {
        console.warn('Initial account sync deferred:', syncErr);
      }

      const normalized = raw.startsWith('#') ? raw.toUpperCase() : '#' + raw.toUpperCase();
      onLinked(normalized);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-[100] bg-zinc-950/70 backdrop-blur-md flex items-center justify-center p-5 no-drag">
      <form onSubmit={submit} className="w-full max-w-lg rounded-[2.5rem] bg-white dark:bg-zinc-900 border border-zinc-200/60 dark:border-zinc-800 p-8 shadow-2xl">
        <div className="w-14 h-14 rounded-2xl bg-zinc-950 dark:bg-white text-white dark:text-zinc-950 flex items-center justify-center mb-6">
          <span className="material-symbols-outlined text-2xl">person_search</span>
        </div>
        <div className="text-[10px] font-black uppercase tracking-[0.25em] text-zinc-400">Welcome to ClashGO</div>
        <h2 className="mt-2 text-3xl font-black tracking-tight text-zinc-950 dark:text-white">Link your Clash account</h2>
        <p className="mt-3 text-sm font-medium leading-6 text-zinc-500">Enter your player tag once. ClashGO will use it for your account dashboard, TH profile and farm configuration.</p>

        <label className="block mt-7 text-[10px] font-black uppercase tracking-[0.2em] text-zinc-500">Player tag</label>
        <div className="mt-2 relative">
          <span className="absolute left-4 top-1/2 -translate-y-1/2 font-black text-zinc-400">#</span>
          <input autoFocus value={tag.replace(/^#/, '')} onChange={e => setTag(e.target.value.replace(/^#/, '').toUpperCase())}
            placeholder="PLAYER TAG" autoComplete="off"
            className="w-full rounded-2xl border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-950 pl-9 pr-4 py-4 text-lg font-mono font-black uppercase tracking-wider outline-none focus:ring-4 focus:ring-zinc-950/5 dark:focus:ring-white/5" />
        </div>
        <p className="mt-2 text-[11px] font-medium text-zinc-400">Found in your Clash of Clans profile. No Supercell password is requested.</p>
        {error && <div className="mt-4 rounded-xl bg-rose-500/10 px-4 py-3 text-xs font-bold text-rose-500">{error}</div>}
        <button type="submit" disabled={busy || !tag.trim()} className="mt-7 w-full rounded-2xl bg-zinc-950 dark:bg-white text-white dark:text-zinc-950 py-4 text-sm font-black tracking-wide disabled:opacity-40">
          {busy ? 'Preparing ClashGO...' : 'Continue'}
        </button>
      </form>
    </div>
  );
};

export default AccountOnboarding;
