import React from 'react';
import { ActivityEvent, AttackReport, BotStats, SessionReportView } from '../types';
import { formatUptime } from '../utils';

interface HomeViewProps {
  stats: BotStats;
  history: AttackReport[];
  activity: ActivityEvent[];
  sessionReport: SessionReportView | null;
  testSessionActive: boolean;
  running: boolean;
  starting: boolean;
  onStart: () => void;
  onStartTestSession: () => void;
  onStartQuickTestSession: () => void;
  onStop: () => void;
  onOpenAutomation: () => void;
  onOpenAccount: () => void;
  onOpenMemberSettings: () => void;
  onOpenVillage: () => void;
  onOpenSettings: () => void;
  licenseReady: boolean;
  licenseRequired: boolean;
  accountLinked: boolean;
  windowsReady: boolean | null;
  readinessIssues: string[];
  startupCheck: {
    ready: boolean;
    checks: Array<{ id: string; label: string; ok: boolean; blocking?: boolean; message: string; action?: string; action_label?: string }>;
  } | null;
  startupCheckRunning: boolean;
  onRunStartupCheck: () => void;
  memberName?: string;
  licensePlan?: string;
  licenseExpiresAt?: string;
  latestBootReport: {
    started_at?: string;
    completed_at?: string;
    outcome?: string;
    final_error?: string;
    suggested_action?: string;
    recovery_used?: string[];
    attempts?: number;
  } | null;
  currentArmy: {
    timestamp?: string;
    ready: boolean;
    uncertain: boolean;
    warnings?: string[];
    units?: Array<{ name: string; category: string; count: number; confidence: number; slot_x: number }>;
    target_town_hall?: number;
    target_label?: string;
  } | null;
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

const friendlyBootAction = (value?: string): string => {
  const raw = String(value || '').trim();
  const text = raw.toLowerCase();
  if (!raw) return 'Réessaie. ClashGO relancera automatiquement les récupérations sûres.';
  if (text.includes('adb')) return 'Vérifie que BlueStacks est lancé et que l’accès ADB est disponible.';
  if (text.includes('restart bluestacks') || text.includes('relaunch bluestacks')) return 'Redémarre BlueStacks puis relance ClashGO.';
  if (text.includes('wait') || text.includes('initializing')) return 'Attends quelques secondes que BlueStacks termine son démarrage puis réessaie.';
  if (text.includes('clash of clans') || text.includes('startapp')) return 'Ouvre Clash of Clans une fois dans BlueStacks puis relance le bot.';
  if (text.includes('screen') || text.includes('capture')) return 'Affiche le village dans BlueStacks puis relance le pré-contrôle.';
  return raw;
};

const friendlyBootError = (value?: string): string => {
  const raw = String(value || '').trim();
  const text = raw.toLowerCase();
  if (!raw) return 'Le dernier démarrage n’a pas abouti.';
  if (text.includes('adb')) return 'La connexion ADB avec BlueStacks n’a pas pu être établie.';
  if (text.includes('boot') && text.includes('timeout')) return 'BlueStacks n’a pas terminé son démarrage à temps.';
  if (text.includes('capture') || text.includes('screen')) return 'ClashGO n’a pas réussi à lire correctement l’écran du jeu.';
  if (text.includes('clash of clans') || text.includes('startapp')) return 'Clash of Clans n’a pas pu être lancé correctement.';
  return raw.length > 180 ? raw.slice(0, 177) + '…' : raw;
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
    case 'recovery': {
      const stage = String(event.fields?.stage || '').toLowerCase();
      const method = String(event.fields?.method || '').toLowerCase();
      const methodLabel =
        method === 'game_restart' ? 'jeu relancé' :
        method === 'adb_reconnect' ? 'ADB reconnecté' :
        method === 'adb_server_reset' ? 'serveur ADB réinitialisé' :
        method === 'bluestacks_relaunch' ? 'BlueStacks relancé' :
        '';

      if (stage === 'failed') {
        return {
          title: 'Récupération à réessayer',
          detail: methodLabel
            ? 'Tentative effectuée : ' + methodLabel + '. ClashGO réessaiera automatiquement.'
            : 'ClashGO réessaiera automatiquement au prochain contrôle.',
          icon: 'warning',
        };
      }
      if (stage === 'success') {
        return {
          title: 'Session récupérée',
          detail: methodLabel ? 'Correction automatique : ' + methodLabel + '.' : 'La session a été stabilisée automatiquement.',
          icon: 'healing',
        };
      }
      return {
        title: 'Récupération automatique',
        detail: 'ClashGO détecte le blocage et applique les corrections sûres dans l’ordre.',
        icon: 'healing',
      };
    }
    case 'speed_profile': {
      const rawProfile = String(event.fields?.profile || event.fields?.mode || '').trim().toLowerCase();
      const reason = String(event.fields?.reason || '').trim().toLowerCase();
      const incident = String(event.fields?.incident || '').trim().toLowerCase();
      const perHour = n('max_attacks_per_hour');
      const perSession = n('max_attacks_per_session');
      const label =
        rawProfile === 'fast' ? 'Rapide' :
        rawProfile === 'cautious' || rawProfile === 'safe' ? 'Prudente' :
        'Normale';

      if (reason === 'safety_governor') {
        const cause =
          incident === 'device_recovery' ? 'récupération de BlueStacks / ADB' :
          incident ? incident.split('_').join(' ') :
          'stabilité de la session';
        return {
          title: 'Cadence sécurisée · ' + label,
          detail: 'ClashGO a ralenti temporairement le rythme après ' + cause + '. Le profil reviendra automatiquement ensuite.',
          icon: 'health_and_safety',
        };
      }

      return {
        title: 'Cadence ajustée · ' + label,
        detail: [
          perHour > 0 ? perHour + ' attaques/h' : '',
          perSession > 0 ? 'session ' + perSession : '',
        ].filter(Boolean).join(' · ') || 'Les réglages membre ont été appliqués.',
        icon: 'speed',
      };
    }
    case 'anomaly': {
      const kind = String(event.fields?.kind || '').toLowerCase();
      if (kind === 'army_guard_rejected_target') {
        return {
          title: 'Base ignorée · armée non conforme',
          detail: 'ClashGO a protégé la session avant tout déploiement et poursuit la recherche.',
          icon: 'shield',
        };
      }
      if (kind === 'army_guard_uncertain') {
        return {
          title: 'Contrôle armée incertain',
          detail: 'La lecture n’était pas assez fiable pour bloquer l’attaque. ClashGO a continué prudemment.',
          icon: 'rule',
        };
      }
      if (kind === 'army_guard_unavailable') {
        return {
          title: 'Contrôle armée indisponible',
          detail: 'Le contrôle n’a pas pu conclure. L’attaque n’a pas été bloquée sur une lecture incertaine.',
          icon: 'shield_question',
        };
      }
      return {
        title: 'Correction automatique',
        detail: 'Une anomalie a été détectée et prise en charge.',
        icon: 'monitor_heart',
      };
    }
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
    stats, history, activity, sessionReport, testSessionActive, running, starting,
    onStart, onStartTestSession, onStartQuickTestSession, onStop, onOpenAutomation, onOpenAccount, onOpenMemberSettings, onOpenVillage, onOpenSettings,
    licenseReady, licenseRequired, accountLinked, windowsReady, readinessIssues,
    startupCheck, startupCheckRunning, onRunStartupCheck,
    memberName, licensePlan, licenseExpiresAt, latestBootReport, currentArmy,
  } = props;

