import React from 'react';
import { ActivateLicense, GetLicensePolicy, GetLicenseState } from '../../wailsjs/go/main/App';

type LicenseState = {
  activated: boolean;
  role?: 'member' | 'developer' | 'admin' | '';
  license_hint?: string;
  offline_until?: string;
  error?: string;
};

type LicensePolicy = {
  enforced: boolean;
  service_configured: boolean;
  service_url?: string;
};

interface LicenseGateProps {
  onReady: (state: LicenseState, policy: LicensePolicy) => void;
}

const LicenseGate: React.FC<LicenseGateProps> = ({ onReady }) => {
  const [policy, setPolicy] = React.useState<LicensePolicy | null>(null);
  const [state, setState] = React.useState<LicenseState | null>(null);
  const [key, setKey] = React.useState('');
  const [busy, setBusy] = React.useState(true);
  const [error, setError] = React.useState('');

  const refresh = React.useCallback(async () => {
    setBusy(true);
    setError('');
    try {
      const [p, s] = await Promise.all([GetLicensePolicy(), GetLicenseState()]);
      const policyValue = p as LicensePolicy;
      const stateValue = s as LicenseState;
      setPolicy(policyValue);
      setState(stateValue);

      if (!policyValue.enforced || stateValue.activated) {
        onReady(stateValue, policyValue);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }, [onReady]);

  React.useEffect(() => { void refresh(); }, [refresh]);

  const activate = async () => {
    const value = key.trim();
    if (!value || busy) return;
    setBusy(true);
    setError('');
    try {
      const next = await ActivateLicense(value);
      const nextState = next as LicenseState;
      setState(nextState);
      setKey('');
      if (nextState.activated) {
        onReady(nextState, policy || { enforced: true, service_configured: true });
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  if (busy && !policy) {
    return (
      <div className="w-screen h-screen bg-zinc-950 text-white grid place-items-center">
        <div className="text-center">
          <div className="w-10 h-10 rounded-2xl border-2 border-zinc-700 border-t-white animate-spin mx-auto" />
          <div className="mt-4 text-[10px] font-black uppercase tracking-[0.25em] text-zinc-500">Checking ClashGO license</div>
        </div>
      </div>
    );
  }

  if (policy && !policy.enforced) {
    return null;
  }

  return (
    <div className="w-screen h-screen bg-zinc-950 text-white flex items-center justify-center px-5">
      <div className="w-full max-w-xl">
        <div className="rounded-[2.5rem] border border-zinc-800 bg-zinc-900/80 p-7 md:p-9 shadow-2xl">
          <div className="text-[10px] font-black uppercase tracking-[0.28em] text-zinc-500">ClashGO License</div>
          <h1 className="mt-3 text-4xl font-black tracking-tight">Activate ClashGO</h1>
          <p className="mt-3 text-sm font-semibold leading-6 text-zinc-400">
            Enter the license supplied by ClashGO. This activation is linked to one machine.
          </p>

          <div className="mt-7 space-y-3">
            <input
              value={key}
              onChange={(e) => setKey(e.target.value.toUpperCase())}
              onKeyDown={(e) => { if (e.key === 'Enter') void activate(); }}
              placeholder="CGO-XXXXXX-XXXXXX-XXXXXX-XXXXXX"
              autoComplete="off"
              spellCheck={false}
              className="w-full h-14 rounded-2xl border border-zinc-700 bg-zinc-950 px-5 font-mono text-sm text-white outline-none transition focus:border-zinc-500 focus:ring-2 focus:ring-white/10"
            />
            <button
              type="button"
              onClick={() => void activate()}
              disabled={busy || !key.trim()}
              className="w-full h-14 rounded-2xl bg-white text-zinc-950 text-xs font-black uppercase tracking-[0.2em] transition active:scale-[0.99] disabled:opacity-30"
            >
              {busy ? 'Checking…' : 'Activate license'}
            </button>
          </div>

          {(error || state?.error) && (
            <div className="mt-4 rounded-2xl border border-rose-500/20 bg-rose-500/10 px-4 py-3 text-xs font-bold text-rose-400">
              {error || state?.error}
            </div>
          )}

          {!policy?.service_configured && (
            <div className="mt-4 rounded-2xl border border-amber-500/20 bg-amber-500/10 px-4 py-3 text-xs font-bold text-amber-400">
              ClashGO licensing service is not configured.
            </div>
          )}

          <div className="mt-6 flex items-center gap-2 text-[10px] font-bold uppercase tracking-widest text-zinc-600">
            <span className="material-symbols-outlined text-sm">lock</span>
            One license · one machine
          </div>
        </div>
      </div>
    </div>
  );
};

export default LicenseGate;
