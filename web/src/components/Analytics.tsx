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
      routineMs: number;
      effectiveRoutineMs: number;
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
        deployMs: 0, cycleMs: 0, routineMs: 0, effectiveRoutineMs: 0, complete: 0,
      };
      row.attacks++;
      row.stars += rep.stars || 0;
      row.goldElixir += (rep.gold_stolen || 0) + (rep.bonus_gold || 0) + (rep.elixir_stolen || 0) + (rep.bonus_elixir || 0);
      row.dark += (rep.dark_elixir_stolen || 0) + (rep.bonus_de || 0);
      row.deployMs += rep.deploy_duration_ms || 0;
      row.cycleMs += rep.cycle_duration_ms || 0;
      row.routineMs += rep.full_routine_duration_ms || 0;
      row.effectiveRoutineMs += rep.full_routine_duration_ms || rep.cycle_duration_ms || 0;
      if (rep.deploy_success) row.complete++;
      map.set(key, row);
    }
    return Array.from(map.values())
      .sort((a, b) => b.attacks - a.attacks)
      .slice(0, 16);
  }, [history]);

  const strategyLab = React.useMemo(() => {
    return strategySideStats
      .filter((row) => row.attacks >= 5)
      .map((row) => {
        const measuredMs = row.effectiveRoutineMs;
        const hours = measuredMs > 0 ? measuredMs / 3_600_000 : 0;
        const yieldPerHour = hours > 0 ? row.goldElixir / hours : 0;
        const confidence = row.attacks >= 25 ? 'Strong' : row.attacks >= 10 ? 'Solid' : 'Building';
        return {
          ...row,
          yieldPerHour,
          avgStars: row.stars / Math.max(1, row.attacks),
          fullDeployRate: row.complete * 100 / Math.max(1, row.attacks),
          confidence,
        };
      })
      .sort((a, b) => {
        if (b.yieldPerHour !== a.yieldPerHour) return b.yieldPerHour - a.yieldPerHour;
        if (b.fullDeployRate !== a.fullDeployRate) return b.fullDeployRate - a.fullDeployRate;
        return b.avgStars - a.avgStars;
      })
      .slice(0, 3);
  }, [strategySideStats]);

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
      let searchN = 0, deployN = 0, captureN = 0, scanN = 0;
      for (const rep of rows) {
        if ((rep.search_duration_ms || 0) > 0) { search += rep.search_duration_ms; searchN++; }
        if ((rep.deploy_duration_ms || 0) > 0) { deploy += rep.deploy_duration_ms; deployN++; }
        if ((rep.capture_ms || 0) > 0) { capture += rep.capture_ms; captureN++; }
        if ((rep.target_scan_ms || 0) > 0) { scan += rep.target_scan_ms; scanN++; }
        if (rep.deploy_success) complete++;
      }
      const n = rows.length;
      return {
        search: searchN > 0 ? search / searchN : 0,
        deploy: deployN > 0 ? deploy / deployN : 0,
        capture: captureN > 0 ? capture / captureN : 0,
        scan: scanN > 0 ? scan / scanN : 0,
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

  const sessionStats = React.useMemo(() => {
    const map = new Map<string, {
      id: string;
      attacks: number;
      stars: number;
      triples: number;
      complete: number;
      gold: number;
      elixir: number;
      dark: number;
      cycleMs: number;
      routineMs: number;
      searchMs: number;
      deployMs: number;
      newestAt: number;
      oldestAt: number;
      firstCycleMs: number;
    }>();
    for (const rep of history ?? []) {
      if (!rep.session_id) continue;
      const row = map.get(rep.session_id) ?? {
        id: rep.session_id, attacks: 0, stars: 0, triples: 0, complete: 0,
        gold: 0, elixir: 0, dark: 0, cycleMs: 0, routineMs: 0, searchMs: 0, deployMs: 0,
        newestAt: 0, oldestAt: 0, firstCycleMs: 0,
      };
      row.attacks++;
      row.stars += rep.stars || 0;
      if ((rep.stars || 0) === 3) row.triples++;
      if (rep.deploy_success) row.complete++;
      row.gold += (rep.gold_stolen || 0) + (rep.bonus_gold || 0);
      row.elixir += (rep.elixir_stolen || 0) + (rep.bonus_elixir || 0);
      row.dark += (rep.dark_elixir_stolen || 0) + (rep.bonus_de || 0);
      row.cycleMs += rep.cycle_duration_ms || 0;
      row.routineMs += rep.full_routine_duration_ms || 0;
      row.searchMs += rep.search_duration_ms || 0;
      row.deployMs += rep.deploy_duration_ms || 0;
      const ts = Date.parse(rep.timestamp || '');
      if (Number.isFinite(ts)) {
        if (ts > row.newestAt) row.newestAt = ts;
        if (row.oldestAt === 0 || ts < row.oldestAt) {
          row.oldestAt = ts;
          row.firstCycleMs = rep.cycle_duration_ms || 0;
        }
      }
      map.set(rep.session_id, row);
    }
    return Array.from(map.values())
      .sort((a, b) => b.newestAt - a.newestAt)
      .slice(0, 8);
  }, [history]);

  const sessionComparison = React.useMemo(() => {
    const summarizeSession = (row: (typeof sessionStats)[number] | undefined) => {
      if (!row) return null;
      const fallbackWallMs = row.attacks <= 1
        ? row.cycleMs
        : Math.max(row.firstCycleMs, row.newestAt - row.oldestAt + row.firstCycleMs);
      const effectiveMs = row.routineMs > 0 ? row.routineMs : fallbackWallMs;
      const hours = effectiveMs > 0 ? effectiveMs / 3_600_000 : 0;
      return {
        attacks: row.attacks,
        gePerHour: hours > 0 ? (row.gold + row.elixir) / hours : 0,
        attacksPerHour: hours > 0 ? row.attacks / hours : 0,
        avgStars: row.stars / Math.max(1, row.attacks),
        fullDeployRate: row.complete * 100 / Math.max(1, row.attacks),
        avgLoopSeconds: effectiveMs / Math.max(1, row.attacks) / 1000,
      };
    };

    const current = summarizeSession(sessionStats[0]);
    const previous = summarizeSession(sessionStats[1]);
    const delta = (a: number, b: number, lowerIsBetter = false) => {
      if (!Number.isFinite(a) || !Number.isFinite(b) || b === 0) return null;
      const raw = (a - b) * 100 / Math.abs(b);
      return lowerIsBetter ? -raw : raw;
    };
    return {
      current,
      previous,
      metrics: current ? [
        { label: 'G+E / h', value: compact(current.gePerHour), delta: previous ? delta(current.gePerHour, previous.gePerHour) : null },
        { label: 'Attacks / h', value: current.attacksPerHour.toFixed(2), delta: previous ? delta(current.attacksPerHour, previous.attacksPerHour) : null },
        { label: 'Avg stars', value: current.avgStars.toFixed(2), delta: previous ? delta(current.avgStars, previous.avgStars) : null },
        { label: 'Full deploy', value: `${current.fullDeployRate.toFixed(0)}%`, delta: previous ? delta(current.fullDeployRate, previous.fullDeployRate) : null },
        { label: 'True loop', value: `${current.avgLoopSeconds.toFixed(0)}s`, delta: previous ? delta(current.avgLoopSeconds, previous.avgLoopSeconds, true) : null },
      ] : [],
    };
  }, [sessionStats]);

  const latencyPercentiles = React.useMemo(() => {
    const percentile = (values: number[], q: number) => {
      const sorted = values.filter((v) => Number.isFinite(v) && v > 0).sort((a, b) => a - b);
      if (sorted.length === 0) return 0;
      if (sorted.length === 1) return sorted[0];
      const pos = (sorted.length - 1) * q;
      const lo = Math.floor(pos);
      const hi = Math.ceil(pos);
      if (lo === hi) return sorted[lo];
      const weight = pos - lo;
      return sorted[lo] * (1 - weight) + sorted[hi] * weight;
    };
    const rows = history ?? [];
    const search = rows.map((r) => r.search_duration_ms || 0);
    const deploy = rows.map((r) => r.deploy_duration_ms || 0);
    const cycle = rows.map((r) => r.cycle_duration_ms || 0);
    return {
      searchP50: percentile(search, 0.50),
      searchP90: percentile(search, 0.90),
      deployP50: percentile(deploy, 0.50),
      deployP90: percentile(deploy, 0.90),
      cycleP50: percentile(cycle, 0.50),
      cycleP90: percentile(cycle, 0.90),
    };
  }, [history]);

  const targetScoreBuckets = React.useMemo(() => {
    const buckets = [
      { label: '<60', min: 1, max: 59, attacks: 0, stars: 0, full: 0, stolen: 0, offered: 0, cycleMs: 0 },
      { label: '60–74', min: 60, max: 74, attacks: 0, stars: 0, full: 0, stolen: 0, offered: 0, cycleMs: 0 },
      { label: '75–89', min: 75, max: 89, attacks: 0, stars: 0, full: 0, stolen: 0, offered: 0, cycleMs: 0 },
      { label: '90–100', min: 90, max: 100, attacks: 0, stars: 0, full: 0, stolen: 0, offered: 0, cycleMs: 0 },
    ];
    for (const rep of history ?? []) {
      const score = rep.target_score || 0;
      if (score <= 0) continue;
      const bucket = buckets.find((b) => score >= b.min && score <= b.max);
      if (!bucket) continue;
      bucket.attacks++;
      bucket.stars += rep.stars || 0;
      if (rep.deploy_success) bucket.full++;
      bucket.stolen += (rep.gold_stolen || 0) + (rep.elixir_stolen || 0);
      bucket.offered += (rep.target_gold || 0) + (rep.target_elixir || 0);
      bucket.cycleMs += rep.cycle_duration_ms || 0;
    }
    return buckets.filter((b) => b.attacks > 0);
  }, [history]);

  const resultTrust = React.useMemo(() => {
    const rows = history ?? [];
    const confidence = { high: 0, medium: 0, low: 0, unknown: 0 };
    const stars = new Map<string, number>();
    const loot = new Map<string, number>();
    for (const rep of rows) {
      const level = rep.result_confidence || 'unknown';
      if (level === 'high' || level === 'medium' || level === 'low') {
        confidence[level]++;
      } else {
        confidence.unknown++;
      }
      const starSource = rep.stars_source || 'legacy';
      const lootSource = rep.loot_source || 'legacy';
      stars.set(starSource, (stars.get(starSource) || 0) + 1);
      loot.set(lootSource, (loot.get(lootSource) || 0) + 1);
    }
    const total = rows.length;
    return {
      total,
      confidence,
      highRate: total > 0 ? confidence.high * 100 / total : 0,
      ocrStars: stars.get('result_ocr') || 0,
      outcomeStars: (stars.get('battle_outcome') || 0) + (stars.get('reconciled_outcome') || 0),
      liveLoot: loot.get('live_delta') || 0,
      ocrLoot: loot.get('result_ocr') || 0,
    };
  }, [history]);

  const endReasonStats = React.useMemo(() => {
    const map = new Map<string, {
      reason: string;
      attacks: number;
      stars: number;
      cycleMs: number;
      destruction: number;
      loot: number;
      full: number;
    }>();
    for (const rep of history ?? []) {
      const reason = rep.battle_end_reason || 'unknown';
      const row = map.get(reason) ?? {
        reason, attacks: 0, stars: 0, cycleMs: 0, destruction: 0, loot: 0, full: 0,
      };
      row.attacks++;
      row.stars += rep.stars || 0;
      row.cycleMs += rep.cycle_duration_ms || 0;
      row.destruction += rep.destruction_pct || 0;
      row.loot += (rep.gold_stolen || 0) + (rep.elixir_stolen || 0) + (rep.dark_elixir_stolen || 0);
      if (rep.deploy_success) row.full++;
      map.set(reason, row);
    }
    return Array.from(map.values()).sort((a, b) => b.attacks - a.attacks);
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
  const avgPreparationSeconds = history?.length
    ? history.reduce((sum, r) => sum + (r.preparation_duration_ms || 0), 0) / history.length / 1000
    : 0;
  const avgCooldownSeconds = history?.length
    ? history.reduce((sum, r) => sum + (r.cooldown_duration_ms || 0), 0) / history.length / 1000
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

  const deploymentSafety = React.useMemo(() => {
    const rows = (history ?? []).filter((r) => Boolean(r.safety_mode));
    if (rows.length === 0) {
      return {
        attacks: 0,
        liveCertified: 0,
        hudSafeRate: 0,
        corridorRate: 0,
        fallbacks: 0,
        modes: [] as Array<{ mode: string; count: number }>,
      };
    }

    const modeCounts = new Map<string, number>();
    let liveCertified = 0;
    let hudSafe = 0;
    let corridor = 0;
    let fallbacks = 0;
    for (const rep of rows) {
      const mode = rep.safety_mode || 'unknown';
      modeCounts.set(mode, (modeCounts.get(mode) || 0) + 1);
      if (rep.red_zone_valid && rep.corridor_verified && rep.hud_safe) liveCertified++;
      if (rep.hud_safe) hudSafe++;
      if (rep.corridor_verified) corridor++;
      if (mode !== 'live_red_zone') fallbacks++;
    }

    return {
      attacks: rows.length,
      liveCertified: liveCertified * 100 / rows.length,
      hudSafeRate: hudSafe * 100 / rows.length,
      corridorRate: corridor * 100 / rows.length,
      fallbacks,
      modes: Array.from(modeCounts.entries())
        .map(([mode, count]) => ({ mode, count }))
        .sort((a, b) => b.count - a.count),
    };
  }, [history]);

  const deployHotPath = React.useMemo(() => {
    const rows = (history ?? []).filter((r) => (r.live_bar_rescans || 0) > 0);
    if (rows.length === 0) {
      return { attacks: 0, avgRescans: 0, avgRescanMs: 0, avgDetectMs: 0, avgClassifyMs: 0, avgTemplatesTried: 0, avgTemplatesMatched: 0, avgCardOCRMs: 0 };
    }
    return {
      attacks: rows.length,
      avgRescans: rows.reduce((sum, r) => sum + (r.live_bar_rescans || 0), 0) / rows.length,
      avgRescanMs: rows.reduce((sum, r) => sum + (r.avg_live_bar_rescan_ms || 0), 0) / rows.length,
      avgDetectMs: rows.reduce((sum, r) => sum + (r.avg_slot_detect_ms || 0), 0) / rows.length,
      avgClassifyMs: rows.reduce((sum, r) => sum + (r.avg_slot_classify_ms || 0), 0) / rows.length,
      avgTemplatesTried: rows.reduce((sum, r) => sum + (r.templates_tried || 0), 0) / rows.length,
      avgTemplatesMatched: rows.reduce((sum, r) => sum + (r.templates_matched || 0), 0) / rows.length,
      avgCardOCRMs: rows.reduce((sum, r) => sum + (r.avg_selected_card_ocr_ms || 0), 0) / rows.length,
    };
  }, [history]);


  const deployBottleneck = React.useMemo(() => {
    const candidates = [
      { key: 'detect', label: 'Position detection', ms: deployHotPath.avgDetectMs },
      { key: 'classify', label: 'Classification', ms: deployHotPath.avgClassifyMs },
      { key: 'ocr', label: 'Selected-card OCR', ms: deployHotPath.avgCardOCRMs },
    ];
    const measured = candidates.filter((x) => Number.isFinite(x.ms) && x.ms > 0);
    if (measured.length === 0) {
      return { label: 'Learning', ms: 0, share: 0 };
    }
    const total = measured.reduce((sum, x) => sum + x.ms, 0);
    const dominant = measured.reduce((best, x) => x.ms > best.ms ? x : best, measured[0]);
    return {
      label: dominant.label,
      ms: dominant.ms,
      share: total > 0 ? dominant.ms * 100 / total : 0,
    };
  }, [deployHotPath]);

  const farmEfficiency = React.useMemo(() => {
    const rows = history ?? [];
    if (rows.length === 0) {
      return {
        attacksPerHour: 0,
        gePerActiveMinute: 0,
        gePerTrueMinute: 0,
        overheadShare: 0,
        avgTrueLoopSeconds: 0,
      };
    }

    let activeMS = 0;
    let trueMS = 0;
    let overheadMS = 0;
    let ge = 0;
    for (const rep of rows) {
      const active = (rep.search_duration_ms || 0) + (rep.deploy_duration_ms || 0) + Math.max(0, (rep.battle_duration_ms || 0) - (rep.deploy_duration_ms || 0));
      const truth = rep.full_routine_duration_ms || rep.cycle_duration_ms || active;
      activeMS += active;
      trueMS += truth;
      overheadMS += Math.max(0, truth - active);
      ge += (rep.gold_stolen || 0) + (rep.bonus_gold || 0) + (rep.elixir_stolen || 0) + (rep.bonus_elixir || 0);
    }

    const hours = trueMS > 0 ? trueMS / 3_600_000 : 0;
    const activeMinutes = activeMS > 0 ? activeMS / 60_000 : 0;
    const trueMinutes = trueMS > 0 ? trueMS / 60_000 : 0;
    return {
      attacksPerHour: hours > 0 ? rows.length / hours : 0,
      gePerActiveMinute: activeMinutes > 0 ? ge / activeMinutes : 0,
      gePerTrueMinute: trueMinutes > 0 ? ge / trueMinutes : 0,
      overheadShare: trueMS > 0 ? overheadMS * 100 / trueMS : 0,
      avgTrueLoopSeconds: trueMS / Math.max(1, rows.length) / 1000,
    };
  }, [history]);

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
      { label: 'Cooldown (intentional)', seconds: avgCooldownSeconds, tunable: false },
      { label: 'Preparation', seconds: avgPreparationSeconds, tunable: true },
      { label: 'Search', seconds: avgSearchSeconds, tunable: true },
      { label: 'Deployment', seconds: avgDeploySeconds, tunable: false },
      { label: 'Combat', seconds: avgCombatSeconds, tunable: true },
    ];
    const total = rows.reduce((sum, row) => sum + row.seconds, 0);
    const tunable = rows.filter((row) => row.tunable);
    return {
      rows: rows.map((row) => ({ ...row, share: total > 0 ? row.seconds * 100 / total : 0 })),
      dominant: rows.reduce((best, row) => row.seconds > best.seconds ? row : best, rows[0]),
      dominantTunable: tunable.reduce((best, row) => row.seconds > best.seconds ? row : best, tunable[0]),
    };
  }, [avgCooldownSeconds, avgPreparationSeconds, avgSearchSeconds, avgDeploySeconds, avgCombatSeconds]);

  const preparationBreakdown = React.useMemo(() => {
    const rows = (history ?? []).filter((r) => (r.preparation_duration_ms || 0) > 0);
    const definitions = [
      { key: 'attack', label: 'Attack button', read: (r: AttackReport) => r.prep_attack_button_ms || 0 },
      { key: 'find', label: 'Find Match', read: (r: AttackReport) => r.prep_find_match_ms || 0 },
      { key: 'armyMenu', label: 'Army menu', read: (r: AttackReport) => r.prep_army_menu_ms || 0 },
      { key: 'armySlot', label: 'Army slot', read: (r: AttackReport) => r.prep_army_slot_ms || 0 },
      { key: 'battle', label: 'Battle button', read: (r: AttackReport) => r.prep_battle_button_ms || 0 },
      { key: 'ready', label: 'Matchmaking ready', read: (r: AttackReport) => r.prep_matchmaking_ready_ms || 0 },
    ];

    const measured = definitions.map((d) => ({
      key: d.key,
      label: d.label,
      ms: rows.length > 0 ? rows.reduce((sum, row) => sum + d.read(row), 0) / rows.length : 0,
    }));
    const measuredTotal = measured.reduce((sum, row) => sum + row.ms, 0);
    const avgTotal = rows.length > 0
      ? rows.reduce((sum, row) => sum + (row.preparation_duration_ms || 0), 0) / rows.length
      : 0;
    const residual = Math.max(0, avgTotal - measuredTotal);
    const all = residual > 1
      ? [...measured, { key: 'other', label: 'Other overhead', ms: residual }]
      : measured;
    const dominant = all.reduce(
      (best, row) => row.ms > best.ms ? row : best,
      all[0] ?? { key: 'none', label: 'Learning', ms: 0 },
    );
    return {
      attacks: rows.length,
      rows: all.map((row) => ({
        ...row,
        share: avgTotal > 0 ? row.ms * 100 / avgTotal : 0,
      })),
      totalMS: avgTotal,
      dominant,
    };
  }, [history]);

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

  const tapTransport = React.useMemo(() => {
    const pipe = stats.adb_health?.pipe_taps_total || 0;
    const legacy = stats.adb_health?.legacy_taps_total || 0;
    const total = pipe + legacy;
    return {
      pipe,
      legacy,
      total,
      pipeRate: total > 0 ? pipe * 100 / total : 0,
      legacyRate: total > 0 ? legacy * 100 / total : 0,
      label: total === 0 ? 'Learning' : legacy >= pipe ? 'Legacy' : 'Pipe',
    };
  }, [stats.adb_health?.pipe_taps_total, stats.adb_health?.legacy_taps_total]);

  const optimizationAdvisor = React.useMemo(() => {
    type Opportunity = {
      key: string;
      label: string;
      evidence: string;
      next: string;
      score: number;
    };

    const opportunities: Opportunity[] = [];
    const reactiveTap = stats.adb_health?.fast_tap_ms || stats.adb_health?.avg_tap_ms || 0;
    const reactiveCapture = stats.adb_health?.fast_capture_ms || stats.adb_health?.avg_capture_ms || 0;
    const nextRate = stats.next_first_pass_rate || 0;
    const nextTransitions = stats.next_transitions || 0;

    if (reactiveTap > 0) {
      opportunities.push({
        key: 'tap',
        label: 'Windows tap transport',
        evidence: `${reactiveTap.toFixed(0)}ms reactive · ${tapTransport.total > 0 ? `${tapTransport.legacyRate.toFixed(0)}% legacy` : 'route learning'}`,
        next: reactiveTap >= 100
          ? 'High enough to justify a Windows-safe transport experiment with instant fallback.'
          : 'Tap transport is already relatively cheap; keep the proven deployment cadence.',
        score: reactiveTap >= 100 ? reactiveTap * 5 : reactiveTap,
      });
    }

    if (preparationBreakdown.attacks > 0 && preparationBreakdown.dominant.ms > 0) {
      opportunities.push({
        key: 'prep',
        label: `Preparation · ${preparationBreakdown.dominant.label}`,
        evidence: `${(preparationBreakdown.dominant.ms / 1000).toFixed(2)}s average · ${preparationBreakdown.totalMS > 0 ? (preparationBreakdown.dominant.ms * 100 / preparationBreakdown.totalMS).toFixed(0) : '0'}% of prep`,
        next: 'Optimize the verified UI transition only; do not replace state confirmation with blind coordinates.',
        score: preparationBreakdown.dominant.ms,
      });
    }

    if (nextTransitions >= 3 && nextRate > 0) {
      const retryShare = Math.max(0, 100 - nextRate);
      opportunities.push({
        key: 'next',
        label: 'Next transition verification',
        evidence: `${nextRate.toFixed(1)}% first-pass · ${(stats.avg_next_verify_probes || 0).toFixed(2)} probes`,
        next: retryShare >= 10
          ? 'Investigate why Clash ignores first taps before shortening any settle delay.'
          : 'First-pass reliability is strong; do not trade it for a more aggressive tap loop.',
        score: (stats.average_next_transition_ms || 0) * (retryShare / 100),
      });
    }

    if (deployHotPath.attacks > 0 && deployBottleneck.ms > 0) {
      opportunities.push({
        key: 'livebar',
        label: `Live bar · ${deployBottleneck.label}`,
        evidence: `${deployBottleneck.ms.toFixed(1)}ms · ${deployBottleneck.share.toFixed(0)}% of measured scan stages`,
        next: 'Optimize this detector in isolation while keeping live card re-indexing after every disappearance.',
        score: deployBottleneck.ms * Math.max(1, deployHotPath.avgRescans),
      });
    }

    if ((stats.average_target_scan_ms || 0) > 0) {
      opportunities.push({
        key: 'loot',
        label: 'Loot OCR',
        evidence: `${(stats.average_target_scan_ms || 0).toFixed(0)}ms average target scan`,
        next: 'Only optimize if it materially exceeds capture latency; target thresholds remain authoritative.',
        score: stats.average_target_scan_ms || 0,
      });
    }

    if (reactiveCapture > 0) {
      opportunities.push({
        key: 'capture',
        label: 'ADB capture',
        evidence: `${reactiveCapture.toFixed(0)}ms reactive capture latency`,
        next: reactiveCapture >= 700
          ? 'BlueStacks/ADB is under pressure; preserve capture gating and investigate transport health.'
          : 'Capture path is healthy enough; avoid removing the 120ms global capture budget.',
        score: reactiveCapture >= 700 ? reactiveCapture * 2 : reactiveCapture,
      });
    }

    const ranked = opportunities
      .filter((x) => Number.isFinite(x.score) && x.score > 0)
      .sort((a, b) => b.score - a.score);

    return {
      top: ranked[0] ?? null,
      items: ranked.slice(0, 4),
      learning: ranked.length === 0,
    };
  }, [
    stats,
    preparationBreakdown,
    deployHotPath,
    deployBottleneck,
    tapTransport,
  ]);

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
        <div className="grid grid-cols-2 md:grid-cols-4 xl:grid-cols-7 gap-3">
          {[
            { label: 'Gold / h', value: compact(stats.gold_per_hour || 0) },
            { label: 'Elixir / h', value: compact(stats.elixir_per_hour || 0) },
            { label: 'DE / h', value: compact(stats.de_per_hour || 0) },
            { label: 'Avg search', value: `${avgSearchSeconds.toFixed(1)}s` },
            { label: 'Loot scan', value: `${(stats.average_target_scan_ms || 0).toFixed(0)}ms` },
            { label: 'Avg deploy', value: `${avgDeploySeconds.toFixed(1)}s` },
            { label: 'Avg cycle', value: `${avgCycleSeconds.toFixed(1)}s` },
            { label: 'Capture', value: `${(stats.average_capture_ms || 0).toFixed(0)}ms` },
            { label: 'Reactive capture', value: `${(stats.adb_health?.fast_capture_ms || stats.adb_health?.avg_capture_ms || 0).toFixed(0)}ms` },
            { label: 'Avg tap', value: `${(stats.adb_health?.avg_tap_ms || 0).toFixed(0)}ms` },
            { label: 'Reactive tap', value: `${(stats.adb_health?.fast_tap_ms || stats.adb_health?.avg_tap_ms || 0).toFixed(0)}ms` },
            { label: 'Tap route', value: tapTransport.total > 0 ? `${tapTransport.legacyRate.toFixed(0)}% legacy` : '—' },
            { label: 'Return home', value: `${((stats.average_return_home_ms || 0) / 1000).toFixed(1)}s` },
            { label: 'Next transition', value: `${(stats.average_next_transition_ms || 0).toFixed(0)}ms` },
          ].map((metric) => (
            <div key={metric.label} className="rounded-2xl bg-white/5 dark:bg-zinc-950/5 border border-white/10 dark:border-zinc-950/10 p-4">
              <div className="text-[9px] font-black uppercase tracking-[0.18em] text-zinc-500">{metric.label}</div>
              <div className="mt-2 text-xl font-black text-white dark:text-zinc-950 tabular-nums">{metric.value}</div>
            </div>
          ))}
        </div>
      </div>

      <div className="xl:col-span-2 bg-zinc-950 dark:bg-white p-7 rounded-[2.5rem] shadow-premium-lg">
        <div className="flex flex-col lg:flex-row lg:items-end lg:justify-between gap-4 mb-6">
          <div>
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-500">Optimization Advisor</div>
            <h3 className="mt-1 text-2xl font-black text-white dark:text-zinc-950 tracking-tight">
              {optimizationAdvisor.top ? optimizationAdvisor.top.label : 'Learning the runtime'}
            </h3>
            <p className="mt-1 text-sm text-zinc-400 dark:text-zinc-500">
              Measured technical bottlenecks only. Never changes red-zone geometry, troop order, strategy or tap cadence automatically.
            </p>
          </div>
          <span className="material-symbols-outlined text-zinc-500 text-3xl">query_stats</span>
        </div>

        {optimizationAdvisor.learning ? (
          <div className="rounded-2xl border border-white/10 dark:border-zinc-950/10 bg-white/5 dark:bg-zinc-950/5 p-5 text-sm font-bold text-zinc-400 dark:text-zinc-500">
            Run a few attacks to build enough latency and transition evidence.
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-3">
            {optimizationAdvisor.items.map((item, index) => (
              <div key={item.key} className="rounded-2xl border border-white/10 dark:border-zinc-950/10 bg-white/5 dark:bg-zinc-950/5 p-5">
                <div className="flex items-center justify-between gap-3">
                  <div className="text-[9px] font-black uppercase tracking-[0.18em] text-zinc-500">
                    {index === 0 ? 'Top measured opportunity' : `Measured #${index + 1}`}
                  </div>
                  <div className="text-[9px] font-black tabular-nums text-zinc-500">#{index + 1}</div>
                </div>
                <div className="mt-3 text-base font-black text-white dark:text-zinc-950">{item.label}</div>
                <div className="mt-1 text-[10px] font-black uppercase tracking-wider text-zinc-500">{item.evidence}</div>
                <div className="mt-4 text-xs font-medium leading-relaxed text-zinc-400 dark:text-zinc-600">{item.next}</div>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="xl:col-span-2 grid grid-cols-1 lg:grid-cols-[1.3fr_1fr] gap-4">
        <div className="bg-white dark:bg-zinc-900 rounded-[2.5rem] border border-zinc-100/70 dark:border-zinc-800/70 p-6 shadow-premium dark:shadow-none">
          <div className="flex items-end justify-between gap-4 mb-5">
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Cycle anatomy</div>
              <h3 className="mt-1 text-xl font-bold text-zinc-950 dark:text-white tracking-tight">Where farming time goes</h3>
            </div>
            <div className="text-[10px] font-black uppercase tracking-widest text-zinc-400">
Best optimization target: {pipeline.dominantTunable.label}
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

      <div className="xl:col-span-2 bg-white dark:bg-zinc-900 p-6 rounded-[2.5rem] border border-zinc-100/70 dark:border-zinc-800/70 shadow-premium dark:shadow-none">
        <div className="flex flex-col md:flex-row md:items-end md:justify-between gap-4 mb-5">
          <div>
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Preparation Breakdown</div>
            <h3 className="mt-1 text-xl font-bold text-zinc-950 dark:text-white tracking-tight">Before matchmaking</h3>
            <p className="text-sm text-zinc-500 mt-1">Measures verified UI steps before search. No delay is shortened until live data proves where time is actually lost.</p>
          </div>
          <div className="text-right">
            <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">{preparationBreakdown.attacks} measured attacks</div>
            <div className="mt-1 text-[9px] font-black uppercase tracking-wider text-zinc-500">
              Bottleneck: {preparationBreakdown.dominant.label}{preparationBreakdown.dominant.ms > 0 ? ` · ${(preparationBreakdown.dominant.ms / 1000).toFixed(2)}s` : ''}
            </div>
          </div>
        </div>
        <div className="grid grid-cols-2 md:grid-cols-4 xl:grid-cols-7 gap-3">
          {preparationBreakdown.rows.map((row) => (
            <div key={row.key} className="rounded-2xl bg-zinc-50 dark:bg-zinc-950/40 border border-zinc-100 dark:border-zinc-800 p-4">
              <div className="text-[9px] font-black uppercase tracking-[0.16em] text-zinc-400">{row.label}</div>
              <div className="mt-2 text-xl font-black text-zinc-950 dark:text-white tabular-nums">{(row.ms / 1000).toFixed(2)}s</div>
              <div className="mt-1 text-[9px] font-bold uppercase tracking-wider text-zinc-400">{row.share.toFixed(0)}% of prep</div>
            </div>
          ))}
          <div className="rounded-2xl bg-zinc-950 dark:bg-white p-4">
            <div className="text-[9px] font-black uppercase tracking-[0.16em] text-zinc-500">Total prep</div>
            <div className="mt-2 text-xl font-black text-white dark:text-zinc-950 tabular-nums">{(preparationBreakdown.totalMS / 1000).toFixed(2)}s</div>
            <div className="mt-1 text-[9px] font-bold uppercase tracking-wider text-zinc-500">Verified path</div>
          </div>
        </div>
      </div>

      <div className="xl:col-span-2 bg-white dark:bg-zinc-900 p-6 rounded-[2.5rem] border border-zinc-100/70 dark:border-zinc-800/70 shadow-premium dark:shadow-none">
        <div className="flex flex-col md:flex-row md:items-end md:justify-between gap-4 mb-5">
          <div>
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Farm Efficiency</div>
            <h3 className="mt-1 text-xl font-bold text-zinc-950 dark:text-white tracking-tight">Real throughput</h3>
            <p className="text-sm text-zinc-500 mt-1">Uses true routine time when available, including return-home and preparation overhead.</p>
          </div>
          <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">History-weighted</div>
        </div>
        <div className="grid grid-cols-2 md:grid-cols-5 gap-3">
          {[
            { label: 'Attacks / h', value: farmEfficiency.attacksPerHour.toFixed(2), detail: 'True loop rate' },
            { label: 'G+E / active min', value: compact(farmEfficiency.gePerActiveMinute), detail: 'Search + deploy + combat' },
            { label: 'G+E / true min', value: compact(farmEfficiency.gePerTrueMinute), detail: 'Includes overhead' },
            { label: 'Overhead share', value: `${farmEfficiency.overheadShare.toFixed(1)}%`, detail: 'Outside active farming' },
            { label: 'Avg true loop', value: `${farmEfficiency.avgTrueLoopSeconds.toFixed(0)}s`, detail: 'Ready-to-ready' },
          ].map((metric) => (
            <div key={metric.label} className="rounded-2xl bg-zinc-50 dark:bg-zinc-950/40 border border-zinc-100 dark:border-zinc-800 p-4">
              <div className="text-[9px] font-black uppercase tracking-[0.16em] text-zinc-400">{metric.label}</div>
              <div className="mt-2 text-xl font-black text-zinc-950 dark:text-white tabular-nums">{metric.value}</div>
              <div className="mt-1 text-[9px] font-bold uppercase tracking-wider text-zinc-400">{metric.detail}</div>
            </div>
          ))}
        </div>
      </div>

      <div className="xl:col-span-2 bg-white dark:bg-zinc-900 p-6 rounded-[2.5rem] border border-zinc-100/70 dark:border-zinc-800/70 shadow-premium dark:shadow-none">
        <div className="flex flex-col md:flex-row md:items-end md:justify-between gap-4 mb-5">
          <div>
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Deployment Safety Contract</div>
            <h3 className="mt-1 text-xl font-bold text-zinc-950 dark:text-white tracking-tight">Red-zone / HUD compliance</h3>
            <p className="text-sm text-zinc-500 mt-1">Passive proof of the safety checks already used by the Windows deployment path.</p>
          </div>
          <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">
            {deploymentSafety.attacks > 0 ? `${deploymentSafety.attacks} measured attacks` : 'Learning'}
          </div>
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
          {[
            { label: 'Live certified', value: deploymentSafety.attacks ? `${deploymentSafety.liveCertified.toFixed(1)}%` : '—', detail: 'Red zone + corridor + HUD' },
            { label: 'Corridor verified', value: deploymentSafety.attacks ? `${deploymentSafety.corridorRate.toFixed(1)}%` : '—', detail: 'Strictly outside red bbox' },
            { label: 'HUD safe', value: deploymentSafety.attacks ? `${deploymentSafety.hudSafeRate.toFixed(1)}%` : '—', detail: 'Endpoints above UI cutoff' },
            { label: 'Fallback paths', value: deploymentSafety.fallbacks.toLocaleString(), detail: deploymentSafety.modes.slice(0, 2).map((m) => `${m.mode}: ${m.count}`).join(' · ') || 'No samples' },
          ].map((metric) => (
            <div key={metric.label} className="rounded-2xl bg-zinc-50 dark:bg-zinc-950/40 border border-zinc-100 dark:border-zinc-800 p-4">
              <div className="text-[9px] font-black uppercase tracking-[0.16em] text-zinc-400">{metric.label}</div>
              <div className="mt-2 text-xl font-black text-zinc-950 dark:text-white tabular-nums">{metric.value}</div>
              <div className="mt-1 text-[9px] font-bold uppercase tracking-wider text-zinc-400">{metric.detail}</div>
            </div>
          ))}
        </div>
      </div>

      <div className="xl:col-span-2 bg-white dark:bg-zinc-900 p-6 rounded-[2.5rem] border border-zinc-100/70 dark:border-zinc-800/70 shadow-premium dark:shadow-none">
        <div className="flex flex-col md:flex-row md:items-end md:justify-between gap-4 mb-5">
          <div>
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Deployment Hot Path</div>
            <h3 className="mt-1 text-xl font-bold text-zinc-950 dark:text-white tracking-tight">Live-bar cost per attack</h3>
            <p className="text-sm text-zinc-500 mt-1">Positions are still rescanned after every card; OCR is now limited to the selected card.</p>
          </div>
          <div className="text-right">
            <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">{deployHotPath.attacks} measured attacks</div>
            <div className="mt-1 text-[9px] font-black uppercase tracking-wider text-zinc-500">
              Bottleneck: {deployBottleneck.label}{deployBottleneck.ms > 0 ? ` · ${deployBottleneck.ms.toFixed(1)}ms / ${deployBottleneck.share.toFixed(0)}%` : ''}
            </div>
          </div>
        </div>
        <div className="grid grid-cols-2 md:grid-cols-4 xl:grid-cols-8 gap-3">
          {[
            { label: 'Rescans / attack', value: deployHotPath.avgRescans.toFixed(1), detail: 'Safety re-indexing kept' },
            { label: 'Total rescan', value: `${deployHotPath.avgRescanMs.toFixed(1)}ms`, detail: 'Per live-bar refresh' },
            { label: 'Position detect', value: `${deployHotPath.avgDetectMs.toFixed(1)}ms`, detail: 'Shared mask scan' },
            { label: 'Classification', value: `${deployHotPath.avgClassifyMs.toFixed(1)}ms`, detail: 'Identity/category matching' },
            { label: 'Templates tried', value: deployHotPath.avgTemplatesTried.toFixed(1), detail: 'Per attack average' },
            { label: 'Templates matched', value: deployHotPath.avgTemplatesMatched.toFixed(1), detail: 'Semantic cards found' },
            { label: 'Selected OCR', value: `${deployHotPath.avgCardOCRMs.toFixed(1)}ms`, detail: 'One chosen card only' },
            { label: 'Estimated scan work', value: `${(deployHotPath.avgRescans * (deployHotPath.avgRescanMs + deployHotPath.avgCardOCRMs)).toFixed(0)}ms`, detail: 'Measured hot-path work' },
          ].map((metric) => (
            <div key={metric.label} className="rounded-2xl bg-zinc-50 dark:bg-zinc-950/40 border border-zinc-100 dark:border-zinc-800 p-4">
              <div className="text-[9px] font-black uppercase tracking-[0.16em] text-zinc-400">{metric.label}</div>
              <div className="mt-2 text-xl font-black text-zinc-950 dark:text-white tabular-nums">{metric.value}</div>
              <div className="mt-1 text-[9px] font-bold uppercase tracking-wider text-zinc-400">{metric.detail}</div>
            </div>
          ))}
        </div>
      </div>

      <div className="xl:col-span-2 grid grid-cols-2 md:grid-cols-4 xl:grid-cols-8 gap-4">
        {[
          { label: 'Targets seen', value: (stats.targets_seen || 0).toLocaleString(), detail: 'This session' },
          { label: 'Accept rate', value: `${(stats.target_acceptance_rate || 0).toFixed(1)}%`, detail: 'Accepted / scanned' },
          { label: 'Skips / attack', value: (stats.avg_skips_per_attack || 0).toFixed(1), detail: 'Lower is faster' },
          { label: 'Next first-pass', value: stats.next_transitions > 0 ? `${(stats.next_first_pass_rate || 0).toFixed(1)}%` : '—', detail: 'No controlled retry' },
          { label: 'Next retries', value: (stats.next_retries || 0).toLocaleString(), detail: `${stats.next_transitions || 0} transitions` },
          { label: 'Verify probes', value: (stats.avg_next_verify_probes || 0).toFixed(2), detail: 'Captures / transition' },
          { label: 'Next latency', value: `${(stats.average_next_transition_ms || 0).toFixed(0)}ms`, detail: 'Tap → transition' },
          { label: 'Recovery success', value: stats.recovery_attempts > 0 ? `${(stats.recovery_success_rate || 0).toFixed(0)}%` : '—', detail: stats.recovery_attempts > 0 ? `${stats.recovery_successes}/${stats.recovery_attempts}` : 'No recoveries' },
        ].map((metric) => (
          <div key={metric.label} className="bg-white dark:bg-zinc-900 rounded-[2rem] border border-zinc-100/70 dark:border-zinc-800/70 p-5 shadow-premium dark:shadow-none">
            <div className="text-[9px] font-black uppercase tracking-[0.18em] text-zinc-400">{metric.label}</div>
            <div className="mt-2 text-2xl font-black text-zinc-950 dark:text-white tabular-nums">{metric.value}</div>
            <div className="mt-1 text-[9px] font-bold uppercase tracking-wider text-zinc-400">{metric.detail}</div>
          </div>
        ))}
      </div>

      <div className="xl:col-span-2 bg-white dark:bg-zinc-900 p-6 rounded-[2.5rem] border border-zinc-100/70 dark:border-zinc-800/70 shadow-premium dark:shadow-none">
        <div className="flex flex-col md:flex-row md:items-end md:justify-between gap-4 mb-5">
          <div>
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Search Intelligence</div>
            <h3 className="mt-1 text-xl font-bold text-zinc-950 dark:text-white tracking-tight">Accepted vs rejected targets</h3>
            <p className="text-sm text-zinc-500 mt-1">Session-only aggregation. Rejected bases stay off disk and never slow the matchmaking path.</p>
          </div>
          <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">
            {(stats.targets_accepted || 0).toLocaleString()} accepted · {Math.max(0, (stats.targets_seen || 0) - (stats.targets_accepted || 0)).toLocaleString()} rejected
          </div>
        </div>
        <div className="grid grid-cols-2 md:grid-cols-3 xl:grid-cols-6 gap-3">
          {[
            { label: 'Accepted G+E', value: compact(stats.avg_accepted_ge || 0), detail: 'Average target value' },
            { label: 'Rejected G+E', value: compact(stats.avg_rejected_ge || 0), detail: 'What thresholds skip' },
            { label: 'Accepted DE', value: compact(stats.avg_accepted_de || 0), detail: 'Average target DE' },
            { label: 'Rejected DE', value: compact(stats.avg_rejected_de || 0), detail: 'Skipped target DE' },
            { label: 'Accepted score', value: `${(stats.avg_accepted_score || 0).toFixed(0)}/100`, detail: 'Target Intelligence' },
            { label: 'Rejected score', value: `${(stats.avg_rejected_score || 0).toFixed(0)}/100`, detail: 'Target Intelligence' },
          ].map((metric) => (
            <div key={metric.label} className="rounded-2xl bg-zinc-50 dark:bg-zinc-950/40 border border-zinc-100 dark:border-zinc-800 p-4">
              <div className="text-[9px] font-black uppercase tracking-[0.16em] text-zinc-400">{metric.label}</div>
              <div className="mt-2 text-xl font-black text-zinc-950 dark:text-white tabular-nums">{metric.value}</div>
              <div className="mt-1 text-[9px] font-bold uppercase tracking-wider text-zinc-400">{metric.detail}</div>
            </div>
          ))}
        </div>
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

      <div className="xl:col-span-2 grid grid-cols-1 lg:grid-cols-2 gap-4">
        <div className="bg-white dark:bg-zinc-900 p-6 rounded-[2.5rem] border border-zinc-100/70 dark:border-zinc-800/70 shadow-premium dark:shadow-none">
          <div className="flex items-center justify-between gap-4 mb-5">
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Tail latency</div>
              <h3 className="mt-1 text-xl font-bold text-zinc-950 dark:text-white tracking-tight">P50 / P90 Timing</h3>
            </div>
            <span className="material-symbols-outlined text-zinc-400">speed</span>
          </div>
          <div className="grid grid-cols-3 gap-3">
            {[
              { label: 'Search', p50: latencyPercentiles.searchP50, p90: latencyPercentiles.searchP90 },
              { label: 'Deploy', p50: latencyPercentiles.deployP50, p90: latencyPercentiles.deployP90 },
              { label: 'Cycle', p50: latencyPercentiles.cycleP50, p90: latencyPercentiles.cycleP90 },
            ].map((row) => (
              <div key={row.label} className="rounded-2xl bg-zinc-50 dark:bg-zinc-950/40 p-4">
                <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">{row.label}</div>
                <div className="mt-2 text-xl font-black text-zinc-950 dark:text-white tabular-nums">{(row.p50 / 1000).toFixed(1)}s</div>
                <div className="mt-1 text-[9px] font-bold uppercase tracking-wider text-zinc-400">P50</div>
                <div className="mt-3 text-sm font-black text-zinc-700 dark:text-zinc-200 tabular-nums">{(row.p90 / 1000).toFixed(1)}s</div>
                <div className="mt-1 text-[9px] font-bold uppercase tracking-wider text-zinc-400">P90</div>
              </div>
            ))}
          </div>
        </div>

        <div className="bg-white dark:bg-zinc-900 p-6 rounded-[2.5rem] border border-zinc-100/70 dark:border-zinc-800/70 shadow-premium dark:shadow-none">
          <div className="flex items-center justify-between gap-4 mb-5">
            <div>
              <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Target Intelligence</div>
              <h3 className="mt-1 text-xl font-bold text-zinc-950 dark:text-white tracking-tight">Score Bucket Results</h3>
            </div>
            <span className="material-symbols-outlined text-zinc-400">target</span>
          </div>
          {targetScoreBuckets.length === 0 ? (
            <div className="py-8 text-center text-zinc-400 text-xs font-black uppercase tracking-widest">Waiting for scored attacks</div>
          ) : (
            <div className="space-y-2">
              {targetScoreBuckets.map((row) => (
                <div key={row.label} className="grid grid-cols-[70px_1fr_1fr_1fr] items-center gap-3 rounded-xl bg-zinc-50 dark:bg-zinc-950/40 px-3 py-3">
                  <div className="text-xs font-black text-zinc-950 dark:text-white">{row.label}</div>
                  <div>
                    <div className="text-[9px] font-black uppercase tracking-wider text-zinc-400">Avg stars</div>
                    <div className="text-sm font-black text-zinc-700 dark:text-zinc-200 tabular-nums">{(row.stars / row.attacks).toFixed(2)}</div>
                  </div>
                  <div>
                    <div className="text-[9px] font-black uppercase tracking-wider text-zinc-400">Loot capture</div>
                    <div className="text-sm font-black text-zinc-700 dark:text-zinc-200 tabular-nums">{row.offered > 0 ? `${(row.stolen * 100 / row.offered).toFixed(0)}%` : '—'}</div>
                  </div>
                  <div>
                    <div className="text-[9px] font-black uppercase tracking-wider text-zinc-400">Cycle</div>
                    <div className="text-sm font-black text-zinc-700 dark:text-zinc-200 tabular-nums">{(row.cycleMs / row.attacks / 1000).toFixed(0)}s</div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      <div className="xl:col-span-2 bg-white dark:bg-zinc-900 p-8 rounded-[3rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none">
        <div className="flex items-center justify-between gap-4 mb-6">
          <div>
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Persistent sessions</div>
            <h3 className="mt-1 text-2xl font-bold text-zinc-950 dark:text-white tracking-tight">Farm Session Comparison</h3>
            <p className="text-sm text-zinc-500 mt-1">New sessions are grouped automatically from saved attack reports.</p>
          </div>
          <div className="text-[10px] font-black uppercase tracking-widest text-zinc-400">{sessionStats.length} recent sessions</div>
        </div>
        {sessionStats.length === 0 ? (
          <div className="py-10 text-center text-zinc-400 text-xs font-black uppercase tracking-widest">
            New attacks will start building session history
          </div>
        ) : (
          <>
            <div className="grid grid-cols-2 md:grid-cols-5 gap-3 mb-6">
              {sessionComparison.metrics.map((metric) => (
                <div key={metric.label} className="rounded-2xl bg-zinc-50 dark:bg-zinc-950/40 border border-zinc-100 dark:border-zinc-800 p-4">
                  <div className="text-[9px] font-black uppercase tracking-[0.16em] text-zinc-400">{metric.label}</div>
                  <div className="mt-2 text-xl font-black text-zinc-950 dark:text-white tabular-nums">{metric.value}</div>
                  <div className={`mt-1 text-[9px] font-black uppercase tracking-wider ${metric.delta == null ? 'text-zinc-400' : metric.delta >= 0 ? 'text-emerald-500' : 'text-rose-500'}`}>
                    {metric.delta == null ? 'No previous session' : `${metric.delta >= 0 ? '+' : ''}${metric.delta.toFixed(0)}% vs previous`}
                  </div>
                </div>
              ))}
            </div>
            <div className="overflow-x-auto">
            <table className="w-full min-w-[980px] text-left">
              <thead>
                <tr className="text-[9px] font-black uppercase tracking-[0.16em] text-zinc-400 border-b border-zinc-100 dark:border-zinc-800">
                  <th className="pb-3 pr-4">Session</th>
                  <th className="pb-3 px-3">Attacks</th>
                  <th className="pb-3 px-3">Avg stars</th>
                  <th className="pb-3 px-3">3★</th>
                  <th className="pb-3 px-3">Full deploy</th>
                  <th className="pb-3 px-3">G / h</th>
                  <th className="pb-3 px-3">E / h</th>
                  <th className="pb-3 px-3">DE / h</th>
                  <th className="pb-3 px-3">Search</th>
                  <th className="pb-3 px-3">Deploy</th>
                  <th className="pb-3 pl-3">True loop</th>
                </tr>
              </thead>
              <tbody>
                {sessionStats.map((row, index) => {
                  // Prefer the explicit ready-to-return-home routine timings.
                  // Older history rows lack that field, so keep the wall-clock
                  // fallback for backward compatibility.
                  const fallbackWallMs = row.attacks <= 1
                    ? row.cycleMs
                    : Math.max(row.firstCycleMs, row.newestAt - row.oldestAt + row.firstCycleMs);
                  const effectiveMs = row.routineMs > 0 ? row.routineMs : fallbackWallMs;
                  const hours = effectiveMs > 0 ? effectiveMs / 3_600_000 : 0;
                  return (
                    <tr key={row.id} className="border-b border-zinc-50 dark:border-zinc-800/60 last:border-0">
                      <td className="py-4 pr-4">
                        <div className="text-sm font-black text-zinc-950 dark:text-white">#{index + 1}</div>
                        <div className="mt-1 text-[9px] font-bold text-zinc-400 font-mono">{row.id.slice(-12)}</div>
                      </td>
                      <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{row.attacks}</td>
                      <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{(row.stars / Math.max(1, row.attacks)).toFixed(2)}</td>
                      <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{Math.round(row.triples / Math.max(1, row.attacks) * 100)}%</td>
                      <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{Math.round(row.complete / Math.max(1, row.attacks) * 100)}%</td>
                      <td className="py-4 px-3 text-sm font-bold text-amber-500 tabular-nums">{hours > 0 ? compact(row.gold / hours) : '—'}</td>
                      <td className="py-4 px-3 text-sm font-bold text-fuchsia-500 tabular-nums">{hours > 0 ? compact(row.elixir / hours) : '—'}</td>
                      <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{hours > 0 ? compact(row.dark / hours) : '—'}</td>
                      <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{(row.searchMs / Math.max(1, row.attacks) / 1000).toFixed(1)}s</td>
                      <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{(row.deployMs / Math.max(1, row.attacks) / 1000).toFixed(1)}s</td>
                      <td className="py-4 pl-3 text-sm font-bold text-zinc-500 tabular-nums">{(effectiveMs / Math.max(1, row.attacks) / 1000).toFixed(0)}s</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          </>
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

      <div className="xl:col-span-2 bg-white dark:bg-zinc-900 p-6 rounded-[2.5rem] border border-zinc-100/70 dark:border-zinc-800/70 shadow-premium dark:shadow-none">
        <div className="flex flex-col md:flex-row md:items-end md:justify-between gap-4 mb-5">
          <div>
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Result Trust</div>
            <h3 className="mt-1 text-xl font-bold text-zinc-950 dark:text-white tracking-tight">Where battle numbers come from</h3>
            <p className="text-sm text-zinc-500 mt-1">ClashGO records whether stars and loot came from result OCR, measured battle outcome or stable live-loot deltas.</p>
          </div>
          <div className="text-[9px] font-black uppercase tracking-widest text-zinc-400">
            {resultTrust.total > 0 ? `${resultTrust.highRate.toFixed(0)}% high-confidence` : 'Waiting for V2 results'}
          </div>
        </div>
        <div className="grid grid-cols-2 md:grid-cols-4 xl:grid-cols-7 gap-3">
          {[
            { label: 'High confidence', value: resultTrust.confidence.high.toLocaleString(), detail: 'Outcome + live delta' },
            { label: 'Medium', value: resultTrust.confidence.medium.toLocaleString(), detail: 'Reliable single source' },
            { label: 'Low / unknown', value: (resultTrust.confidence.low + resultTrust.confidence.unknown).toLocaleString(), detail: 'Fallback / legacy rows' },
            { label: 'Stars via OCR', value: resultTrust.ocrStars.toLocaleString(), detail: 'Result screen' },
            { label: 'Stars via outcome', value: resultTrust.outcomeStars.toLocaleString(), detail: 'Destruction / reconciliation' },
            { label: 'Loot live delta', value: resultTrust.liveLoot.toLocaleString(), detail: 'Preferred stable source' },
            { label: 'Loot via OCR', value: resultTrust.ocrLoot.toLocaleString(), detail: 'Result screen fallback' },
          ].map((metric) => (
            <div key={metric.label} className="rounded-2xl bg-zinc-50 dark:bg-zinc-950/40 border border-zinc-100 dark:border-zinc-800 p-4">
              <div className="text-[9px] font-black uppercase tracking-[0.16em] text-zinc-400">{metric.label}</div>
              <div className="mt-2 text-xl font-black text-zinc-950 dark:text-white tabular-nums">{metric.value}</div>
              <div className="mt-1 text-[9px] font-bold uppercase tracking-wider text-zinc-400">{metric.detail}</div>
            </div>
          ))}
        </div>
      </div>

      <div className="xl:col-span-2 bg-white dark:bg-zinc-900 p-8 rounded-[3rem] border border-zinc-100/50 dark:border-zinc-800/50 shadow-premium dark:shadow-none">
        <div className="flex items-center justify-between gap-4 mb-6">
          <div>
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-400">Battle termination</div>
            <h3 className="mt-1 text-2xl font-bold text-zinc-950 dark:text-white tracking-tight">Why Battles End</h3>
            <p className="text-sm text-zinc-500 mt-1">Natural result, thresholds, stalls and timeouts measured separately.</p>
          </div>
          <span className="material-symbols-outlined text-zinc-400">flag</span>
        </div>
        {endReasonStats.length === 0 ? (
          <div className="py-10 text-center text-zinc-400 text-xs font-black uppercase tracking-widest">Waiting for completed attacks</div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[760px] text-left">
              <thead>
                <tr className="text-[9px] font-black uppercase tracking-[0.16em] text-zinc-400 border-b border-zinc-100 dark:border-zinc-800">
                  <th className="pb-3 pr-4">Reason</th>
                  <th className="pb-3 px-3">Attacks</th>
                  <th className="pb-3 px-3">Avg stars</th>
                  <th className="pb-3 px-3">Avg destruction</th>
                  <th className="pb-3 px-3">Full deploy</th>
                  <th className="pb-3 px-3">Avg loot</th>
                  <th className="pb-3 pl-3">Avg cycle</th>
                </tr>
              </thead>
              <tbody>
                {endReasonStats.map((row) => (
                  <tr key={row.reason} className="border-b border-zinc-50 dark:border-zinc-800/60 last:border-0">
                    <td className="py-4 pr-4 text-xs font-black uppercase tracking-wider text-zinc-950 dark:text-white">{row.reason.replaceAll('_', ' ')}</td>
                    <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{row.attacks}</td>
                    <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{(row.stars / row.attacks).toFixed(2)}</td>
                    <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{(row.destruction / row.attacks).toFixed(0)}%</td>
                    <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{Math.round(row.full / row.attacks * 100)}%</td>
                    <td className="py-4 px-3 text-sm font-bold text-zinc-500 tabular-nums">{compact(row.loot / row.attacks)}</td>
                    <td className="py-4 pl-3 text-sm font-bold text-zinc-500 tabular-nums">{(row.cycleMs / row.attacks / 1000).toFixed(0)}s</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="xl:col-span-2 bg-zinc-950 dark:bg-white p-8 rounded-[3rem] shadow-premium-lg">
        <div className="flex flex-col md:flex-row md:items-end md:justify-between gap-4 mb-6">
          <div>
            <div className="text-[10px] font-black uppercase tracking-[0.22em] text-zinc-500">Strategy Lab</div>
            <h3 className="mt-1 text-2xl font-black text-white dark:text-zinc-950 tracking-tight">Observed Farming Leaders</h3>
            <p className="text-sm text-zinc-400 dark:text-zinc-500 mt-1">Requires at least 5 attacks per strategy × side. Observation only; ClashGO never changes your strategy from this panel.</p>
          </div>
          <span className="material-symbols-outlined text-zinc-500">science</span>
        </div>

        {strategyLab.length === 0 ? (
          <div className="py-10 text-center text-zinc-500 text-xs font-black uppercase tracking-widest">
            Need 5+ attacks on the same strategy × side to compare reliably
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            {strategyLab.map((row, index) => (
              <div key={row.key} className="rounded-2xl border border-white/10 dark:border-zinc-950/10 bg-white/5 dark:bg-zinc-950/5 p-5">
                <div className="flex items-center justify-between gap-3">
                  <div className="text-[9px] font-black uppercase tracking-[0.18em] text-zinc-500">Observed #{index + 1}</div>
                  <div className="text-[9px] font-black uppercase tracking-wider text-zinc-500">{row.confidence} · n={row.attacks}</div>
                </div>
                <div className="mt-3 text-lg font-black text-white dark:text-zinc-950 truncate">{row.strategy}</div>
                <div className="mt-1 text-[10px] font-black uppercase tracking-widest text-zinc-500">{row.side}</div>
                <div className="mt-5 grid grid-cols-3 gap-2">
                  <div>
                    <div className="text-[8px] font-black uppercase tracking-wider text-zinc-500">G+E / h</div>
                    <div className="mt-1 text-sm font-black text-white dark:text-zinc-950 tabular-nums">{compact(row.yieldPerHour)}</div>
                  </div>
                  <div>
                    <div className="text-[8px] font-black uppercase tracking-wider text-zinc-500">Stars</div>
                    <div className="mt-1 text-sm font-black text-white dark:text-zinc-950 tabular-nums">{row.avgStars.toFixed(2)}</div>
                  </div>
                  <div>
                    <div className="text-[8px] font-black uppercase tracking-wider text-zinc-500">Full deploy</div>
                    <div className="mt-1 text-sm font-black text-white dark:text-zinc-950 tabular-nums">{row.fullDeployRate.toFixed(0)}%</div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
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
