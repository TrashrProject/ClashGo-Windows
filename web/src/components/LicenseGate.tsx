import React from 'react';
import { ActivateLicense, GetLicensePolicy, GetLicenseState, RefreshLicense } from '../../wailsjs/go/main/App';
import logo from '../assets/images/clashgo-logo.png';

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

const friendlyLicenseError = (value: unknown): string => {
  const raw = value instanceof Error ? value.message : String(value || '');
  const text = raw.toLowerCase();
  if (text.includes('already activated on another machine') || text.includes('machine mismatch')) {
    return 'Cette licence est déjà liée à un autre PC. Demande une réinitialisation de la machine.';
  }
  if (text.includes('expired')) {
    return 'Cette licence a expiré. Elle doit être renouvelée avant de pouvoir utiliser ClashGO.';
  }
  if (text.includes('invalid') || text.includes('revoked')) {
    return 'Cette licence est invalide ou a été désactivée.';
  }
  if (text.includes('not configured')) {
    return 'Le service de licence ClashGO n’est pas encore configuré.';
  }
  if (text.includes('deadline exceeded') || text.includes('timeout') || text.includes('timed out')) {
    return 'Le serveur de licence met trop de temps à répondre. Réessaie dans quelques secondes.';
  }
  if (text.includes('service') || text.includes('network') || text.includes('fetch') || text.includes('connection') || text.includes('no such host')) {
    return 'Impossible de joindre le service de licence. Vérifie ta connexion Internet puis réessaie.';
  }
  if (text.includes('http 5') || text.includes('internal server error')) {
    return 'Le service de licence rencontre un problème temporaire. Réessaie dans quelques instants.';
  }
  if (text.includes('decode license response')) {
    return 'Réponse du service de licence invalide. Réessaie dans quelques instants.';
  }
  return raw || 'Impossible de vérifier la licence pour le moment.';
};

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
      setError(friendlyLicenseError(e));
    } finally {
      setBusy(false);
    }
  }, [onReady]);

  React.useEffect(() => { void refresh(); }, [refresh]);

  const refreshExisting = React.useCallback(async () => {
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      const next = await RefreshLicense();
      const nextState = next as LicenseState;
      setState(nextState);
      if (nextState.activated) {
        onReady(nextState, policy || { enforced: true, service_configured: true });
      } else if (nextState.error) {
        setError(friendlyLicenseError(nextState.error));
      }
    } catch (e) {
      setError(friendlyLicenseError(e));
    } finally {
      setBusy(false);
    }
  }, [busy, onReady, policy]);

  React.useEffect(() => {
    if (!policy?.enforced || state?.activated || !state?.license_hint) return;

    const retry = () => { void refreshExisting(); };
    const timer = window.setInterval(retry, 60_000);
    const onVisible = () => {
      if (document.visibilityState === 'visible') retry();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('focus', retry);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('focus', retry);
    };
  }, [policy?.enforced, state?.activated, state?.license_hint, refreshExisting]);

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
      setError(friendlyLicenseError(e));
    } finally {
      setBusy(false);
    }
  };

  if (busy && !policy) {
    return (
      <div className="w-screen h-screen bg-zinc-950 text-white grid place-items-center">
        <div className="text-center">
          <div className="w-10 h-10 rounded-2xl border-2 border-zinc-700 border-t-white animate-spin mx-auto" />
          <div className="mt-4 text-[10px] font-black uppercase tracking-[0.25em] text-zinc-500">Vérification de la licence</div>
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
          <div className="flex items-center gap-4">
            <div className="grid h-14 w-14 place-items-center rounded-2xl border border-zinc-800 bg-zinc-950">
              <img src={logo} alt="ClashGO" className="h-9 w-9 object-contain invert" />
            </div>
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.28em] text-zinc-500">CLASHGO</div>
              <div className="text-sm font-bold text-zinc-300">Espace membre</div>
            </div>
          </div>
          <h1 className="mt-6 text-4xl font-black tracking-tight">Activer ClashGO</h1>
          <p className="mt-3 text-sm font-semibold leading-6 text-zinc-400">
            Entre la clé qui t’a été fournie. Une licence est liée à un seul PC à la fois.
          </p>
          <div className="mt-5 grid grid-cols-3 gap-2">
            {[
              ['bolt', 'Activation rapide'],
              ['computer', 'Liée à ce PC'],
              ['shield', 'Accès sécurisé'],
            ].map(([icon, label]) => (
              <div key={label} className="rounded-xl border border-zinc-800 bg-zinc-950/60 px-3 py-3 text-center">
                <span className="material-symbols-outlined text-base text-zinc-400">{icon}</span>
                <div className="mt-1 text-[9px] font-black uppercase tracking-wider text-zinc-500">{label}</div>
              </div>
            ))}
          </div>

          <div className="mt-7 space-y-3">
            {!state?.activated && state?.license_hint && (
              <div className="rounded-2xl border border-zinc-700 bg-zinc-950/70 p-4">
                <div className="text-[9px] font-black uppercase tracking-[0.2em] text-zinc-500">Licence déjà enregistrée</div>
                <div className="mt-1 font-mono text-sm font-bold text-zinc-200">{state.license_hint}</div>
                <div className="mt-2 text-xs font-semibold leading-5 text-zinc-500">
                  Si tu viens de renouveler ta licence, aucun nouveau code n’est nécessaire.
                </div>
                <button
                  type="button"
                  onClick={() => void refreshExisting()}
                  disabled={busy}
                  className="mt-4 w-full h-12 rounded-xl bg-emerald-500 text-zinc-950 text-[10px] font-black uppercase tracking-[0.16em] transition active:scale-[0.99] disabled:opacity-40"
                >
                  {busy ? 'Vérification…' : 'J’AI RENOUVELÉ · ACTUALISER'}
                </button>
              </div>
            )}

            {(!state?.license_hint || state?.activated) && (
              <>
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
              {busy ? 'Vérification…' : 'Activer ma licence'}
            </button>
              </>
            )}
          </div>

          {(error || state?.error) && (
            <div className="mt-4 rounded-2xl border border-rose-500/20 bg-rose-500/10 px-4 py-3 text-xs font-bold text-rose-400">
              {friendlyLicenseError(error || state?.error)}
            </div>
          )}

          {!policy?.service_configured && (
            <div className="mt-4 rounded-2xl border border-amber-500/20 bg-amber-500/10 px-4 py-3 text-xs font-bold text-amber-400">
              Le service de licence ClashGO n’est pas encore configuré.
            </div>
          )}

          <div className="mt-6 flex items-center gap-2 text-[10px] font-bold uppercase tracking-widest text-zinc-600">
            <span className="material-symbols-outlined text-sm">lock</span>
            1 licence · 1 PC
          </div>
        </div>
      </div>
    </div>
  );
};

export default LicenseGate;
