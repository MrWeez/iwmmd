package secrets

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestBoxRoundTripAndHMAC(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	box, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	value := []byte("smtp-password")
	encrypted, err := box.Encrypt(value)
	if err != nil {
		t.Fatal(err)
	}
	if encrypted == string(value) {
		t.Fatal("ciphertext equals plaintext")
	}
	decrypted, err := box.Decrypt(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decrypted, value) {
		t.Fatalf("got %q, want %q", decrypted, value)
	}

	token := box.HMAC("session-token")
	if !box.VerifyHMAC("session-token", token) {
		t.Fatal("valid HMAC rejected")
	}
	if box.VerifyHMAC("other-token", token) {
		t.Fatal("HMAC accepted wrong value")
	}
}

func TestBoxRejectsWrongKey(t *testing.T) {
	key := make([]byte, 32)
	other := make([]byte, 32)
	other[0] = 1
	box, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := box.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	otherBox, err := New(other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := otherBox.Decrypt(encrypted); err == nil {
		t.Fatal("decrypt with wrong key succeeded")
	}
}
