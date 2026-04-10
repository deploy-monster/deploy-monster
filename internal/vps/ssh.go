package vps

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"golang.org/x/crypto/ssh"
)

const (
	sshPoolCleanupInterval = 5 * time.Minute
	sshPoolIdleTimeout     = 10 * time.Minute
	sshDialTimeout         = 10 * time.Second
)

// SSHPool manages persistent SSH connections to remote servers.
//
// The pool is shared across the process and is read from hot paths
// (bootstrap, agent provisioning, swarm init). Design notes for Tier 66:
//
//   - Connections are keyed by "host:port" addr strings. Callers never
//     touch map keys directly; all mutation goes through getOrCreate,
//     removeByAddr, Close, and cleanupIdle.
//   - Every public method releases p.mu before performing network I/O
//     (ssh.Dial, client.Close). Holding the lock across Close can block
//     the whole pool for the TCP FIN timeout, and holding it across Dial
//     would serialise every new connection.
//   - lastUsed writes happen under p.mu (write lock). Before Tier 66 the
//     cache-hit fast path wrote lastUsed while holding only an RLock,
//     which the race detector correctly flagged as a data race against
//     the cleanup sweep.
//   - The cleanup goroutine is driven by stopCh and tracked via wg so
//     Close() can block until it has fully exited. Without that, the
//     goroutine would outlive the pool and tick forever in the
//     background.
type SSHPool struct {
	mu         sync.RWMutex
	clients    map[string]*sshConn
	knownHosts map[string]ssh.PublicKey
	logger     *slog.Logger

	// Shutdown plumbing. stopOnce guards close(stopCh) against double
	// Close — a real risk because the pool is a process-wide singleton
	// that test fixtures also construct directly. wg tracks the
	// cleanupLoop goroutine so Close can wait for it to exit before
	// tearing the client map down.
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

type sshConn struct {
	client   *ssh.Client
	lastUsed time.Time
}

// NewSSHPool creates a new SSH connection pool. The returned pool owns
// a background goroutine that sweeps idle connections every
// sshPoolCleanupInterval; callers must invoke Close() to stop it.
func NewSSHPool(logger *slog.Logger) *SSHPool {
	if logger == nil {
		logger = slog.Default()
	}
	pool := &SSHPool{
		clients:    make(map[string]*sshConn),
		knownHosts: make(map[string]ssh.PublicKey),
		logger:     logger,
		stopCh:     make(chan struct{}),
	}

	pool.wg.Add(1)
	core.SafeGo(logger, "ssh-pool-cleanup", pool.cleanupLoop)

	return pool
}

// AddKnownHost registers a trusted host public key for SSH verification.
func (p *SSHPool) AddKnownHost(host string, key ssh.PublicKey) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.knownHosts[host] = key
}

