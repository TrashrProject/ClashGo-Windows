import React from 'react';
import { ActivityEvent, AttackReport, BotStats, SessionReportView } from '../types';
import { formatUptime } from '../utils';

interface HomeViewProps {
  stats: BotStats;
  history: AttackReport[];
  activity: ActivityEvent[];
  sessionReport: SessionReportView | null;
  running: boolean;
  starting: boolean;
  onStart: () => void;
  onStartTestSession: () => void;
  onStop: () => void;
  onOpenAutomation: () => void;
  onOpenAccount: () => void;
  onOpenMemberSettings: () => void;
  onOpenSettings: () => void;
  licenseReady: boolean;
  accountLinked: boolean;
  windowsReady: boolean | null;
  readinessIssues: string[];
  startupCheck: {
    ready: boolean;
    checks: Array<{ id: string; label: string; ok: boolean; message: string; action?: string; action_label?: string }>;
  } | null;
  startupCheckRunning: boolean;
  onRunStartupCheck: () => void;
  memberName?: string;
  licensePlan?: string;
  licenseExpiresAt?: string;
}

const licenseRemainingLabel = (expiresAt?: string): { label: string; urgent: boolean } => {
  if (!expiresAt) return { label: 'À vie', urgent: false };
  const expiry = new Date(expiresAt).getTime();
  if (!Number.isFinite(expiry)) return { label: 'Expiration inconnue', urgent: false };
  const remaining = expiry - Date.now();
  if (remaining <= 0) return { label: 'Expirée', urgent: true };
  const hours = Math.ceil(remaining / 3_600_000);
  if (hours <= 24) return { label: 'Expire aujourd’hui', urgent: true };
  const days = Math.ceil(hours / 24);
  return { label: `${days} j restants`, urgent: days <= 3 };
};

const formatLoot = (value: number): string => {
  if (!Number.isFinite(value) || value <= 0) return '0';
  if (value >= 1_000_000) return (value / 1_000_000).toFixed(value >= 10_000_000 ? 0 : 1) + 'M';
  if (value >= 1_000) return Math.round(value / 1_000) + 'K';
  return Math.round(value).toLocaleString();
};

const activityLabel = (event: ActivityEvent): { title: string; detail: string; icon: string } => {
  const n = (key: string): number => {
    const value = event.fields?.[key];
    return typeof value === 'number' && Number.isFinite(value) ? value : 0;
  };

  switch (event.type) {
    case 'search_started':
      return { title: 'Recherche lancée', detail: 'ClashGO cherche un village rentable.', icon: 'search' };
    case 'target_found':
      return {
        title: event.fields?.accept === true ? 'Village accepté' : 'Village analysé',
        detail: event.fields?.accept === true
          ? formatLoot(n('gold')) + ' or · ' + formatLoot(n('elixir')) + ' élixir'
          : 'Analyse des ressources terminée.',
        icon: 'target',
      };
    case 'attack_started':
      return { title: 'Attaque lancée', detail: 'Déploiement automatique en cours.', icon: 'swords' };
    case 'attack_finished':
      return {
        title: 'Attaque terminée · ' + n('stars') + '★',
        detail: formatLoot(n('gold')) + ' or récupéré.',
        icon: 'military_tech',
      };
    case 'return_home':
      return {
        title: event.fields?.success === true ? 'Retour au village' : 'Retour au village en cours',
        detail: 'ClashGO prépare le prochain cycle.',
        icon: 'home',
      };
    case 'session_complete':
      return {
        title: 'Session terminée',
        detail: event.fields?.reason === 'attack_cap'
          ? `Limite atteinte · ${n('attacks')} / ${n('cap')} attaques.`
          : 'ClashGO a terminé la session.',
        icon: 'task_alt',
      };
    case 'recovery':
      return {
        title: event.fields?.stage === 'success' ? 'Récupération terminée' : 'Récupération automatique',
        detail: 'ClashGO stabilise automatiquement la session.',
        icon: 'healing',
      };
    case 'speed_profile': {
      const profile = String(event.fields?.profile || '');
      const perHour = n('max_attacks_per_hour');
      const perSession = n('max_attacks_per_session');
      const label = profile === 'fast' ? 'Rapide' : profile === 'cautious' ? 'Prudente' : 'Normale';
      return {
        title: 'Cadence ajustée · ' + label,
        detail: [
          perHour > 0 ? perHour + ' attaques/h' : '',
          perSession > 0 ? 'session ' + perSession : '',
        ].filter(Boolean).join(' · ') || 'Les réglages membre ont été appliqués.',
        icon: 'speed',
      };
    }
    case 'anomaly':
      return {
        title: 'Correction automatique',
        detail: 'Une anomalie a été détectée et prise en charge.',
        icon: 'monitor_heart',
      };
    default:
      return {
        title: event.type.split('_').join(' '),
        detail: 'Activité ClashGO',
        icon: 'bolt',
      };
  }
};

