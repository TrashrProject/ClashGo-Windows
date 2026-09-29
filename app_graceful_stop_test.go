package main

import (
	"testing"
	"time"

	"github.com/Ducky705/ClashGO/internal/bot"
)

func TestGracefulStopReachedIgnoresOldCompletedAttack(t *testing.T) {
	sequenceStart := time.Now().UTC().Truncate(time.Second)
	oldReport := bot.AttackReport{
		Timestamp:            sequenceStart.Add(-2 * time.Minute).Format(time.RFC3339),
		ReturnHomeDurationMS: 1200,
		ReturnHomeSuccess:    true,
	}
	if gracefulStopReached([]bot.AttackReport{oldReport}, sequenceStart.Unix()) {
		t.Fatal("old completed attack must not satisfy graceful stop for a newer sequence")
	}
}

func TestGracefulStopReachedRequiresConfirmedReturnHome(t *testing.T) {
	sequenceStart := time.Now().UTC().Truncate(time.Second)
	report := bot.AttackReport{
		Timestamp:            sequenceStart.Add(30 * time.Second).Format(time.RFC3339),
		ReturnHomeDurationMS: 1200,
		ReturnHomeSuccess:    false,
	}
	if gracefulStopReached([]bot.AttackReport{report}, sequenceStart.Unix()) {
		t.Fatal("failed return-home must not complete graceful stop")
	}
}

func TestGracefulStopReachedAcceptsCurrentSequenceCompletion(t *testing.T) {
	sequenceStart := time.Now().UTC().Truncate(time.Second)
	report := bot.AttackReport{
		Timestamp:            sequenceStart.Add(30 * time.Second).Format(time.RFC3339),
		ReturnHomeDurationMS: 1200,
		ReturnHomeSuccess:    true,
	}
	if !gracefulStopReached([]bot.AttackReport{report}, sequenceStart.Unix()) {
		t.Fatal("completed attack from the active sequence should satisfy graceful stop")
	}
}

func TestGracefulStopReachedRejectsIncompleteReport(t *testing.T) {
	sequenceStart := time.Now().UTC().Truncate(time.Second)
	report := bot.AttackReport{
		Timestamp:         sequenceStart.Add(30 * time.Second).Format(time.RFC3339),
		ReturnHomeSuccess: true,
	}
	if gracefulStopReached([]bot.AttackReport{report}, sequenceStart.Unix()) {
		t.Fatal("report published before ReturnHome must not satisfy graceful stop")
	}
}
