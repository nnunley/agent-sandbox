package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/agent-sandbox/incus-dispatcher/grantauth"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	var err error
	switch command {
	case "keygen":
		err = runKeygen(args, os.Stdout)
	case "mint":
		err = runMint(args, os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `laneq-grant: PASETO grant issuer for laneq host-to-host authentication

Usage:
  laneq-grant keygen --out-priv <path> --out-pub <path> [--force]
  laneq-grant mint --sub <id> --aud <uri> --ttl <duration> --client-pub <path> [options]

Commands:

  keygen
    Generate an Ed25519 keypair and save the private key (PKCS#8 PRIVATE PEM)
    and public key (PKIX PUBLIC PEM) to the specified paths.
    Fails if --out-priv already exists unless --force is given.

  mint
    Load or generate an issuer private key and mint a PASETO v4.public grant
    with sender-constraint (client public key binding).

Keygen flags:
  --out-priv <path>   Write PKCS#8 private key PEM (mode 0600)
  --out-pub <path>    Write PKIX public key PEM (mode 0644)
  --force             Overwrite existing --out-priv file

Mint flags:
  --sub <id>          Subject claim (identity; Phase 1: "agent-host")
  --aud <uri>         Audience claim (target laneq; e.g., "laneq://host:port")
  --ttl <duration>    Time-to-live (e.g., "30m", "1h"; parsed by time.ParseDuration)
  --client-pub <path> Path to client public key PEM file (sender-constraint binding)
  --issuer-key <path> Path to issuer private key (created if missing; default: ~/.laneq/issuer.key)
  --iss <id>          Issuer claim (default: "mac")
  --kid <id>          Issuer key id (footer; default: "k1")
  --client-kid <id>   Client key id in cnf (default: "c1")
  --out <path>        Write grant to file instead of stdout (mode 0600)
`)
}

// runKeygen generates an Ed25519 keypair and writes it to disk atomically.
// Fails if --out-priv exists unless --force is given.
// Uses atomic file creation (O_EXCL) to prevent TOCTOU races on the clobber protection.
func runKeygen(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // Discard flag errors
	outPriv := fs.String("out-priv", "", "path to write private key PEM (mode 0600)")
	outPub := fs.String("out-pub", "", "path to write public key PEM (mode 0644)")
	force := fs.Bool("force", false, "overwrite existing --out-priv")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("invalid keygen flags: %w", err)
	}

	if *outPriv == "" {
		return fmt.Errorf("keygen: --out-priv is required")
	}
	if *outPub == "" {
		return fmt.Errorf("keygen: --out-pub is required")
	}

	// Generate a new key before opening files (fail early on key gen errors).
	key, err := grantauth.NewKey()
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	// Export private key (PKCS#8 PEM).
	privPEM, err := key.PrivateKeyPEM()
	if err != nil {
		return fmt.Errorf("export private key: %w", err)
	}

	// Export public key (PKIX PEM).
	pubPEM, err := key.PublicKeyPEM()
	if err != nil {
		return fmt.Errorf("export public key: %w", err)
	}

	// Atomically create or overwrite the private key file.
	// Use O_EXCL to fail if file exists (unless --force).
	var privFlags int
	if *force {
		privFlags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	} else {
		privFlags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}

	privFile, err := os.OpenFile(*outPriv, privFlags, 0600)
	if err != nil {
		if os.IsExist(err) && !*force {
			return fmt.Errorf("--out-priv %s already exists; use --force to clobber", *outPriv)
		}
		return fmt.Errorf("create private key file: %w", err)
	}
	_, err = privFile.WriteString(privPEM)
	if err != nil {
		privFile.Close()
		return fmt.Errorf("write private key: %w", err)
	}
	if err := privFile.Sync(); err != nil {
		privFile.Close()
		return fmt.Errorf("sync private key: %w", err)
	}
	privFile.Close()

	// Write public key with mode 0644 (readable by all).
	if err := os.WriteFile(*outPub, []byte(pubPEM), 0644); err != nil {
		return fmt.Errorf("write public key: %w", err)
	}

	return nil
}

