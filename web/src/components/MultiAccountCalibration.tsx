import React from 'react';
import {
  CaptureMultiAccountCalibrationFrame,
  SaveMultiAccountCalibration,
} from '../../wailsjs/go/main/App';

type ManagedAccount = {
  id: string;
  label?: string;
  enabled: boolean;
  switch_slot?: number;
};

type CalibrationFrame = {
  data_url: string;
  width: number;
  height: number;
};

type Rect = { x1: number; y1: number; x2: number; y2: number };

type Calibration = {
  version: number;
  width: number;
  height: number;
  settings_button: Rect;
  supercell_id_button: Rect;
  switch_account_button: Rect;
  account_slots: Record<string, Rect>;
};

type Step = {
  key: 'settings_button' | 'supercell_id_button' | 'switch_account_button' | 'slot';
  slot?: number;
  title: string;
  instruction: string;
};

const emptyRect = (): Rect => ({ x1: 0, y1: 0, x2: 0, y2: 0 });

const rectAround = (x: number, y: number, w: number, h: number): Rect => ({
  x1: Math.max(0, Math.round(x - 12)),
  y1: Math.max(0, Math.round(y - 12)),
  x2: Math.min(w, Math.round(x + 12)),
  y2: Math.min(h, Math.round(y + 12)),
});

