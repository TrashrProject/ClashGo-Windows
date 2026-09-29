import React from 'react';
import { InterfaceLevel, TabType } from '../types';
import logo from '../assets/images/clashgo-logo.png';

interface SidebarProps {
  tab: TabType;
  interfaceLevel: InterfaceLevel;
  setTab: (tab: TabType) => void;
  expanded: boolean;
  setExpanded: (expanded: boolean) => void;
  running: boolean;
  starting: boolean;
  onStart: () => void;
  onStop: () => void;
  licenseActivated: boolean;
  licenseRole: 'member' | 'developer' | 'admin' | '';
  memberName?: string;
  licensePlan?: string;
  startReady: boolean;
  startBlockedReason?: string;
}

const Sidebar: React.FC<SidebarProps> = React.memo(({
  tab,
  setTab,
  expanded,
  setExpanded,
  running,
  starting,
  onStart,
  onStop,
  interfaceLevel,
  licenseActivated,
  licenseRole,
  memberName,
  licensePlan,
  startReady,
  startBlockedReason,
}) => {
  const menuItems: { id: TabType; label: string; icon: string; minLevel: InterfaceLevel }[] = [
    { id: 'dashboard', label: 'Accueil', icon: 'home', minLevel: 'simple' },
    { id: 'config', label: 'Automatisation', icon: 'auto_awesome', minLevel: 'simple' },
    { id: 'account', label: 'Mon ClashGO', icon: 'account_circle', minLevel: 'simple' },
    { id: 'activity', label: 'Activité', icon: 'timeline', minLevel: 'advanced' },
    { id: 'analytics', label: 'Statistiques', icon: 'monitoring', minLevel: 'advanced' },
    { id: 'settings', label: 'Paramètres', icon: 'settings', minLevel: 'advanced' },
    { id: 'developer', label: 'Support', icon: 'support_agent', minLevel: 'developer' },
  ];

  const levelRank: Record<InterfaceLevel, number> = {
    simple: 0,
    advanced: 1,
    developer: 2,
  };
  const visibleItems = menuItems.filter((item) => levelRank[interfaceLevel] >= levelRank[item.minLevel]);

  return (
    <aside
      className={`fixed left-0 top-0 h-full bg-white dark:bg-zinc-900 border-r border-zinc-100/50 dark:border-zinc-800/50 z-50 transition-[width,background-color] duration-200 ease-out group overflow-hidden ${expanded ? 'w-64 shadow-premium-lg dark:shadow-none' : 'w-20'}`}
      style={{
        position: 'fixed',
        left: 0,
        top: 0,
        height: '100%',
        width: expanded ? '256px' : '80px',
        zIndex: 50,
        willChange: 'width'
      }}
      onMouseEnter={() => setExpanded(true)}
      onMouseLeave={() => setExpanded(false)}
    >
      <div className="draggable absolute top-0 left-0 right-0 h-8 z-10" />
      <div className="flex flex-col h-full py-8 px-4 relative z-20">
        <div className="flex items-center mb-10 px-0 overflow-hidden draggable">
          <div className="w-12 h-12 flex-shrink-0 flex items-center justify-center no-drag">
            <img
              src={logo}
              alt="ClashGO Logo"
              className="w-9 h-9 object-contain dark:invert"
            />
          </div>
          <div className={`transition-[opacity,transform] duration-200 ease-out ml-2 ${expanded ? 'opacity-100 translate-x-0' : 'opacity-0 -translate-x-2 w-0 overflow-hidden'}`}>
            <h1 className="font-headline text-lg font-bold tracking-tight whitespace-nowrap text-zinc-950 dark:text-white">Clash<span className="text-zinc-400 dark:text-zinc-500 font-medium ml-0.5">GO</span></h1>
          </div>
        </div>

        <nav className="space-y-1.5 flex-grow">
          {visibleItems.map((item) => (
            <button
              key={item.id}
              onClick={() => setTab(item.id)}
              title={expanded ? undefined : item.label}
              aria-label={item.label}
              className={`w-full flex items-center h-11 rounded-xl transition-all duration-150 active:scale-[0.97] relative group/btn ${
                tab === item.id
                  ? 'bg-zinc-950 dark:bg-zinc-800 text-white dark:text-zinc-200 shadow-premium dark:shadow-none'
                  : 'text-zinc-500 dark:text-zinc-500 hover:bg-zinc-100/80 dark:hover:bg-zinc-800/40 hover:text-zinc-900 dark:hover:text-zinc-200'
              }`}
            >
              <div className="w-11 h-11 flex-shrink-0 flex items-center justify-center">
                <span className={`material-symbols-outlined text-[20px] transition-transform duration-150 ${tab === item.id ? 'scale-110' : 'group-hover/btn:scale-110'}`}>
                  {item.icon}
                </span>
              </div>
              <span className={`text-[13px] font-bold tracking-tight transition-[opacity,transform] duration-200 ease-out whitespace-nowrap ${expanded ? 'opacity-100 translate-x-0' : 'opacity-0 -translate-x-2 w-0 overflow-hidden'}`}>
                {item.label}
              </span>
            </button>
          ))}
        </nav>

        <div className="mt-auto space-y-3">
          <button
            type="button"
            onClick={() => setTab('account')}
            title={expanded ? undefined : 'Mon ClashGO'}
            className="w-full rounded-2xl border border-zinc-100 dark:border-zinc-800 bg-zinc-50/70 dark:bg-zinc-950/30 overflow-hidden text-left hover:border-zinc-300 dark:hover:border-zinc-700 transition"
          >
            <div className="flex items-center min-h-14">
              <div className="w-12 h-12 flex-shrink-0 grid place-items-center">
                <div className={
                  'w-8 h-8 rounded-xl grid place-items-center ' +
                  (licenseActivated
                    ? 'bg-emerald-500/10 text-emerald-500'
                    : 'bg-zinc-200 text-zinc-500 dark:bg-zinc-800')
                }>
                  <span className="material-symbols-outlined text-[18px]">
                    {licenseActivated ? 'verified_user' : 'person'}
                  </span>
                </div>
              </div>
              <div className={
                'min-w-0 pr-3 transition-[opacity,transform] duration-200 ' +
                (expanded ? 'opacity-100 translate-x-0' : 'opacity-0 -translate-x-2 w-0 overflow-hidden')
              }>
                <div className="truncate text-[11px] font-black text-zinc-950 dark:text-white">
                  {memberName || (licenseActivated ? 'Membre ClashGO' : 'ClashGO')}
                </div>
                <div className="mt-0.5 flex items-center gap-1.5 text-[8px] font-black uppercase tracking-wider text-zinc-400">
                  <span>{licenseActivated ? (licenseRole || 'member') : 'Non activé'}</span>
                  {licenseActivated && licensePlan && <span>· {licensePlan === 'free_2d' ? 'Free 2J' : licensePlan === 'week_1' ? '1 sem.' : licensePlan === 'month_1' ? '1 mois' : 'À vie'}</span>}
                </div>
              </div>
            </div>
          </button>

          <button
            onClick={starting ? undefined : (running ? onStop : (startReady ? onStart : undefined))}
            disabled={starting || (!running && !startReady)}
            title={
              expanded
                ? (!running && !startReady ? startBlockedReason : undefined)
                : (starting
                    ? 'Démarrage du bot'
                    : running
                      ? 'Arrêter le bot'
                      : startReady
                        ? 'Démarrer le bot'
                        : (startBlockedReason || 'Configuration incomplète'))
            }
            className={`w-full h-12 rounded-2xl font-black text-[10px] tracking-[0.2em] transition-all duration-200 flex items-center relative overflow-hidden group/start ${
              starting
                ? 'bg-amber-50 dark:bg-amber-950/20 text-amber-600 dark:text-amber-400 border border-amber-100 dark:border-amber-900/30 cursor-wait'
                : running
                  ? 'bg-rose-50 dark:bg-rose-950/30 text-rose-600 dark:text-rose-400 hover:bg-rose-100 dark:hover:bg-rose-950/50 border border-rose-100 dark:border-rose-900/30'
                  : !startReady
                    ? 'bg-zinc-100 dark:bg-zinc-900 text-zinc-400 dark:text-zinc-600 border border-zinc-200 dark:border-zinc-800 cursor-not-allowed'
                    : 'bg-zinc-950 dark:bg-zinc-800 text-white dark:text-zinc-300 hover:bg-zinc-800 dark:hover:bg-zinc-700 shadow-premium dark:shadow-none border border-transparent dark:border-zinc-700/50'
            }`}
          >
            <div className="w-12 h-12 flex-shrink-0 flex items-center justify-center z-10 transition-transform duration-200 group-hover/start:scale-110">
              <span className={`material-symbols-outlined text-[18px] ${starting ? 'animate-spin' : ''}`}>
                {starting ? 'progress_activity' : (running ? 'stop' : 'play_arrow')}
              </span>
            </div>
            <span className={`transition-[opacity,transform] duration-200 ease-out whitespace-nowrap z-10 ${expanded ? 'opacity-100 translate-x-0' : 'opacity-0 w-0 -translate-x-2 overflow-hidden'}`}>
              {starting ? 'DÉMARRAGE...' : (running ? 'ARRÊTER' : startReady ? 'DÉMARRER' : 'À CONFIGURER')}
            </span>
          </button>
        </div>
      </div>
    </aside>
  );
});

export default Sidebar;
