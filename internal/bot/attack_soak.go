package bot

const attackSoakTarget = 3

func (b *Bot) resetAttackSoak(reason string) {
	if b == nil {
		return
	}
	if prev := b.cleanAttackStreak.Swap(0); prev > 0 {
		b.logger.Warn().
			Int32("previous_streak", prev).
			Str("reason", reason).
			Msg("clean attack streak reset")
	}
}

func (b *Bot) recordAttackSoak(rep AttackReport, recoveryDelta, blueStacksRestartDelta int32) {
	if b == nil {
		return
	}
	clean := rep.DeploySuccess &&
		rep.ReturnHomeSuccess &&
		rep.ParsedResults &&
		recoveryDelta == 0 &&
		blueStacksRestartDelta == 0

	if !clean {
		b.logger.Warn().
			Bool("deploy_success", rep.DeploySuccess).
			Bool("return_home_success", rep.ReturnHomeSuccess).
			Bool("parsed_results", rep.ParsedResults).
			Int32("recovery_delta", recoveryDelta).
			Int32("bluestacks_restart_delta", blueStacksRestartDelta).
			Msg("attack does not qualify for clean soak streak")
		b.resetAttackSoak("attack_not_clean")
		return
	}

	streak := b.cleanAttackStreak.Add(1)
	b.logger.Info().
		Int32("clean_attack_streak", streak).
		Int("target", attackSoakTarget).
		Msg("clean attack soak progress")

	if streak >= attackSoakTarget && b.soakValidated.CompareAndSwap(false, true) {
		b.logger.Info().
			Int32("clean_attack_streak", streak).
			Msg("attack soak validated: 3 consecutive clean attacks")
		if b.telemetry != nil {
			b.telemetry.Emit("attack_soak_validated", map[string]any{
				"streak": streak,
				"target": attackSoakTarget,
			})
		}
	}
}
