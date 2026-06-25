package grantauth_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	paseto "aidanwoods.dev/go-paseto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

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

func TestFileGrantSource_TrimsSurroundingWhitespace(t *testing.T) {
	// Verify that surrounding whitespace (trailing newline, leading/trailing spaces)
	// is trimmed from the returned token. Grant files commonly have trailing newlines
	// from echo or systemd-credential delivery.
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

	// Write grant with leading space, trailing newline, and trailing space.
	os.WriteFile(grantFile, []byte(" "+grant+"\n "), 0644)

	source, _ := grantauth.NewFileGrantSource(grantFile)
	got, err := source.Current()
	if err != nil {
		t.Fatalf("Current with surrounded token: %v", err)
	}

	// Returned token should equal the bare grant (no leading/trailing whitespace).
	if got != grant {
		t.Errorf("got %q, want %q (unpadded)", got, grant)
	}

	// Verify it has no leading/trailing whitespace.
	if got != strings.TrimSpace(got) {
		t.Errorf("returned token still has surrounding whitespace: %q", got)
	}

	// Verify it round-trips through PASETO parsing (interop check).
	parser := paseto.NewParserWithoutExpiryCheck()
	_, err = parser.ParseV4Public(issuer.PublicKey(), got, nil)
	if err != nil {
		t.Errorf("trimmed token failed PASETO parse: %v", err)
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

// ClientInterceptor tests

func TestLoadEd25519PrivateKeyPEM_LoadsAndRoundTrips(t *testing.T) {
	// Generate a key, export it to PKCS#8 PEM, save to file, reload, verify it works.
	tmpdir := t.TempDir()
	keyFile := filepath.Join(tmpdir, "key.pem")

	// Generate original key.
	origKey, err := grantauth.NewKey()
	if err != nil {
		t.Fatalf("NewKey: %v", err)
	}

	// Export to PEM.
	pem, err := origKey.PrivateKeyPEM()
	if err != nil {
		t.Fatalf("PrivateKeyPEM: %v", err)
	}
	if len(pem) == 0 || !strings.Contains(pem, "-----BEGIN PRIVATE KEY-----") {
		t.Fatalf("unexpected PEM format: %q", pem[:50])
	}

	// Save to file.
	err = os.WriteFile(keyFile, []byte(pem), 0600)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Load from file.
	loadedKey, err := grantauth.LoadEd25519PrivateKeyPEM(keyFile)
	if err != nil {
		t.Fatalf("LoadEd25519PrivateKeyPEM: %v", err)
	}

	// Verify loaded key can sign and verify.
	proof, err := loadedKey.SignProof(grantauth.ProofParams{
		Aud:    aud,
		Method: "/laneq.Laneq/Take",
		Nonce:  "n1",
		Now:    time.Unix(1782000000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("SignProof with loaded key: %v", err)
	}

	// Verify the proof with the loaded key's public key.
	tok, err := paseto.NewParserWithoutExpiryCheck().ParseV4Public(loadedKey.PublicKey(), proof, nil)
	if err != nil {
		t.Fatalf("Verify proof: %v", err)
	}

	var method string
	if err := tok.Get("method", &method); err != nil || method != "/laneq.Laneq/Take" {
		t.Errorf("method claim: %q (err %v)", method, err)
	}
}

func TestLoadEd25519PrivateKeyPEM_ErrorOnMissingFile(t *testing.T) {
	_, err := grantauth.LoadEd25519PrivateKeyPEM("/nonexistent/path/key.pem")
	if err == nil {
		t.Fatalf("LoadEd25519PrivateKeyPEM: expected error on missing file, got nil")
	}
}

func TestLoadEd25519PrivateKeyPEM_ErrorOnInvalidPEM(t *testing.T) {
	tmpdir := t.TempDir()
	keyFile := filepath.Join(tmpdir, "invalid.pem")
	os.WriteFile(keyFile, []byte("not a valid PEM"), 0600)

	_, err := grantauth.LoadEd25519PrivateKeyPEM(keyFile)
	if err == nil {
		t.Fatalf("LoadEd25519PrivateKeyPEM: expected error on invalid PEM, got nil")
	}
}

func TestNewClientInterceptor_AttachesGrantAndProof(t *testing.T) {
	// SCENARIO-0117: Create issuer, client, and grant.
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

	// Create a GrantSource that returns the grant.
	source := &fakeGrantSource{grant: grant}

	// Create the interceptor.
	interceptor := grantauth.NewClientInterceptor(source, client, aud)

	// Fake invoker that captures the context and metadata.
	var capturedCtx context.Context
	var capturedMethod string
	var invokerCalled bool

	fakeInvoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		capturedCtx = ctx
		capturedMethod = method
		invokerCalled = true
		return nil
	}

	// Call the interceptor.
	method := "/laneq.Laneq/Take"
	ctx := context.Background()
	req := struct{}{}
	reply := struct{}{}
	err := interceptor(ctx, method, &req, &reply, nil, fakeInvoker)
	if err != nil {
		t.Fatalf("interceptor call: %v", err)
	}

	// The fake invoker should have been called.
	if !invokerCalled {
		t.Fatalf("fake invoker was not called")
	}

	// Extract metadata from the captured context.
	md, ok := metadata.FromOutgoingContext(capturedCtx)
	if !ok {
		t.Fatalf("no outgoing metadata found in context")
	}

	// Verify grant is present.
	grants := md.Get(grantauth.GrantMetadataKey)
	if len(grants) == 0 {
		t.Fatalf("grant metadata not found")
	}
	if grants[0] != grant {
		t.Errorf("grant metadata: got %q, want %q", grants[0], grant)
	}

	// Verify proof is present and valid.
	proofs := md.Get(grantauth.ProofMetadataKey)
	if len(proofs) == 0 {
		t.Fatalf("proof metadata not found")
	}
	proof := proofs[0]

	// Parse and verify the proof against the client's public key.
	tok, err := paseto.NewParserWithoutExpiryCheck().ParseV4Public(client.PublicKey(), proof, nil)
	if err != nil {
		t.Fatalf("proof verification failed: %v", err)
	}

	// Verify proof claims.
	var proofAud, proofMethod, proofNonce string
	if err := tok.Get("aud", &proofAud); err != nil || proofAud != aud {
		t.Errorf("proof aud: got %q (err %v), want %q", proofAud, err, aud)
	}
	if err := tok.Get("method", &proofMethod); err != nil || proofMethod != method {
		t.Errorf("proof method: got %q (err %v), want %q", proofMethod, err, method)
	}
	if err := tok.Get("nonce", &proofNonce); err != nil || proofNonce == "" {
		t.Errorf("proof nonce: got %q (err %v), expected non-empty", proofNonce, err)
	}

	// Verify method in captured context matches.
	if capturedMethod != method {
		t.Errorf("captured method: got %q, want %q", capturedMethod, method)
	}
}

func TestNewClientInterceptor_FailsClosedOnGrantSourceError(t *testing.T) {
	// SCENARIO-0117: if GrantSource.Current() fails, the RPC must not be sent.
	client, _ := grantauth.NewKey()
	source := &fakeGrantSource{err: fmt.Errorf("grant source unavailable")}

	interceptor := grantauth.NewClientInterceptor(source, client, aud)

	invokerCalled := false
	fakeInvoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		// This must NOT be called if grant source fails.
		invokerCalled = true
		return nil
	}

	method := "/laneq.Laneq/Take"
	ctx := context.Background()
	req := struct{}{}
	reply := struct{}{}
	err := interceptor(ctx, method, &req, &reply, nil, fakeInvoker)
	if err == nil {
		t.Fatalf("interceptor: expected error on GrantSource failure, got nil")
	}
	if !strings.Contains(err.Error(), "grant source unavailable") {
		t.Errorf("interceptor error: got %q, want to contain %q", err.Error(), "grant source unavailable")
	}
	if invokerCalled {
		t.Errorf("fake invoker was called despite GrantSource error (fail-closed violation)")
	}
}

func TestNewClientInterceptor_FreshNoncePerCall(t *testing.T) {
	// SCENARIO-0117: each RPC must have a unique nonce for replay resistance.
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

	source := &fakeGrantSource{grant: grant}
	interceptor := grantauth.NewClientInterceptor(source, client, aud)

	// Call the interceptor twice and capture the proofs.
	nonces := make([]string, 2)
	for callNum := 0; callNum < 2; callNum++ {
		fakeInvoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			// Extract the proof from metadata.
			md, ok := metadata.FromOutgoingContext(ctx)
			if !ok {
				t.Fatalf("call %d: no metadata in context", callNum)
			}
			proofs := md.Get(grantauth.ProofMetadataKey)
			if len(proofs) == 0 {
				t.Fatalf("call %d: no proof metadata", callNum)
			}

			// Parse proof and extract nonce.
			tok, err := paseto.NewParserWithoutExpiryCheck().ParseV4Public(client.PublicKey(), proofs[0], nil)
			if err != nil {
				t.Fatalf("call %d: proof verification failed: %v", callNum, err)
			}

			var nonce string
			if err := tok.Get("nonce", &nonce); err != nil {
				t.Fatalf("call %d: nonce extraction failed: %v", callNum, err)
			}

			nonces[callNum] = nonce
			return nil
		}

		method := "/laneq.Laneq/Take"
		ctx := context.Background()
		req := struct{}{}
		reply := struct{}{}
		err := interceptor(ctx, method, &req, &reply, nil, fakeInvoker)
		if err != nil {
			t.Fatalf("call %d: interceptor error: %v", callNum, err)
		}
	}

	// Verify both nonces are non-empty and different.
	if nonces[0] == "" {
		t.Errorf("call 0: nonce is empty")
	}
	if nonces[1] == "" {
		t.Errorf("call 1: nonce is empty")
	}
	if nonces[0] == nonces[1] {
		t.Errorf("nonces are identical (replay attack risk): %q == %q", nonces[0], nonces[1])
	}
}

// Fake GrantSource for testing.
type fakeGrantSource struct {
	grant string
	err   error
}

func (f *fakeGrantSource) Current() (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.grant, nil
}

