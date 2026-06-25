package queue

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/agent-sandbox/incus-dispatcher/grantauth"
	"github.com/agent-sandbox/incus-dispatcher/queue/laneqpb"
)

// TestLaneqAuthWire proves SCENARIO-0117 (enforce accepts), SCENARIO-0118 (enforce rejects),
// and SCENARIO-0119 (log-only allows) — the complete PASETO grant auth contract end-to-end
// against a REAL laneq server running in different auth modes.
//
// This test:
// 1. Generates issuer and client Ed25519 keypairs
// 2. Writes the issuer public key to a temp file (for LANEQ_AUTH_PUBKEY_PATHS)
// 3. Writes the client private key to a temp file
// 4. Mints a PASETO grant (issuer signing, cnf binding client key)
// 5. Writes the grant to a temp file
// 6. Starts the laneq gRPC server in enforce mode with the issuer pubkey
// 7. Tests:
//    a. Positive (SCENARIO-0117): Client WITH auth interceptor → enforce accepts
//    b. Negative (SCENARIO-0118): Client WITHOUT auth interceptor → enforce rejects (Unauthenticated)
//    c. Log-only (SCENARIO-0119): Restarts server in log-only mode, client without auth → accepts
// 8. Cleans up temp files and kills the server subprocess
//
// Gated: if LANEQ_AUTH_WIRE != "1", the test is skipped (so default `go test ./...` stays green).
// If LANEQ_SRC is unset, defaults to /Users/ndn/development/laneq.
func TestLaneqAuthWire(t *testing.T) {
	// Gate: only run if LANEQ_AUTH_WIRE=1
	if os.Getenv("LANEQ_AUTH_WIRE") != "1" {
		t.Skip("real-wire PASETO auth e2e test; set LANEQ_AUTH_WIRE=1")
	}

	// Verify laneq source exists
	laneqSrc := os.Getenv("LANEQ_SRC")
	if laneqSrc == "" {
		laneqSrc = "/Users/ndn/development/laneq"
	}
	if _, err := os.Stat(laneqSrc); err != nil {
		t.Skipf("LANEQ_SRC %s not found: %v", laneqSrc, err)
	}

	// Verify uv is available
	if _, err := exec.LookPath("uv"); err != nil {
		t.Skipf("uv not found: %v", err)
	}

	// Create temp directory for all artifacts
	tempDir := t.TempDir()
	issuerPubKeyFile := filepath.Join(tempDir, "issuer-pub.pem")
	clientPrivKeyFile := filepath.Join(tempDir, "client-priv.pem")
	grantFile := filepath.Join(tempDir, "grant.paseto")
	dbFile := filepath.Join(tempDir, "laneq.db")

	// Generate issuer and client keypairs
	issuerKey, err := grantauth.NewKey()
	if err != nil {
		t.Fatalf("generate issuer key: %v", err)
	}

	clientKey, err := grantauth.NewKey()
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}

	// Write issuer public key to temp file
	issuerPubPEM, err := issuerKey.PublicKeyPEM()
	if err != nil {
		t.Fatalf("issuer public key PEM: %v", err)
	}
	if err := os.WriteFile(issuerPubKeyFile, []byte(issuerPubPEM), 0644); err != nil {
		t.Fatalf("write issuer pubkey file: %v", err)
	}
	t.Logf("✓ Issuer pubkey written to %s", issuerPubKeyFile)

	// Write client private key to temp file
	clientPrivPEM, err := clientKey.PrivateKeyPEM()
	if err != nil {
		t.Fatalf("client private key PEM: %v", err)
	}
	if err := os.WriteFile(clientPrivKeyFile, []byte(clientPrivPEM), 0644); err != nil {
		t.Fatalf("write client privkey file: %v", err)
	}
	t.Logf("✓ Client privkey written to %s", clientPrivKeyFile)

	// Get client public key PEM (for cnf binding)
	clientPubPEM, err := clientKey.PublicKeyPEM()
	if err != nil {
		t.Fatalf("client public key PEM: %v", err)
	}

	// Pick a high port to avoid collisions
	port := findFreePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	aud := fmt.Sprintf("laneq://authwire-test:%d", port)

	// Mint the grant
	now := time.Now()
	grant, err := issuerKey.MintGrant(grantauth.GrantParams{
		Iss:                "test-issuer",
		Sub:                "agent-host",
		Aud:                aud,
		ClientPublicKeyPEM: clientPubPEM,
		ClientKid:          "client-key-1",
		Kid:                "issuer-key-1",
		Now:                now,
		TTL:                30 * time.Minute,
		JTI:                fmt.Sprintf("jti-%d", now.UnixNano()),
	})
	if err != nil {
		t.Fatalf("mint grant: %v", err)
	}

	// Write grant to temp file
	if err := os.WriteFile(grantFile, []byte(grant), 0644); err != nil {
		t.Fatalf("write grant file: %v", err)
	}
	t.Logf("✓ Grant minted and written to %s", grantFile)
	t.Logf("  Audience: %s", aud)

	// Start laneq server in enforce mode
	t.Logf("Starting laneq gRPC server in enforce mode at %s...", addr)
	serverProc := startLaneqServer(t, laneqSrc, addr, dbFile, issuerPubKeyFile, aud, "enforce")
	defer killLaneqServer(t, serverProc)

	// Wait for server to be reachable
	waitForServer(t, addr, 30*time.Second)
	t.Logf("✓ Server is ready at %s", addr)

	// === SCENARIO-0117: ENFORCE ACCEPTS AUTHENTICATED CLIENT ===
	t.Run("SCENARIO-0117-enforce-accept-auth", func(t *testing.T) {
		t.Logf("Testing: enforce mode accepts valid PASETO auth")

		// Create file-based grant source
		grantSrc, err := grantauth.NewFileGrantSource(grantFile)
		if err != nil {
			t.Fatalf("create grant source: %v", err)
		}

		// Dial with auth interceptor
		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithChainUnaryInterceptor(
				grantauth.NewClientInterceptor(grantSrc, clientKey, aud),
			),
		)
		if err != nil {
			t.Fatalf("dial with interceptor: %v", err)
		}
		defer conn.Close()

		q := NewLaneqQueueWithConn(conn, "scenario-0117")

		// Push a directive
		dir := Directive{
			Intent:      "scenario-0117-test",
			Importance:  ImportanceNormal,
		}
		id, err := q.Push(dir)
		if err != nil {
			t.Fatalf("push with auth: %v", err)
		}
		t.Logf("✓ Push succeeded with auth, directive ID: %s", id)

		// Peek to verify
		d, err := q.Peek()
		if err != nil {
			t.Fatalf("peek with auth: %v", err)
		}
		if d.ID != id {
			t.Fatalf("peeked directive has wrong ID: got %s, want %s", d.ID, id)
		}
		t.Logf("✓ Peek succeeded, verified ID matches: %s", id)

		// Take (Claim) to verify full lifecycle
		claimed, _, err := q.Claim("test-worker", time.Minute)
		if err != nil {
			t.Fatalf("claim with auth: %v", err)
		}
		if claimed.ID != id {
			t.Fatalf("claimed directive has wrong ID: got %s, want %s", claimed.ID, id)
		}
		if claimed.Intent != "scenario-0117-test" {
			t.Fatalf("claimed directive has wrong intent: got %s, want scenario-0117-test", claimed.Intent)
		}
		t.Logf("✓ Claim succeeded, directive claimed: %s", claimed.ID)
		t.Logf("✓✓ SCENARIO-0117 PASSED: enforce mode accepts authenticated client")


		conn.Close()
	})

	// === SCENARIO-0118: ENFORCE REJECTS UNAUTHENTICATED CLIENT ===
	t.Run("SCENARIO-0118-enforce-reject-unauth", func(t *testing.T) {
		t.Logf("Testing: enforce mode rejects unauthenticated client")

		// Dial WITHOUT auth interceptor (plain insecure)
		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			t.Fatalf("dial without interceptor: %v", err)
		}
		defer conn.Close()

		q := NewLaneqQueue(laneqpb.NewLaneqClient(conn), "scenario-0118")

		// Try to push without auth
		dir := Directive{
			Intent:      "scenario-0118-test",
			Importance:  ImportanceNormal,
		}
		_, err = q.Push(dir)
		if err == nil {
			t.Fatalf("push without auth should fail, but succeeded")
		}

		// Verify the error is Unauthenticated
		st, ok := status.FromError(err)
		if !ok {
			t.Fatalf("push error is not gRPC status: %v", err)
		}
		if st.Code() != codes.Unauthenticated {
			t.Fatalf("push without auth returned code %v, want Unauthenticated", st.Code())
		}
		t.Logf("✓ Push without auth rejected with Unauthenticated: %v", st.Message())
		t.Logf("✓✓ SCENARIO-0118 PASSED: enforce mode rejects unauthenticated client")
	})

	// === SCENARIO-0119: LOG-ONLY ALLOWS UNAUTHENTICATED CLIENT ===
	t.Run("SCENARIO-0119-log-only-allow-unauth", func(t *testing.T) {
		t.Logf("Testing: log-only mode allows unauthenticated client (safe rollout)")

		// Kill the enforce-mode server
		killLaneqServer(t, serverProc)
		time.Sleep(500 * time.Millisecond) // brief delay for cleanup

		// Create a fresh DB for log-only mode (don't reuse the enforce mode DB)
		logOnlyDbFile := filepath.Join(tempDir, "laneq-log-only.db")
		_ = os.Remove(logOnlyDbFile)
		_ = os.Remove(logOnlyDbFile + "-shm")
		_ = os.Remove(logOnlyDbFile + "-wal")


		// Start laneq server in log-only mode
		t.Logf("Restarting laneq server in log-only mode...")
		logOnlyLogFile := filepath.Join(tempDir, "laneq-log-only.log")
		serverProc = startLaneqServerWithLogFile(t, laneqSrc, addr, logOnlyDbFile, issuerPubKeyFile, aud, "log-only", logOnlyLogFile)
		defer killLaneqServer(t, serverProc)

		// Wait for server to be ready
		waitForServer(t, addr, 30*time.Second)
		t.Logf("✓ Server is ready in log-only mode at %s", addr)

		// Dial WITHOUT auth interceptor
		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			t.Fatalf("dial without interceptor: %v", err)
		}
		defer conn.Close()

		q := NewLaneqQueue(laneqpb.NewLaneqClient(conn), "scenario-0119")

		// Try to push without auth — should succeed in log-only mode
		time.Sleep(1 * time.Second) // Give server more time to settle

		dir := Directive{
			Intent:      "scenario-0119-test",
			Importance:  ImportanceNormal,
		}
		id, err := q.Push(dir)
		if err != nil {
			t.Fatalf("push without auth in log-only mode failed: %v", err)
		}
		t.Logf("✓ Push succeeded (allowed) in log-only mode, directive ID: %s", id)

		// Verify with Peek
		d, err := q.Peek()
		if err != nil {
			t.Fatalf("peek in log-only mode: %v", err)
		}
		if d.ID != id {
			t.Fatalf("peeked directive has wrong ID: got %s, want %s", d.ID, id)
		}
		t.Logf("✓ Peek succeeded, verified ID: %s", id)
		t.Logf("✓✓ SCENARIO-0119 PASSED: log-only mode allows unauthenticated client (safe rollout)")
		conn.Close()
	})
}

