import React from 'react';
import { AttackReport, BotStats, VillageResourceSnapshot } from '../types';

interface AnalyticsProps {
  stats: BotStats;
  resourceHistory: VillageResourceSnapshot[];
  history: AttackReport[];
}

const Analytics: React.FC<AnalyticsProps> = React.memo(({ stats, resourceHistory, history }) => {
  // `color` drives Tailwind bar classes; `hex` feeds the conic-gradient
  // (Tailwind class names are NOT valid CSS color values — using them
  // inside the gradient string would silently drop the donut).
  const starData = [
    { label: '3 Stars', count: stats.stars_3, color: 'bg-emerald-500', hex: '#10b981', bg: 'bg-emerald-500/10' },
    { label: '2 Stars', count: stats.stars_2, color: 'bg-zinc-800', hex: '#27272a', bg: 'bg-zinc-800/10' },
    { label: '1 Star', count: stats.stars_1, color: 'bg-zinc-400', hex: '#a1a1aa', bg: 'bg-zinc-400/10' },
    { label: '0 Stars', count: stats.stars_0, color: 'bg-rose-500', hex: '#f43f5e', bg: 'bg-rose-500/10' },
  ];

  const validResourceHistory = resourceHistory.filter((s) => s.valid);
  const firstResource = validResourceHistory[0];
  const lastResource = validResourceHistory[validResourceHistory.length - 1];
  const resourceDelta = {
    gold: firstResource && lastResource ? lastResource.gold - firstResource.gold : 0,
    elixir: firstResource && lastResource ? lastResource.elixir - firstResource.elixir : 0,
    dark: firstResource && lastResource ? lastResource.dark_elixir - firstResource.dark_elixir : 0,
  };
  const formatSigned = (v: number) => (v > 0 ? '+' : '') + v.toLocaleString();

  const strategyStats = React.useMemo(() => {
    const map = new Map<string, {
      name: string;
      attacks: number;
      stars: number;
      gold: number;
      elixir: number;
      dark: number;
      completeDeploys: number;
    }>();
    for (const rep of history ?? []) {
      const name = rep.strategy || 'Unknown';
      const row = map.get(name) ?? {
        name, attacks: 0, stars: 0, gold: 0, elixir: 0, dark: 0, completeDeploys: 0,
      };
      row.attacks++;
      row.stars += rep.stars || 0;
      row.gold += (rep.gold_stolen || 0) + (rep.bonus_gold || 0);
      row.elixir += (rep.elixir_stolen || 0) + (rep.bonus_elixir || 0);
      row.dark += (rep.dark_elixir_stolen || 0) + (rep.bonus_de || 0);
      if (rep.deploy_success) row.completeDeploys++;
      map.set(name, row);
    }
    return Array.from(map.values())
      .sort((a, b) => b.attacks - a.attacks)
      .slice(0, 8);
  }, [history]);

  const strategySideStats = React.useMemo(() => {
    const map = new Map<string, {
      key: string;
      strategy: string;
      side: string;
      attacks: number;
      stars: number;
      goldElixir: number;
      dark: number;
      deployMs: number;
      cycleMs: number;
      complete: number;
    }>();
    for (const rep of history ?? []) {
      const strategy = rep.strategy || 'Unknown';
      const side = rep.deploy_side && rep.deploy_side !== 'Unknown'
        ? rep.deploy_side
        : (rep.target_edge || 'Unknown');
      const key = `${strategy}::${side}`;
      const row = map.get(key) ?? {
        key, strategy, side, attacks: 0, stars: 0, goldElixir: 0, dark: 0,
        deployMs: 0, cycleMs: 0, complete: 0,
      };
      row.attacks++;
      row.stars += rep.stars || 0;
      row.goldElixir += (rep.gold_stolen || 0) + (rep.bonus_gold || 0) + (rep.elixir_stolen || 0) + (rep.bonus_elixir || 0);
      row.dark += (rep.dark_elixir_stolen || 0) + (rep.bonus_de || 0);
      row.deployMs += rep.deploy_duration_ms || 0;
      row.cycleMs += rep.cycle_duration_ms || 0;
      if (rep.deploy_success) row.complete++;
      map.set(key, row);
    }
    return Array.from(map.values())
      .sort((a, b) => b.attacks - a.attacks)
      .slice(0, 16);
  }, [history]);

  const modeStats = React.useMemo(() => {
    const map = new Map<string, {
      mode: string;
      attacks: number;
      stars: number;
      fullDeploys: number;
      searchMs: number;
      deployMs: number;
      cycleMs: number;
      captureMs: number;
      scanMs: number;
      goldElixir: number;
    }>();
    for (const rep of history ?? []) {
      const mode = rep.runtime_mode || 'Unknown';
      const row = map.get(mode) ?? {
        mode, attacks: 0, stars: 0, fullDeploys: 0,
        searchMs: 0, deployMs: 0, cycleMs: 0, captureMs: 0, scanMs: 0, goldElixir: 0,
      };
      row.attacks++;
      row.stars += rep.stars || 0;
      if (rep.deploy_success) row.fullDeploys++;
      row.searchMs += rep.search_duration_ms || 0;
      row.deployMs += rep.deploy_duration_ms || 0;
      row.cycleMs += rep.cycle_duration_ms || 0;
      row.captureMs += rep.capture_ms || 0;
      row.scanMs += rep.target_scan_ms || 0;
      row.goldElixir += (rep.gold_stolen || 0) + (rep.elixir_stolen || 0);
      map.set(mode, row);
    }
    const order: Record<string, number> = { Fast: 0, Balanced: 1, Safe: 2, Unknown: 3 };
    return Array.from(map.values()).sort((a, b) =>
      (order[a.mode] ?? 9) - (order[b.mode] ?? 9)
    );
  }, [history]);

  const performanceGuard = React.useMemo(() => {
    const recent = (history ?? []).slice(0, 5);
    const baseline = (history ?? []).slice(5, 20);
    if (recent.length < 3 || baseline.length < 5) {
      return { status: 'Learning', reasons: ['Need more attacks for a stable baseline'] };
    }

    const summarize = (rows: AttackReport[]) => {
      let search = 0, deploy = 0, complete = 0, capture = 0, scan = 0;
      for (const rep of rows) {
        search += rep.search_duration_ms || 0;
        deploy += rep.deploy_duration_ms || 0;
        capture += rep.capture_ms || 0;
        scan += rep.target_scan_ms || 0;
        if (rep.deploy_success) complete++;
      }
      const n = rows.length;
      return {
        search: search / n,
        deploy: deploy / n,
        capture: capture / n,
        scan: scan / n,
        completeRate: complete * 100 / n,
      };
    };

    const now = summarize(recent);
    const before = summarize(baseline);
    const reasons: string[] = [];
    const slower = (a: number, b: number, pct: number) => b > 0 && a > b * (1 + pct / 100);

    if (slower(now.search, before.search, 35)) reasons.push('Search time is >35% slower than baseline');
    if (slower(now.deploy, before.deploy, 35)) reasons.push('Deployment time is >35% slower than baseline');
    if (slower(now.capture, before.capture, 40)) reasons.push('ADB capture latency is >40% slower');
    if (slower(now.scan, before.scan, 50)) reasons.push('Loot OCR is >50% slower');
    if (before.completeRate - now.completeRate >= 20) reasons.push('Full-deploy rate dropped by at least 20 points');

    return {
      status: reasons.length === 0 ? 'Healthy' : 'Watch',
      reasons: reasons.length === 0 ? ['Recent attacks are within the learned baseline'] : reasons,
    };
  }, [history]);

  const sideStats = React.useMemo(() => {
    const map = new Map<string, {
      side: string;
      attacks: number;
      stars: number;
      loot: number;
      deployMs: number;
      complete: number;
    }>();
    for (const rep of history ?? []) {
      const side = rep.deploy_side && rep.deploy_side !== 'Unknown'
        ? rep.deploy_side
        : (rep.target_edge || 'Unknown');
      const row = map.get(side) ?? { side, attacks: 0, stars: 0, loot: 0, deployMs: 0, complete: 0 };
      row.attacks++;
      row.stars += rep.stars || 0;
      row.loot += (rep.gold_stolen || 0) + (rep.bonus_gold || 0) + (rep.elixir_stolen || 0) + (rep.bonus_elixir || 0);
      row.deployMs += rep.deploy_duration_ms || 0;
      if (rep.deploy_success) row.complete++;
      map.set(side, row);
    }
    return Array.from(map.values()).sort((a, b) => b.attacks - a.attacks);
  }, [history]);

  const totalAttacks = stats.stars_3 + stats.stars_2 + stats.stars_1 + stats.stars_0;
  const getPercent = (count: number) => totalAttacks > 0 ? Math.round((count / totalAttacks) * 100) : 0;
  const threeStarRate = totalAttacks > 0 ? Math.round((stats.stars_3 / totalAttacks) * 100) : 0;
  const avgSearchSeconds = history?.length
    ? history.reduce((sum, r) => sum + (r.search_duration_ms || 0), 0) / history.length / 1000
    : 0;
  const avgCycleSeconds = history?.length
    ? history.reduce((sum, r) => sum + (r.cycle_duration_ms || 0), 0) / history.length / 1000
    : 0;
  const avgDeploySeconds = history?.length
    ? history.reduce((sum, r) => sum + (r.deploy_duration_ms || 0), 0) / history.length / 1000
    : 0;
  const avgBattleSeconds = history?.length
    ? history.reduce((sum, r) => sum + (r.battle_duration_ms || 0), 0) / history.length / 1000
    : 0;
  const avgCombatSeconds = Math.max(0, avgBattleSeconds - avgDeploySeconds);
  const compact = (v: number) => {
    const abs = Math.abs(v);
    if (abs >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}M`;
    if (abs >= 1_000) return `${(v / 1_000).toFixed(1)}K`;
    return Math.round(v).toLocaleString();
  };

  const lootCapture = React.useMemo(() => {
    let offeredGE = 0, stolenGE = 0, offeredDE = 0, stolenDE = 0, targetScore = 0, scored = 0;
    for (const rep of history ?? []) {
      const offered = (rep.target_gold || 0) + (rep.target_elixir || 0);
      if (offered > 0) {
        offeredGE += offered;
        stolenGE += (rep.gold_stolen || 0) + (rep.elixir_stolen || 0);
      }
      if ((rep.target_de || 0) > 0) {
        offeredDE += rep.target_de || 0;
        stolenDE += rep.dark_elixir_stolen || 0;
      }
      if ((rep.target_score || 0) > 0) {
        targetScore += rep.target_score || 0;
        scored++;
      }
    }
    return {
      geRate: offeredGE > 0 ? stolenGE * 100 / offeredGE : 0,
      deRate: offeredDE > 0 ? stolenDE * 100 / offeredDE : 0,
      avgTargetScore: scored > 0 ? targetScore / scored : 0,
      offeredGE,
      stolenGE,
    };
  }, [history]);

  const pipeline = React.useMemo(() => {
    const rows = [
      { label: 'Search', seconds: avgSearchSeconds },
      { label: 'Deployment', seconds: avgDeploySeconds },
      { label: 'Combat', seconds: avgCombatSeconds },
    ];
    const total = rows.reduce((sum, row) => sum + row.seconds, 0);
    return {
      rows: rows.map((row) => ({ ...row, share: total > 0 ? row.seconds * 100 / total : 0 })),
      dominant: rows.reduce((best, row) => row.seconds > best.seconds ? row : best, rows[0]),
    };
  }, [avgSearchSeconds, avgDeploySeconds, avgCombatSeconds]);

  const recentPerformance = React.useMemo(() => {
    const summarize = (rows: AttackReport[]) => {
      const n = rows.length;
      if (n === 0) {
        return {
          attacks: 0, avgStars: 0, threeStarRate: 0, fullDeployRate: 0,
          avgSearchMs: 0, avgDeployMs: 0, avgCycleMs: 0,
          avgGold: 0, avgElixir: 0, avgDE: 0,
        };
      }
      let stars = 0, triples = 0, complete = 0;
      let searchMs = 0, deployMs = 0, cycleMs = 0;
      let gold = 0, elixir = 0, de = 0;
      for (const rep of rows) {
        stars += rep.stars || 0;
        if ((rep.stars || 0) === 3) triples++;
        if (rep.deploy_success) complete++;
        searchMs += rep.search_duration_ms || 0;
        deployMs += rep.deploy_duration_ms || 0;
        cycleMs += rep.cycle_duration_ms || 0;
        gold += (rep.gold_stolen || 0) + (rep.bonus_gold || 0);
        elixir += (rep.elixir_stolen || 0) + (rep.bonus_elixir || 0);
        de += (rep.dark_elixir_stolen || 0) + (rep.bonus_de || 0);
      }
      return {
        attacks: n,
        avgStars: stars / n,
        threeStarRate: triples * 100 / n,
        fullDeployRate: complete * 100 / n,
        avgSearchMs: searchMs / n,
        avgDeployMs: deployMs / n,
        avgCycleMs: cycleMs / n,
        avgGold: gold / n,
        avgElixir: elixir / n,
        avgDE: de / n,
      };
    };

    const current = summarize((history ?? []).slice(0, 10));
    const previous = summarize((history ?? []).slice(10, 20));
    return { current, previous };
  }, [history]);

  const perfDelta = (current: number, previous: number, lowerIsBetter = false) => {
    if (!Number.isFinite(current) || !Number.isFinite(previous) || previous === 0) return null;
    const pct = ((current - previous) / Math.abs(previous)) * 100;
    return lowerIsBetter ? -pct : pct;
  };

  const bestRecords = React.useMemo(() => {
    let bestLoot: AttackReport | null = null;
    let fastestClean: AttackReport | null = null;
    for (const rep of history ?? []) {
      const loot = (rep.gold_stolen || 0) + (rep.bonus_gold || 0) + (rep.elixir_stolen || 0) + (rep.bonus_elixir || 0);
      const bestLootValue = bestLoot
        ? (bestLoot.gold_stolen || 0) + (bestLoot.bonus_gold || 0) + (bestLoot.elixir_stolen || 0) + (bestLoot.bonus_elixir || 0)
        : -1;
      if (loot > bestLootValue) bestLoot = rep;
      if (rep.deploy_success && (rep.deploy_duration_ms || 0) > 0) {
        if (!fastestClean || rep.deploy_duration_ms < fastestClean.deploy_duration_ms) fastestClean = rep;
      }
    }
    return { bestLoot, fastestClean };
  }, [history]);

  // CSS-only donut (conic-gradient — no chart dependency). Each
  // segment's sweep is the star-rate percentage mapped to degrees;
  // zero-count segments collapse to a 0deg stop and stay invisible.
  let sweep = 0;
  const gradientStops = starData.map((s) => {
    const from = sweep;
    sweep += getPercent(s.count) * 3.6;
    return `${s.hex} ${from}deg ${Math.max(sweep, from)}deg`;
  });
  // Donut is only rendered when totalAttacks > 0; zero-count segments
  // collapse to a 0deg stop and stay invisible.
  const donutBg = `conic-gradient(${gradientStops.join(', ')})`;

  return (
    <div className="grid grid-cols-1 xl:grid-cols-2 gap-8 max-w-6xl mx-auto">

      <div className="xl:col-span-2 bg-zinc-950 dark:bg-white p-7 rounded-[2.5rem] shadow-premium-lg">
        <div className="flex flex-col lg:flex-row lg:items-end lg:justify-between gap-5 mb-6">
          <div>
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-500">Intelligence V2</div>
            <h3 className="mt-2 text-2xl font-black text-white dark:text-zinc-950 tracking-tight">Farm Velocity</h3>
            <p className="mt-1 text-sm text-zinc-400 dark:text-zinc-500">The numbers that show whether ClashGO is farming fast, not just staying busy.</p>
          </div>
          <div className="flex items-center gap-2 text-[10px] font-black uppercase tracking-widest text-zinc-500">
            <span className="px-3 py-2 rounded-full bg-white/5 dark:bg-zinc-950/5 border border-white/10 dark:border-zinc-950/10">
              {stats.speed_profile || 'Balanced'} mode
            </span>
            <span className="px-3 py-2 rounded-full bg-white/5 dark:bg-zinc-950/5 border border-white/10 dark:border-zinc-950/10">
              Health {stats.health_score ?? 100}/100
            </span>
            <span>{stats.telemetry_events?.toLocaleString?.() ?? 0} events</span>
          </div>
        </div>
        <div className="grid grid-cols-2 md:grid-cols-4 xl:grid-cols-8 gap-3">
          {[
            { label: 'Gold / h', value: compact(stats.gold_per_hour || 0) },
            { label: 'Elixir / h', value: compact(stats.elixir_per_hour || 0) },
            { label: 'DE / h', value: compact(stats.de_per_hour || 0) },
            { label: 'Avg search', value: `${avgSearchSeconds.toFixed(1)}s` },
            { label: 'Loot scan', value: `${(stats.average_target_scan_ms || 0).toFixed(0)}ms` },
            { label: 'Avg deploy', value: `${avgDeploySeconds.toFixed(1)}s` },
            { label: 'Avg cycle', value: `${avgCycleSeconds.toFixed(1)}s` },
            { label: 'Capture', value: `${(stats.average_capture_ms || 0).toFixed(0)}ms` },
          ].map((metric) => (
            <div key={metric.label} className="rounded-2xl bg-white/5 dark:bg-zinc-950/5 border border-white/10 dark:border-zinc-950/10 p-4">
              <div className="text-[9px] font-black uppercase tracking-[0.18em] text-zinc-500">{metric.label}</div>
              <div className="mt-2 text-xl font-black text-white dark:text-zinc-950 tabular-nums">{metric.value}</div>
            </div>
          ))}
        </div>
      </div>

      <div className="xl:col-span-2 grid grid-cols-1 lg:grid-cols-[1.3fr_1fr] gap-4">
        <div className="bg-white dark:bg-zinc-900 rounded-[2.5rem] border border-zinc-100/70 dark:border-zinc-800/70 p-6 shadow-premium dark:shadow-none">
          <div className="flex items-end justify-between gap-4 mb-5">
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Cycle anatomy</div>
              <h3 className="mt-1 text-xl font-bold text-zinc-950 dark:text-white tracking-tight">Where farming time goes</h3>
            </div>
            <div className="text-[10px] font-black uppercase tracking-widest text-zinc-400">
              Largest: {pipeline.dominant.label}
            </div>
          </div>
          <div className="space-y-4">
            {pipeline.rows.map((row) => (
              <div key={row.label}>
                <div className="flex items-center justify-between text-[10px] font-black uppercase tracking-wider">
                  <span className="text-zinc-500">{row.label}</span>
                  <span className="text-zinc-950 dark:text-white tabular-nums">{row.seconds.toFixed(1)}s · {row.share.toFixed(0)}%</span>
                </div>
                <div className="mt-2 h-2.5 rounded-full bg-zinc-100 dark:bg-zinc-800 overflow-hidden">
                  <div className="h-full rounded-full bg-zinc-950 dark:bg-white transition-all duration-700" style={{ width: `${Math.max(2, row.share)}%` }} />
                </div>
              </div>
            ))}
          </div>
        </div>

        <div className="bg-white dark:bg-zinc-900 rounded-[2.5rem] border border-zinc-100/70 dark:border-zinc-800/70 p-6 shadow-premium dark:shadow-none">
          <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Loot conversion</div>
          <h3 className="mt-1 text-xl font-bold text-zinc-950 dark:text-white tracking-tight">Target → Collected</h3>
          <div className="mt-5 grid grid-cols-2 gap-3">
            <div className="rounded-2xl bg-zinc-50 dark:bg-zinc-950/40 p-4">
              <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">G+E capture</div>
              <div className="mt-1 text-2xl font-black text-zinc-950 dark:text-white tabular-nums">{lootCapture.geRate.toFixed(1)}%</div>
            </div>
            <div className="rounded-2xl bg-zinc-50 dark:bg-zinc-950/40 p-4">
              <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">DE capture</div>
              <div className="mt-1 text-2xl font-black text-zinc-950 dark:text-white tabular-nums">{lootCapture.deRate.toFixed(1)}%</div>
            </div>
            <div className="rounded-2xl bg-zinc-50 dark:bg-zinc-950/40 p-4">
              <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">Avg target score</div>
              <div className="mt-1 text-2xl font-black text-zinc-950 dark:text-white tabular-nums">{lootCapture.avgTargetScore.toFixed(0)}/100</div>
            </div>
            <div className="rounded-2xl bg-zinc-50 dark:bg-zinc-950/40 p-4">
              <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">Collected G+E</div>
              <div className="mt-1 text-2xl font-black text-zinc-950 dark:text-white tabular-nums">{compact(lootCapture.stolenGE)}</div>
            </div>
          </div>
        </div>
      </div>

      <div className="xl:col-span-2 grid grid-cols-2 md:grid-cols-4 gap-4">
        {[
          { label: 'Targets seen', value: (stats.targets_seen || 0).toLocaleString(), detail: 'This session' },
          { label: 'Accept rate', value: `${(stats.target_acceptance_rate || 0).toFixed(1)}%`, detail: 'Accepted / scanned' },
          { label: 'Skips / attack', value: (stats.avg_skips_per_attack || 0).toFixed(1), detail: 'Lower is faster' },
          { label: 'Recovery success', value: stats.recovery_attempts > 0 ? `${(stats.recovery_success_rate || 0).toFixed(0)}%` : '—', detail: stats.recovery_attempts > 0 ? `${stats.recovery_successes}/${stats.recovery_attempts}` : 'No recoveries' },
        ].map((metric) => (
          <div key={metric.label} className="bg-white dark:bg-zinc-900 rounded-[2rem] border border-zinc-100/70 dark:border-zinc-800/70 p-5 shadow-premium dark:shadow-none">
            <div className="text-[9px] font-black uppercase tracking-[0.18em] text-zinc-400">{metric.label}</div>
            <div className="mt-2 text-2xl font-black text-zinc-950 dark:text-white tabular-nums">{metric.value}</div>
            <div className="mt-1 text-[9px] font-bold uppercase tracking-wider text-zinc-400">{metric.detail}</div>
          </div>
        ))}
      </div>

      <div className="xl:col-span-2 bg-white dark:bg-zinc-900 p-8 rounded-[3rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none">
        <div className="flex flex-col lg:flex-row lg:items-end lg:justify-between gap-4 mb-6">
          <div>
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Recent form</div>
            <h3 className="mt-1 text-2xl font-bold text-zinc-950 dark:text-white tracking-tight">Last 10 Attacks</h3>
            <p className="text-sm text-zinc-500 mt-1">Compared with the previous 10 attacks when enough history exists.</p>
          </div>
          <div className="text-[10px] font-black uppercase tracking-widest text-zinc-400">
            {recentPerformance.current.attacks}/10 sampled
          </div>
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 xl:grid-cols-7 gap-3">
          {[
            { label: 'Avg stars', value: recentPerformance.current.avgStars.toFixed(2), delta: perfDelta(recentPerformance.current.avgStars, recentPerformance.previous.avgStars) },
            { label: '3★ rate', value: `${recentPerformance.current.threeStarRate.toFixed(0)}%`, delta: perfDelta(recentPerformance.current.threeStarRate, recentPerformance.previous.threeStarRate) },
            { label: 'Full deploy', value: `${recentPerformance.current.fullDeployRate.toFixed(0)}%`, delta: perfDelta(recentPerformance.current.fullDeployRate, recentPerformance.previous.fullDeployRate) },
            { label: 'Search', value: `${(recentPerformance.current.avgSearchMs / 1000).toFixed(1)}s`, delta: perfDelta(recentPerformance.current.avgSearchMs, recentPerformance.previous.avgSearchMs, true) },
            { label: 'Deploy', value: `${(recentPerformance.current.avgDeployMs / 1000).toFixed(1)}s`, delta: perfDelta(recentPerformance.current.avgDeployMs, recentPerformance.previous.avgDeployMs, true) },
            { label: 'Cycle', value: `${(recentPerformance.current.avgCycleMs / 1000).toFixed(0)}s`, delta: perfDelta(recentPerformance.current.avgCycleMs, recentPerformance.previous.avgCycleMs, true) },
            { label: 'Avg G+E', value: compact(recentPerformance.current.avgGold + recentPerformance.current.avgElixir), delta: perfDelta(recentPerformance.current.avgGold + recentPerformance.current.avgElixir, recentPerformance.previous.avgGold + recentPerformance.previous.avgElixir) },
          ].map((metric) => (
            <div key={metric.label} className="rounded-2xl border border-zinc-100 dark:border-zinc-800 bg-zinc-50/60 dark:bg-zinc-950/30 p-4">
              <div className="text-[9px] font-black uppercase tracking-[0.16em] text-zinc-400">{metric.label}</div>
              <div className="mt-2 text-xl font-black text-zinc-950 dark:text-white tabular-nums">{metric.value}</div>
              <div className={`mt-1 text-[9px] font-black uppercase tracking-wider ${
                metric.delta == null ? 'text-zinc-400' : metric.delta >= 0 ? 'text-emerald-500' : 'text-rose-500'
              }`}>
                {metric.delta == null ? 'No baseline' : `${metric.delta >= 0 ? '+' : ''}${metric.delta.toFixed(0)}% vs prev`}
              </div>
            </div>
          ))}
        </div>

        <div className="mt-4 grid grid-cols-1 md:grid-cols-2 gap-3">
          <div className="rounded-2xl border border-zinc-100 dark:border-zinc-800 p-4 flex items-center justify-between gap-4">
            <div>
              <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">Best G+E attack</div>
              <div className="mt-1 text-lg font-black text-zinc-950 dark:text-white">
                {bestRecords.bestLoot ? compact(
                  (bestRecords.bestLoot.gold_stolen || 0) + (bestRecords.bestLoot.bonus_gold || 0) +
                  (bestRecords.bestLoot.elixir_stolen || 0) + (bestRecords.bestLoot.bonus_elixir || 0)
                ) : '—'}
              </div>
            </div>
            <span className="material-symbols-outlined text-zinc-400">trophy</span>
          </div>
          <div className="rounded-2xl border border-zinc-100 dark:border-zinc-800 p-4 flex items-center justify-between gap-4">
            <div>
              <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">Fastest clean deploy</div>
              <div className="mt-1 text-lg font-black text-zinc-950 dark:text-white">
                {bestRecords.fastestClean ? `${(bestRecords.fastestClean.deploy_duration_ms / 1000).toFixed(1)}s` : '—'}
              </div>
            </div>
            <span className="material-symbols-outlined text-zinc-400">speed</span>
          </div>
        </div>
      </div>

      <div className="xl:col-span-2 bg-white dark:bg-zinc-900 p-8 rounded-[3rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none">
        <div className="flex flex-col md:flex-row md:items-end md:justify-between gap-4 mb-6">
          <div>
            <h3 className="text-2xl font-bold text-zinc-950 dark:text-white tracking-tight">Village Resource Tracking</h3>
            <p className="text-sm text-zinc-500 mt-1">Automatic BlueStacks snapshots. No manual entry required.</p>
          </div>
          <div className="text-[10px] font-black uppercase tracking-widest text-zinc-400">
            {validResourceHistory.length} snapshots
          </div>
        </div>

        {lastResource ? (
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            {[
              { label: 'Gold', current: lastResource.gold, delta: resourceDelta.gold, icon: 'monetization_on' },
              { label: 'Elixir', current: lastResource.elixir, delta: resourceDelta.elixir, icon: 'water_drop' },
              { label: 'Dark Elixir', current: lastResource.dark_elixir, delta: resourceDelta.dark, icon: 'opacity' },
            ].map((item) => (
              <div key={item.label} className="rounded-2xl border border-zinc-100 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-950/30 p-5">
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <div className="text-[10px] font-black uppercase tracking-[0.18em] text-zinc-400">{item.label}</div>
                    <div className="mt-2 text-2xl font-black text-zinc-950 dark:text-white tabular-nums">{item.current.toLocaleString()}</div>
                  </div>
                  <span className="material-symbols-outlined text-zinc-400">{item.icon}</span>
                </div>
                <div className={`mt-3 text-xs font-black tabular-nums ${item.delta >= 0 ? 'text-emerald-500' : 'text-rose-500'}`}>
                  {formatSigned(item.delta)} since first snapshot
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="py-10 text-center text-zinc-400 text-xs font-black uppercase tracking-widest">
            Waiting for ClashGO to scan the home village
          </div>
        )}
      </div>

      <div className="bg-white dark:bg-zinc-900 p-8 rounded-[3rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none group transition-all duration-500">
        <div className="flex justify-between items-center mb-8">
          <h3 className="text-2xl font-bold text-zinc-950 dark:text-white tracking-tight">Attack Success</h3>
          <div className="w-12 h-12 rounded-2xl bg-zinc-50 dark:bg-zinc-800 flex items-center justify-center border border-zinc-100 dark:border-zinc-700 shadow-sm transition-transform group-hover:scale-110">
            <span className="material-symbols-outlined text-zinc-500 dark:text-zinc-500">equalizer</span>
          </div>
        </div>
        {totalAttacks === 0 ? (
          <div className="py-12 text-center text-zinc-400 dark:text-zinc-700 text-[11px] font-black uppercase tracking-[0.3em] italic">
            No attacks recorded yet // Run the bot to populate analytics
          </div>
        ) : (
        <div className="grid grid-cols-1 sm:grid-cols-[auto_1fr] gap-10 items-center">
          {/* Star-distribution donut — center hole carries the 3★ rate. */}
          <div className="relative w-36 h-36 mx-auto rounded-full transition-transform duration-500 group-hover:scale-[1.03]" style={{ background: donutBg }}>
            <div className="absolute inset-[16px] bg-white dark:bg-zinc-900 rounded-full flex flex-col items-center justify-center border border-zinc-100 dark:border-zinc-800/60 shadow-sm">
              <span className="text-3xl font-bold text-zinc-950 dark:text-white tabular-nums tracking-tight leading-none">{threeStarRate}%</span>
              <span className="mt-1.5 text-[9px] font-black text-zinc-400 dark:text-zinc-600 uppercase tracking-[0.2em]">3★ Rate</span>
            </div>
          </div>
          <div className="space-y-6">
            {starData.map((item, idx) => {
              const percent = getPercent(item.count);
              return (
                <div key={idx} className="space-y-4">
                  <div className="flex justify-between text-[11px] font-black text-zinc-500 dark:text-zinc-500 uppercase tracking-[0.2em]">
                    <span>{item.label}</span>
                    <span className="text-zinc-950 dark:text-white tabular-nums">{item.count} <span className="text-zinc-400 dark:text-zinc-700 ml-2 font-bold">({percent}%)</span></span>
                  </div>
                  <div className="w-full bg-zinc-50 dark:bg-zinc-800/50 h-3.5 rounded-full overflow-hidden p-0.5 border border-zinc-100/50 dark:border-zinc-800/50">
                    <div 
                      className={`h-full transition-all duration-1000 rounded-full ${item.color} shadow-sm`} 
                      style={{ width: `${percent}%` }}
                    ></div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
        )}
      </div>

      <div className="xl:col-span-2 bg-white dark:bg-zinc-900 p-8 rounded-[3rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none">
        <div className="flex items-center justify-between gap-4 mb-6">
          <div>
            <h3 className="text-2xl font-bold text-zinc-950 dark:text-white tracking-tight">Strategy Performance</h3>
            <p className="text-sm text-zinc-500 mt-1">Built automatically from saved attack history.</p>
          </div>
          <div className="text-[10px] font-black uppercase tracking-widest text-zinc-400">{history?.length ?? 0} attacks</div>
        </div>

        {strategyStats.length === 0 ? (
          <div className="py-10 text-center text-zinc-400 text-xs font-black uppercase tracking-widest">
            Waiting for attack history
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[720px] text-left">
              <thead>
                <tr className="text-[9px] font-black uppercase tracking-[0.16em] text-zinc-400 border-b border-zinc-100 dark:border-zinc-800">
                  <th className="pb-3 pr-4">Strategy</th>
                  <th className="pb-3 px-3">Attacks</th>
                  <th className="pb-3 px-3">Avg stars</th>
                  <th className="pb-3 px-3">Full deploy</th>
                  <th className="pb-3 px-3">Avg gold</th>
                  <th className="pb-3 px-3">Avg elixir</th>
                  <th className="pb-3 pl-3">Avg DE</th>
                </tr>
              </thead>
              <tbody>
                {strategyStats.map((row) => (
                  <tr key={row.name} className="border-b border-zinc-50 dark:border-zinc-800/60 last:border-0">
                    <td className="py-4 pr-4 text-sm font-black text-zinc-950 dark:text-white">{row.name}</td>
                    <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{row.attacks}</td>
                    <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{(row.stars / Math.max(1, row.attacks)).toFixed(2)}</td>
                    <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{Math.round((row.completeDeploys / Math.max(1, row.attacks)) * 100)}%</td>
                    <td className="py-4 px-3 text-sm font-bold text-amber-500 tabular-nums">{Math.round(row.gold / Math.max(1, row.attacks)).toLocaleString()}</td>
                    <td className="py-4 px-3 text-sm font-bold text-fuchsia-500 tabular-nums">{Math.round(row.elixir / Math.max(1, row.attacks)).toLocaleString()}</td>
                    <td className="py-4 pl-3 text-sm font-bold text-zinc-500 tabular-nums">{Math.round(row.dark / Math.max(1, row.attacks)).toLocaleString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="xl:col-span-2 bg-white dark:bg-zinc-900 p-8 rounded-[3rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none">
        <div className="flex items-center justify-between gap-4 mb-6">
          <div>
            <h3 className="text-2xl font-bold text-zinc-950 dark:text-white tracking-tight">Deployment Side Performance</h3>
            <p className="text-sm text-zinc-500 mt-1">Measured from the physical side actually used outside the live red zone.</p>
          </div>
          <div className="text-[10px] font-black uppercase tracking-widest text-zinc-400">Observed only · no automatic strategy changes</div>
        </div>
        {sideStats.length === 0 ? (
          <div className="py-10 text-center text-zinc-400 text-xs font-black uppercase tracking-widest">Waiting for attack history</div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
            {sideStats.map((row) => (
              <div key={row.side} className="rounded-2xl border border-zinc-100 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-950/30 p-5">
                <div className="flex items-center justify-between gap-3">
                  <div className="text-sm font-black uppercase tracking-wider text-zinc-950 dark:text-white">{row.side}</div>
                  <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">{row.attacks} attacks</div>
                </div>
                <div className="mt-5 grid grid-cols-2 gap-4">
                  <div>
                    <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">Avg stars</div>
                    <div className="mt-1 text-xl font-black text-zinc-950 dark:text-white tabular-nums">{(row.stars / Math.max(1, row.attacks)).toFixed(2)}</div>
                  </div>
                  <div>
                    <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">Full deploy</div>
                    <div className="mt-1 text-xl font-black text-zinc-950 dark:text-white tabular-nums">{Math.round(row.complete / Math.max(1, row.attacks) * 100)}%</div>
                  </div>
                  <div>
                    <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">Avg loot</div>
                    <div className="mt-1 text-sm font-black text-zinc-950 dark:text-white tabular-nums">{compact(row.loot / Math.max(1, row.attacks))}</div>
                  </div>
                  <div>
                    <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">Avg deploy</div>
                    <div className="mt-1 text-sm font-black text-zinc-950 dark:text-white tabular-nums">{(row.deployMs / Math.max(1, row.attacks) / 1000).toFixed(1)}s</div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="xl:col-span-2 grid grid-cols-1 lg:grid-cols-[0.9fr_1.4fr] gap-4">
        <div className="bg-white dark:bg-zinc-900 p-6 rounded-[2.5rem] border border-zinc-100/70 dark:border-zinc-800/70 shadow-premium dark:shadow-none">
          <div className="flex items-center justify-between gap-3">
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Performance Guard</div>
              <h3 className="mt-1 text-xl font-bold text-zinc-950 dark:text-white tracking-tight">{performanceGuard.status}</h3>
            </div>
            <span className="material-symbols-outlined text-zinc-400">
              {performanceGuard.status === 'Healthy' ? 'verified' : performanceGuard.status === 'Watch' ? 'monitor_heart' : 'school'}
            </span>
          </div>
          <div className="mt-5 space-y-2">
            {performanceGuard.reasons.map((reason) => (
              <div key={reason} className="rounded-xl bg-zinc-50 dark:bg-zinc-950/40 px-3 py-2 text-[10px] font-bold text-zinc-500">
                {reason}
              </div>
            ))}
          </div>
          <p className="mt-4 text-[9px] font-bold uppercase tracking-wider text-zinc-400">
            Observation only — never changes deployment coordinates or strategy.
          </p>
        </div>

        <div className="bg-white dark:bg-zinc-900 p-6 rounded-[2.5rem] border border-zinc-100/70 dark:border-zinc-800/70 shadow-premium dark:shadow-none">
          <div className="flex items-center justify-between gap-4 mb-5">
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Runtime comparison</div>
              <h3 className="mt-1 text-xl font-bold text-zinc-950 dark:text-white tracking-tight">Fast / Balanced / Safe</h3>
            </div>
            <div className="text-[9px] font-black uppercase tracking-wider text-zinc-400">{history?.length ?? 0} attacks</div>
          </div>
          {modeStats.length === 0 ? (
            <div className="py-8 text-center text-zinc-400 text-xs font-black uppercase tracking-widest">Waiting for runtime samples</div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[760px] text-left">
                <thead>
                  <tr className="text-[9px] font-black uppercase tracking-[0.16em] text-zinc-400 border-b border-zinc-100 dark:border-zinc-800">
                    <th className="pb-3 pr-3">Mode</th>
                    <th className="pb-3 px-3">Attacks</th>
                    <th className="pb-3 px-3">Avg stars</th>
                    <th className="pb-3 px-3">Full deploy</th>
                    <th className="pb-3 px-3">Search</th>
                    <th className="pb-3 px-3">Deploy</th>
                    <th className="pb-3 px-3">Capture</th>
                    <th className="pb-3 pl-3">G+E</th>
                  </tr>
                </thead>
                <tbody>
                  {modeStats.map((row) => (
                    <tr key={row.mode} className="border-b border-zinc-50 dark:border-zinc-800/60 last:border-0">
                      <td className="py-4 pr-3 text-xs font-black uppercase tracking-wider text-zinc-950 dark:text-white">{row.mode}</td>
                      <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{row.attacks}</td>
                      <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{(row.stars / Math.max(1, row.attacks)).toFixed(2)}</td>
                      <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{Math.round(row.fullDeploys / Math.max(1, row.attacks) * 100)}%</td>
                      <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{(row.searchMs / Math.max(1, row.attacks) / 1000).toFixed(1)}s</td>
                      <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{(row.deployMs / Math.max(1, row.attacks) / 1000).toFixed(1)}s</td>
                      <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{(row.captureMs / Math.max(1, row.attacks)).toFixed(0)}ms</td>
                      <td className="py-4 pl-3 text-sm font-bold text-zinc-500 tabular-nums">{compact(row.goldElixir / Math.max(1, row.attacks))}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>

      <div className="xl:col-span-2 bg-white dark:bg-zinc-900 p-8 rounded-[3rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none">
        <div className="flex items-center justify-between gap-4 mb-6">
          <div>
            <h3 className="text-2xl font-bold text-zinc-950 dark:text-white tracking-tight">Strategy × Deploy Side</h3>
            <p className="text-sm text-zinc-500 mt-1">Observed combinations only. These numbers never change deployment automatically.</p>
          </div>
          <div className="text-[10px] font-black uppercase tracking-widest text-zinc-400">Top 16 samples</div>
        </div>

        {strategySideStats.length === 0 ? (
          <div className="py-10 text-center text-zinc-400 text-xs font-black uppercase tracking-widest">Waiting for attack history</div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[900px] text-left">
              <thead>
                <tr className="text-[9px] font-black uppercase tracking-[0.16em] text-zinc-400 border-b border-zinc-100 dark:border-zinc-800">
                  <th className="pb-3 pr-4">Strategy</th>
                  <th className="pb-3 px-3">Side</th>
                  <th className="pb-3 px-3">Attacks</th>
                  <th className="pb-3 px-3">Avg stars</th>
                  <th className="pb-3 px-3">Full deploy</th>
                  <th className="pb-3 px-3">Avg G+E</th>
                  <th className="pb-3 px-3">Avg DE</th>
                  <th className="pb-3 px-3">Deploy</th>
                  <th className="pb-3 pl-3">Cycle</th>
                </tr>
              </thead>
              <tbody>
                {strategySideStats.map((row) => (
                  <tr key={row.key} className="border-b border-zinc-50 dark:border-zinc-800/60 last:border-0">
                    <td className="py-4 pr-4 text-sm font-black text-zinc-950 dark:text-white">{row.strategy}</td>
                    <td className="py-4 px-3 text-xs font-black uppercase tracking-wider text-zinc-500">{row.side}</td>
                    <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{row.attacks}</td>
                    <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{(row.stars / Math.max(1, row.attacks)).toFixed(2)}</td>
                    <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{Math.round(row.complete / Math.max(1, row.attacks) * 100)}%</td>
                    <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{compact(row.goldElixir / Math.max(1, row.attacks))}</td>
                    <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{compact(row.dark / Math.max(1, row.attacks))}</td>
                    <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{(row.deployMs / Math.max(1, row.attacks) / 1000).toFixed(1)}s</td>
                    <td className="py-4 pl-3 text-sm font-bold text-zinc-500 tabular-nums">{(row.cycleMs / Math.max(1, row.attacks) / 1000).toFixed(0)}s</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="bg-white dark:bg-zinc-900 p-8 rounded-[3rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none transition-all duration-500">
        <div className="flex justify-between items-center mb-8">
          <h3 className="text-2xl font-bold text-zinc-950 dark:text-white tracking-tight">Financial Performance</h3>
          <div className="w-12 h-12 rounded-2xl bg-zinc-50 dark:bg-zinc-800 flex items-center justify-center border border-zinc-100 dark:border-zinc-700 shadow-sm">
            <span className="material-symbols-outlined text-zinc-500 dark:text-zinc-500">insights</span>
          </div>
        </div>
        {stats.attacks_completed === 0 ? (
          <div className="py-12 text-center text-zinc-400 dark:text-zinc-700 text-[11px] font-black uppercase tracking-[0.3em] italic">
            No revenue recorded yet // Run the bot to populate metrics
          </div>
        ) : (
        <div className="space-y-8">
          {[
            { label: 'Total Revenue', value: (stats.total_gold + stats.total_elixir).toLocaleString(), icon: 'account_balance_wallet', color: 'text-zinc-950 dark:text-zinc-100' },
            { label: 'Avg Gold / Attack', value: stats.attacks_completed > 0 ? Math.round(stats.total_gold / stats.attacks_completed).toLocaleString() : '0', icon: 'monetization_on', color: 'text-amber-500' },
            { label: 'Avg Elixir / Attack', value: stats.attacks_completed > 0 ? Math.round(stats.total_elixir / stats.attacks_completed).toLocaleString() : '0', icon: 'water_drop', color: 'text-fuchsia-500' },
            { label: 'Avg Dark Elixir / Attack', value: stats.attacks_completed > 0 ? Math.round(stats.total_de / stats.attacks_completed).toLocaleString() : '0', icon: 'water_drop', color: 'text-zinc-950 dark:text-zinc-100' }
          ].map((m, i) => (
            <div key={i} className="flex justify-between items-center group">
              <div className="flex items-center gap-6">
                <div className="w-14 h-14 rounded-2xl bg-zinc-50 dark:bg-zinc-800 flex items-center justify-center border border-zinc-100 dark:border-zinc-700 shadow-sm group-hover:scale-110 transition-transform duration-300">
                  <span className={`material-symbols-outlined ${m.color} text-2xl`}>{m.icon}</span>
                </div>
                <div>
                  <span className="block text-[10px] font-black text-zinc-500 dark:text-zinc-500 uppercase tracking-[0.2em] mb-1">{m.label}</span>
                  <div className="text-xs text-zinc-400 dark:text-zinc-700 font-bold uppercase tracking-widest">Calculated Average</div>
                </div>
              </div>
              <span className="text-3xl font-bold text-zinc-950 dark:text-white tracking-tight tabular-nums">{m.value}</span>
            </div>
          ))}
        </div>
        )}
      </div>

    </div>
  );
});

export default Analytics;