  const lastAttack = history && history.length > 0 ? history[0] : undefined;
  const [sessionCopyState, setSessionCopyState] = React.useState<'idle' | 'copied' | 'error'>('idle');
  const recentActivity = React.useMemo(
    () => (activity || [])
      .filter((event) => event.type !== 'state_changed' && event.type !== 'target_skipped')
      .slice(0, 3),
    [activity],
  );

  const runtimeSpeedLabel = React.useMemo(() => {
    const raw = String(stats.member_speed_profile || stats.speed_profile || '').trim().toLowerCase();
    if (raw === 'fast') return 'Rapide';
    if (raw === 'safe' || raw === 'cautious') return 'Prudente';
    if (raw === 'balanced' || raw === 'normal') return 'Normale';
    return 'Normale';
  }, [stats.member_speed_profile, stats.speed_profile]);

  const armyStatus = React.useMemo(() => {
    if (!currentArmy) {
      return { label: 'En attente', tone: 'neutral' as const, detail: 'Aucune lecture récente' };
    }
    const ts = currentArmy.timestamp ? new Date(currentArmy.timestamp).getTime() : NaN;
    const ageMinutes = Number.isFinite(ts)
      ? Math.max(0, Math.round((Date.now() - ts) / 60_000))
      : null;

    if (ageMinutes == null || ageMinutes > 10) {
      return {
        label: 'À contrôler',
        tone: 'neutral' as const,
        detail: ageMinutes == null ? 'Date de lecture inconnue' : 'Dernière lecture il y a ' + ageMinutes + ' min',
      };
    }
    if (currentArmy.uncertain) {
      return { label: 'Incertaine', tone: 'amber' as const, detail: 'Lecture à confirmer' };
    }
    if (!currentArmy.ready) {
      return {
        label: 'Non conforme',
        tone: 'rose' as const,
        detail: currentArmy.warnings?.[0] || 'Composition différente du plan',
      };
    }
    return {
      label: 'Prête',
      tone: 'emerald' as const,
      detail: currentArmy.target_label ||
        (currentArmy.target_town_hall ? 'Profil HDV ' + currentArmy.target_town_hall : 'Composition validée'),
    };
  }, [currentArmy]);

