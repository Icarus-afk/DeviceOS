package sparkdb_test

import (
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/lohtbrok/deviceos/internal/sparkdb"
)

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	hexKey := hex.EncodeToString(key)

	plaintext := []byte("hello sparkdb - this is sensitive data")
	cipherHex, err := sparkdb.Encrypt(plaintext, hexKey)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if cipherHex == "" {
		t.Fatal("expected non-empty ciphertext")
	}

	decrypted, err := sparkdb.Decrypt(cipherHex, hexKey)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("expected %q, got %q", plaintext, decrypted)
	}
}

func TestEncryptInvalidKey(t *testing.T) {
	_, err := sparkdb.Encrypt([]byte("test"), "invalid")
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestDecryptInvalidKey(t *testing.T) {
	_, err := sparkdb.Decrypt("abcd", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestDecryptInvalidCiphertext(t *testing.T) {
	key := hex.EncodeToString(make([]byte, 32))
	_, err := sparkdb.Decrypt("aaa", key)
	if err == nil {
		t.Fatal("expected error for invalid ciphertext")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := hex.EncodeToString(func() []byte { b := make([]byte, 32); rand.Read(b); return b }())
	key2 := hex.EncodeToString(func() []byte { b := make([]byte, 32); rand.Read(b); return b }())

	cipherHex, err := sparkdb.Encrypt([]byte("secret"), key1)
	if err != nil {
		t.Fatal(err)
	}

	_, err = sparkdb.Decrypt(cipherHex, key2)
	if err == nil {
		t.Fatal("expected error with wrong key")
	}
}

func TestEncryptEmpty(t *testing.T) {
	key := hex.EncodeToString(make([]byte, 32))
	cipherHex, err := sparkdb.Encrypt([]byte{}, key)
	if err != nil {
		t.Fatalf("encrypt empty: %v", err)
	}
	decrypted, err := sparkdb.Decrypt(cipherHex, key)
	if err != nil {
		t.Fatalf("decrypt empty: %v", err)
	}
	if len(decrypted) != 0 {
		t.Fatal("expected empty decrypted bytes")
	}
}

func TestShortKey(t *testing.T) {
	_, err := sparkdb.Encrypt([]byte("test"), "aabb") // 2 bytes, not 32
	if err == nil {
		t.Fatal("expected error for short key")
	}
}
