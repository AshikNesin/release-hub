package srv

// The bundled 1Password Connect server. Connect is two processes,
// connect-api (REST) and connect-sync (keeps the encrypted vault copy
// current), normally two containers beside the app. The Docker image copies
// both executables from 1Password's own images at a pinned version and
// digest (Dockerfile), so with OP_CONNECT_CREDENTIALS set and no
// OP_CONNECT_HOST this process runs them itself: one container, no second
// service. Ports are fixed on loopback, the bus is wired with
// OP_BUS_PORT/OP_BUS_PEERS (1Password's setup for hosts without broadcast),
// and the credentials reach the children through OP_SESSION in their
// environment only. A child that exits is restarted with backoff.

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type bundledPorts struct{ API, Sync, APIBus, SyncBus int }

var defaultBundledPorts = bundledPorts{API: 11280, Sync: 11281, APIBus: 11282, SyncBus: 11283}

var bundledConnectHost = "http://127.0.0.1:" + strconv.Itoa(defaultBundledPorts.API)

const (
	bundledHealthyAfter = time.Minute
	bundledMaxBackoff   = time.Minute
)

type bundledConnect struct {
	mu       sync.Mutex
	state    vaultStatus
	running  map[string]*exec.Cmd
	stopping bool
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func (b *bundledConnect) status() vaultStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

func (b *bundledConnect) set(state, detail string) {
	b.mu.Lock()
	b.state = vaultStatus{State: state, Detail: detail}
	b.mu.Unlock()
}

// sessionFromCredentials turns OP_CONNECT_CREDENTIALS (the
// 1password-credentials.json contents, as JSON or base64) into the
// OP_SESSION value Connect reads: the file, base64 encoded.
func sessionFromCredentials(raw string) (string, error) {
	v := unquote(raw)
	js := []byte(v)
	if !strings.HasPrefix(v, "{") {
		dec, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			dec, err = base64.RawStdEncoding.DecodeString(v)
		}
		if err == nil {
			js = dec
		}
	}
	var probe map[string]any
	if json.Unmarshal(js, &probe) == nil {
		if _, ok := probe["deviceUuid"]; ok {
			return base64.StdEncoding.EncodeToString(js), nil
		}
	}
	return "", errors.New("OP_CONNECT_CREDENTIALS must hold the 1password-credentials.json file, as JSON or base64")
}

type bundledOptions struct {
	session string
	dir     string // Connect's HOME (its encrypted vault cache) lives here
	binDir  string // where connect-api and connect-sync are; "" = PATH
	ports   bundledPorts
	backoff time.Duration
}

func startBundledConnectFromEnv(creds, dataDir string) *bundledConnect {
	session, err := sessionFromCredentials(creds)
	if err != nil {
		slog.Error("bundled 1Password Connect can't start", "error", err)
		return &bundledConnect{state: vaultStatus{State: "error", Code: "credentials", Detail: err.Error()}}
	}
	return startBundledConnect(bundledOptions{
		session: session,
		dir:     filepath.Join(dataDir, "op-connect"),
		binDir:  unquote(envStr("OP_CONNECT_BIN_DIR")),
		ports:   defaultBundledPorts,
		backoff: time.Second,
	})
}

func startBundledConnect(opt bundledOptions) *bundledConnect {
	ctx, cancel := context.WithCancel(context.Background())
	b := &bundledConnect{
		state:   vaultStatus{State: "starting", Detail: "Starting the bundled 1Password Connect server."},
		running: map[string]*exec.Cmd{},
		cancel:  cancel,
	}
	home := filepath.Join(opt.dir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		b.set("error", "Can't create the Connect data directory: "+err.Error())
		return b
	}
	bins := map[string]string{}
	for _, name := range []string{"connect-sync", "connect-api"} {
		path, err := lookBinary(opt.binDir, name)
		if err != nil {
			b.set("error", fmt.Sprintf("%s is not in this image (%v). Build release-hub from its Dockerfile, or set OP_CONNECT_HOST to a Connect server.", name, err))
			return b
		}
		bins[name] = path
	}
	for _, name := range []string{"connect-sync", "connect-api"} {
		b.wg.Add(1)
		go b.supervise(ctx, name, bins[name], home, connectEnv(name, home, opt), opt.backoff)
	}
	return b
}

