package crypto

import (
	"bytes"
	"testing"
)

func TestAESGCMRoundTripAndAuthentication(t *testing.T) {
	box, err := NewAESGCM(bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte(`{"secret":"never-return-this"}`)
	nonce, ciphertext, err := box.Encrypt(plaintext, []byte("tenant/account"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("never-return-this")) {
		t.Fatal("ciphertext contains plaintext secret")
	}
	got, err := box.Decrypt(nonce, ciphertext, []byte("tenant/account"))
	if err != nil || !bytes.Equal(got, plaintext) {
		t.Fatalf("round trip = %q, %v", got, err)
	}
	ciphertext[0] ^= 1
	if _, err := box.Decrypt(nonce, ciphertext, []byte("tenant/account")); err == nil {
		t.Fatal("tampered ciphertext was accepted")
	}
}

func TestAESGCMRejectsWrongKeySize(t *testing.T) {
	if _, err := NewAESGCM(make([]byte, 31)); err == nil {
		t.Fatal("expected key-size error")
	}
}
