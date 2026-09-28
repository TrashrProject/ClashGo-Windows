//go:build !windows

package adb

// ZoomOutSafe keeps the existing native pinch implementation on non-Windows
// platforms. The Windows build overrides this with the BlueStacks host-key path.
func (c *Client) ZoomOutSafe() error {
	return c.PinchZoom(true)
}