func lookBinary(dir, name string) (string, error) {
	if dir != "" {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err != nil {
			return "", err
		}
		return p, nil
	}
	return exec.LookPath(name)
}

func connectEnv(name, home string, opt bundledOptions) []string {
	api := name == "connect-api"
	port, bus, peer := opt.ports.Sync, opt.ports.SyncBus, opt.ports.APIBus
	if api {
		port, bus, peer = opt.ports.API, opt.ports.APIBus, opt.ports.SyncBus
	}
	return []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=" + home,
		"OP_SESSION=" + opt.session,
		"OP_HTTP_PORT=" + strconv.Itoa(port),
		"OP_BUS_PORT=" + strconv.Itoa(bus),
		"OP_BUS_PEERS=localhost:" + strconv.Itoa(peer),
		"OP_LOG_LEVEL=info",
		"SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt",
	}
}

func (b *bundledConnect) supervise(ctx context.Context, name, path, home string, env []string, first time.Duration) {
	defer b.wg.Done()
	delay := first
	for {
		cmd := exec.Command(path)
		cmd.Env = env
		cmd.Dir = home
		out, _ := cmd.StdoutPipe()
		cmd.Stderr = cmd.Stdout
		started := time.Now()
		if err := cmd.Start(); err != nil {
			b.set("error", fmt.Sprintf("Can't start %s: %v", name, err))
			slog.Error("bundled 1Password Connect can't start", "process", name, "error", err)
		} else {
			b.mu.Lock()
			if b.stopping {
				b.mu.Unlock()
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				return
			}
			b.running[name] = cmd
			if len(b.running) == 2 {
				b.state = vaultStatus{State: "ready", Detail: "Bundled Connect server running."}
			}
			b.mu.Unlock()
			last := logConnectOutput(name, out)
			err := cmd.Wait()
			b.mu.Lock()
			delete(b.running, name)
			stopping := b.stopping
			b.mu.Unlock()
			if stopping {
				return
			}
			if time.Since(started) >= bundledHealthyAfter {
				delay = first
			}
			slog.Warn("bundled 1Password Connect process exited; restarting", "process", name, "error", err, "in", delay, "output", last())
			b.set("starting", fmt.Sprintf("Restarting %s (it exited: %v).", name, err))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, bundledMaxBackoff)
	}
}

// logConnectOutput forwards a child's log lines at debug level and keeps
// the last few for the exit warning.
func logConnectOutput(name string, r io.Reader) func() []string {
	var mu sync.Mutex
	recent := make([]string, 0, 5)
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			line := sc.Text()
			slog.Debug("connect", "process", name, "line", line)
			mu.Lock()
			if len(recent) == 5 {
				recent = recent[1:]
			}
			recent = append(recent, line)
			mu.Unlock()
		}
	}()
	return func() []string {
		<-done
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), recent...)
	}
}

// stop terminates both processes (SIGTERM, then SIGKILL after 3 s).
func (b *bundledConnect) stop() {
	if b == nil || b.cancel == nil {
		return
	}
	b.mu.Lock()
	b.stopping = true
	procs := make([]*exec.Cmd, 0, len(b.running))
	for _, c := range b.running {
		procs = append(procs, c)
	}
	b.mu.Unlock()
	b.cancel()
	for _, c := range procs {
		_ = c.Process.Signal(os.Interrupt)
		go func(c *exec.Cmd) {
			time.Sleep(3 * time.Second)
			_ = c.Process.Kill()
		}(c)
	}
	b.wg.Wait()
}
