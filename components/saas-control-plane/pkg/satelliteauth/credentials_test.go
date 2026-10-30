package satelliteauth

import "testing"

func TestCredentialHashAndVerify(t *testing.T) {
	plaintext, encoded, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if plaintext == "" || encoded == "" {
		t.Fatal("credential or hash is empty")
	}
	if plaintext == encoded {
		t.Fatal("plaintext credential was stored as its hash")
	}
	if !Verify(encoded, plaintext) {
		t.Fatal("generated credential did not verify")
	}
	if Verify(encoded, plaintext+"wrong") {
		t.Fatal("wrong credential verified")
	}
	if Verify("malformed", plaintext) {
		t.Fatal("malformed hash verified")
	}
}

func TestCredentialHashesUseUniqueSalt(t *testing.T) {
	first, err := Hash("same-credential")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Hash("same-credential")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("credential hashes reused a salt")
	}
}
