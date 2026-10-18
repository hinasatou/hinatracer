package app

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"hinatracer/internal/config"
	"hinatracer/internal/webserver"
)

// cliOptions are the command-line switches.
type cliOptions struct {
	Web     bool
	Listen  string
	Host    string
	Port    int
	Help    bool
	Version bool
	// Config overrides the config file path (also $HINATRACER_CONFIG).
	Config string
	// ServerMode: headless WebUI for system services (deb package). Data
	// defaults to /var/lib/hinatracer and self-update install is off.
	ServerMode bool
}

// serverMode is set by --server-mode.
var serverMode bool

const cliUsage = `HinaTracer %s

Usage:
  hinatracer                      start the desktop app
  hinatracer --web [options]      run only the WebUI server (no window)
  hinatracer <command> ...        control a running HinaTracer, e.g.
                                  "hinatracer ping list" (see "hinatracer help")
  hinatracer web passwd           set the WebUI password in the config file

Options:
  --web                 run the WebUI without opening a window
  --listen ADDR         listen address, e.g. 127.0.0.1:8790, :8080, [::1]:8790, 0.0.0.0:8790
  --web-host HOST       listen host (default: saved setting or 127.0.0.1)
  --web-port PORT       listen port (default: saved setting or 8790)
  --config FILE         use this config file (default: $HINATRACER_CONFIG,
                        else the per-user config directory)
  --server-mode         run as a system service: WebUI only, data in
                        /var/lib/hinatracer ($HINATRACER_DATA_DIR), updates
                        are reported but never installed
  --version             print the version
  -h, --help            show this help

Access protection:
  Listening on any address other than 127.0.0.1 / ::1 / localhost needs a
  password. Set it on the WebUI page of the desktop app, or put it in the
  HINATRACER_WEB_PASSWORD environment variable.

AI clients (MCP):
  While the WebUI runs, an MCP server is available at http://HOST:PORT/mcp
  (Streamable HTTP). Send an API token from the WebUI page as
  "Authorization: Bearer <token>".

Settings, ping targets and traceroute tabs are shared with the desktop app.
`