export default function MultiAccountCalibration({
  accounts,
  disabled,
  calibrated,
  onSaved,
}: {
  accounts: ManagedAccount[];
  disabled: boolean;
  calibrated: boolean;
  onSaved: () => void;
}) {
  const slots = React.useMemo(
    () => Array.from(new Set(
      accounts
        .filter((account) => account.enabled && Number(account.switch_slot || 0) > 0)
        .map((account) => Number(account.switch_slot))
    )).sort((a, b) => a - b),
    [accounts]
  );

  const steps = React.useMemo<Step[]>(() => [
    {
      key: 'settings_button',
      title: '1 · Bouton Réglages',
      instruction: 'Dans BlueStacks, reviens au village principal. Capture puis clique au centre du bouton Réglages.',
    },
    {
      key: 'supercell_id_button',
      title: '2 · Supercell ID',
      instruction: 'Ouvre manuellement les Réglages dans BlueStacks. Recapture puis clique au centre du bouton Supercell ID.',
    },
    {
      key: 'switch_account_button',
      title: '3 · Changer de compte',
      instruction: 'Ouvre manuellement le panneau Supercell ID. Recapture puis clique au centre de Changer de compte.',
    },
    ...slots.map((slot) => ({
      key: 'slot' as const,
      slot,
      title: `Slot ${slot}`,
      instruction: `Ouvre le sélecteur de comptes Supercell ID. Recapture puis clique au centre du compte correspondant au slot ${slot}.`,
    })),
  ], [slots]);

  const [open, setOpen] = React.useState(false);
  const [stepIndex, setStepIndex] = React.useState(0);
  const [frame, setFrame] = React.useState<CalibrationFrame | null>(null);
  const [busy, setBusy] = React.useState(false);
  const [message, setMessage] = React.useState('');
  const [error, setError] = React.useState('');
  const [draft, setDraft] = React.useState<Calibration>({
    version: 1,
    width: 0,
    height: 0,
    settings_button: emptyRect(),
    supercell_id_button: emptyRect(),
    switch_account_button: emptyRect(),
    account_slots: {},
  });

  const reset = () => {
    setStepIndex(0);
    setFrame(null);
    setMessage('');
    setError('');
    setDraft({
      version: 1,
      width: 0,
      height: 0,
      settings_button: emptyRect(),
      supercell_id_button: emptyRect(),
      switch_account_button: emptyRect(),
      account_slots: {},
    });
  };

  const capture = async () => {
    if (disabled || busy) return;
    setBusy(true);
    setError('');
    setMessage('');
    try {
      const value = await CaptureMultiAccountCalibrationFrame() as CalibrationFrame;
      if (!value?.data_url || !value.width || !value.height) {
        throw new Error('Capture de calibration vide.');
      }
      if (draft.width > 0 && (draft.width !== value.width || draft.height !== value.height)) {
        throw new Error(`La résolution a changé pendant la calibration (${draft.width}×${draft.height} → ${value.width}×${value.height}). Recommence la calibration.`);
      }
      setDraft((current) => ({
        ...current,
        width: current.width || value.width,
        height: current.height || value.height,
      }));
      setFrame(value);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const choosePoint = (event: React.MouseEvent<HTMLImageElement>) => {
    if (!frame || busy) return;
    const step = steps[stepIndex];
    if (!step) return;

    const bounds = event.currentTarget.getBoundingClientRect();
    const x = (event.clientX - bounds.left) * (frame.width / bounds.width);
    const y = (event.clientY - bounds.top) * (frame.height / bounds.height);
    const rect = rectAround(x, y, frame.width, frame.height);

    setDraft((current) => {
      if (step.key === 'slot' && step.slot) {
        return {
          ...current,
          account_slots: {
            ...current.account_slots,
            [String(step.slot)]: rect,
          },
        };
      }
      return { ...current, [step.key]: rect };
    });

    const next = stepIndex + 1;
    setStepIndex(next);
    setFrame(null);
    if (next < steps.length) {
      setMessage('Zone enregistrée. Place maintenant BlueStacks sur l’écran demandé à l’étape suivante, puis recapture.');
    } else {
      setMessage('Toutes les zones sont enregistrées. Tu peux sauvegarder la calibration.');
    }
  };

  const save = async () => {
    if (disabled || busy || stepIndex < steps.length || draft.width <= 0 || draft.height <= 0) return;
    setBusy(true);
    setError('');
    try {
      await SaveMultiAccountCalibration(JSON.stringify(draft));
      setMessage('Calibration enregistrée. La rotation multi-compte peut maintenant utiliser ces zones.');
      onSaved();
      setOpen(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const current = steps[stepIndex];

  return (
    <div className="mt-4 rounded-2xl border border-zinc-200 dark:border-zinc-700 bg-white/60 dark:bg-zinc-950/30 p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">Calibration guidée</div>
          <div className="mt-1 text-sm font-black text-zinc-950 dark:text-white">
            {calibrated ? 'Recalibrer le sélecteur Supercell ID' : 'Calibrer le sélecteur Supercell ID'}
          </div>
        </div>
        <button
          type="button"
          disabled={disabled}
          onClick={() => {
            if (!open) reset();
            setOpen((value) => !value);
          }}
          className="rounded-xl border border-zinc-200 dark:border-zinc-700 px-4 py-2 text-[9px] font-black uppercase tracking-widest text-zinc-600 dark:text-zinc-300 disabled:opacity-40"
        >
          {open ? 'Fermer' : calibrated ? 'Recalibrer' : 'Calibrer'}
        </button>
      </div>

      {open && (
        <div className="mt-4 space-y-4">
          {slots.length === 0 && (
            <div className="rounded-xl bg-amber-500/10 px-4 py-3 text-[10px] font-bold text-amber-600 dark:text-amber-400">
              Configure d’abord au moins un compte activé avec un numéro de slot.
            </div>
          )}

          {current && slots.length > 0 && (
            <>
              <div className="rounded-xl bg-sky-500/10 px-4 py-3">
                <div className="text-[10px] font-black uppercase tracking-widest text-sky-600 dark:text-sky-400">{current.title}</div>
                <div className="mt-1 text-xs font-semibold text-zinc-600 dark:text-zinc-300">{current.instruction}</div>
                <div className="mt-2 text-[10px] font-semibold text-zinc-500">
                  ClashGO ne navigue pas automatiquement pendant cette calibration : tu gardes le contrôle de BlueStacks.
                </div>
              </div>

              <button
                type="button"
                onClick={() => void capture()}
                disabled={disabled || busy}
                className="rounded-xl bg-zinc-950 dark:bg-white px-4 py-2.5 text-[10px] font-black uppercase tracking-widest text-white dark:text-zinc-950 disabled:opacity-40"
              >
                {busy ? 'Capture…' : frame ? 'Recapturer cet écran' : 'Capturer cet écran'}
              </button>

              {frame && (
                <div className="overflow-hidden rounded-2xl border border-zinc-200 dark:border-zinc-700 bg-black">
                  <div className="px-3 py-2 text-[9px] font-black uppercase tracking-widest text-zinc-400">
                    Clique au centre de la zone demandée · {frame.width}×{frame.height}
                  </div>
                  <img
                    src={frame.data_url}
                    alt="Capture BlueStacks pour calibration multi-compte"
                    onClick={choosePoint}
                    className="block w-full cursor-crosshair select-none"
                    draggable={false}
                  />
                </div>
              )}
            </>
          )}

          {!current && slots.length > 0 && (
            <div className="rounded-xl bg-emerald-500/10 px-4 py-3 text-[10px] font-bold text-emerald-600 dark:text-emerald-400">
              Calibration complète : {steps.length} zones enregistrées.
            </div>
          )}

          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              onClick={reset}
              disabled={busy}
              className="rounded-xl border border-zinc-200 dark:border-zinc-700 px-4 py-2 text-[9px] font-black uppercase tracking-widest text-zinc-500 disabled:opacity-40"
            >
              Recommencer
            </button>
            <button
              type="button"
              onClick={() => void save()}
              disabled={disabled || busy || stepIndex < steps.length || steps.length === 0}
              className="rounded-xl bg-emerald-500 px-4 py-2 text-[9px] font-black uppercase tracking-widest text-white disabled:opacity-40"
            >
              Enregistrer la calibration
            </button>
          </div>
        </div>
      )}

      {message && <div className="mt-3 text-[10px] font-bold text-emerald-500">{message}</div>}
      {error && <div className="mt-3 text-[10px] font-bold text-rose-500">{error}</div>}
    </div>
  );
}