const HomeView: React.FC<HomeViewProps> = React.memo((props) => {
  const {
    stats, history, activity, sessionReport, running, starting,
    onStart, onStop, onOpenAutomation, onOpenAccount, onOpenMemberSettings, onOpenSettings,
    licenseReady, accountLinked, windowsReady, readinessIssues,
    startupCheck, startupCheckRunning, onRunStartupCheck,
    memberName, licensePlan, licenseExpiresAt,
  } = props;

  const lastAttack = history && history.length > 0 ? history[0] : undefined;
  const recentActivity = React.useMemo(
    () => (activity || [])
      .filter((event) => event.type !== 'state_changed' && event.type !== 'target_skipped')
      .slice(0, 3),
    [activity],
  );

  const sessionCap = Math.max(0, Number(stats.session_attack_cap || 0));
  const sessionAttacks = Math.max(0, Number(stats.session_attacks || 0));
  const sessionProgress = sessionCap > 0
    ? Math.max(0, Math.min(100, Math.round((sessionAttacks / sessionCap) * 100)))
    : 0;

  const runCheckAction = React.useCallback((action?: string) => {
    switch (action) {
      case 'account':
        onOpenAccount();
        break;
      case 'automation':
        onOpenAutomation();
        break;
      case 'member_settings':
        onOpenMemberSettings();
        break;
      case 'settings':
        onOpenSettings();
        break;
      default:
        break;
    }
  }, [onOpenAccount, onOpenAutomation, onOpenMemberSettings, onOpenSettings]);

  const botLabel = starting ? 'Démarrage…' : running ? 'Bot en cours' : 'Bot arrêté';
  const botSub = starting
    ? 'ClashGO prépare BlueStacks et l’automatisation.'
    : running
      ? 'L’automatisation est active. Tu peux laisser ClashGO travailler.'
      : 'Vérifie les quatre états ci-dessous puis lance le bot.';

  const statusDot = running
    ? 'bg-emerald-400 animate-pulse'
    : starting
      ? 'bg-amber-400 animate-pulse'
      : 'bg-zinc-600';

  const windowsBlocked = !running && !starting && windowsReady === false;
  const actionClass = running
    ? 'bg-rose-500 text-white hover:bg-rose-400'
    : starting
      ? 'bg-amber-400/20 text-amber-300 dark:text-amber-700'
      : windowsBlocked
        ? 'bg-amber-400 text-zinc-950 hover:bg-amber-300'
        : 'bg-white dark:bg-zinc-950 text-zinc-950 dark:text-white hover:scale-[1.01]';

  return (
    <div className="space-y-6 max-w-6xl mx-auto">
      <section className="rounded-[2.25rem] bg-zinc-950 dark:bg-white text-white dark:text-zinc-950 p-7 md:p-9 shadow-premium-lg overflow-hidden">
        <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-8">
          <div className="min-w-0">
            <div className="flex items-center gap-2 text-[10px] font-black uppercase tracking-[0.25em] text-zinc-400 dark:text-zinc-500">
              <span className={'w-2 h-2 rounded-full ' + statusDot} />
              ClashGO
            </div>
            <h2 className="mt-3 text-3xl md:text-4xl font-black tracking-tight">{botLabel}</h2>
            <p className="mt-2 max-w-xl text-sm font-semibold text-zinc-400 dark:text-zinc-600">{botSub}</p>
            <div className="mt-4 flex flex-wrap gap-2">
              {memberName && (
                <span className="rounded-full border border-white/10 dark:border-zinc-200 bg-white/5 dark:bg-zinc-100 px-3 py-1.5 text-[9px] font-black uppercase tracking-widest text-zinc-300 dark:text-zinc-600">
                  Membre · {memberName}
                </span>
              )}
              {licenseReady && (
                <span className="rounded-full border border-white/10 dark:border-zinc-200 bg-white/5 dark:bg-zinc-100 px-3 py-1.5 text-[9px] font-black uppercase tracking-widest text-zinc-300 dark:text-zinc-600">
                  Plan · {licensePlan === 'free_2d' ? 'FREE 2J' : licensePlan === 'week_1' ? '1 SEMAINE' : licensePlan === 'month_1' ? '1 MOIS' : 'À VIE'}
                </span>
              )}
              {licenseReady && (() => {
                const remaining = licenseRemainingLabel(licenseExpiresAt);
                return (
                  <span className={
                    'rounded-full border px-3 py-1.5 text-[9px] font-black uppercase tracking-widest ' +
                    (remaining.urgent
                      ? 'border-amber-400/40 bg-amber-400/10 text-amber-300 dark:text-amber-600'
                      : 'border-white/10 dark:border-zinc-200 bg-white/5 dark:bg-zinc-100 text-zinc-300 dark:text-zinc-600')
                  }>
                    Licence · {remaining.label}
                  </span>
                );
              })()}
              <button
                type="button"
                onClick={onOpenMemberSettings}
                className="rounded-full border border-white/10 dark:border-zinc-200 bg-white/5 dark:bg-zinc-100 px-3 py-1.5 text-[9px] font-black uppercase tracking-widest text-zinc-300 dark:text-zinc-600 transition hover:border-white/30 dark:hover:border-zinc-400"
                title="Ouvrir les réglages de cadence"
              >
                Cadence · {stats.speed_profile === 'fast' ? 'Rapide' : stats.speed_profile === 'cautious' ? 'Prudente' : 'Normale'}
              </button>
              <span className={
                'rounded-full border px-3 py-1.5 text-[9px] font-black uppercase tracking-widest ' +
                ((stats.health_score ?? 100) >= 85
                  ? 'border-emerald-400/20 bg-emerald-400/10 text-emerald-400 dark:text-emerald-600'
                  : (stats.health_score ?? 100) >= 65
                    ? 'border-amber-400/20 bg-amber-400/10 text-amber-400 dark:text-amber-600'
                    : 'border-rose-400/20 bg-rose-400/10 text-rose-400 dark:text-rose-600')
              }>
                Santé · {Math.max(0, Math.min(100, stats.health_score ?? 100))}/100
              </span>
            </div>
          </div>
          <button
            type="button"
            onClick={starting ? undefined : (running ? onStop : windowsBlocked ? onOpenSettings : onStart)}
            disabled={starting}
            className={'h-14 px-7 rounded-2xl font-black text-xs uppercase tracking-[0.2em] transition-all active:scale-[0.98] disabled:cursor-wait ' + actionClass}
          >
            {starting ? 'DÉMARRAGE…' : running ? 'ARRÊTER LE BOT' : windowsBlocked ? 'VÉRIFIER WINDOWS' : 'DÉMARRER LE BOT'}
          </button>
        </div>
      </section>

      <section className="grid grid-cols-2 xl:grid-cols-4 gap-3">
        {[
          {
            label: 'Licence',
            value: licenseReady ? 'Prête' : 'À activer',
            ok: licenseReady,
            icon: 'license',
          },
          {
            label: 'Compte Clash',
            value: accountLinked ? 'Lié' : 'À lier',
            ok: accountLinked,
            icon: 'person',
          },
          {
            label: 'Windows / BlueStacks',
            value: windowsReady === null ? 'Vérification…' : windowsReady ? 'Prêt' : 'À vérifier',
            ok: windowsReady === true,
            pending: windowsReady === null,
            icon: 'computer',
          },
          {
            label: 'Bot',
            value: starting ? 'Démarrage' : running ? 'Actif' : 'En attente',
            ok: running,
            pending: starting,
            icon: 'smart_toy',
          },
        ].map((item) => (
          <div key={item.label} className="rounded-2xl border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 px-4 py-4">
            <div className="flex items-center justify-between gap-3">
              <span className="material-symbols-outlined text-lg text-zinc-400">{item.icon}</span>
              <span className={
                'w-2 h-2 rounded-full ' +
                (item.ok ? 'bg-emerald-500' : item.pending ? 'bg-amber-400 animate-pulse' : 'bg-zinc-300 dark:bg-zinc-700')
              } />
            </div>
            <div className="mt-3 text-sm font-black text-zinc-950 dark:text-white">{item.value}</div>
            <div className="mt-1 text-[9px] font-black uppercase tracking-[0.18em] text-zinc-400">{item.label}</div>
          </div>
        ))}
      </section>

      {!running && (
        <section className="rounded-[1.75rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 shadow-premium dark:shadow-none">
          {!licenseReady ? (
            <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
              <div>
                <div className="text-[9px] font-black uppercase tracking-[0.2em] text-amber-500">Étape suivante</div>
                <div className="mt-1 text-lg font-black text-zinc-950 dark:text-white">Activer ta licence ClashGO</div>
                <div className="mt-1 text-xs font-semibold text-zinc-500">L’activation ouvre ton espace membre et lie la licence à ce PC.</div>
              </div>
              <button type="button" onClick={onOpenAccount} className="rounded-xl bg-zinc-950 dark:bg-white px-5 py-3 text-[10px] font-black uppercase tracking-widest text-white dark:text-zinc-950">
                Ouvrir Mon ClashGO
              </button>
            </div>
          ) : !accountLinked ? (
            <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
              <div>
                <div className="text-[9px] font-black uppercase tracking-[0.2em] text-amber-500">Étape suivante</div>
                <div className="mt-1 text-lg font-black text-zinc-950 dark:text-white">Lier ton compte Clash</div>
                <div className="mt-1 text-xs font-semibold text-zinc-500">Ton tag joueur permet à ClashGO de choisir automatiquement le bon profil HDV.</div>
              </div>
              <button type="button" onClick={onOpenAccount} className="rounded-xl bg-zinc-950 dark:bg-white px-5 py-3 text-[10px] font-black uppercase tracking-widest text-white dark:text-zinc-950">
                Lier mon compte
              </button>
            </div>
          ) : windowsReady === false ? (
            <div className="flex flex-col md:flex-row md:items-start md:justify-between gap-4">
              <div className="min-w-0">
                <div className="text-[9px] font-black uppercase tracking-[0.2em] text-amber-500">À corriger avant de démarrer</div>
                <div className="mt-1 text-lg font-black text-zinc-950 dark:text-white">Environnement Windows incomplet</div>
                <div className="mt-3 grid gap-2">
                  {(readinessIssues.length ? readinessIssues : ['Un élément Windows ou ADB doit être vérifié.']).slice(0, 4).map((issue) => (
                    <div key={issue} className="flex items-start gap-2 text-xs font-semibold text-zinc-500">
                      <span className="material-symbols-outlined mt-[-1px] text-sm text-amber-500">warning</span>
                      <span>{issue}</span>
                    </div>
                  ))}
                </div>
              </div>
              <button type="button" onClick={onOpenSettings} className="shrink-0 rounded-xl bg-zinc-950 dark:bg-white px-5 py-3 text-[10px] font-black uppercase tracking-widest text-white dark:text-zinc-950">
                Voir le diagnostic
              </button>
            </div>
          ) : windowsReady === true && startupCheck && !startupCheck.ready ? (() => {
            const blocked = startupCheck.checks.find((check) => !check.ok);
            return (
              <div className="flex flex-col md:flex-row md:items-start md:justify-between gap-4">
                <div>
                  <div className="text-[9px] font-black uppercase tracking-[0.2em] text-amber-500">Dernier point à corriger</div>
                  <div className="mt-1 text-lg font-black text-zinc-950 dark:text-white">
                    {blocked?.label || 'Pré-contrôle ClashGO'}
                  </div>
                  <div className="mt-1 text-xs font-semibold text-zinc-500">
                    {blocked?.message || 'Un élément doit encore être vérifié avant le démarrage.'}
                  </div>
                </div>
                {blocked?.action ? (
                  <button
                    type="button"
                    onClick={() => runCheckAction(blocked.action)}
                    className="shrink-0 rounded-xl bg-zinc-950 dark:bg-white px-5 py-3 text-[10px] font-black uppercase tracking-widest text-white dark:text-zinc-950"
                  >
                    {blocked.action_label || 'Corriger'}
                  </button>
                ) : (
                  <button
                    type="button"
                    onClick={onRunStartupCheck}
                    className="shrink-0 rounded-xl bg-zinc-950 dark:bg-white px-5 py-3 text-[10px] font-black uppercase tracking-widest text-white dark:text-zinc-950"
                  >
                    Revérifier
                  </button>
                )}
              </div>
            );
          })() : windowsReady === true && startupCheck?.ready ? (
            <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
              <div>
                <div className="text-[9px] font-black uppercase tracking-[0.2em] text-emerald-500">Tout est prêt</div>
                <div className="mt-1 text-lg font-black text-zinc-950 dark:text-white">ClashGO peut démarrer</div>
                <div className="mt-1 text-xs font-semibold text-zinc-500">Licence, compte Clash, Windows et configuration du bot sont validés.</div>
              </div>
              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  onClick={onStartTestSession}
                  className="rounded-xl border border-zinc-200 dark:border-zinc-700 px-5 py-3 text-[10px] font-black uppercase tracking-widest text-zinc-600 dark:text-zinc-300 hover:text-zinc-950 dark:hover:text-white"
                >
                  Test · 10 attaques
                </button>
                <button type="button" onClick={onStart} className="rounded-xl bg-emerald-500 px-5 py-3 text-[10px] font-black uppercase tracking-widest text-white">
                  Démarrer maintenant
                </button>
              </div>
            </div>
          ) : (
            <div className="flex items-center gap-3 text-sm font-semibold text-zinc-500">
              <span className="material-symbols-outlined animate-spin text-base">progress_activity</span>
              {windowsReady === null ? 'Vérification de BlueStacks et ADB…' : 'Pré-contrôle complet de ClashGO…'}
            </div>
          )}
        </section>
      )}

      {(running || starting) && sessionCap > 0 && (
        <section className="rounded-[1.75rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 shadow-premium dark:shadow-none">
          <div className="flex flex-col md:flex-row md:items-end md:justify-between gap-4">
            <div>
              <div className="text-[9px] font-black uppercase tracking-[0.2em] text-zinc-400">Session en cours</div>
              <div className="mt-1 text-xl font-black text-zinc-950 dark:text-white">
                {sessionAttacks} / {sessionCap} attaques
              </div>
              <div className="mt-1 text-xs font-semibold text-zinc-500">
                {Math.max(0, sessionCap - sessionAttacks)} attaque{Math.max(0, sessionCap - sessionAttacks) > 1 ? 's' : ''} restante{Math.max(0, sessionCap - sessionAttacks) > 1 ? 's' : ''} avant l’arrêt propre.
              </div>
            </div>
            <div className="text-2xl font-black tabular-nums text-zinc-950 dark:text-white">{sessionProgress}%</div>
          </div>
          <div className="mt-4 h-2 overflow-hidden rounded-full bg-zinc-100 dark:bg-zinc-800">
            <div
              className="h-full rounded-full bg-emerald-500 transition-[width] duration-500"
              style={{ width: sessionProgress + '%' }}
            />
          </div>
        </section>
      )}

      <section className="rounded-[1.75rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 shadow-premium dark:shadow-none">
        <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-4">
          <div>
            <div className="text-[9px] font-black uppercase tracking-[0.2em] text-zinc-400">Pré-contrôle</div>
            <div className="mt-1 text-lg font-black text-zinc-950 dark:text-white">Tester ma configuration</div>
            <div className="mt-1 text-xs font-semibold text-zinc-500">
              Vérifie la licence, le compte Clash, les fichiers, BlueStacks, ADB, l’instance et la stratégie sans lancer d’attaque.
            </div>
          </div>
          <button
            type="button"
            disabled={startupCheckRunning}
            onClick={onRunStartupCheck}
            className="shrink-0 rounded-xl bg-zinc-950 dark:bg-white px-5 py-3 text-[10px] font-black uppercase tracking-widest text-white dark:text-zinc-950 disabled:opacity-40"
          >
            {startupCheckRunning ? 'Vérification…' : 'Tout vérifier'}
          </button>
        </div>

        {startupCheck && (
          <div className="mt-4">
            <div className={
              'rounded-xl px-4 py-3 text-sm font-black ' +
              (startupCheck.ready
                ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
                : 'bg-amber-500/10 text-amber-700 dark:text-amber-300')
            }>
              {startupCheck.ready
                ? 'Configuration prête : ClashGO peut démarrer.'
                : 'Un ou plusieurs points doivent être corrigés avant le démarrage.'}
            </div>
            <div className="mt-3 grid md:grid-cols-2 gap-2">
              {startupCheck.checks.map((check) => (
                <div key={check.id} className="rounded-xl border border-zinc-100 dark:border-zinc-800 p-3">
                  <div className="flex items-start gap-3">
                    <span className={
                      'material-symbols-outlined text-base ' +
                      (check.ok ? 'text-emerald-500' : 'text-rose-500')
                    }>
                      {check.ok ? 'check_circle' : 'cancel'}
                    </span>
                    <div className="min-w-0 flex-1">
                      <div className="text-xs font-black text-zinc-950 dark:text-white">{check.label}</div>
                      <div className="mt-0.5 text-[10px] font-semibold text-zinc-500">{check.message}</div>
                      {!check.ok && check.action && (
                        <button
                          type="button"
                          onClick={() => runCheckAction(check.action)}
                          className="mt-2 rounded-lg border border-zinc-200 dark:border-zinc-700 px-3 py-1.5 text-[9px] font-black uppercase tracking-widest text-zinc-500 transition hover:border-zinc-400 hover:text-zinc-950 dark:hover:text-white"
                        >
                          {check.action_label || 'Corriger'}
                        </button>
                      )}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}
      </section>

      <section className="grid grid-cols-2 xl:grid-cols-4 gap-4">
        {[
          { label: 'Attaques', value: String(stats.attacks_completed || 0), icon: 'swords' },
          { label: 'Or', value: formatLoot(stats.total_gold || 0), icon: 'paid' },
          { label: 'Elixir', value: formatLoot(stats.total_elixir || 0), icon: 'water_drop' },
          { label: 'Temps actif', value: formatUptime(stats.uptime || 0), icon: 'schedule' },
        ].map((item) => (
          <div key={item.label} className="rounded-[1.75rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 shadow-premium dark:shadow-none">
            <span className="material-symbols-outlined text-lg text-zinc-400">{item.icon}</span>
            <div className="mt-4 text-2xl font-black text-zinc-950 dark:text-white tabular-nums">{item.value}</div>
            <div className="mt-1 text-[10px] font-black uppercase tracking-[0.2em] text-zinc-400">{item.label}</div>
          </div>
        ))}
      </section>

      {!running && !starting && sessionReport && sessionReport.attacks > 0 && (
        <section className="rounded-[2rem] border border-emerald-200/70 dark:border-emerald-900/40 bg-emerald-50/60 dark:bg-emerald-950/10 p-5 shadow-premium dark:shadow-none">
          <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-5">
            <div>
              <div className="flex items-center gap-2 text-[9px] font-black uppercase tracking-[0.2em] text-emerald-600 dark:text-emerald-400">
                <span className="material-symbols-outlined text-base">task_alt</span>
                Dernière session
              </div>
              <div className="mt-2 text-xl font-black text-zinc-950 dark:text-white">
                {sessionReport.attacks} attaque{sessionReport.attacks > 1 ? 's' : ''} terminée{sessionReport.attacks > 1 ? 's' : ''}
              </div>
              <div className="mt-1 text-xs font-semibold text-zinc-500">
                {sessionReport.recommendations?.[0] || 'Rapport sauvegardé automatiquement par ClashGO.'}
              </div>
            </div>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-2 lg:min-w-[560px]">
              {[
                ['Or / h', formatLoot(Math.round(sessionReport.gold_per_hour || 0))],
                ['Élixir / h', formatLoot(Math.round(sessionReport.elixir_per_hour || 0))],
                ['Étoiles moy.', (sessionReport.average_stars || 0).toFixed(1)],
                ['Santé', Math.max(0, Math.min(100, sessionReport.health_score || 0)) + '/100'],
              ].map(([label, value]) => (
                <div key={String(label)} className="rounded-2xl border border-emerald-100/80 dark:border-emerald-900/30 bg-white/80 dark:bg-zinc-900/70 px-3 py-3">
                  <div className="text-sm font-black text-zinc-950 dark:text-white tabular-nums">{value}</div>
                  <div className="mt-1 text-[8px] font-black uppercase tracking-wider text-zinc-400">{label}</div>
                </div>
              ))}
            </div>
          </div>
        </section>
      )}

      {(running || (stats.attacks_completed || 0) > 0) && (
        <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 shadow-premium dark:shadow-none">
          <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Performance</div>
              <div className="mt-1 text-sm font-black text-zinc-950 dark:text-white">
                Résumé de la session
              </div>
            </div>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-2 md:min-w-[560px]">
              {[
                ['Or / h', formatLoot(Math.round(stats.gold_per_hour || 0))],
                ['Élixir / h', formatLoot(Math.round(stats.elixir_per_hour || 0))],
                ['Taux 3★', `${Math.round(stats.three_star_rate || 0)} %`],
                ['Skips / attaque', (stats.avg_skips_per_attack || 0).toFixed(1)],
              ].map(([label, value]) => (
                <div key={String(label)} className="rounded-2xl bg-zinc-50 dark:bg-zinc-800/60 px-3 py-3">
                  <div className="text-sm font-black text-zinc-950 dark:text-white tabular-nums">{value}</div>
                  <div className="mt-1 text-[8px] font-black uppercase tracking-wider text-zinc-400">{label}</div>
                </div>
              ))}
            </div>
          </div>
        </section>
      )}

      <section className="grid lg:grid-cols-2 gap-6">
        <div className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6 shadow-premium dark:shadow-none">
          <div className="flex items-center justify-between gap-4">
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Dernière attaque</div>
              <h3 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">
                {lastAttack ? String(lastAttack.stars) + '★ · ' + (lastAttack.strategy || 'Stratégie') : 'Aucune attaque pour le moment'}
              </h3>
            </div>
            {lastAttack && (
              <span className={'px-3 py-1.5 rounded-xl text-[10px] font-black uppercase tracking-widest ' + (lastAttack.deploy_success ? 'bg-emerald-500/10 text-emerald-500' : 'bg-amber-500/10 text-amber-500')}>
                {lastAttack.deploy_success ? 'Complète' : 'Partielle'}
              </span>
            )}
          </div>

          {lastAttack ? (
            <div className="mt-5 grid grid-cols-3 gap-3">
              {[
                ['Or', lastAttack.gold_stolen],
                ['Elixir', lastAttack.elixir_stolen],
                ['Élixir noir', lastAttack.dark_elixir_stolen],
              ].map(([label, value]) => (
                <div key={String(label)} className="rounded-2xl bg-zinc-50 dark:bg-zinc-800/60 p-4">
                  <div className="text-lg font-black">{formatLoot(Number(value))}</div>
                  <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">{label}</div>
                </div>
              ))}
            </div>
          ) : (
            <p className="mt-4 text-sm font-semibold text-zinc-500">Le résultat de ta première attaque apparaîtra ici automatiquement.</p>
          )}
        </div>

        <div className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6 shadow-premium dark:shadow-none">
          <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Accès rapides</div>
          <h3 className="mt-1 text-xl font-black text-zinc-950 dark:text-white">L’essentiel en deux clics</h3>
          <div className="mt-5 grid gap-3">
            <button type="button" onClick={onOpenAutomation} className="w-full flex items-center justify-between rounded-2xl bg-zinc-50 dark:bg-zinc-800/60 px-4 py-4 text-left hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors">
              <span>
                <span className="block text-sm font-black text-zinc-950 dark:text-white">Automatisation</span>
                <span className="block mt-0.5 text-xs font-semibold text-zinc-500">Butin ciblé, armée et comportement d’attaque</span>
              </span>
              <span className="material-symbols-outlined text-zinc-400">chevron_right</span>
            </button>
            <button type="button" onClick={onOpenAccount} className="w-full flex items-center justify-between rounded-2xl bg-zinc-50 dark:bg-zinc-800/60 px-4 py-4 text-left hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors">
              <span>
                <span className="block text-sm font-black text-zinc-950 dark:text-white">Mon ClashGO</span>
                <span className="block mt-0.5 text-xs font-semibold text-zinc-500">Licence, réglages membre et profil du village</span>
              </span>
              <span className="material-symbols-outlined text-zinc-400">chevron_right</span>
            </button>
          </div>
        </div>
      </section>

      {recentActivity.length > 0 && (
        <section className="rounded-[2rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-6 shadow-premium dark:shadow-none">
          <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Activité récente</div>
          <div className="mt-4 grid md:grid-cols-3 gap-3">
            {recentActivity.map((event, index) => {
              const item = activityLabel(event);
              const at = new Date(event.at);
              return (
                <div key={event.at + '-' + index} className="rounded-2xl bg-zinc-50 dark:bg-zinc-800/60 px-4 py-3">
                  <div className="flex items-start gap-3">
                    <div className="grid h-9 w-9 shrink-0 place-items-center rounded-xl bg-white dark:bg-zinc-900">
                      <span className="material-symbols-outlined text-base text-zinc-400">{item.icon}</span>
                    </div>
                    <div className="min-w-0">
                      <div className="text-xs font-black text-zinc-800 dark:text-zinc-100">{item.title}</div>
                      <div className="mt-0.5 truncate text-[10px] font-semibold text-zinc-500">{item.detail}</div>
                      <div className="mt-1 text-[9px] font-bold text-zinc-400">
                        {Number.isNaN(at.getTime()) ? event.at : at.toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' })}
                      </div>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        </section>
      )}
    </div>
  );
});

export default HomeView;
