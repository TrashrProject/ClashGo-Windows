import React from 'react';
import FarmCompositionPanel from './FarmCompositionPanel';

interface ConfigViewProps {
  goldThreshold: number;
  setGoldThreshold: (v: number) => void;
  elixirThreshold: number;
  setElixirThreshold: (v: number) => void;
  deThreshold: number;
  setDeThreshold: (v: number) => void;
  selectedStrategy: string;
  setSelectedStrategy: (v: string) => void;
  strategies: string[];
  searchEnabled: boolean;
  setSearchEnabled: (v: boolean) => void;
  upgradeWalls: boolean;
  setUpgradeWalls: (v: boolean) => void;
  stallTimer: number;
  setStallTimer: (v: number) => void;
  lootExitEnabled: boolean;
  setLootExitEnabled: (v: boolean) => void;
  lootExitPercent: number;
  setLootExitPercent: (v: number) => void;
  simpleMode: boolean;
  onSetSimpleMode: (enabled: boolean) => Promise<void>;
  simplePreferences: {
    autoDonate: boolean;
    donateOnlyRequested: boolean;
    useHeroes: boolean;
    useClanCastle: boolean;
    waitForFullArmy: boolean;
    autoRetrain: boolean;
    autoUpgradeWalls: boolean;
    lootPreset: 'relaxed' | 'balanced' | 'rich';
  };
  onSaveSimplePreferences: (prefs: {
    autoDonate: boolean;
    donateOnlyRequested: boolean;
    useHeroes: boolean;
    useClanCastle: boolean;
    waitForFullArmy: boolean;
    autoRetrain: boolean;
    autoUpgradeWalls: boolean;
    lootPreset: 'relaxed' | 'balanced' | 'rich';
  }) => Promise<void>;
  // Returns the underlying SaveConfig promise so ConfigView can own
  // the save-status indicator (green flash / red flash + inline
  // "Saved!" / "Save failed" pill) and surface success or failure to
  // the user. Errors thrown by Wails are intentionally surfaced.
  onSave: () => Promise<void>;
}

type SaveStatus = 'idle' | 'saving' | 'saved' | 'error';

// Range bounds for the numeric fields. Out-of-range values get a red
// ring + aria-invalid so the user sees the problem before saving.
const THRESHOLD_MAX = 10_000_000;
const STALL_MAX = 600;
const invalid = (v: number, max: number): boolean =>
  !Number.isFinite(v) || v < 0 || v > max;