// parseCLI parses args (without the program name). ok=false means no CLI
// switches were given and the desktop app should start.
func parseCLI(args []string, out io.Writer) (opt cliOptions, ok bool, err error) {
	if len(args) == 0 {
		return opt, false, nil
	}
	fs := flag.NewFlagSet("hinatracer", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.Usage = func() { fmt.Fprintf(out, cliUsage, appVersion) }
	fs.BoolVar(&opt.Web, "web", false, "")
	fs.StringVar(&opt.Listen, "listen", "", "")
	fs.StringVar(&opt.Host, "web-host", "", "")
	fs.IntVar(&opt.Port, "web-port", 0, "")
	fs.BoolVar(&opt.Help, "help", false, "")
	fs.BoolVar(&opt.Help, "h", false, "")
	fs.BoolVar(&opt.Version, "version", false, "")
	fs.StringVar(&opt.Config, "config", "", "")
	fs.BoolVar(&opt.ServerMode, "server-mode", false, "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return cliOptions{Help: true}, true, nil
		}
		return opt, true, err
	}
	if fs.NArg() > 0 {
		return opt, true, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if opt.Listen != "" || opt.Host != "" || opt.Port != 0 || opt.ServerMode {
		opt.Web = true
	}
	return opt, true, nil
}

// resolveListen merges CLI switches over the saved config.
func resolveListen(opt cliOptions, cfg config.Config) (string, int, error) {
	host, port := strings.TrimSpace(cfg.WebHost), cfg.WebPort
	if host == "" {
		host = webserver.DefaultHost
	}
	if !webserver.ValidPort(port) {
		port = webserver.DefaultPort
	}
	if opt.Listen != "" {
		h, p, err := webserver.ParseListen(opt.Listen)
		if err != nil {
			return "", 0, err
		}
		host, port = h, p
	}
	if opt.Host != "" {
		h, err := webserver.NormalizeHost(opt.Host)
		if err != nil {
			return "", 0, err
		}
		host = h
	}
	if opt.Port != 0 {
		if !webserver.ValidPort(opt.Port) {
			return "", 0, webserver.ErrBadListen
		}
		port = opt.Port
	}
	return host, port, nil
}

// headlessLogPath is where headless mode writes its log (the Windows GUI
// build has no console when started from Explorer).
func headlessLogPath() string {
	return filepath.Join(config.DataDir(), "web.log")
}

// cliLogger writes to stdout (when available) and the log file.
type cliLogger struct{ w []io.Writer }

func (l cliLogger) Printf(format string, args ...any) {
	line := time.Now().Format("2006-01-02 15:04:05 ") + fmt.Sprintf(format, args...) + "\n"
	for _, w := range l.w {
		_, _ = io.WriteString(w, line)
	}
}

// runCLI handles CLI mode and returns the exit code.
func runCLI(opt cliOptions) int {
	if opt.Help {
		fmt.Fprintf(os.Stdout, cliUsage, appVersion)
		return 0
	}
	if opt.Version {
		fmt.Fprintln(os.Stdout, appVersion)
		return 0
	}
	if !opt.Web {
		fmt.Fprintf(os.Stderr, cliUsage, appVersion)
		return 2
	}
	applyServerMode(opt)
	ensureConfigDir()
	if d := os.Getenv("HINATRACER_DATA_DIR"); d != "" {
		_ = os.MkdirAll(d, 0o700)
	}
	logs := []io.Writer{os.Stdout}
	if f, err := os.OpenFile(headlessLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		defer f.Close()
		logs = append(logs, f)
	}
	log := cliLogger{logs}

	webEnvPassword = func() string { return os.Getenv("HINATRACER_WEB_PASSWORD") }
	a := newApp()
	host, port, err := resolveListen(opt, a.cfg)
	if err != nil {
		log.Printf("invalid listen address: %v", err)
		return 2
	}
	a.startUpdateThrottle()
	a.pingMgr.SetOnUpdate(a.requestUIUpdate)
	safeGo("bootstrap", a.bootstrapAfterWindow)
	if err := a.startWeb(host, port); err != nil {
		if errors.Is(err, webserver.ErrNeedPassword) {
			log.Printf("refusing to listen on %s without a password: set one on the WebUI page or in HINATRACER_WEB_PASSWORD", webserver.JoinListen(host, port))
		} else {
			log.Printf("cannot start WebUI on %s: %v", webserver.JoinListen(host, port), err)
		}
		return 1
	}
	st := a.web.srv.Status()
	log.Printf("HinaTracer %s WebUI listening on %s", appVersion, webserver.JoinListen(st.Host, st.Port))
	log.Printf("open %s", st.URL)
	for _, u := range webserver.LANURLs(st.Host, st.Port) {
		log.Printf("LAN: %s", u)
	}
	if a.webPasswordHash() != "" {
		log.Printf("access password: required")
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Printf("shutting down")
	a.stopWeb()
	_ = a.run(func() {
		a.pingMgr.Stop()
		a.hist.Close()
		a.stopAllTraces()
		a.persist()
	})
	return 0
}

// applyServerMode sets the config path and server defaults before anything
// reads the config.
func applyServerMode(opt cliOptions) {
	if opt.Config != "" {
		config.OverridePath = opt.Config
	}
	if !opt.ServerMode {
		return
	}
	serverMode = true
	if opt.Config == "" && os.Getenv("HINATRACER_CONFIG") == "" {
		config.OverridePath = "/etc/hinatracer/config.json"
	}
	if os.Getenv("HINATRACER_DATA_DIR") == "" {
		_ = os.Setenv("HINATRACER_DATA_DIR", "/var/lib/hinatracer")
	}
}
