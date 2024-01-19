package sshtunnel

import (
	"fmt"
	"golang.org/x/term"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/crypto/ssh"
)

type Endpoint struct {
	Host string
	Port int
	User string
}

func NewEndpoint(s string) *Endpoint {
	endpoint := &Endpoint{
		Host: s,
	}
	if parts := strings.Split(endpoint.Host, "@"); len(parts) > 1 {
		endpoint.User = parts[0]
		endpoint.Host = parts[1]
	}
	if parts := strings.Split(endpoint.Host, ":"); len(parts) > 1 {
		endpoint.Host = parts[0]
		endpoint.Port, _ = strconv.Atoi(parts[1])
	}
	return endpoint
}

func (endpoint *Endpoint) String() string {
	return fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port)
}

type SSHTunnel struct {
	Local        *Endpoint
	Server       *Endpoint
	Remote       *Endpoint
	Config       *ssh.ClientConfig
	listener     *net.Listener
	SshClient    *ssh.Client
	logger       zerolog.Logger
	LogKeepAlive bool
}

func (tunnel *SSHTunnel) Start() {
	tunnel.Local.Port = (*tunnel.listener).Addr().(*net.TCPAddr).Port
	go func() {
		tunnel.Local.Port = (*tunnel.listener).Addr().(*net.TCPAddr).Port
		for {
			conn, err := (*tunnel.listener).Accept()
			if err != nil {
				tunnel.logger.Error().Err(err).Msg("error accepting connection")
				return
			}
			tunnel.logger.Debug().
				Interface("l-port", conn.LocalAddr()).
				Interface("r-port", conn.RemoteAddr()).
				Msg("local connection accepted")
			go tunnel.forward(conn)
		}
	}()
}

func (tunnel *SSHTunnel) Stop() {
	(*tunnel.listener).Close()
}

var connOpen int64

func (tunnel *SSHTunnel) forward(localConn net.Conn) {
	defer localConn.Close()
	serverConn, err := ssh.Dial("tcp", tunnel.Server.String(), tunnel.Config)
	if err != nil {
		tunnel.logger.Error().Err(err).Msg("server dial error")
		return
	}
	defer serverConn.Close()

	tunnel.logger.Debug().Str("connected-to", tunnel.Server.String()).Msg("connected (1 of 1)")
	remoteConn, err := serverConn.Dial("tcp", tunnel.Remote.String())
	if err != nil {
		tunnel.logger.Error().Err(err).Msg("remote dial error")
		return
	}
	defer remoteConn.Close()

	tunnel.logger.Debug().Str("connected-to", tunnel.Remote.String()).Msg("connected (2 of 1)")
	copyConn := func(writer, reader net.Conn) {
		_, err := io.Copy(writer, reader)
		if err != nil {
			tunnel.logger.Error().Err(err).Msg("io.Copy error")
		}
	}

	quit := make(chan bool, 2)

	go func(client *ssh.Client, quit chan bool) {
		for {
			timer := time.NewTimer(5 * time.Second)
			ok := make(chan bool, 2)
			go func(ok chan bool) {
				tn := time.Now()
				_, _, err = client.SendRequest("keepalive@golang.org", true, nil)
				if err != nil {
					activeConnections := atomic.AddInt64(&connOpen, -1)
					tunnel.logger.Debug().Err(err).Int64("open-conn", activeConnections).Msg("keep-alive error, closing")
					client.Close()
					quit <- true
					return
				}
				timeTook := time.Since(tn)
				if timeTook < 3*time.Second {
					time.Sleep(3*time.Second - timeTook)
				}
				ok <- true
			}(ok)

			select {
			case <-quit:
				timer.Stop()
				return
			case <-ok:
				if tunnel.LogKeepAlive {
					tunnel.logger.Debug().Msg("got keepalive packet")
				}
				timer.Stop()
				continue
			case <-timer.C:
				client.Close()
			}
		}
	}(serverConn, quit)

	atomic.AddInt64(&connOpen, 1)

	go copyConn(localConn, remoteConn)
	copyConn(remoteConn, localConn)
}

func PrivateKeyFile(file string) (ssh.AuthMethod, error) {
	if strings.HasPrefix(file, "~") {
		uhd, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("error looking up user home dir, and key path (%s) is relative: %w", file, err)
		}
		file = strings.Replace(file, "~", uhd, 1)
	}
	buffer, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("error reading key from %s: %w", file, err)
	}

	key, err := ssh.ParsePrivateKey(buffer)

	if _, ok := err.(*ssh.PassphraseMissingError); ok && term.IsTerminal(syscall.Stdin) {
		fmt.Printf("Enter passphrase for private key %s: ", file)
		phrase, err := term.ReadPassword(syscall.Stdin)

		if err != nil {
			return nil, fmt.Errorf("error reading passphrase from terminal: %w", err)
		}

		key, err = ssh.ParsePrivateKeyWithPassphrase(buffer, phrase)
		if err != nil {
			return nil, fmt.Errorf("error parsing private key with passphrase file from %s: %w", file, err)
		}

	} else if err != nil {
		return nil, fmt.Errorf("error parsing private key file from %s: %w", file, err)
	}

	return ssh.PublicKeys(key), nil
}

func NewSSHTunnel(tunnel string, auth ssh.AuthMethod, destination string, logger zerolog.Logger) (*SSHTunnel, error) {
	// A random port will be chosen for us.
	localEndpoint := NewEndpoint("localhost:0")

	server := NewEndpoint(tunnel)
	if server.Port == 0 {
		server.Port = 22
	}

	sshTunnel := &SSHTunnel{
		Config: &ssh.ClientConfig{
			User:    server.User,
			Auth:    []ssh.AuthMethod{auth},
			Timeout: 5 * time.Second,
			HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
				// Always accept key.
				return nil
			},
		},
		Local:        localEndpoint,
		Server:       server,
		Remote:       NewEndpoint(destination),
		logger:       logger,
		LogKeepAlive: false,
	}

	listener, err := net.Listen("tcp", sshTunnel.Local.String())
	if err != nil {
		return nil, err
	}
	sshTunnel.listener = &listener

	return sshTunnel, nil
}