// runMint loads or creates an issuer key and mints a PASETO grant with sender-constraint.
// Uses atomic key creation (O_EXCL) to ensure the trust root is never clobbered.
func runMint(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("mint", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // Discard flag errors

	sub := fs.String("sub", "", "subject claim (identity)")
	aud := fs.String("aud", "", "audience claim (target laneq instance)")
	ttlStr := fs.String("ttl", "30m", "time-to-live (e.g. '30m', '1h')")
	clientPubPath := fs.String("client-pub", "", "path to client public key PEM")
	issuerKeyPath := fs.String("issuer-key", "", "path to issuer private key")
	iss := fs.String("iss", "mac", "issuer claim")
	kid := fs.String("kid", "k1", "issuer key id (footer)")
	clientKid := fs.String("client-kid", "c1", "client key id (cnf)")
	outPath := fs.String("out", "", "path to write grant (default: stdout)")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("invalid mint flags: %w", err)
	}

	// Validate required flags.
	if *sub == "" {
		return fmt.Errorf("mint: --sub is required")
	}
	if *aud == "" {
		return fmt.Errorf("mint: --aud is required")
	}
	if *clientPubPath == "" {
		return fmt.Errorf("mint: --client-pub is required")
	}

	// Parse TTL.
	ttl, err := time.ParseDuration(*ttlStr)
	if err != nil {
		return fmt.Errorf("mint: parse --ttl: %w", err)
	}

	// Expand issuer key path; default to ~/.laneq/issuer.key.
	if *issuerKeyPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("mint: get home dir: %w", err)
		}
		*issuerKeyPath = filepath.Join(home, ".laneq", "issuer.key")
	}

	// Load or generate issuer key atomically.
	// The issuer key is the trust root and MUST NEVER be clobbered.
	var issuerKey *grantauth.Key
	var isNewKey bool

	// Try to atomically create the file; if it exists, load it instead.
	// This ensures exactly one key is ever created.
	dir := filepath.Dir(*issuerKeyPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mint: create directory %s: %w", dir, err)
	}

	// Attempt atomic creation with O_EXCL.
	keyFile, err := os.OpenFile(*issuerKeyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		// Successfully created a new file; generate and persist the key.
		issuerKey, err = grantauth.NewKey()
		if err != nil {
			keyFile.Close()
			return fmt.Errorf("mint: generate issuer key: %w", err)
		}

		privPEM, err := issuerKey.PrivateKeyPEM()
		if err != nil {
			keyFile.Close()
			return fmt.Errorf("mint: export issuer private key: %w", err)
		}

		_, err = keyFile.WriteString(privPEM)
		if err != nil {
			keyFile.Close()
			return fmt.Errorf("mint: write issuer key: %w", err)
		}
		if err := keyFile.Sync(); err != nil {
			keyFile.Close()
			return fmt.Errorf("mint: sync issuer key: %w", err)
		}
		keyFile.Close()

		isNewKey = true
		fmt.Fprintf(os.Stderr, "created new issuer key at %s\n", *issuerKeyPath)
	} else if os.IsExist(err) {
		// File already exists; load it.
		// Retry a few times in case another goroutine is still writing.
		const maxRetries = 10
		for attempt := 0; attempt < maxRetries; attempt++ {
			issuerKey, err = grantauth.LoadEd25519PrivateKeyPEM(*issuerKeyPath)
			if err == nil {
				break
			}
			// If this is the last attempt, return the error.
			if attempt == maxRetries-1 {
				return fmt.Errorf("mint: load issuer key: %w", err)
			}
			// Brief sleep before retry.
			time.Sleep(time.Millisecond)
		}
		isNewKey = false
	} else {
		return fmt.Errorf("mint: create issuer key file: %w", err)
	}

	// Read client public key PEM.
	clientPubData, err := os.ReadFile(*clientPubPath)
	if err != nil {
		return fmt.Errorf("mint: read client public key: %w", err)
	}
	clientPubPEM := string(clientPubData)
	if clientPubPEM == "" {
		return fmt.Errorf("mint: client public key file is empty")
	}

	// Generate a unique JTI (token ID).
	jti, err := randomHexString(16) // 16 bytes = 32 hex chars
	if err != nil {
		return fmt.Errorf("mint: generate jti: %w", err)
	}

	// Mint the grant.
	now := time.Now()
	grant, err := issuerKey.MintGrant(grantauth.GrantParams{
		Iss:                *iss,
		Sub:                *sub,
		Aud:                *aud,
		ClientPublicKeyPEM: clientPubPEM,
		ClientKid:          *clientKid,
		Kid:                *kid,
		Now:                now,
		TTL:                ttl,
		JTI:                jti,
	})
	if err != nil {
		return fmt.Errorf("mint: mint grant: %w", err)
	}

	_ = isNewKey // Suppress unused variable; kept for future logging/metrics

	// Write grant to file or stdout.
	if *outPath != "" {
		// Write to file with mode 0600.
		if err := os.WriteFile(*outPath, []byte(grant+"\n"), 0600); err != nil {
			return fmt.Errorf("mint: write grant file: %w", err)
		}
	} else {
		// Write to stdout.
		fmt.Fprintf(stdout, "%s\n", grant)
	}

	return nil
}

// randomHexString generates a random hex string of length 2*numBytes.
func randomHexString(numBytes int) (string, error) {
	b := make([]byte, numBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}
