// Package grantauth mints and signs the laneq host-to-host auth tokens used by the
// dispatcher's laneq client: an issuer-minted, sender-constrained PASETO v4.public
// grant (with a cnf binding the client key) and a per-request proof signed by the
// client key. The wire contract (PASETO v4.public, JSON claims with unix-int
// timestamps) matches the laneq Python verifier (laneq.auth). See
// docs/superpowers/specs/2026-06-24-laneq-grant-paseto-design.md.
package grantauth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	paseto "aidanwoods.dev/go-paseto"
)

// Metadata keys carrying the grant and per-request proof on each gRPC call.
const (
	GrantMetadataKey = "laneq-grant"
	ProofMetadataKey = "laneq-proof"
)

// Key is an Ed25519 keypair used to mint grants (issuer) or sign proofs (client).
type Key struct {
	priv   ed25519.PrivateKey
	secret paseto.V4AsymmetricSecretKey
}

// NewKey generates a fresh Ed25519 keypair.
func NewKey() (*Key, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}
	return KeyFromEd25519(priv)
}

// KeyFromEd25519 wraps an existing Ed25519 private key (e.g. loaded from disk).
func KeyFromEd25519(priv ed25519.PrivateKey) (*Key, error) {
	secret, err := paseto.NewV4AsymmetricSecretKeyFromEd25519(priv)
	if err != nil {
		return nil, fmt.Errorf("paseto secret key: %w", err)
	}
	return &Key{priv: priv, secret: secret}, nil
}

// PublicKey returns the PASETO v4.public verification key.
func (k *Key) PublicKey() paseto.V4AsymmetricPublicKey {
	pub, _ := paseto.NewV4AsymmetricPublicKeyFromEd25519(k.priv.Public().(ed25519.PublicKey))
	return pub
}

// PublicKeyPEM returns the PKIX/PEM-encoded public key — the form embedded in a
// grant's cnf and the form the laneq verifier (pyseto) loads.
func (k *Key) PublicKeyPEM() (string, error) {
	der, err := x509.MarshalPKIXPublicKey(k.priv.Public())
	if err != nil {
		return "", fmt.Errorf("marshal public key: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), nil
}

// GrantParams are the inputs for minting an identity grant (issuer side).
type GrantParams struct {
	Iss                string
	Sub                string
	Aud                string
	ClientPublicKeyPEM string // bound into cnf (sender-constraint)
	ClientKid          string
	Kid                string // issuer key id (footer)
	Now                time.Time
	TTL                time.Duration
	JTI                string
}

// MintGrant signs a sender-constrained grant with this (issuer) key.
func (k *Key) MintGrant(p GrantParams) (string, error) {
	claims := map[string]any{
		"iss": p.Iss,
		"sub": p.Sub,
		"aud": p.Aud,
		"iat": p.Now.Unix(),
		"nbf": p.Now.Unix(),
		"exp": p.Now.Add(p.TTL).Unix(),
		"jti": p.JTI,
		"cnf": map[string]string{"kid": p.ClientKid, "key": p.ClientPublicKeyPEM},
	}
	return k.sign(claims, map[string]string{"kid": p.Kid})
}

// ProofParams are the inputs for a per-request proof (client side).
type ProofParams struct {
	Aud    string
	Method string // the gRPC full method, e.g. "/laneq.Laneq/Take"
	Nonce  string
	Now    time.Time
}

// SignProof signs a per-request proof with this (client) key.
func (k *Key) SignProof(p ProofParams) (string, error) {
	claims := map[string]any{
		"aud":    p.Aud,
		"method": p.Method,
		"iat":    p.Now.Unix(),
		"nonce":  p.Nonce,
	}
	return k.sign(claims, nil)
}

func (k *Key) sign(claims map[string]any, footer map[string]string) (string, error) {
	body, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	var footerBytes []byte
	if footer != nil {
		if footerBytes, err = json.Marshal(footer); err != nil {
			return "", fmt.Errorf("marshal footer: %w", err)
		}
	}
	tok, err := paseto.NewTokenFromClaimsJSON(body, footerBytes)
	if err != nil {
		return "", fmt.Errorf("build token: %w", err)
	}
	return tok.V4Sign(k.secret, nil), nil
}

// GrantSource is the interface for loading the current PASETO grant token.
// Reload is mtime-driven: the issuer's renewal helper rewrites the token file before exp;
// FileGrantSource re-stats on every Current() call and reloads when the file's mtime changes,
// so a renewed token is picked up promptly. A static file is never re-fetched.
// Phase 1 serves a single host-level grant. Phase-2/ITER-0008 per-role grant selection
// (sub=temporal-writer|daemon-consumer) will be a separate GrantSource implementation.
type GrantSource interface {
	// Current returns the current grant token string, or an error if unavailable.
	Current() (string, error)
}

// FileGrantSourceOptions configures a FileGrantSource (currently unused but kept for future extensibility).
type FileGrantSourceOptions struct{}

// FileGrantSource loads a PASETO grant from a file, caches it in memory,
// and reloads when the file's modification time changes.
type FileGrantSource struct {
	path          string
	mu            sync.Mutex
	cachedToken   string
	cachedModTime time.Time
}

// NewFileGrantSource creates a FileGrantSource that loads grants from the given path.
// Reload is purely mtime-driven: on each Current() call, the file is stat'd;
// if mtime matches the cached mtime, the cached token is returned; otherwise,
// the file is read and re-cached with the new mtime.
// This satisfies AC-2's "reloads on file change" contract: the issuer's renewal
// helper (launchd/cron) rewrites the token file before expiry, changing mtime,
// and FileGrantSource promptly picks up the new token.
func NewFileGrantSource(path string, optFuncs ...func(*FileGrantSourceOptions)) (*FileGrantSource, error) {
	opts := &FileGrantSourceOptions{}
	for _, fn := range optFuncs {
		fn(opts)
	}

	return &FileGrantSource{
		path: path,
	}, nil
}

// Current returns the current grant token, reading from the file if necessary.
// It caches by file modification time and reloads if the mtime changes.
// On each call, the file is stat'd; if mtime matches the cached mtime, the
// cached token is returned. If mtime differs, the file is re-read, trimmed,
// and re-cached.
// Returns an error if the file cannot be read, is empty, or contains only whitespace.
func (s *FileGrantSource) Current() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stat the file to check its modification time.
	stat, err := os.Stat(s.path)
	if err != nil {
		return "", fmt.Errorf("stat grant file: %w", err)
	}

	modTime := stat.ModTime()

	// Check if we have a cached token with matching mtime.
	if s.cachedToken != "" && modTime == s.cachedModTime {
		return s.cachedToken, nil
	}

	// Read and cache the token.
	data, err := os.ReadFile(s.path)
	if err != nil {
		return "", fmt.Errorf("read grant file: %w", err)
	}

	// Trim surrounding whitespace (grant files may have trailing newlines from echo or systemd-credential).
	token := strings.TrimSpace(string(data))
	// Reject empty or whitespace-only files.
	if token == "" {
		return "", fmt.Errorf("grant file is empty or whitespace-only")
	}

	s.cachedToken = token
	s.cachedModTime = modTime

	return token, nil
}
