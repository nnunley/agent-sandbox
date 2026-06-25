package main

import (
	"bytes"
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

// runKeygen generates an Ed25519 keypair and writes it to disk.
// Fails if --out-priv exists unless --force is given.
func runKeygen(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	var buf bytes.Buffer
	fs.SetOutput(&buf) // Capture flag errors
	outPriv := fs.String("out-priv", "", "path to write private key PEM (mode 0600)")
	outPub := fs.String("out-pub", "", "path to write public key PEM (mode 0644)")
	force := fs.Bool("force", false, "overwrite existing --out-priv")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("invalid keygen flags: %v", err)
	}

	if *outPriv == "" {
		return fmt.Errorf("keygen: --out-priv is required")
	}
	if *outPub == "" {
		return fmt.Errorf("keygen: --out-pub is required")
	}

	// Check if private key already exists.
	if _, err := os.Stat(*outPriv); err == nil {
		if !*force {
			return fmt.Errorf("--out-priv %s already exists; use --force to clobber", *outPriv)
		}
	}

	// Generate a new key.
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

	// Write private key with mode 0600 (readable only by owner).
	if err := os.WriteFile(*outPriv, []byte(privPEM), 0600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}

	// Write public key with mode 0644 (readable by all).
	if err := os.WriteFile(*outPub, []byte(pubPEM), 0644); err != nil {
		return fmt.Errorf("write public key: %w", err)
	}

	return nil
}

// runMint loads or creates an issuer key and mints a PASETO grant with sender-constraint.
func runMint(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("mint", flag.ContinueOnError)
	var buf bytes.Buffer
	fs.SetOutput(&buf) // Capture flag errors

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

	// Load or generate issuer key.
	var issuerKey *grantauth.Key
	if _, err := os.Stat(*issuerKeyPath); err == nil {
		// Key exists; load it.
		issuerKey, err = grantauth.LoadEd25519PrivateKeyPEM(*issuerKeyPath)
		if err != nil {
			return fmt.Errorf("mint: load issuer key: %w", err)
		}
	} else if os.IsNotExist(err) {
		// Key doesn't exist; generate it.
		issuerKey, err = grantauth.NewKey()
		if err != nil {
			return fmt.Errorf("mint: generate issuer key: %w", err)
		}

		// Create directory if needed.
		dir := filepath.Dir(*issuerKeyPath)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("mint: create directory %s: %w", dir, err)
		}

		// Persist the private key with mode 0600.
		privPEM, err := issuerKey.PrivateKeyPEM()
		if err != nil {
			return fmt.Errorf("mint: export issuer private key: %w", err)
		}
		if err := os.WriteFile(*issuerKeyPath, []byte(privPEM), 0600); err != nil {
			return fmt.Errorf("mint: write issuer key: %w", err)
		}

		// Notify via stderr that a new issuer key was created.
		fmt.Fprintf(os.Stderr, "created new issuer key at %s\n", *issuerKeyPath)
	} else {
		return fmt.Errorf("mint: stat issuer key path: %w", err)
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
	jti := randomHexString(16) // 16 bytes = 32 hex chars

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
func randomHexString(numBytes int) string {
	b := make([]byte, numBytes)
	if _, err := rand.Read(b); err != nil {
		panic(err) // Should never happen in normal operation
	}
	return hex.EncodeToString(b)
}
