//go:build !darwin && !windows

package adb

import (
	"context"
	"errors"
	"time"
)

func (c *Client) ensureBlueStacksPlatform(_ context.Context, _, _, _ int) error {
	return errors.New("BlueStacks auto-config is not supported on this platform")
}

func (c *Client) EnsureBlueStacksMac(_, _, _ int) error {
	return errors.New("BlueStacks auto-config is not supported on this platform")
}

func (c *Client) EnsureBlueStacksMacCtx(_ context.Context, _, _, _ int) error {
	return errors.New("BlueStacks auto-config is not supported on this platform")
}

func (c *Client) isBlueStacksDevice(string) bool { return true }

func (c *Client) waitForVMProcess(_ context.Context, _ time.Duration) error {
	return errors.New("BlueStacks auto-config is not supported on this platform")
}

func (c *Client) waitForBlueStacksADB(_ context.Context, _ time.Duration) error {
	return errors.New("BlueStacks auto-config is not supported on this platform")
}

func (c *Client) launchBlueStacks(_ bool, _, _, _ int) error {
	return errors.New("BlueStacks auto-config is not supported on this platform")
}

func (c *Client) writeResolutionDefaultsIfPlistExists(_, _, _ int) {}
func (c *Client) firstVMSignal() string                               { return "" }
func (c *Client) tcpScanListens(_ []int, _ time.Duration) []int      { return nil }
