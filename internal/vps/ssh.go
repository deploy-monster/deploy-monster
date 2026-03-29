package vps

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHPool manages persistent SSH connections to remote servers.
type SSHPool struct {
	mu         sync.RWMutex
	clients    map[string]*sshConn
	knownHosts map[string]ssh.PublicKey
	logger     *slog.Logger
}

type sshConn struct {
	client   *ssh.Client
	lastUsed time.Time
}

// NewSSHPool creates a new SSH connection pool.
func NewSSHPool(logger *slog.Logger) *SSHPool {
	pool := &SSHPool{
		clients:    make(map[string]*sshConn),
		knownHosts: make(map[string]ssh.PublicKey),
		logger:     logger,
	}

	// Cleanup idle connections every 5 minutes
	go pool.cleanupLoop()

	return pool
}

// AddKnownHost registers a trusted host public key for SSH verification.
func (p *SSHPool) AddKnownHost(host string, key ssh.PublicKey) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.knownHosts[host] = key
}

// Execute runs a command on a remote server via SSH.
func (p *SSHPool) Execute(ctx context.Context, host string, port int, user string, key []byte, command string) (string, error) {
	client, err := p.getOrCreate(host, port, user, key)
	if err != nil {
		return "", fmt.Errorf("ssh connect: %w", err)
	}

	session, err := client.NewSession()
	if err != nil {
		// Connection might be stale, remove and retry
		p.remove(host)
		client, err = p.getOrCreate(host, port, user, key)
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
func (p *SSHPool) Upload(ctx context.Context, host string, port int, user string, key []byte, localContent []byte, remotePath string) error {
	client, err := p.getOrCreate(host, port, user, key)
	if err != nil {
		return fmt.Errorf("ssh connect: %w", err)
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	// Use stdin pipe for file content
	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}

	go func() {
		defer stdin.Close()
		fmt.Fprintf(stdin, "C0644 %d %s\n", len(localContent), remotePath)
		stdin.Write(localContent)
		fmt.Fprint(stdin, "\x00")
	}()

	return session.Run("scp -t " + remotePath)
}

// Close closes all SSH connections.
func (p *SSHPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for host, conn := range p.clients {
		conn.client.Close()
		delete(p.clients, host)
	}
}

func (p *SSHPool) getOrCreate(host string, port int, user string, key []byte) (*ssh.Client, error) {
	addr := fmt.Sprintf("%s:%d", host, port)

	p.mu.RLock()
	if conn, ok := p.clients[addr]; ok {
		conn.lastUsed = time.Now()
		p.mu.RUnlock()
		return conn.client, nil
	}
	p.mu.RUnlock()

	// Parse private key
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse ssh key: %w", err)
	}

	p.mu.Lock()
	if p.knownHosts == nil {
		p.knownHosts = make(map[string]ssh.PublicKey)
	}
	knownKey, hasKnownKey := p.knownHosts[host]
	p.mu.Unlock()

	var hostKeyCallback ssh.HostKeyCallback
	if hasKnownKey {
		hostKeyCallback = ssh.FixedHostKey(knownKey)
	} else {
		hostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			p.mu.Lock()
			p.knownHosts[host] = key
			p.mu.Unlock()
			p.logger.Warn("trusted new SSH host key on first use", "host", host, "fingerprint", ssh.FingerprintSHA256(key))
			return nil
		}
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	p.clients[addr] = &sshConn{client: client, lastUsed: time.Now()}
	p.mu.Unlock()

	p.logger.Info("SSH connection established", "host", addr, "user", user)
	return client, nil
}

func (p *SSHPool) remove(host string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if conn, ok := p.clients[host]; ok {
		conn.client.Close()
		delete(p.clients, host)
	}
}

func (p *SSHPool) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		p.mu.Lock()
		for addr, conn := range p.clients {
			if time.Since(conn.lastUsed) > 10*time.Minute {
				conn.client.Close()
				delete(p.clients, addr)
				p.logger.Debug("SSH connection closed (idle)", "host", addr)
			}
		}
		p.mu.Unlock()
	}
}
