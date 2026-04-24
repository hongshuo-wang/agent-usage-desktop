package configmanager

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sort"
	"strings"

	"github.com/zalando/go-keyring"
)

const encPrefix = "enc:"

const (
	encryptionKeyLength = 32
	keyringService      = "agent-usage-desktop"
	keyringUser         = "config-manager-encryption-key"
)

var (
	errInvalidEncryptionKeyLength   = fmt.Errorf("encryption key must be %d bytes", encryptionKeyLength)
	errCiphertextTooShort           = errors.New("ciphertext too short")
	errMachineIdentifierUnavailable = errors.New("machine identifier unavailable")
	keyringGet                      = keyring.Get
	keyringSet                      = keyring.Set
	readMachineIDFile               = os.ReadFile
	listNetInterfaces               = net.Interfaces
)

func Encrypt(plaintext string, key []byte) (string, error) {
	if err := validateEncryptionKey(key); err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	payload := append(nonce, ciphertext...)
	return encPrefix + base64.StdEncoding.EncodeToString(payload), nil
}

func Decrypt(value string, key []byte) (string, error) {
	if !strings.HasPrefix(value, encPrefix) {
		return value, nil
	}

	if err := validateEncryptionKey(key); err != nil {
		return "", err
	}

	payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, encPrefix))
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(payload) < nonceSize+gcm.Overhead() {
		return "", errCiphertextTooShort
	}

	nonce := payload[:nonceSize]
	ciphertext := payload[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func MaskAPIKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}

	return key[:4] + "..." + key[len(key)-4:]
}

func GetOrCreateEncryptionKey() ([]byte, error) {
	stored, err := keyringGet(keyringService, keyringUser)
	if err == nil {
		key, decodeErr := base64.StdEncoding.DecodeString(stored)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode keyring encryption key: %w", decodeErr)
		}
		if validateErr := validateEncryptionKey(key); validateErr != nil {
			return nil, validateErr
		}
		return key, nil
	}

	fallbackKey, deriveErr := deriveMachineFallbackKey()

	if errors.Is(err, keyring.ErrNotFound) {
		if deriveErr == nil {
			log.Printf("configmanager: warning: using machine-derived encryption key")
			encoded := base64.StdEncoding.EncodeToString(fallbackKey)
			if setErr := keyringSet(keyringService, keyringUser, encoded); setErr == nil {
				return fallbackKey, nil
			}

			log.Printf("configmanager: warning: keychain unavailable, using machine-derived encryption key")
			return fallbackKey, nil
		}

		key := make([]byte, encryptionKeyLength)
		if _, readErr := rand.Read(key); readErr != nil {
			return nil, readErr
		}
		encoded := base64.StdEncoding.EncodeToString(key)
		if setErr := keyringSet(keyringService, keyringUser, encoded); setErr == nil {
			return key, nil
		}

		return nil, fmt.Errorf("keyring unavailable: %w; machine fallback unavailable: %w", err, deriveErr)
	}

	if deriveErr != nil {
		return nil, fmt.Errorf("keyring unavailable: %w; machine fallback unavailable: %w", err, deriveErr)
	}
	log.Printf("configmanager: warning: keychain unavailable, using machine-derived encryption key")
	return fallbackKey, nil
}

func deriveMachineFallbackKey() ([]byte, error) {
	identifier, err := machineIdentifier()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(identifier))
	key := make([]byte, len(sum))
	copy(key, sum[:])
	return key, nil
}

func machineIdentifier() (string, error) {
	paths := []string{
		"/etc/machine-id",
		"/var/lib/dbus/machine-id",
		"/sys/class/dmi/id/product_uuid",
	}

	for _, path := range paths {
		data, err := readMachineIDFile(path)
		if err != nil {
			continue
		}
		identifier := strings.TrimSpace(string(data))
		if identifier != "" {
			return identifier, nil
		}
	}

	ifaces, err := listNetInterfaces()
	if err == nil {
		var addrs []string
		for _, iface := range ifaces {
			if addr := strings.TrimSpace(iface.HardwareAddr.String()); addr != "" {
				addrs = append(addrs, addr)
			}
		}
		if len(addrs) > 0 {
			sort.Strings(addrs)
			return strings.Join(addrs, ","), nil
		}
	}

	return "", errMachineIdentifierUnavailable
}

func validateEncryptionKey(key []byte) error {
	if len(key) != encryptionKeyLength {
		return errInvalidEncryptionKeyLength
	}

	return nil
}