// Execute runs a command on a remote server via SSH. If the cached
// client's session fails (typically because the TCP connection went
// stale), Execute removes the dead entry and reconnects once.
func (p *SSHPool) Execute(ctx context.Context, host string, port int, user string, key []byte, command string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	addr := fmt.Sprintf("%s:%d", host, port)

	client, err := p.getOrCreateCtx(ctx, host, port, user, key)
	if err != nil {
		return "", fmt.Errorf("ssh connect: %w", err)
	}

	session, err := client.NewSession()
	if err != nil {
		// Connection is stale — drop it and reconnect once. The bug in
		// the pre-Tier-66 code was that remove was called with `host`
		// (no port) while the map is keyed by "host:port", so the stale
		// entry was never actually evicted. Use addr here so the cache
		// ends up consistent.
		p.removeByAddr(addr)
		client, err = p.getOrCreateCtx(ctx, host, port, user, key)
		if err != nil {
			return "", fmt.Errorf("ssh reconnect: %w", err)
		}
		session, err = client.NewSession()
		if err != nil {
			return "", fmt.Errorf("ssh session: %w", err)
		}
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	if err != nil {
		return string(output), fmt.Errorf("ssh exec: %w\noutput: %s", err, string(output))
	}

	return string(output), nil
}

// Upload transfers a file to a remote server using SCP over SSH.
//
// The SCP protocol requires writing a header, the payload, and a null
// terminator to the session's stdin, then closing stdin so the remote
// scp server advances to the next command. Before Tier 66 the writer
// goroutine silently swallowed every write error — a broken pipe would
// look like a successful upload with an empty file. Now we capture the
// first write error into a channel and surface it alongside any
// Run()-level error.
func (p *SSHPool) Upload(ctx context.Context, host string, port int, user string, key []byte, localContent []byte, remotePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	client, err := p.getOrCreateCtx(ctx, host, port, user, key)
	if err != nil {
		return fmt.Errorf("ssh connect: %w", err)
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("ssh stdin pipe: %w", err)
	}

	writeErrCh := make(chan error, 1)
	core.SafeGo(p.logger, "scp-upload", func() {
		defer func() {
			if cerr := stdin.Close(); cerr != nil {
				// A closed stdin from the peer is expected; only
				// surface it if we did not already record an earlier
				// write error.
				select {
				case writeErrCh <- cerr:
				default:
				}
			}
			close(writeErrCh)
		}()
		if _, werr := fmt.Fprintf(stdin, "C0644 %d %s\n", len(localContent), remotePath); werr != nil {
			writeErrCh <- werr
			return
		}
		if _, werr := stdin.Write(localContent); werr != nil {
			writeErrCh <- werr
			return
		}
		if _, werr := io.WriteString(stdin, "\x00"); werr != nil {
			writeErrCh <- werr
			return
		}
	})

	runErr := session.Run("scp -t " + remotePath)

	// Drain the writer so we do not leak the goroutine if Run returned
	// early. The channel is buffered+closed, so this is bounded.
	var writeErr error
	for e := range writeErrCh {
		if writeErr == nil && e != nil {
			writeErr = e
		}
	}

	if runErr != nil {
		return fmt.Errorf("ssh scp: %w", runErr)
	}
	if writeErr != nil {
		return fmt.Errorf("ssh scp write: %w", writeErr)
	}
	return nil
}

// Close stops the background cleanup loop and tears down every cached
// SSH client. Safe to call multiple times; second and subsequent calls
// are no-ops. Close releases the pool mutex before calling client.Close
// so a hanging TCP teardown cannot stall unrelated callers.
func (p *SSHPool) Close() {
	p.stopOnce.Do(func() {
		if p.stopCh != nil {
			close(p.stopCh)
		}
	})
	p.wg.Wait()

	// Snapshot every client handle under the lock, then release the
	// lock before calling Close() on any of them. Prior to Tier 66 the
	// lock was held across every Close(), which meant one hung peer
	// could freeze the entire pool.
	p.mu.Lock()
	clients := make([]*ssh.Client, 0, len(p.clients))
	for addr, conn := range p.clients {
		clients = append(clients, conn.client)
		delete(p.clients, addr)
	}
	p.mu.Unlock()

	for _, c := range clients {
		_ = c.Close()
	}
}

// getOrCreate is the context-free entry point retained for existing
// test call sites. Production code uses getOrCreateCtx so a hanging
// handshake can be cancelled via the request context.
func (p *SSHPool) getOrCreate(host string, port int, user string, key []byte) (*ssh.Client, error) {
	return p.getOrCreateCtx(context.Background(), host, port, user, key)
}

// getOrCreateCtx returns a cached SSH client for host:port, dialing a
// new one if necessary. The dial happens without the pool mutex held
// so a slow handshake does not block concurrent callers targeting
// other hosts. Cancelling ctx aborts a hanging TCP dial.
func (p *SSHPool) getOrCreateCtx(ctx context.Context, host string, port int, user string, key []byte) (*ssh.Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	addr := fmt.Sprintf("%s:%d", host, port)

	// Fast path: cache hit. Lock briefly to touch lastUsed so the
	// cleanup sweep sees the fresh timestamp. Before Tier 66 this path
	// wrote lastUsed while holding only an RLock, which the race
	// detector flagged as a data race against the cleanup sweep.
	p.mu.Lock()
	if conn, ok := p.clients[addr]; ok {
		conn.lastUsed = time.Now()
		p.mu.Unlock()
		return conn.client, nil
	}
	// Snapshot the known-host pin (if any) while we already hold the
	// lock. We pass it into the ClientConfig below — the callback
	// itself still acquires the lock to write a newly-trusted key back.
	knownKey, hasKnownKey := p.knownHosts[host]
	p.mu.Unlock()

	// Parse private key outside the lock — CPU work, no reason to
	// serialise concurrent creators.
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse ssh key: %w", err)
	}

	var hostKeyCallback ssh.HostKeyCallback
	if hasKnownKey {
		hostKeyCallback = ssh.FixedHostKey(knownKey)
	} else {
		hostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			p.mu.Lock()
			// Lazy-init: tests construct SSHPool via struct literal and
			// may leave knownHosts nil, which would panic on the
			// assignment below. NewSSHPool always initialises it.
			if p.knownHosts == nil {
				p.knownHosts = make(map[string]ssh.PublicKey)
			}
			p.knownHosts[host] = key
			p.mu.Unlock()
			p.logger.Warn("trusted new SSH host key on first use",
				"host", host,
				"fingerprint", ssh.FingerprintSHA256(key),
			)
			return nil
		}
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCallback,
		Timeout:         sshDialTimeout,
	}

	// Dial with context-awareness: dial the TCP socket via a
	// context-aware dialer so ctx cancellation aborts a hanging
	// handshake, then layer SSH on top. This is the standard pattern
	// for a context-aware ssh.Dial. Before Tier 66 the pool used
	// ssh.Dial directly, which ignores ctx and only respects its own
	// 10-second timeout — meaning a request cancellation could not
	// actually interrupt a slow bootstrap.
	dialer := &net.Dialer{Timeout: sshDialTimeout}
	tcpConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	sshConnRaw, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, config)
	if err != nil {
		_ = tcpConn.Close()
		return nil, err
	}
	client := ssh.NewClient(sshConnRaw, chans, reqs)

	// Install. If another caller won the race and already installed a
	// client for this addr, drop ours and return theirs.
	p.mu.Lock()
	if existing, ok := p.clients[addr]; ok {
		existing.lastUsed = time.Now()
		p.mu.Unlock()
		_ = client.Close()
		return existing.client, nil
	}
	p.clients[addr] = &sshConn{client: client, lastUsed: time.Now()}
	p.mu.Unlock()

	p.logger.Info("SSH connection established", "host", addr, "user", user)
	return client, nil
}