const ConfigView: React.FC<ConfigViewProps> = React.memo(({
  goldThreshold, setGoldThreshold,
  elixirThreshold, setElixirThreshold,
  deThreshold, setDeThreshold,
  selectedStrategy, setSelectedStrategy,
  strategies,
  searchEnabled, setSearchEnabled,
  upgradeWalls, setUpgradeWalls,
  stallTimer, setStallTimer,
  lootExitEnabled, setLootExitEnabled,
  lootExitPercent, setLootExitPercent,
  simpleMode,
  onSetSimpleMode,
  simplePreferences,
  onSaveSimplePreferences,
  onSave
}) => {
  const [isOpen, setIsOpen] = React.useState(false);
  const [saveStatus, setSaveStatus] = React.useState<SaveStatus>('idle');
  const [advancedOpen, setAdvancedOpen] = React.useState(false);
  const [simpleModeBusy, setSimpleModeBusy] = React.useState(false);
  const [simplePrefsBusy, setSimplePrefsBusy] = React.useState(false);
  const [simplePrefsStatus, setSimplePrefsStatus] = React.useState<'idle' | 'saved' | 'error'>('idle');
  const [lastSaveError, setLastSaveError] = React.useState<string | null>(null);
  const dropdownRef = React.useRef<HTMLDivElement>(null);
  const savedTimerRef = React.useRef<number | null>(null);
  const errorTimerRef = React.useRef<number | null>(null);

  React.useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
      if (savedTimerRef.current) window.clearTimeout(savedTimerRef.current);
      if (errorTimerRef.current) window.clearTimeout(errorTimerRef.current);
    };
  }, []);

  // Form submit handler — owns the save-state transitions so the user
  // can see the result of their click (green ✓ flash on success,
  // red ✕ flash + inline error on failure). Errors thrown by the
  // Wails SaveConfig IPC are surfaced here rather than being
  // swallowed by App.tsx.
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (saveStatus === 'saving') return;
    if (savedTimerRef.current) { window.clearTimeout(savedTimerRef.current); savedTimerRef.current = null; }
    if (errorTimerRef.current) { window.clearTimeout(errorTimerRef.current); errorTimerRef.current = null; }
    setSaveStatus('saving');
    setLastSaveError(null);
    try {
      await onSave();
      setSaveStatus('saved');
      savedTimerRef.current = window.setTimeout(() => {
        setSaveStatus('idle');
        savedTimerRef.current = null;
      }, 1800);
    } catch (err) {
      setSaveStatus('error');
      setLastSaveError(err instanceof Error ? err.message : String(err));
      errorTimerRef.current = window.setTimeout(() => {
        setSaveStatus('idle');
        setLastSaveError(null);
        errorTimerRef.current = null;
      }, 3000);
    }
  };

  // Visual state for the submit button.
  const saveButtonClasses =
    saveStatus === 'saved'
      ? 'bg-emerald-500 text-white shadow-emerald-500/40'
      : saveStatus === 'error'
        ? 'bg-rose-500 text-white shadow-rose-500/40'
        : saveStatus === 'saving'
          ? 'bg-zinc-700 text-white cursor-wait'
          : 'bg-zinc-950 dark:bg-zinc-800 text-white dark:text-zinc-100 hover:bg-zinc-800 dark:hover:bg-zinc-700 shadow-premium-hover dark:shadow-none';

  const saveButtonIcon =
    saveStatus === 'saving'
      ? 'progress_activity'
      : saveStatus === 'saved'
        ? 'check_circle'
        : saveStatus === 'error'
          ? 'error'
          : 'save';

  const saveButtonLabel =
    saveStatus === 'saving'
      ? 'Saving…'
      : saveStatus === 'saved'
        ? 'Saved'
        : saveStatus === 'error'
          ? 'Failed'
          : 'Save Settings';

  const thresholdItems = [
    { label: 'Min Gold', value: goldThreshold, setter: setGoldThreshold, icon: 'monetization_on', color: 'text-amber-500', bg: 'bg-amber-500/10' },
    { label: 'Min Elixir', value: elixirThreshold, setter: setElixirThreshold, icon: 'water_drop', color: 'text-fuchsia-500', bg: 'bg-fuchsia-500/10' },
    { label: 'Min Dark Elixir', value: deThreshold, setter: setDeThreshold, icon: 'water_drop', color: 'text-zinc-950 dark:text-zinc-100', bg: 'bg-zinc-100 dark:bg-zinc-800' },
  ];

  const stallInvalid = invalid(stallTimer, STALL_MAX);
  const lootExitInvalid = invalid(lootExitPercent, 100);
  const anyInvalid = stallInvalid || lootExitInvalid || thresholdItems.some((t) => invalid(t.value, THRESHOLD_MAX));

  return (
    <div className="max-w-4xl mx-auto">

      <form onSubmit={handleSubmit} className="space-y-8">
        <section className="bg-white dark:bg-zinc-900 p-8 rounded-[3rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none">
          <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-6">
            <div className="max-w-2xl">
              <div className="flex items-center gap-3">
                <div className="w-12 h-12 rounded-2xl bg-emerald-500/10 text-emerald-500 flex items-center justify-center">
                  <span className="material-symbols-outlined">auto_awesome</span>
                </div>
                <div>
                  <h3 className="text-2xl font-black text-zinc-950 dark:text-white tracking-tight">Easy Mode</h3>
                  <p className="text-sm text-zinc-500 font-medium mt-1">
                    Recommended for beginners. Link your Clash account once, start the bot, and ClashGO handles the technical choices automatically.
                  </p>
                </div>
              </div>
              <div className="mt-5 flex flex-wrap gap-2">
                {['Town Hall detected', 'Army chosen automatically', 'Verifies the selected army', 'Loot tracked automatically', 'Account kept in sync'].map((label) => (
                  <span key={label} className="px-3 py-2 rounded-xl bg-zinc-50 dark:bg-zinc-800 text-[10px] font-black uppercase tracking-wider text-zinc-500">
                    {label}
                  </span>
                ))}
              </div>
            </div>

            <button
              type="button"
              disabled={simpleModeBusy}
              onClick={async () => {
                if (simpleModeBusy) return;
                setSimpleModeBusy(true);
                try {
                  await onSetSimpleMode(!simpleMode);
                } finally {
                  setSimpleModeBusy(false);
                }
              }}
              className={`shrink-0 px-6 py-4 rounded-2xl text-sm font-black transition-all border disabled:opacity-50 ${
                simpleMode
                  ? 'bg-emerald-500 text-white border-emerald-400'
                  : 'bg-zinc-950 dark:bg-white text-white dark:text-zinc-950 border-transparent'
              }`}
            >
              {simpleModeBusy ? 'Updating…' : simpleMode ? 'Automatic ✓' : 'Enable Automatic'}
            </button>
          </div>

          {simpleMode && (
            <div className="mt-6 rounded-2xl border border-emerald-500/20 bg-emerald-500/[0.06] p-5">
              <div className="flex items-start gap-4">
                <div className="w-10 h-10 shrink-0 rounded-xl bg-emerald-500 text-white flex items-center justify-center shadow-sm">
                  <span className="material-symbols-outlined text-xl">school</span>
                </div>
                <div className="min-w-0 flex-1">
                  <div className="text-sm font-black text-zinc-950 dark:text-white">New to ClashGO? You only need 3 steps.</div>
                  <div className="mt-4 grid grid-cols-1 md:grid-cols-3 gap-3">
                    {[
                      { n: '1', title: 'Open BlueStacks', text: 'Launch Clash of Clans normally. ClashGO connects to it for you.' },
                      { n: '2', title: 'Link your account', text: 'Your Town Hall and recommended farm army are selected automatically.' },
                      { n: '3', title: 'Press Start', text: 'ClashGO verifies the army recipe, handles village tasks one at a time, finds a base, attacks and recovers by itself.' },
                    ].map((step) => (
                      <div key={step.n} className="rounded-xl border border-emerald-500/15 bg-white/80 dark:bg-zinc-950/40 p-4">
                        <div className="flex items-center gap-2">
                          <span className="w-6 h-6 rounded-full bg-emerald-500 text-white text-[10px] font-black flex items-center justify-center">{step.n}</span>
                          <span className="text-xs font-black text-zinc-900 dark:text-white">{step.title}</span>
                        </div>
                        <p className="mt-2 text-[11px] leading-relaxed text-zinc-500">{step.text}</p>
                      </div>
                    ))}
                  </div>
                  <div className="mt-4 flex items-start gap-2 rounded-xl bg-white/70 dark:bg-zinc-900/60 px-4 py-3 text-[11px] text-zinc-500">
                    <span className="material-symbols-outlined text-base text-emerald-500">info</span>
                    <span>
                      You do not need to understand ADB, OCR, templates, coordinates or internal retry timers. ClashGO keeps those technical details automatic unless you deliberately open Advanced controls.
                    </span>
                  </div>
                </div>
              </div>
            </div>
          )}

          {simpleMode && (
            <div className="mt-6 rounded-2xl border border-zinc-200/70 dark:border-zinc-800 bg-zinc-50/70 dark:bg-zinc-950/30 p-5">
              <div className="flex items-start justify-between gap-4 mb-5">
                <div>
                  <div className="text-base font-black text-zinc-950 dark:text-white">My preferences</div>
                  <div className="text-xs text-zinc-500 mt-1">Choose what ClashGO is allowed to do. Several options can be enabled, but the automation brain executes only one task at a time.</div>
                </div>
                <div className={simplePrefsStatus === 'saved' ? "text-[10px] font-black uppercase tracking-widest text-emerald-500" : simplePrefsStatus === 'error' ? "text-[10px] font-black uppercase tracking-widest text-rose-500" : "text-[10px] font-black uppercase tracking-widest text-zinc-400"}>
                  {simplePrefsBusy ? 'Saving…' : simplePrefsStatus === 'saved' ? 'Saved ✓' : simplePrefsStatus === 'error' ? 'Save failed' : 'Auto-saved'}
                </div>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                {[
                  { key: 'autoRetrain', icon: 'autorenew', title: 'Keep my army recipe ready', text: 'Reapply the selected army recipe automatically when ClashGO detects a mismatch.', value: simplePreferences.autoRetrain },
                  { key: 'autoUpgradeWalls', icon: 'construction', title: 'Spend on walls automatically', text: 'After an attack, queue wall maintenance as its own safe automation task when enabled.', value: simplePreferences.autoUpgradeWalls },
                  { key: 'waitForFullArmy', icon: 'verified', title: 'Verify army before attacking', text: 'Only start matchmaking after the selected army recipe is visually confirmed.', value: simplePreferences.waitForFullArmy },
                  { key: 'useHeroes', icon: 'shield_person', title: 'Use heroes', text: 'Use available heroes during farming attacks.', value: simplePreferences.useHeroes },
                  { key: 'useClanCastle', icon: 'fort', title: 'Use Clan Castle', text: 'Use available Clan Castle reinforcements in attacks.', value: simplePreferences.useClanCastle },
                  { key: 'autoDonate', icon: 'volunteer_activism', title: 'Automatic clan donations', text: 'Donate only when ClashGO can positively match a requested troop. Unknown requests are skipped.', value: simplePreferences.autoDonate },
                ].map((item) => (
                  <button
                    key={item.key}
                    type="button"
                    role="switch"
                    aria-checked={item.value}
                    disabled={simplePrefsBusy}
                    onClick={async () => {
                      if (simplePrefsBusy) return;
                      const next = { ...simplePreferences, [item.key]: !item.value };
                      setSimplePrefsBusy(true);
                      setSimplePrefsStatus('idle');
                      try {
                        await onSaveSimplePreferences(next);
                        setSimplePrefsStatus('saved');
                        window.setTimeout(() => setSimplePrefsStatus('idle'), 1600);
                      } catch {
                        setSimplePrefsStatus('error');
                      } finally {
                        setSimplePrefsBusy(false);
                      }
                    }}
                    className={item.value ? "rounded-2xl border border-emerald-500/30 bg-emerald-500/[0.06] p-4 text-left transition-all disabled:opacity-40" : "rounded-2xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900/60 p-4 text-left transition-all disabled:opacity-40"}
                  >
                    <div className="flex items-start gap-3">
                      <div className={item.value ? "w-10 h-10 rounded-xl flex items-center justify-center shrink-0 bg-emerald-500 text-white" : "w-10 h-10 rounded-xl flex items-center justify-center shrink-0 bg-zinc-100 dark:bg-zinc-800 text-zinc-500"}>
                        <span className="material-symbols-outlined text-lg">{item.icon}</span>
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center justify-between gap-3">
                          <span className="text-sm font-black text-zinc-900 dark:text-white">{item.title}</span>
                          <span className={item.value ? "text-[10px] font-black uppercase tracking-wider text-emerald-500" : "text-[10px] font-black uppercase tracking-wider text-zinc-400"}>{item.value ? 'On' : 'Off'}</span>
                        </div>
                        <p className="mt-1 text-[11px] leading-relaxed text-zinc-500">{item.text}</p>
                      </div>
                    </div>
                  </button>
                ))}
              </div>

              <div className="mt-5">
                <div className="text-xs font-black text-zinc-900 dark:text-white mb-2">Base search</div>
                <div className="grid grid-cols-3 gap-2">
                  {[
                    { key: 'relaxed' as const, label: 'Fast', text: 'More bases' },
                    { key: 'balanced' as const, label: 'Balanced', text: 'Recommended' },
                    { key: 'rich' as const, label: 'Rich only', text: 'More loot' },
                  ].map((preset) => {
                    const active = simplePreferences.lootPreset === preset.key;
                    return (
                      <button
                        key={preset.key}
                        type="button"
                        disabled={simplePrefsBusy}
                        onClick={async () => {
                          if (simplePrefsBusy || active) return;
                          setSimplePrefsBusy(true);
                          setSimplePrefsStatus('idle');
                          try {
                            await onSaveSimplePreferences({ ...simplePreferences, lootPreset: preset.key });
                            setSimplePrefsStatus('saved');
                            window.setTimeout(() => setSimplePrefsStatus('idle'), 1600);
                          } catch {
                            setSimplePrefsStatus('error');
                          } finally {
                            setSimplePrefsBusy(false);
                          }
                        }}
                        className={active ? "rounded-xl border border-emerald-500 bg-emerald-500 px-3 py-3 text-center text-white transition-all" : "rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 px-3 py-3 text-center text-zinc-600 dark:text-zinc-300 transition-all"}
                      >
                        <div className="text-xs font-black">{preset.label}</div>
                        <div className={active ? "text-[9px] mt-1 text-white/80" : "text-[9px] mt-1 text-zinc-400"}>{preset.text}</div>
                      </button>
                    );
                  })}
                </div>
              </div>
            </div>
          )}

          <div className="mt-6 pt-5 border-t border-zinc-100 dark:border-zinc-800 flex items-center justify-between gap-4">
            <div>
              <div className="text-sm font-black text-zinc-900 dark:text-white">Advanced controls</div>
              <div className="text-xs text-zinc-500 mt-1">Optional. Beginners can leave this closed — the defaults are designed to work without manual tuning.</div>
            </div>
            <button
              type="button"
              onClick={() => setAdvancedOpen(v => !v)}
              className="px-4 py-2.5 rounded-xl border border-zinc-200 dark:border-zinc-700 text-xs font-black text-zinc-600 dark:text-zinc-300"
            >
              {advancedOpen ? 'Hide advanced' : 'I know what I’m doing'}
            </button>
          </div>
        </section>

        {(!simpleMode || advancedOpen) && (
          <>
        {/* Resource Thresholds */}
        <div className="bg-white dark:bg-zinc-900 p-8 rounded-[3rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none space-y-10 transition-all duration-500">
          <div className="flex justify-between items-start">
            <div>
              <h3 className="text-2xl font-bold text-zinc-950 dark:text-white mb-2 tracking-tight">Search Settings</h3>
              <p className="text-sm text-zinc-500 dark:text-zinc-500 font-medium">Minimum loot requirements for engagement.</p>
            </div>
            {!searchEnabled && (
              <div className="px-4 py-2 rounded-xl bg-amber-500/10 border border-amber-500/30 text-[10px] font-black text-amber-600 dark:text-amber-400 uppercase tracking-[0.2em] whitespace-nowrap">
                Disabled
              </div>
            )}
          </div>

          {/* Thresholds — three across on md+ */}
          <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
            {thresholdItems.map((item, idx) => {
              const itemInvalid = invalid(item.value, THRESHOLD_MAX);
              return (
                <div key={idx} className="space-y-4">
                  <label className="flex items-center gap-3 text-[11px] font-black text-zinc-500 dark:text-zinc-500 uppercase tracking-[0.2em] px-1">
                    <div className={`w-8 h-8 rounded-xl ${item.bg} flex items-center justify-center border border-zinc-100/10`}>
                      <span className={`material-symbols-outlined text-base ${item.color}`}>{item.icon}</span>
                    </div>
                    {item.label}
                  </label>
                  <div className="relative group">
                    <input
                      type="number"
                      inputMode="numeric"
                      min={0}
                      max={THRESHOLD_MAX}
                      value={item.value}
                      onChange={e => item.setter(parseInt(e.target.value) || 0)}
                      disabled={!searchEnabled}
                      aria-invalid={itemInvalid}
                      className={`w-full bg-zinc-50/50 dark:bg-zinc-950/40 border rounded-2xl py-4 px-6 text-base font-bold text-zinc-900 dark:text-white focus:outline-none focus:ring-4 transition-all tabular-nums ${
                        itemInvalid
                          ? 'border-rose-400/60 focus:border-rose-500 focus:ring-rose-500/10'
                          : 'border-zinc-100 dark:border-zinc-800 focus:ring-zinc-950/5 dark:focus:ring-white/5 focus:border-zinc-300 dark:focus:border-zinc-700'
                      } ${!searchEnabled ? 'opacity-30 cursor-not-allowed' : 'group-hover:bg-white dark:group-hover:bg-zinc-950/60'}`}
                    />
                  </div>
                  {itemInvalid && (
                    <p className="px-1 text-[10px] font-bold text-rose-500 uppercase tracking-widest" role="alert">
                      Max {THRESHOLD_MAX.toLocaleString()}
                    </p>
                  )}
                </div>
              );
            })}
          </div>

          {/* Strategy + stall timer — two across on md+ */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
            <div className="space-y-4">
              <div className="flex items-center justify-between px-1">
                <label className="flex items-center gap-3 text-[11px] font-black text-zinc-500 dark:text-zinc-500 uppercase tracking-[0.2em]">
                  <div className="w-8 h-8 rounded-xl bg-zinc-50 dark:bg-zinc-800 flex items-center justify-center text-zinc-500 dark:text-zinc-500 border border-zinc-100/10">
                    <span className="material-symbols-outlined text-base">precision_manufacturing</span>
                  </div>
                  Attack Strategy
                </label>
                <span className="text-[10px] font-black text-zinc-400 dark:text-zinc-600 uppercase tracking-widest tabular-nums">
                  {(strategies ?? []).length} available
                </span>
              </div>
              <div className="relative" ref={dropdownRef}>
                <div
                  role="combobox"
                  aria-expanded={isOpen}
                  aria-haspopup="listbox"
                  aria-disabled={!searchEnabled}
                  tabIndex={searchEnabled ? 0 : -1}
                  onClick={() => searchEnabled && setIsOpen(!isOpen)}
                  onKeyDown={(e) => {
                    if (!searchEnabled) return;
                    if (e.key === 'Enter' || e.key === ' ' || e.key === 'ArrowDown') {
                      e.preventDefault();
                      setIsOpen(true);
                    } else if (e.key === 'Escape') {
                      setIsOpen(false);
                    }
                  }}
                  className={`w-full bg-zinc-50/50 dark:bg-zinc-950/40 border border-zinc-100 dark:border-zinc-800 rounded-2xl py-4 px-6 text-base font-bold text-zinc-900 dark:text-white cursor-pointer flex justify-between items-center transition-all ${isOpen ? 'ring-4 ring-zinc-950/5 dark:ring-white/5 border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900' : 'hover:bg-white dark:hover:bg-zinc-900'} ${!searchEnabled ? 'opacity-30 cursor-not-allowed' : ''}`}
                >
                  <span className="truncate">
                    {selectedStrategy ? selectedStrategy.split('/').pop()?.replace('.yaml', '').replace('.csv', '') : 'Standard Protocol'}
                  </span>
                  <span className={`material-symbols-outlined text-zinc-500 transition-transform duration-500 ${isOpen ? 'rotate-180 text-zinc-950 dark:text-white' : ''}`}>
                    expand_more
                  </span>
                </div>

                {isOpen && searchEnabled && (
                  <div role="listbox" className="dropdown-pop absolute top-[calc(100%+12px)] left-0 w-full bg-white dark:bg-zinc-900 border border-zinc-100 dark:border-zinc-800 rounded-2xl shadow-premium-lg dark:shadow-2xl z-50 py-3 max-h-72 overflow-y-auto">
                    {(strategies ?? []).map((s, idx) => {
                      const isActive = !!selectedStrategy && selectedStrategy.endsWith(s);
                      return (
                        <div
                          key={idx}
                          role="option"
                          aria-selected={isActive}
                          onClick={() => { setSelectedStrategy(s); setIsOpen(false); }}
                          className={`px-6 py-3 text-sm font-bold cursor-pointer transition-colors ${isActive ? 'bg-zinc-50 dark:bg-zinc-800 text-zinc-950 dark:text-white' : 'text-zinc-500 dark:text-zinc-400 hover:bg-zinc-50 dark:hover:bg-zinc-800/50 hover:text-zinc-950 dark:hover:text-white'}`}
                        >
                          {s.replace('.yaml', '').replace('.csv', '')}
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            </div>

            <div className="space-y-4">
              <label className="flex items-center gap-3 text-[11px] font-black text-zinc-500 dark:text-zinc-500 uppercase tracking-[0.2em] px-1">
                <div className="w-8 h-8 rounded-xl bg-zinc-50 dark:bg-zinc-800 flex items-center justify-center text-zinc-500 dark:text-zinc-500 border border-zinc-100/10">
                  <span className="material-symbols-outlined text-base">timer</span>
                </div>
                Stall Timer (Seconds)
              </label>
              <div className="relative group">
                <input
                  type="number"
                  inputMode="numeric"
                  min={0}
                  max={STALL_MAX}
                  value={stallTimer}
                  onChange={e => setStallTimer(parseInt(e.target.value) || 0)}
                  aria-invalid={stallInvalid}
                  placeholder="0 to disable"
                  className={`w-full bg-zinc-50/50 dark:bg-zinc-950/40 border rounded-2xl py-4 px-6 text-base font-bold text-zinc-900 dark:text-white focus:outline-none focus:ring-4 transition-all tabular-nums ${
                    stallInvalid
                      ? 'border-rose-400/60 focus:border-rose-500 focus:ring-rose-500/10'
                      : 'border-zinc-100 dark:border-zinc-800 focus:ring-zinc-950/5 dark:focus:ring-white/5 focus:border-zinc-300 dark:focus:border-zinc-700'
                  } group-hover:bg-white dark:group-hover:bg-zinc-950/60`}
                />
              </div>
              {stallInvalid && (
                <p className="px-1 text-[10px] font-bold text-rose-500 uppercase tracking-widest" role="alert">
                  Max {STALL_MAX}s
                </p>
              )}
            </div>
          </div>
        </div>

        <FarmCompositionPanel />

        {/* Operational Toggles */}
        <div className="bg-white dark:bg-zinc-900 p-8 rounded-[3rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none space-y-8 transition-all duration-500">
          <button
            type="button"
            role="switch"
            aria-checked={searchEnabled}
            onClick={() => setSearchEnabled(!searchEnabled)}
            className="w-full flex items-center justify-between group cursor-pointer text-left"
          >
            <div className="max-w-[80%]">
              <span className="block text-lg font-bold text-zinc-950 dark:text-white mb-1 tracking-tight">Enable Search</span>
              <span className="block text-sm text-zinc-500 dark:text-zinc-500 font-medium">Automatically skip bases that don't meet loot requirements.</span>
            </div>
            <div className={`w-14 h-7 rounded-full transition-all duration-500 relative shrink-0 ${searchEnabled ? 'bg-emerald-500/80' : 'bg-zinc-200 dark:bg-zinc-800'}`}>
               <div className={`absolute top-1 w-5 h-5 rounded-full transition-all duration-500 shadow-lg ${searchEnabled ? 'left-8 bg-white' : 'left-1 bg-white dark:bg-zinc-500'}`}></div>
            </div>
          </button>

          <div className="h-px bg-zinc-50 dark:bg-zinc-800/50 w-full"></div>

          <div className="space-y-5">
            <button
              type="button"
              role="switch"
              aria-checked={lootExitEnabled}
              onClick={() => setLootExitEnabled(!lootExitEnabled)}
              className="w-full flex items-center justify-between group cursor-pointer text-left"
            >
              <div className="max-w-[80%]">
                <span className="block text-lg font-bold text-zinc-950 dark:text-white mb-1 tracking-tight">Exit by Loot Collected</span>
                <span className="block text-sm text-zinc-500 dark:text-zinc-500 font-medium">
                  End the battle once the configured percentage of the starting available loot has been collected.
                </span>
              </div>
              <div className={`w-14 h-7 rounded-full transition-all duration-500 relative shrink-0 ${lootExitEnabled ? 'bg-emerald-500/80' : 'bg-zinc-200 dark:bg-zinc-800'}`}>
                <div className={`absolute top-1 w-5 h-5 rounded-full transition-all duration-500 shadow-lg ${lootExitEnabled ? 'left-8 bg-white' : 'left-1 bg-white dark:bg-zinc-500'}`}></div>
              </div>
            </button>

            <div className={`rounded-2xl border p-5 transition-all ${lootExitEnabled ? 'border-emerald-500/20 bg-emerald-500/5' : 'border-zinc-100 dark:border-zinc-800 opacity-45'}`}>
              <div className="flex items-center justify-between mb-4">
                <div>
                  <div className="text-[11px] font-black text-zinc-500 uppercase tracking-[0.2em]">Loot exit threshold</div>
                  <div className="text-xs text-zinc-400 mt-1">0–100% of the base's starting available loot</div>
                </div>
                <div className="text-3xl font-black text-zinc-950 dark:text-white tabular-nums">{lootExitPercent}%</div>
              </div>

              <input
                type="range"
                min={0}
                max={100}
                step={1}
                value={lootExitPercent}
                disabled={!lootExitEnabled}
                onChange={(e) => setLootExitPercent(Number(e.target.value))}
                className="w-full accent-emerald-500 disabled:cursor-not-allowed"
                aria-label="Loot exit percentage"
              />

              <div className="mt-4 flex items-center gap-3">
                <input
                  type="number"
                  min={0}
                  max={100}
                  value={lootExitPercent}
                  disabled={!lootExitEnabled}
                  aria-invalid={lootExitInvalid}
                  onChange={(e) => {
                    const raw = Number(e.target.value);
                    setLootExitPercent(Number.isFinite(raw) ? Math.max(0, Math.min(100, raw)) : 0);
                  }}
                  className={`w-28 bg-white dark:bg-zinc-950 border rounded-xl py-2.5 px-3 text-sm font-black tabular-nums focus:outline-none focus:ring-4 transition-all ${lootExitInvalid ? 'border-rose-400 focus:ring-rose-500/10' : 'border-zinc-200 dark:border-zinc-700 focus:ring-emerald-500/10'}`}
                />
                <span className="text-xs font-medium text-zinc-500">
                  {lootExitEnabled
                    ? lootExitPercent === 0
                      ? 'Exit as soon as the battle monitor confirms the fight can be surrendered.'
                      : `Exit after about ${lootExitPercent}% of the initial loot has been collected.`
                    : 'Disabled — battle ends normally.'}
                </span>
              </div>
            </div>
          </div>

          <div className="h-px bg-zinc-50 dark:bg-zinc-800/50 w-full"></div>

          <button
            type="button"
            role="switch"
            aria-checked={upgradeWalls}
            onClick={() => setUpgradeWalls(!upgradeWalls)}
            className="w-full flex items-center justify-between group cursor-pointer text-left"
          >
            <div className="max-w-[80%]">
              <span className="block text-lg font-bold text-zinc-950 dark:text-white mb-1 tracking-tight">Upgrade Walls</span>
              <span className="block text-sm text-zinc-500 dark:text-zinc-500 font-medium">Automatically use spare gold to upgrade walls.</span>
            </div>
            <div className={`w-14 h-7 rounded-full transition-all duration-500 relative shrink-0 ${upgradeWalls ? 'bg-emerald-500/80' : 'bg-zinc-200 dark:bg-zinc-800'}`}>
               <div className={`absolute top-1 w-5 h-5 rounded-full transition-all duration-500 shadow-lg ${upgradeWalls ? 'left-8 bg-white' : 'left-1 bg-white dark:bg-zinc-500'}`}></div>
            </div>
          </button>
        </div>


        <div className="flex flex-col items-end gap-3 pt-4">
          {anyInvalid && (
            <div className="px-4 py-2 rounded-xl bg-amber-500/10 border border-amber-500/30 text-[11px] font-bold text-amber-600 dark:text-amber-400 tracking-wider" role="alert">
              <span className="material-symbols-outlined text-sm align-middle mr-1">warning</span>
              Some values are out of range — fix them before saving.
            </div>
          )}
          {lastSaveError && saveStatus === 'error' && (
            <div
              role="status"
              aria-live="polite"
              className="px-4 py-2 rounded-xl bg-rose-500/10 border border-rose-500/30 text-[11px] font-bold text-rose-600 dark:text-rose-400 tracking-wider max-w-md text-right"
              title={lastSaveError}
            >
              <span className="material-symbols-outlined text-sm align-middle mr-1">error</span>
              Save failed: {lastSaveError.length > 80 ? lastSaveError.slice(0, 77) + '…' : lastSaveError}
            </div>
          )}
          <button
            type="submit"
            disabled={saveStatus === 'saving' || anyInvalid}
            aria-label={`${saveButtonLabel} — saves your config to the bot`}
            data-testid="config-save-btn"
            data-save-state={saveStatus}
            className={`group h-16 px-12 font-black text-xs uppercase tracking-[0.3em] rounded-3xl transition-all duration-300 active:scale-[0.98] flex items-center gap-4 border border-transparent dark:border-white/10 shadow-lg disabled:opacity-40 disabled:cursor-not-allowed ${saveButtonClasses}`}
          >
            {saveButtonLabel}
            <span className={`material-symbols-outlined text-lg transition-transform ${saveStatus === 'saving' ? 'animate-spin' : saveStatus === 'idle' ? 'group-hover:translate-x-1' : ''}`}>
              {saveButtonIcon}
            </span>
          </button>
        </div>
          </>
        )}
      </form>
    </div>
  );
});

export default ConfigView;
