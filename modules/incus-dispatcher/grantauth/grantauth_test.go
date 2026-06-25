package grantauth_test

import (
	"os"
	"path/filepath"
	"sync"
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

// FileGrantSource tests

func TestFileGrantSource_CurrentReadsAndCachesGrant(t *testing.T) {
	// SCENARIO-0117: file-backed grant loader caches in memory and reloads on file change.
	tmpdir := t.TempDir()
	grantFile := filepath.Join(tmpdir, "grant.txt")

	// Write a valid grant to the file.
	issuer, _ := grantauth.NewKey()
	client, _ := grantauth.NewKey()
	clientPEM, _ := client.PublicKeyPEM()
	now := time.Unix(1782000000, 0).UTC()
	grant1, _ := issuer.MintGrant(grantauth.GrantParams{
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
	os.WriteFile(grantFile, []byte(grant1), 0644)

	source, err := grantauth.NewFileGrantSource(grantFile)
	if err != nil {
		t.Fatalf("NewFileGrantSource: %v", err)
	}

	// First call should read and cache.
	got, err := source.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if got != grant1 {
		t.Errorf("Current = %q, want %q", got, grant1)
	}

	// Second call should return cached value (same address verification not needed, just token equality).
	got2, err := source.Current()
	if err != nil {
		t.Fatalf("Current (2nd call): %v", err)
	}
	if got2 != grant1 {
		t.Errorf("Current (2nd call) = %q, want %q", got2, grant1)
	}
}

func TestFileGrantSource_ReloadsOnMtimeChange(t *testing.T) {
	// When file mtime changes, reread and re-cache.
	// Use os.Chtimes for deterministic mtime control.
	tmpdir := t.TempDir()
	grantFile := filepath.Join(tmpdir, "grant.txt")

	issuer, _ := grantauth.NewKey()
	client, _ := grantauth.NewKey()
	clientPEM, _ := client.PublicKeyPEM()
	now := time.Unix(1782000000, 0).UTC()

	// Write grant1 with mtime T1.
	grant1, _ := issuer.MintGrant(grantauth.GrantParams{
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
	t1 := time.Unix(1700000000, 0)
	os.WriteFile(grantFile, []byte(grant1), 0644)
	os.Chtimes(grantFile, t1, t1)

	source, _ := grantauth.NewFileGrantSource(grantFile)

	got1, _ := source.Current()
	if got1 != grant1 {
		t.Errorf("First read: got %q, want %q", got1, grant1)
	}

	// Write grant2 with a different mtime T1+2s.
	grant2, _ := issuer.MintGrant(grantauth.GrantParams{
		Iss:                "mac",
		Sub:                "agent-host",
		Aud:                aud,
		ClientPublicKeyPEM: clientPEM,
		ClientKid:          "c1",
		Kid:                "k1",
		Now:                now.Add(1 * time.Second),
		TTL:                30 * time.Minute,
		JTI:                "j2",
	})
	t2 := t1.Add(2 * time.Second)
	os.WriteFile(grantFile, []byte(grant2), 0644)
	os.Chtimes(grantFile, t2, t2)

	// Current should detect mtime change and reread.
	got2, _ := source.Current()
	if got2 != grant2 {
		t.Errorf("After mtime change: got %q, want %q", got2, grant2)
	}
}

func TestFileGrantSource_NoReloadOnSameMtime(t *testing.T) {
	// If file is rewritten with same mtime (rare but documents the contract),
	// the cached token is returned (mtime-driven reload).
	tmpdir := t.TempDir()
	grantFile := filepath.Join(tmpdir, "grant.txt")

	issuer, _ := grantauth.NewKey()
	client, _ := grantauth.NewKey()
	clientPEM, _ := client.PublicKeyPEM()
	now := time.Unix(1782000000, 0).UTC()

	grant1, _ := issuer.MintGrant(grantauth.GrantParams{
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
	t1 := time.Unix(1700000000, 0)
	os.WriteFile(grantFile, []byte(grant1), 0644)
	os.Chtimes(grantFile, t1, t1)

	source, _ := grantauth.NewFileGrantSource(grantFile)
	cached, _ := source.Current()

	// Rewrite with same mtime — simulates a no-op rewrite.
	grant2, _ := issuer.MintGrant(grantauth.GrantParams{
		Iss:                "mac",
		Sub:                "agent-host",
		Aud:                aud,
		ClientPublicKeyPEM: clientPEM,
		ClientKid:          "c1",
		Kid:                "k1",
		Now:                now.Add(1 * time.Second),
		TTL:                30 * time.Minute,
		JTI:                "j2",
	})
	os.WriteFile(grantFile, []byte(grant2), 0644)
	os.Chtimes(grantFile, t1, t1) // Same mtime

	// Current should return cached token (grant1), not the new content (grant2).
	got, _ := source.Current()
	if got != cached {
		t.Errorf("Same mtime rewrite: expected cache hit, got new content")
	}
}

func TestFileGrantSource_ErrorOnMissingFile(t *testing.T) {
	source, err := grantauth.NewFileGrantSource("/nonexistent/path/grant.txt")
	if err != nil {
		t.Fatalf("NewFileGrantSource should not error on missing file at construction: %v", err)
	}

	// First Current() call should fail.
	_, err = source.Current()
	if err == nil {
		t.Errorf("Current on missing file: expected error, got nil")
	}
}

func TestFileGrantSource_ErrorOnEmptyFile(t *testing.T) {
	tmpdir := t.TempDir()
	grantFile := filepath.Join(tmpdir, "grant.txt")
	os.WriteFile(grantFile, []byte(""), 0644)

	source, _ := grantauth.NewFileGrantSource(grantFile)

	_, err := source.Current()
	if err == nil {
		t.Errorf("Current on empty file: expected error, got nil")
	}
}

func TestFileGrantSource_ErrorOnWhitespaceOnlyFile(t *testing.T) {
	tmpdir := t.TempDir()
	grantFile := filepath.Join(tmpdir, "grant.txt")
	// Write only whitespace: spaces, newlines, tabs.
	os.WriteFile(grantFile, []byte("  \n\t  \n"), 0644)

	source, _ := grantauth.NewFileGrantSource(grantFile)

	_, err := source.Current()
	if err == nil {
		t.Errorf("Current on whitespace-only file: expected error, got nil")
	}
}

func TestFileGrantSource_ConcurrentReads(t *testing.T) {
	// Verify that concurrent Current() calls are race-free.
	tmpdir := t.TempDir()
	grantFile := filepath.Join(tmpdir, "grant.txt")

	issuer, _ := grantauth.NewKey()
	client, _ := grantauth.NewKey()
	clientPEM, _ := client.PublicKeyPEM()
	now := time.Unix(1782000000, 0).UTC()
	grant, _ := issuer.MintGrant(grantauth.GrantParams{
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
	os.WriteFile(grantFile, []byte(grant), 0644)

	source, _ := grantauth.NewFileGrantSource(grantFile)

	// Launch multiple goroutines calling Current() concurrently.
	const numGoroutines = 10
	var wg sync.WaitGroup
	results := make([]string, numGoroutines)
	errors := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tok, err := source.Current()
			results[idx] = tok
			errors[idx] = err
		}(i)
	}
	wg.Wait()

	// All should succeed and return the same token.
	for i := 0; i < numGoroutines; i++ {
		if errors[i] != nil {
			t.Errorf("goroutine %d: Current failed: %v", i, errors[i])
		}
		if results[i] != grant {
			t.Errorf("goroutine %d: got %q, want %q", i, results[i], grant)
		}
	}
}

