package configmanager

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	plaintext := "sk-test-abcdefghijklmnopqrstuvwxyz"

	encrypted, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	if encrypted == plaintext {
		t.Fatalf("expected encrypted value to differ from plaintext")
	}

	if len(encrypted) <= len(encPrefix) || encrypted[:len(encPrefix)] != encPrefix {
		t.Fatalf("expected encrypted value to have prefix %q, got %q", encPrefix, encrypted)
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt returned error: %v", err)
	}

	if decrypted != plaintext {
		t.Fatalf("decrypted plaintext mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptPlaintext(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	plaintext := "not-encrypted"

	decrypted, err := Decrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Decrypt returned error: %v", err)
	}

	if decrypted != plaintext {
		t.Fatalf("plaintext passthrough mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptEncryptedPayloadTooShort(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("aes.NewCipher returned error: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("cipher.NewGCM returned error: %v", err)
	}

	shortPayload := make([]byte, gcm.NonceSize()+gcm.Overhead()-1)
	value := encPrefix + base64.StdEncoding.EncodeToString(shortPayload)

	_, err = Decrypt(value, key)
	if err == nil {
		t.Fatalf("expected Decrypt to return an error for short encrypted payload")
	}

	if !errors.Is(err, errCiphertextTooShort) {
		t.Fatalf("expected error %q, got %v", errCiphertextTooShort, err)
	}
}

func TestEncryptInvalidKeyLength(t *testing.T) {
	_, err := Encrypt("secret", []byte("short"))
	if err == nil {
		t.Fatalf("expected Encrypt to return an error for invalid key length")
	}

	if !errors.Is(err, errInvalidEncryptionKeyLength) {
		t.Fatalf("expected error %q, got %v", errInvalidEncryptionKeyLength, err)
	}
}

func TestDecryptInvalidKeyLength(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	encrypted, err := Encrypt("secret", key)
	if err != nil {
		t.Fatalf("Encrypt returned error: %v", err)
	}

	_, err = Decrypt(encrypted, []byte("short"))
	if err == nil {
		t.Fatalf("expected Decrypt to return an error for invalid key length")
	}

	if !errors.Is(err, errInvalidEncryptionKeyLength) {
		t.Fatalf("expected error %q, got %v", errInvalidEncryptionKeyLength, err)
	}
}

func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{name: "long key", key: "abcdefghijklmnopqrstuvwxyz", want: "abcd...wxyz"},
		{name: "length eight", key: "12345678", want: "***"},
		{name: "short key", key: "123", want: "***"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MaskAPIKey(tc.key)
			if got != tc.want {
				t.Fatalf("MaskAPIKey(%q) = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

func TestGetOrCreateEncryptionKeyErrNotFoundStoresRandomKeyWhenMachineIDUnavailable(t *testing.T) {
	originalKeyringGet := keyringGet
	originalKeyringSet := keyringSet
	originalMachineIDReader := readMachineIDFile
	originalListNetInterfaces := listNetInterfaces
	t.Cleanup(func() {
		keyringGet = originalKeyringGet
		keyringSet = originalKeyringSet
		readMachineIDFile = originalMachineIDReader
		listNetInterfaces = originalListNetInterfaces
	})

	var storedValue string
	getCalls := 0
	keyringGet = func(_, _ string) (string, error) {
		getCalls++
		if getCalls == 1 {
			return "", keyring.ErrNotFound
		}
		return storedValue, nil
	}
	keyringSet = func(_, _ string, value string) error {
		storedValue = value
		return nil
	}
	readMachineIDFile = func(path string) ([]byte, error) {
		return nil, os.ErrNotExist
	}
	listNetInterfaces = func() ([]net.Interface, error) {
		return nil, errors.New("network interfaces unavailable")
	}

	first, err := GetOrCreateEncryptionKey()
	if err != nil {
		t.Fatalf("first GetOrCreateEncryptionKey returned error: %v", err)
	}
	second, err := GetOrCreateEncryptionKey()
	if err != nil {
		t.Fatalf("second GetOrCreateEncryptionKey returned error: %v", err)
	}

	if string(first) != string(second) {
		t.Fatalf("expected stable key across calls, got different keys")
	}
	if len(first) != encryptionKeyLength {
		t.Fatalf("expected %d-byte key, got %d", encryptionKeyLength, len(first))
	}
	if storedValue == "" {
		t.Fatal("expected generated key to be stored in keyring")
	}
}

func TestGetOrCreateEncryptionKeyStableWhenKeyringSetFailsThenRecovers(t *testing.T) {
	originalKeyringGet := keyringGet
	originalKeyringSet := keyringSet
	originalMachineIDReader := readMachineIDFile
	originalListNetInterfaces := listNetInterfaces
	t.Cleanup(func() {
		keyringGet = originalKeyringGet
		keyringSet = originalKeyringSet
		readMachineIDFile = originalMachineIDReader
		listNetInterfaces = originalListNetInterfaces
	})

	expectedMachineID := "machine-id-stable"
	sum := sha256.Sum256([]byte(expectedMachineID))
	expectedKey := sum[:]

	var storedValue string
	keyringGet = func(_, _ string) (string, error) {
		if storedValue == "" {
			return "", keyring.ErrNotFound
		}
		return storedValue, nil
	}
	keyringSet = func(_, _ string, value string) error {
		storedValue = value
		return errors.New("temporary keyring set failure")
	}
	readMachineIDFile = func(path string) ([]byte, error) {
		if path == "/etc/machine-id" {
			return []byte(expectedMachineID), nil
		}
		return nil, os.ErrNotExist
	}
	listNetInterfaces = func() ([]net.Interface, error) {
		return nil, errors.New("not needed")
	}

	first, err := GetOrCreateEncryptionKey()
	if err != nil {
		t.Fatalf("first GetOrCreateEncryptionKey returned error: %v", err)
	}

	keyringSet = func(_, _ string, value string) error {
		storedValue = value
		return nil
	}
	second, err := GetOrCreateEncryptionKey()
	if err != nil {
		t.Fatalf("second GetOrCreateEncryptionKey returned error: %v", err)
	}

	if string(first) != string(second) {
		t.Fatalf("expected stable key across keyring recovery")
	}
	if string(first) != string(expectedKey) {
		t.Fatalf("expected derived fallback key, got different key")
	}
}

func TestGetOrCreateEncryptionKeyErrNotFoundSetErrorFallsBackToDerivedKey(t *testing.T) {
	originalKeyringGet := keyringGet
	originalKeyringSet := keyringSet
	originalMachineIDReader := readMachineIDFile
	originalListNetInterfaces := listNetInterfaces
	t.Cleanup(func() {
		keyringGet = originalKeyringGet
		keyringSet = originalKeyringSet
		readMachineIDFile = originalMachineIDReader
		listNetInterfaces = originalListNetInterfaces
	})

	expectedMachineID := "machine-id-abc123"
	sum := sha256.Sum256([]byte(expectedMachineID))
	expectedKey := sum[:]

	keyringGet = func(_, _ string) (string, error) {
		return "", keyring.ErrNotFound
	}
	keyringSet = func(_, _ string, _ string) error {
		return errors.New("keyring unavailable")
	}
	readMachineIDFile = func(path string) ([]byte, error) {
		if path == "/etc/machine-id" {
			return []byte(expectedMachineID), nil
		}
		return nil, os.ErrNotExist
	}
	listNetInterfaces = func() ([]net.Interface, error) {
		return nil, errors.New("not needed")
	}

	key, err := GetOrCreateEncryptionKey()
	if err != nil {
		t.Fatalf("GetOrCreateEncryptionKey returned error: %v", err)
	}
	if string(key) != string(expectedKey) {
		t.Fatalf("expected derived fallback key, got different key")
	}
}

func TestGetOrCreateEncryptionKeyReadErrorFallsBackToDerivedKey(t *testing.T) {
	originalKeyringGet := keyringGet
	originalKeyringSet := keyringSet
	originalMachineIDReader := readMachineIDFile
	originalListNetInterfaces := listNetInterfaces
	t.Cleanup(func() {
		keyringGet = originalKeyringGet
		keyringSet = originalKeyringSet
		readMachineIDFile = originalMachineIDReader
		listNetInterfaces = originalListNetInterfaces
	})

	expectedMachineID := "machine-id-def456"
	sum := sha256.Sum256([]byte(expectedMachineID))
	expectedKey := sum[:]

	keyringGet = func(_, _ string) (string, error) {
		return "", errors.New("keyring unavailable")
	}
	keyringSet = func(_, _ string, _ string) error {
		t.Fatalf("keyringSet should not be called on generic keyring read error")
		return nil
	}
	readMachineIDFile = func(path string) ([]byte, error) {
		if path == "/etc/machine-id" {
			return []byte(expectedMachineID), nil
		}
		return nil, os.ErrNotExist
	}
	listNetInterfaces = func() ([]net.Interface, error) {
		return nil, errors.New("not needed")
	}

	key, err := GetOrCreateEncryptionKey()
	if err != nil {
		t.Fatalf("GetOrCreateEncryptionKey returned error: %v", err)
	}
	if string(key) != string(expectedKey) {
		t.Fatalf("expected derived fallback key, got different key")
	}
}

func TestGetOrCreateEncryptionKeyFallbackUsesNetworkInterfaceWhenMachineIDFilesMissing(t *testing.T) {
	originalKeyringGet := keyringGet
	originalKeyringSet := keyringSet
	originalMachineIDReader := readMachineIDFile
	originalListNetInterfaces := listNetInterfaces
	t.Cleanup(func() {
		keyringGet = originalKeyringGet
		keyringSet = originalKeyringSet
		readMachineIDFile = originalMachineIDReader
		listNetInterfaces = originalListNetInterfaces
	})

	mac := "02:00:00:00:00:01"
	sum := sha256.Sum256([]byte(mac))
	expectedKey := sum[:]

	keyringGet = func(_, _ string) (string, error) {
		return "", errors.New("keyring unavailable")
	}
	keyringSet = func(_, _ string, _ string) error {
		t.Fatal("keyringSet should not be called on generic keyring read error")
		return nil
	}
	readMachineIDFile = func(path string) ([]byte, error) {
		return nil, os.ErrNotExist
	}
	listNetInterfaces = func() ([]net.Interface, error) {
		return []net.Interface{{HardwareAddr: mustParseMAC(t, mac)}}, nil
	}

	key, err := GetOrCreateEncryptionKey()
	if err != nil {
		t.Fatalf("GetOrCreateEncryptionKey returned error: %v", err)
	}
	if string(key) != string(expectedKey) {
		t.Fatalf("expected network-interface-derived fallback key, got different key")
	}
}

func mustParseMAC(t *testing.T, value string) net.HardwareAddr {
	t.Helper()
	addr, err := net.ParseMAC(value)
	if err != nil {
		t.Fatalf("ParseMAC(%q): %v", value, err)
	}
	return addr
}