  const startupCheckSummary = React.useMemo(() => {
    const checks = startupCheck?.checks || [];
    const attention = checks.filter((check) => !check.ok);
    const blocking = attention.filter((check) => check.blocking !== false);
    const advisory = attention.filter((check) => check.blocking === false);
    return {
      total: checks.length,
      passed: checks.length - attention.length,
      attention,
      blocking,
      advisory,
    };
  }, [startupCheck]);

  const sessionCap = Math.max(0, Number(stats.session_attack_cap || 0));
  const sessionAttacks = Math.max(0, Number(stats.session_attacks || 0));
  const sessionProgress = sessionCap > 0
    ? Math.max(0, Math.min(100, Math.round((sessionAttacks / sessionCap) * 100)))
    : 0;

  const lastSessionValidation = React.useMemo(() => {
    if (!sessionReport || sessionReport.attacks <= 0) return null;
    if (sessionReport.attacks < 5) {
      return {
        ok: false,
        neutral: true,
        label: 'Échantillon court',
        detail: 'Moins de 5 attaques · garde ce rapport comme indication, pas comme validation longue.',
      };
    }
    const deployOK = (sessionReport.full_deploy_rate || 0) >= 90;
    const homeOK = (sessionReport.return_home_rate || 0) >= 90;
    const healthOK = (sessionReport.health_score || 0) >= 75;
    const ok = deployOK && homeOK && healthOK;
    return {
      ok,
      neutral: false,
      label: ok ? 'Validation technique OK' : 'À surveiller avant session longue',
      detail: ok
        ? 'Déploiement ≥90 % · retour village ≥90 % · santé ≥75.'
        : [
            !deployOK ? 'déploiement <90 %' : '',
            !homeOK ? 'retour village <90 %' : '',
            !healthOK ? 'santé <75' : '',
          ].filter(Boolean).join(' · '),
    };
  }, [sessionReport]);

