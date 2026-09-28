export interface BotStats {
  attacks_completed: number;
  search_skips: number;
  total_gold: number;
  total_elixir: number;
  total_de: number;
  stars_0: number;
  stars_1: number;
  stars_2: number;
  stars_3: number;
  uptime: number; // nanoseconds
  // Device-independent CPU metrics. cpu_time_sec is absolute CPU seconds
  // since process start (comparable across machines). cpu_cores is CPU usage
  // as a fraction of one core over the last sample window (1.0 = one full
  // core busy). To show a 0-100% number on a specific host, multiply
  // cpu_cores by that host's logical core count.
  cpu_time_sec: number;
  cpu_cores: number;
  recovery_attempts: number;
  recovery_successes: number;
  bluestacks_restarts: number;
  gold_per_hour: number;
  elixir_per_hour: number;
  de_per_hour: number;
  average_stars: number;
  three_star_rate: number;
  average_capture_ms: number;
  last_capture_ms: number;
  telemetry_events: number;
  anomalies: number;
  targets_skipped: number;
  health_score: number;
  speed_profile: string;
  targets_seen: number;
  targets_accepted: number;
  target_acceptance_rate: number;
  avg_skips_per_attack: number;
  recovery_success_rate: number;
  average_target_scan_ms: number;
  last_target_scan_ms: number;
  average_return_home_ms: number;
  last_return_home_ms: number;
  average_next_transition_ms: number;
  last_next_transition_ms: number;
  avg_accepted_ge: number;
  avg_rejected_ge: number;
  avg_accepted_de: number;
  avg_rejected_de: number;
  avg_accepted_score: number;
  avg_rejected_score: number;
  adb_health: {
    avg_capture_ms: number;
    fast_capture_ms: number;
    avg_tap_ms: number;
    fast_tap_ms: number;
    taps_total: number;
    consecutive_fails: number;
    captures_total: number;
    errors_total: number;
    last_error?: string;
  };
}

export interface ActivityEvent {
  type: string;
  at: string;
  session_id?: string;
  fields?: Record<string, unknown>;
}

export interface AttackReplayPoint {
  x: number;
  y: number;
}

export interface AttackReplayEventView {
  offset_ms: number;
  kind: string;
  name?: string;
  category?: string;
  count?: number;
  slot_x?: number;
  slot_y?: number;
  deploy_side?: string;
  p1: AttackReplayPoint;
  p2: AttackReplayPoint;
}

export interface AttackReplayView {
  available: boolean;
  timestamp?: string;
  strategy?: string;
  complete: boolean;
  events: AttackReplayEventView[];
}

export interface VillageResourceSnapshot {
  timestamp: string;
  gold: number;
  elixir: number;
  dark_elixir: number;
  gold_valid: boolean;
  elixir_valid: boolean;
  dark_valid: boolean;
  valid: boolean;
}

export interface AttackReport {
  timestamp: string;
  session_id?: string;
  strategy: string;
  target_edge: string;
  deploy_side: string;
  deploy_success: boolean;
  undeployed_slots: number;
  deploy_error?: string;
  parsed_results: boolean;
  stars: number;
  gold_stolen: number;
  elixir_stolen: number;
  dark_elixir_stolen: number;
  bonus_gold: number;
  bonus_elixir: number;
  bonus_de: number;
  total_attacks_session: number;
  search_skips: number;
  search_duration_ms: number;
  cycle_duration_ms: number;
  deploy_duration_ms: number;
  battle_duration_ms: number;
  target_gold: number;
  target_elixir: number;
  target_de: number;
  target_score: number;
  runtime_mode: string;
  capture_ms: number;
  target_scan_ms: number;
  live_bar_rescans: number;
  avg_live_bar_rescan_ms: number;
  avg_slot_detect_ms: number;
  avg_slot_classify_ms: number;
  templates_tried: number;
  templates_matched: number;
  avg_selected_card_ocr_ms: number;
  preparation_duration_ms: number;
  cooldown_duration_ms: number;
  battle_end_reason: string;
  destruction_pct: number;
  town_hall_destroyed: boolean;
  return_home_duration_ms: number;
  return_home_success: boolean;
  full_routine_duration_ms: number;
}

export interface BotConfig {
  search: {
    min_loot_gold: number;
    min_loot_elixir: number;
    min_loot_de: number;
    enabled: boolean;
  };
  upgrade: {
    upgrade_walls: boolean;
  };
  attack: {
    strategy_file: string;
    stall_timer_seconds: number;
    loot_exit_enabled: boolean;
    loot_exit_percent: number;
  };
}

export type TabType = 'dashboard' | 'account' | 'analytics' | 'config' | 'settings';

// UpdateStatus mirrors internal/updater.Status (Go side).
// Casing follows Wails JSON convention (snake_case). Keep field names
// in sync with the Go struct — renaming here without updating
// updater.Status will break the banner silently.
export interface UpdateStatus {
  current_version: string;
  latest_version: string;
  available: boolean;
  // 'idle' | 'checking' | 'available' | 'downloading' | 'ready' | 'installing' | 'error' | 'up_to_date'
  state: string;
  progress: number; // 0..1
  notes: string;
  release_url: string;
  asset_name: string;
  download_path: string;
  expected_size: number;
  downloaded_size: number;
  error: string;
  last_checked_unix: number;
  skip_version: string;
  min_supported: string;
}

export const DEFAULT_UPDATE_STATUS: UpdateStatus = {
  current_version: '',
  latest_version: '',
  available: false,
  state: 'idle',
  progress: 0,
  notes: '',
  release_url: '',
  asset_name: '',
  download_path: '',
  expected_size: 0,
  downloaded_size: 0,
  error: '',
  last_checked_unix: 0,
  skip_version: '',
  min_supported: '',
};


export interface PlatformInstanceDiagnostic {
  name: string;
  adb_port: number;
  preferred: boolean;
}

export interface PlatformDiagnostics {
  os: string;
  supported: boolean;
  adb_executable: string;
  adb_found: boolean;
  bluestacks_player: string;
  bluestacks_player_found: boolean;
  bluestacks_config: string;
  bluestacks_config_found: boolean;
  bluestacks_running: boolean;
  adb_setting_present: boolean;
  adb_enabled: boolean;
  preferred_instance: string;
  instances: PlatformInstanceDiagnostic[];
}

export interface SystemDiagnostics {
  os: string;
  arch: string;
  version: string;
  assets_dir: string;
  config_dir: string;
  assets_ready: boolean;
  missing_assets: string[];
  configured_instance: string;
  emulator: PlatformDiagnostics;
}
