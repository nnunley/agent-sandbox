package grantauth_test

import (
	"testing"
	"time"

	paseto "aidanwoods.dev/go-paseto"

	"github.com/agent-sandbox/incus-dispatcher/grantauth"
)

const aud = "laneq://agent-host:9999"

func TestMintGrant_VerifiesAndUsesIntTimestamps(t *testing.T) {
	issuer, err := grantauth.NewKey()
	if err != nil {
		t.Fatalf("issuer key: %v", err)
	}
	client, err := grantauth.NewKey()
	if err != nil {
		t.Fatalf("client key: %v", err)
	}
	clientPEM, err := client.PublicKeyPEM()
	if err != nil {
		t.Fatalf("client pem: %v", err)
	}

	now := time.Unix(1782000000, 0).UTC()
	grant, err := issuer.MintGrant(grantauth.GrantParams{
		Iss:                "mac",
		Sub:                "agent-host",
		Aud:                aud,
		ClientPublicKeyPEM: clientPEM,
		ClientKid:          "c1",
		Kid:                "k1",
		Now:                now,
		TTL:                30 * time.Minute,
		JTI:                "j1",
	})
	if err != nil {
		t.Fatalf("mint grant: %v", err)
	}

	tok, err := paseto.NewParserWithoutExpiryCheck().ParseV4Public(issuer.PublicKey(), grant, nil)
	if err != nil {
		t.Fatalf("grant failed to verify against issuer public key: %v", err)
	}

	// Interop-critical: timestamps must be JSON integers (unix seconds), matching the
	// laneq Python verifier — NOT RFC3339 strings.
	var exp int64
	if err := tok.Get("exp", &exp); err != nil {
		t.Fatalf("exp claim is not an integer (interop break with pyseto): %v", err)
	}
	if exp != now.Add(30*time.Minute).Unix() {
		t.Errorf("exp = %d, want %d", exp, now.Add(30*time.Minute).Unix())
	}
	var sub string
	if err := tok.Get("sub", &sub); err != nil || sub != "agent-host" {
		t.Errorf("sub = %q (err %v), want agent-host", sub, err)
	}
	// cnf carries the client public key PEM.
	cnf := struct {
		Kid string `json:"kid"`
		Key string `json:"key"`
	}{}
	if err := tok.Get("cnf", &cnf); err != nil {
		t.Fatalf("cnf claim: %v", err)
	}
	if cnf.Key != clientPEM {
		t.Errorf("cnf.key did not round-trip the client PEM")
	}
}

func TestSignProof_VerifiesAndBindsMethod(t *testing.T) {
	client, err := grantauth.NewKey()
	if err != nil {
		t.Fatalf("client key: %v", err)
	}
	now := time.Unix(1782000000, 0).UTC()
	proof, err := client.SignProof(grantauth.ProofParams{
		Aud:    aud,
		Method: "/laneq.Laneq/Take",
		Nonce:  "n1",
		Now:    now,
	})
	if err != nil {
		t.Fatalf("sign proof: %v", err)
	}

	tok, err := paseto.NewParserWithoutExpiryCheck().ParseV4Public(client.PublicKey(), proof, nil)
	if err != nil {
		t.Fatalf("proof failed to verify against client public key: %v", err)
	}
	var method string
	if err := tok.Get("method", &method); err != nil || method != "/laneq.Laneq/Take" {
		t.Errorf("method = %q (err %v)", method, err)
	}
	var iat int64
	if err := tok.Get("iat", &iat); err != nil || iat != now.Unix() {
		t.Errorf("iat = %d (err %v), want %d", iat, err, now.Unix())
	}
}

func TestPublicKeyPEM_IsParseablePKIX(t *testing.T) {
	k, err := grantauth.NewKey()
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	pem, err := k.PublicKeyPEM()
	if err != nil {
		t.Fatalf("pem: %v", err)
	}
	if len(pem) == 0 || pem[:27] != "-----BEGIN PUBLIC KEY-----\n" {
		t.Errorf("unexpected PEM header: %q", pem[:30])
	}
}
