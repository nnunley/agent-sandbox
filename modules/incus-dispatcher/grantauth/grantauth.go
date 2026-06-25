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
// Implementations may cache the token and refresh it on file changes or expiry.
type GrantSource interface {
	// Current returns the current grant token string, or an error if unavailable.
	Current() (string, error)
}

// FileGrantSourceOptions configures a FileGrantSource.
type FileGrantSourceOptions struct {
	// Now is the clock used for time-based checks; defaults to time.Now.
	Now func() time.Time
	// IssuerPublicKey is optional; if provided, enables expiry-aware reload checks.
	// Store as a *V4AsymmetricPublicKey since the struct is not nilable.
	IssuerPublicKey *paseto.V4AsymmetricPublicKey
}

// FileGrantSource loads a PASETO grant from a file, caches it in memory,
// and reloads when the file's modification time changes or the cached grant
// approaches expiry.
type FileGrantSource struct {
	path                string
	issuerPublicKey     *paseto.V4AsymmetricPublicKey
	now                 func() time.Time
	mu                  sync.Mutex
	cachedToken         string
	cachedModTime       time.Time
	cachedExp           int64 // unix seconds; 0 if unknown
	refreshThresholdSec int64 // reload when remaining TTL <= this many seconds (90% of typical 30min = 1620s)
}

// NewFileGrantSource creates a FileGrantSource that loads grants from the given path.
// The source accepts optional configuration via functional options.
//
// Example with default (mtime-only reload):
//
//	source, err := NewFileGrantSource("/path/to/grant.txt")
//
// Example with expiry-aware reload:
//
//	source, err := NewFileGrantSource("/path/to/grant.txt", func(opts *FileGrantSourceOptions) {
//	    opts.IssuerPublicKey = issuerKey.PublicKey()
//	})
func NewFileGrantSource(path string, optFuncs ...func(*FileGrantSourceOptions)) (*FileGrantSource, error) {
	opts := &FileGrantSourceOptions{
		Now: time.Now,
	}
	for _, fn := range optFuncs {
		fn(opts)
	}

	return &FileGrantSource{
		path:                path,
		issuerPublicKey:     opts.IssuerPublicKey,
		now:                 opts.Now,
		refreshThresholdSec: 1620, // ~90% of 30-minute typical TTL
	}, nil
}

// Current returns the current grant token, reading from the file if necessary.
// It caches by file modification time and reloads if the mtime changes.
// If an issuer public key was configured, it also checks for upcoming expiry
// and reloads the file if the cached grant is within the refresh threshold.
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
		// Mtime matches. Check expiry-based refresh threshold if we have issuer key.
		if s.issuerPublicKey != nil && s.cachedExp > 0 {
			now := s.now().Unix()
			remainingTTL := s.cachedExp - now
			if remainingTTL > s.refreshThresholdSec {
				// Token is valid and not yet within refresh threshold.
				return s.cachedToken, nil
			}
			// Within refresh threshold; attempt to reread below.
		} else {
			// No expiry check configured; mtime match is sufficient.
			return s.cachedToken, nil
		}
	}

	// Read and cache the token.
	data, err := os.ReadFile(s.path)
	if err != nil {
		return "", fmt.Errorf("read grant file: %w", err)
	}

	token := string(data)
	if token == "" {
		return "", fmt.Errorf("grant file is empty")
	}

	// Attempt to parse expiry if issuer key is available.
	var exp int64
	if s.issuerPublicKey != nil {
		parser := paseto.NewParserWithoutExpiryCheck()
		tok, err := parser.ParseV4Public(*s.issuerPublicKey, token, nil)
		if err == nil {
			// Parsed successfully; extract exp claim.
			if err := tok.Get("exp", &exp); err == nil {
				s.cachedExp = exp
			}
		}
		// If parsing fails, we continue with the token anyway (graceful degradation).
	}

	s.cachedToken = token
	s.cachedModTime = modTime

	return token, nil
}