// removeByAddr evicts a cached connection by its "host:port" key and
// closes the underlying client outside of the pool lock. The Tier 66
// fix renamed this from remove() and moved the Close call out from
// under the mutex.
func (p *SSHPool) removeByAddr(addr string) {
	p.mu.Lock()
	conn, ok := p.clients[addr]
	if ok {
		delete(p.clients, addr)
	}
	p.mu.Unlock()

	if ok && conn != nil && conn.client != nil {
		_ = conn.client.Close()
	}
}

// remove is kept as a thin wrapper for backwards compatibility with
// existing tests that call pool.remove("host:port"). New call sites
// should use removeByAddr directly.
func (p *SSHPool) remove(addr string) {
	p.removeByAddr(addr)
}

func (p *SSHPool) cleanupLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(sshPoolCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.cleanupIdle()
		case <-p.stopCh:
			return
		}
	}
}

// cleanupIdle evicts connections that have been idle for longer than
// sshPoolIdleTimeout. Eviction is split into a snapshot phase (under
// the lock) and a close phase (without the lock) so a slow peer cannot
// block the pool.
func (p *SSHPool) cleanupIdle() {
	now := time.Now()
	p.mu.Lock()
	toClose := make([]*ssh.Client, 0)
	closedAddrs := make([]string, 0)
	for addr, conn := range p.clients {
		if now.Sub(conn.lastUsed) > sshPoolIdleTimeout {
			toClose = append(toClose, conn.client)
			closedAddrs = append(closedAddrs, addr)
			delete(p.clients, addr)
		}
	}
	p.mu.Unlock()

	for i, c := range toClose {
		if c != nil {
			_ = c.Close()
		}
		p.logger.Debug("SSH connection closed (idle)", "host", closedAddrs[i])
	}
}