  const copySessionSummary = React.useCallback(async () => {
    if (!sessionReport) return;
    const lines = [
      'ClashGO · Dernière session',
      'Attaques: ' + (sessionReport.attacks || 0),
      'Déploiements complets: ' + Math.round(sessionReport.full_deploy_rate || 0) + ' %',
      'Retour village: ' + Math.round(sessionReport.return_home_rate || 0) + ' %',
      'Zéro-touch: ' + Math.round(sessionReport.zero_touch_rate || 0) + ' %',
      'Santé runtime: ' + Math.max(0, Math.min(100, sessionReport.health_score || 0)) + '/100',
      'Anomalies: ' + (sessionReport.anomalies || 0),
      'Récupérations: ' + (sessionReport.recovery_attempts || 0) +
        ((sessionReport.recovery_attempts || 0) > 0
          ? ' · ' + Math.round(sessionReport.recovery_success_rate || 0) + ' % réussies'
          : ''),
      'Validation: ' + (lastSessionValidation?.label || 'Non disponible'),
      sessionReport.recommendations?.[0] ? 'Note: ' + sessionReport.recommendations[0] : '',
    ].filter(Boolean).join('\n');

    try {
      await navigator.clipboard.writeText(lines);
      setSessionCopyState('copied');
    } catch {
      setSessionCopyState('error');
    }
    window.setTimeout(() => setSessionCopyState('idle'), 2500);
  }, [sessionReport, lastSessionValidation]);

  const validationNextStep = React.useMemo(() => {
    if (running || starting || testSessionActive || !startupCheck?.ready || windowsReady !== true) return null;

    if (!sessionReport || sessionReport.attacks <= 0) {
      return {
        tone: 'sky' as const,
        icon: 'science',
        title: 'Étape conseillée · test express',
        detail: 'Commence par 3 attaques pour valider rapidement le démarrage, le déploiement et le retour au village.',
        action: 'quick' as const,
        actionLabel: 'Lancer le test 3',
      };
    }

    if (sessionReport.attacks <= 3) {
      return {
        tone: 'sky' as const,
        icon: 'experiment',
        title: 'Test express terminé',
        detail: 'Passe maintenant à 10 attaques pour vérifier la stabilité sur plusieurs cycles.',
        action: 'standard' as const,
        actionLabel: 'Passer au test 10',
      };
    }

    if (lastSessionValidation?.ok) {
      return {
        tone: 'emerald' as const,
        icon: 'verified',
        title: 'Validation terminée',
        detail: 'Les indicateurs techniques sont propres. Tu peux lancer ta session normale.',
        action: 'normal' as const,
        actionLabel: 'Lancer la session normale',
      };
    }

    return {
      tone: 'amber' as const,
      icon: 'warning',
      title: 'Reste en validation',
      detail: lastSessionValidation?.detail || 'Vérifie les indicateurs avant de lancer une session longue.',
      action: 'standard' as const,
      actionLabel: 'Relancer le test 10',
    };
  }, [lastSessionValidation, running, sessionReport, starting, startupCheck?.ready, testSessionActive, windowsReady]);

