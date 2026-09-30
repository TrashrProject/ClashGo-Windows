//go:build windows

package licensing

import (
	"encoding/base64"
	"errors"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const protectedSecretPrefix = "dpapi:"

func protectSecret(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	raw := []byte(value)
	in := windows.DataBlob{Size: uint32(len(raw)), Data: &raw[0]}
	var out windows.DataBlob
	if err := windows.CryptProtectData(
		&in,
		nil,
		nil,
		0,
		nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN,
		&out,
	); err != nil {
		return "", err
	}
	if out.Data == nil || out.Size == 0 {
		return "", errors.New("DPAPI returned an empty protected secret")
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(out.Data))))

	protected := append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...)
	return protectedSecretPrefix + base64.StdEncoding.EncodeToString(protected), nil
}

func unprotectSecret(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, protectedSecretPrefix) {
		// Legacy ClashGO builds stored the key directly. The caller migrates it
		// to DPAPI on the next save.
		return value, nil
	}

	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, protectedSecretPrefix))
	if err != nil {
		return "", err
	}
	if len(raw) == 0 {
		return "", errors.New("empty DPAPI payload")
	}

	in := windows.DataBlob{Size: uint32(len(raw)), Data: &raw[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(
		&in,
		nil,
		nil,
		0,
		nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN,
		&out,
	); err != nil {
		return "", err
	}
	if out.Data == nil || out.Size == 0 {
		return "", errors.New("DPAPI returned an empty secret")
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(out.Data))))

	plain := append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...)
	return string(plain), nil
}

func secretNeedsMigration(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.HasPrefix(value, protectedSecretPrefix)
}
