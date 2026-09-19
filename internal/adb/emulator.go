package adb

import "context"

// EnsureBlueStacks is the platform-neutral emulator entry point used by
// boot/recovery. OS-specific files implement ensureBlueStacksPlatform.
func (c *Client) EnsureBlueStacks(width, height, dpi int) error {
	return c.EnsureBlueStacksCtx(context.Background(), width, height, dpi)
}

// EnsureBlueStacksCtx is the cancellable platform-neutral entry point.
func (c *Client) EnsureBlueStacksCtx(ctx context.Context, width, height, dpi int) error {
	return c.ensureBlueStacksPlatform(ctx, width, height, dpi)
}