  const runCheckAction = React.useCallback((action?: string) => {
    switch (action) {
      case 'license_account':
        onOpenAccount();
        break;
      case 'village':
      case 'account':
        onOpenVillage();
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
  }, [onOpenAccount, onOpenAutomation, onOpenMemberSettings, onOpenSettings, onOpenVillage]);

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
  const blockingCheck = !running && !starting
    ? startupCheck?.checks.find((check) => !check.ok && check.blocking !== false)
    : undefined;
  const startBlocked = Boolean(blockingCheck);
  const actionClass = running
    ? 'bg-rose-500 text-white hover:bg-rose-400'
    : starting
      ? 'bg-amber-400/20 text-amber-300 dark:text-amber-700'
      : startBlocked || windowsBlocked
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
              {testSessionActive && (
                <span className="rounded-full border border-sky-400/30 bg-sky-400/10 px-3 py-1.5 text-[9px] font-black uppercase tracking-widest text-sky-300 dark:text-sky-600">
                  Mode test · {sessionCap > 0 ? sessionCap : '…'} attaques
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
                Cadence · {runtimeSpeedLabel}
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
              <button
                type="button"
                onClick={onOpenVillage}
                title={armyStatus.detail}
                className={
                  'rounded-full border px-3 py-1.5 text-[9px] font-black uppercase tracking-widest transition ' +
                  (armyStatus.tone === 'emerald'
                    ? 'border-emerald-400/20 bg-emerald-400/10 text-emerald-400 dark:text-emerald-600'
                    : armyStatus.tone === 'amber'
                      ? 'border-amber-400/20 bg-amber-400/10 text-amber-400 dark:text-amber-600'
                      : armyStatus.tone === 'rose'
                        ? 'border-rose-400/20 bg-rose-400/10 text-rose-400 dark:text-rose-600'
                        : 'border-white/10 dark:border-zinc-200 bg-white/5 dark:bg-zinc-100 text-zinc-300 dark:text-zinc-600')
                }
              >
                Armée · {armyStatus.label}
              </button>
            </div>
          </div>
          <button
            type="button"
            onClick={
              starting
                ? undefined
                : running
                  ? onStop
                  : blockingCheck?.action
                    ? () => runCheckAction(blockingCheck.action)
                    : blockingCheck
                      ? onRunStartupCheck
                      : windowsBlocked
                        ? onOpenSettings
                        : onStart
            }
            disabled={starting || startupCheckRunning}
            className={'h-14 px-7 rounded-2xl font-black text-xs uppercase tracking-[0.2em] transition-all active:scale-[0.98] disabled:cursor-wait ' + actionClass}
            title={blockingCheck ? blockingCheck.message : undefined}
          >
            {starting
              ? 'DÉMARRAGE…'
              : startupCheckRunning
                ? 'VÉRIFICATION…'
                : running
                  ? 'ARRÊTER LE BOT'
                  : blockingCheck
                    ? (blockingCheck.action_label || 'CORRIGER LA CONFIGURATION')
                    : windowsBlocked
                      ? 'VÉRIFIER WINDOWS'
                      : 'DÉMARRER LE BOT'}
          </button>
        </div>
      </section>

      <section className="grid grid-cols-2 xl:grid-cols-4 gap-3">
        {[
          {
            label: 'Licence',
            value: licenseReady ? 'Active' : licenseRequired ? 'À activer' : 'Mode beta',
            ok: licenseReady,
            optional: !licenseReady && !licenseRequired,
            icon: 'license',
          },
          {
            label: 'Compte Clash',
            value: accountLinked ? 'Lié' : 'Optionnel',
            ok: accountLinked,
            optional: !accountLinked,
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
                (item.ok ? 'bg-emerald-500' : item.pending ? 'bg-amber-400 animate-pulse' : item.optional ? 'bg-amber-400' : 'bg-zinc-300 dark:bg-zinc-700')
              } />
            </div>
            <div className="mt-3 text-sm font-black text-zinc-950 dark:text-white">{item.value}</div>
            <div className="mt-1 text-[9px] font-black uppercase tracking-[0.18em] text-zinc-400">{item.label}</div>
          </div>
        ))}
      </section>

      {!running && (
        <section className="rounded-[1.75rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 shadow-premium dark:shadow-none">
          {licenseRequired && !licenseReady ? (
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
            const blocked = startupCheck.checks.find((check) => !check.ok && check.blocking !== false);
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
          })() : windowsReady === true && startupCheck?.ready && !accountLinked ? (
            <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
              <div>
                <div className="text-[9px] font-black uppercase tracking-[0.2em] text-amber-500">Optionnel</div>
                <div className="mt-1 text-lg font-black text-zinc-950 dark:text-white">Lier ton compte Clash</div>
                <div className="mt-1 text-xs font-semibold text-zinc-500">
                  ClashGO peut déjà démarrer avec ta configuration locale. Le tag joueur sert surtout à sélectionner automatiquement le profil HDV.
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                <button type="button" onClick={onOpenVillage} className="rounded-xl border border-zinc-200 dark:border-zinc-700 px-5 py-3 text-[10px] font-black uppercase tracking-widest text-zinc-600 dark:text-zinc-300">
                  Lier mon compte
                </button>
                <button type="button" onClick={onStart} className="rounded-xl bg-emerald-500 px-5 py-3 text-[10px] font-black uppercase tracking-widest text-white">
                  Démarrer sans le lier
                </button>
              </div>
            </div>
          ) : windowsReady === true && startupCheck?.ready ? (
            <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
              <div>
                <div className="text-[9px] font-black uppercase tracking-[0.2em] text-emerald-500">Tout est prêt</div>
                <div className="mt-1 text-lg font-black text-zinc-950 dark:text-white">ClashGO peut démarrer</div>
                <div className="mt-1 text-xs font-semibold text-zinc-500">Licence, Windows et configuration du bot sont validés.</div>
              </div>
              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  onClick={onStartQuickTestSession}
                  className="rounded-xl border border-sky-200 dark:border-sky-900/50 bg-sky-50 dark:bg-sky-950/20 px-5 py-3 text-[10px] font-black uppercase tracking-widest text-sky-700 dark:text-sky-300 hover:border-sky-400"
                >
                  Test express · 3
                </button>
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
              <div className="text-[9px] font-black uppercase tracking-[0.2em] text-zinc-400">{testSessionActive ? 'Session test en cours' : 'Session en cours'}</div>
              <div className="mt-1 text-xl font-black text-zinc-950 dark:text-white">
                {sessionAttacks} / {sessionCap} attaques
              </div>
              <div className="mt-1 text-xs font-semibold text-zinc-500">
                {Math.max(0, sessionCap - sessionAttacks)} attaque{Math.max(0, sessionCap - sessionAttacks) > 1 ? 's' : ''} restante{Math.max(0, sessionCap - sessionAttacks) > 1 ? 's' : ''} avant l’arrêt propre.{testSessionActive ? ' Tes réglages personnels seront ensuite restaurés.' : ''}
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

      {latestBootReport && latestBootReport.outcome && latestBootReport.outcome !== 'ok' && (
        <section className="rounded-[1.75rem] border border-amber-200 dark:border-amber-900/50 bg-amber-50/80 dark:bg-amber-950/20 p-5">
          <div className="flex flex-col md:flex-row md:items-start md:justify-between gap-4">
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="material-symbols-outlined text-lg text-amber-500">warning</span>
                <div className="text-[9px] font-black uppercase tracking-[0.2em] text-amber-600 dark:text-amber-400">Dernier démarrage</div>
              </div>
              <div className="mt-2 text-base font-black text-zinc-950 dark:text-white">
                {friendlyBootError(latestBootReport.final_error)}
              </div>
              <div className="mt-2 text-xs font-semibold leading-5 text-zinc-600 dark:text-zinc-400">
                {friendlyBootAction(latestBootReport.suggested_action)}
              </div>
              <div className="mt-3 flex flex-wrap gap-2 text-[9px] font-black uppercase tracking-wider text-zinc-400">
                {latestBootReport.attempts && latestBootReport.attempts > 0 && (
                  <span>{latestBootReport.attempts} tentative{latestBootReport.attempts > 1 ? 's' : ''}</span>
                )}
                {(latestBootReport.recovery_used?.length || 0) > 0 && (
                  <span>· récupération auto utilisée</span>
                )}
              </div>
            </div>
            <div className="flex shrink-0 flex-wrap gap-2">
              <button
                type="button"
                disabled={startupCheckRunning}
                onClick={onRunStartupCheck}
                className="rounded-xl bg-amber-500 px-4 py-2.5 text-[9px] font-black uppercase tracking-widest text-zinc-950 disabled:opacity-40"
              >
                {startupCheckRunning ? 'Vérification…' : 'Re-tester maintenant'}
              </button>
              <button
                type="button"
                onClick={onOpenSettings}
                className="rounded-xl border border-amber-300 dark:border-amber-800 px-4 py-2.5 text-[9px] font-black uppercase tracking-widest text-amber-700 dark:text-amber-300"
              >
                Voir le diagnostic
              </button>
            </div>
          </div>
        </section>
      )}

      <section className="rounded-[1.75rem] border border-zinc-100 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 shadow-premium dark:shadow-none">
        <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-4">
          <div>
            <div className="text-[9px] font-black uppercase tracking-[0.2em] text-zinc-400">Pré-contrôle</div>
            <div className="mt-1 text-lg font-black text-zinc-950 dark:text-white">Tester ma configuration</div>
            <div className="mt-1 text-xs font-semibold text-zinc-500">
              Vérifie la licence, les fichiers, BlueStacks, ADB, l’instance et la stratégie sans lancer d’attaque. Le compte Clash reste facultatif.
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
            {startupCheckSummary.attention.length > 0 && (
              <div className="mt-3 grid md:grid-cols-2 gap-2">
                {startupCheckSummary.attention.map((check) => (
                  <div key={check.id} className="rounded-xl border border-zinc-100 dark:border-zinc-800 p-3">
                    <div className="flex items-start gap-3">
                      <span className={
                        'material-symbols-outlined text-base ' +
                        (check.blocking === false ? 'text-amber-500' : 'text-rose-500')
                      }>
                        {check.blocking === false ? 'info' : 'cancel'}
                      </span>
                      <div className="min-w-0 flex-1">
                        <div className="text-xs font-black text-zinc-950 dark:text-white">{check.label}</div>
                        <div className="mt-0.5 text-[10px] font-semibold text-zinc-500">{check.message}</div>
                        {check.action && (
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
            )}

            <details className="mt-3 rounded-xl border border-zinc-100 dark:border-zinc-800 bg-zinc-50/40 dark:bg-zinc-950/20 overflow-hidden">
              <summary className="cursor-pointer list-none flex items-center justify-between gap-4 px-4 py-3">
                <div>
                  <div className="text-[10px] font-black uppercase tracking-[0.16em] text-zinc-500">
                    Contrôles réussis · {startupCheckSummary.passed}/{startupCheckSummary.total}
                  </div>
                  <div className="mt-0.5 text-[10px] font-semibold text-zinc-400">
                    Ouvre uniquement si tu veux voir le détail complet.
                  </div>
                </div>
                <span className="material-symbols-outlined text-zinc-400">expand_more</span>
              </summary>
              <div className="border-t border-zinc-100 dark:border-zinc-800 p-3 grid md:grid-cols-2 gap-2">
                {startupCheck.checks.filter((check) => check.ok).map((check) => (
                  <div key={check.id} className="flex items-start gap-2 rounded-lg px-2 py-2">
                    <span className="material-symbols-outlined text-sm text-emerald-500">check_circle</span>
                    <div className="min-w-0">
                      <div className="text-[10px] font-black text-zinc-800 dark:text-zinc-200">{check.label}</div>
                      <div className="mt-0.5 text-[9px] font-semibold text-zinc-400">{check.message}</div>
                    </div>
                  </div>
                ))}
              </div>
            </details>
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

      {!running && !starting && validationNextStep && (
        <section className={
          'rounded-[1.75rem] border p-5 shadow-premium dark:shadow-none ' +
          (validationNextStep.tone === 'emerald'
            ? 'border-emerald-200/70 bg-emerald-50/70 dark:border-emerald-900/40 dark:bg-emerald-950/10'
            : validationNextStep.tone === 'amber'
              ? 'border-amber-200/70 bg-amber-50/70 dark:border-amber-900/40 dark:bg-amber-950/10'
              : 'border-sky-200/70 bg-sky-50/70 dark:border-sky-900/40 dark:bg-sky-950/10')
        }>
          <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
            <div className="flex items-start gap-3 min-w-0">
              <span className={
                'material-symbols-outlined mt-0.5 text-xl ' +
                (validationNextStep.tone === 'emerald'
                  ? 'text-emerald-500'
                  : validationNextStep.tone === 'amber'
                    ? 'text-amber-500'
                    : 'text-sky-500')
              }>
                {validationNextStep.icon}
              </span>
              <div>
                <div className="text-[9px] font-black uppercase tracking-[0.2em] text-zinc-400">Parcours de validation</div>
                <div className="mt-1 text-base font-black text-zinc-950 dark:text-white">{validationNextStep.title}</div>
                <div className="mt-1 text-xs font-semibold text-zinc-500">{validationNextStep.detail}</div>
              </div>
            </div>
            <button
              type="button"
              onClick={
                validationNextStep.action === 'quick'
                  ? onStartQuickTestSession
                  : validationNextStep.action === 'standard'
                    ? onStartTestSession
                    : onStart
              }
              className={
                'shrink-0 rounded-xl px-5 py-3 text-[10px] font-black uppercase tracking-widest text-white transition active:scale-[0.98] ' +
                (validationNextStep.tone === 'emerald'
                  ? 'bg-emerald-500 hover:bg-emerald-400'
                  : validationNextStep.tone === 'amber'
                    ? 'bg-amber-500 hover:bg-amber-400'
                    : 'bg-sky-500 hover:bg-sky-400')
              }
            >
              {validationNextStep.actionLabel}
            </button>
          </div>
        </section>
      )}

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
              {lastSessionValidation && (
                <div className={
                  'mt-3 inline-flex items-center gap-2 rounded-xl px-3 py-2 text-[10px] font-black ' +
                  (lastSessionValidation.neutral
                    ? 'bg-zinc-100 text-zinc-500 dark:bg-zinc-800 dark:text-zinc-400'
                    : lastSessionValidation.ok
                      ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
                      : 'bg-amber-500/10 text-amber-600 dark:text-amber-400')
                }>
                  <span className="material-symbols-outlined text-sm">
                    {lastSessionValidation.neutral ? 'science' : lastSessionValidation.ok ? 'verified' : 'warning'}
                  </span>
                  <span>{lastSessionValidation.label}</span>
                  <span className="font-semibold normal-case tracking-normal opacity-80">· {lastSessionValidation.detail}</span>
                </div>
              )}
              <div className="mt-3 flex flex-wrap gap-2 text-[9px] font-black uppercase tracking-wider text-zinc-400">
                <span>{sessionReport.anomalies || 0} anomalie{sessionReport.anomalies === 1 ? '' : 's'}</span>
                <span>·</span>
                <span>
                  {sessionReport.recovery_attempts > 0
                    ? Math.round(sessionReport.recovery_success_rate || 0) + ' % récupérations réussies'
                    : 'Aucune récupération nécessaire'}
                </span>
                <span>·</span>
                <span>{Math.round(sessionReport.zero_touch_rate || 0)} % zéro-touch</span>
              </div>
            </div>
            <div className="lg:min-w-[560px]">
              <div className="grid grid-cols-2 md:grid-cols-4 gap-2">
                {[
                  ['Attaques', String(sessionReport.attacks || 0)],
                  ['Déploiements', Math.round(sessionReport.full_deploy_rate || 0) + ' %'],
                  ['Retour village', Math.round(sessionReport.return_home_rate || 0) + ' %'],
                  ['Santé', Math.max(0, Math.min(100, sessionReport.health_score || 0)) + '/100'],
                ].map(([label, value]) => (
                  <div key={String(label)} className="rounded-2xl border border-emerald-100/80 dark:border-emerald-900/30 bg-white/80 dark:bg-zinc-900/70 px-3 py-3">
                    <div className="text-sm font-black text-zinc-950 dark:text-white tabular-nums">{value}</div>
                    <div className="mt-1 text-[8px] font-black uppercase tracking-wider text-zinc-400">{label}</div>
                  </div>
                ))}
              </div>
              <div className="mt-3 flex justify-end">
                <button
                  type="button"
                  onClick={() => void copySessionSummary()}
                  className="rounded-xl border border-emerald-200 dark:border-emerald-900/60 bg-white/80 dark:bg-zinc-900 px-4 py-2 text-[9px] font-black uppercase tracking-widest text-emerald-700 dark:text-emerald-300 transition hover:border-emerald-400"
                >
                  <span className="material-symbols-outlined mr-2 align-middle text-sm">content_copy</span>
                  {sessionCopyState === 'copied' ? 'Copié' : sessionCopyState === 'error' ? 'Copie impossible' : 'Copier le résumé'}
                </button>
              </div>
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