// findFreePort finds an available port on the local machine.
func findFreePort(t *testing.T) int {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port
}

// startLaneqServer starts a real laneq gRPC server subprocess in the given auth mode.
// Returns the *exec.Cmd so the caller can manage the process.
func startLaneqServer(t *testing.T, laneqSrc, addr, dbFile, pubKeyFile, aud, mode string) *exec.Cmd {
	logFile := filepath.Join(t.TempDir(), "laneq-"+mode+".log")
	return startLaneqServerWithLogFile(t, laneqSrc, addr, dbFile, pubKeyFile, aud, mode, logFile)
}

// startLaneqServerWithLogFile starts a real laneq gRPC server subprocess with an explicit log file.
func startLaneqServerWithLogFile(t *testing.T, laneqSrc, addr, dbFile, pubKeyFile, aud, mode, logFile string) *exec.Cmd {
	// Ensure LANEQ_DB exists (can be empty file)
	_ = os.WriteFile(dbFile, []byte{}, 0644)

	cmd := exec.Command("uv", "run", "--project", laneqSrc, "laneq-grpc", "--addr", addr)
	cmd.Env = append(os.Environ(),
		"LANEQ_DB="+dbFile,
		"LANEQ_AUTH_MODE="+mode,
		"LANEQ_AUTH_AUDIENCE="+aud,
		"LANEQ_AUTH_PUBKEY_PATHS="+pubKeyFile,
		"LANEQ_AUTH_SKEW_SECONDS=30",
	)


	// Capture output for debugging
	outFile, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("create laneq log file: %v", err)
	}
	cmd.Stdout = outFile
	cmd.Stderr = outFile

	if err := cmd.Start(); err != nil {
		t.Fatalf("start laneq server: %v", err)
	}

	t.Logf("Started laneq server (PID %d, mode=%s, log=%s)", cmd.Process.Pid, mode, logFile)
	return cmd
}

// killLaneqServer kills a running laneq server subprocess.
func killLaneqServer(t *testing.T, cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	t.Logf("Killed laneq server (PID %d)", cmd.Process.Pid)
}

// waitForServer waits for a gRPC server to be reachable at the given address.
// Times out after the given duration.
func waitForServer(t *testing.T, addr string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("server at %s not reachable after %v", addr, timeout)
}
