package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"dooh/internal/config"
)

// Exit codes for structured error handling.
const (
	ExitOK             = 0
	ExitGeneral        = 1
	ExitUsage          = 2
	ExitAuth           = 3
	ExitNotFound       = 4
	ExitPermission     = 5
	ExitConflict       = 6
)

// ExitError wraps an error with a specific exit code.
type ExitError struct {
	Code    int
	Message string
}

func (e *ExitError) Error() string { return e.Message }

type globalOpts struct {
	Profile    string
	ProfileSet bool
	ConfigPath string
	JSON       bool
	Quiet      bool
}

type runtime struct {
	opts              globalOpts
	config            config.Config
	profile           config.Profile
	context           contextState
	aiProfileEnforced bool
}

// Run executes dooh CLI commands.
func Run(args []string, stdout io.Writer) error {
	opts, rest, err := parseGlobal(args)
	if err != nil {
		return err
	}
	ctx, _ := readContextState()

	envProfile := strings.TrimSpace(os.Getenv("DOOH_PROFILE"))
	effectiveProfile := "default"
	switch {
	case opts.ProfileSet && strings.TrimSpace(opts.Profile) != "":
		effectiveProfile = strings.TrimSpace(opts.Profile)
	case hasAIEnvKey():
		effectiveProfile = "ai"
	case envProfile != "":
		effectiveProfile = envProfile
	case strings.TrimSpace(ctx.Profile) != "":
		effectiveProfile = strings.TrimSpace(ctx.Profile)
	}

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return err
	}
	opts.Profile = effectiveProfile
	rt := runtime{opts: opts, config: cfg, profile: config.Resolve(cfg, opts.Profile), context: ctx, aiProfileEnforced: hasAIEnvKey() && !opts.ProfileSet}

	if len(rest) == 0 {
		printUsage(stdout)
		return nil
	}

	switch rest[0] {
	case "version", "--version", "-v":
		_, _ = fmt.Fprintln(stdout, "0.3.0")
		return nil
	case "config":
		return runConfig(rt, rest[1:], stdout)
	case "db":
		return runDB(rt, rest[1:], stdout)
	case "setup":
		return runSetup(rt, rest[1:], stdout)
	case "demo":
		return runDemo(rt, rest[1:], stdout)
	case "login":
		return runLogin(rt, rest[1:], stdout)
	case "env":
		return runEnv(rt, rest[1:], stdout)
	case "context":
		return runContext(rt, rest[1:], stdout)
	case "user":
		return runUser(rt, rest[1:], stdout)
	case "key":
		return runKey(rt, rest[1:], stdout)
	case "task":
		return runTask(rt, rest[1:], stdout)
	case "collection":
		return runCollection(rt, rest[1:], stdout)
	case "export":
		return runExport(rt, rest[1:], stdout)
	case "tui":
		return runTUI(rt, rest[1:], stdout)
	case "whoami":
		return runWhoAmI(rt, rest[1:], stdout)
	case "event":
		return runEvent(rt, rest[1:], stdout)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", rest[0])
	}
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "dooh (pronounced duo) - local-first task manager for a human + ai pair")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "global flags:")
	_, _ = fmt.Fprintln(w, "  --profile <name>   select config profile (default, human, ai)")
	_, _ = fmt.Fprintln(w, "  --config <path>    override config file path")
	_, _ = fmt.Fprintln(w, "  --json             output machine-readable JSON")
	_, _ = fmt.Fprintln(w, "  --quiet, -q        suppress context banner on write commands")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "commands:")
	_, _ = fmt.Fprintln(w, "  task         manage tasks (add, list, show, update, complete, ...)")
	_, _ = fmt.Fprintln(w, "  collection   manage collections (add, list, show, link, unlink)")
	_, _ = fmt.Fprintln(w, "  event        query audit event log")
	_, _ = fmt.Fprintln(w, "  export       export site data to JSON files")
	_, _ = fmt.Fprintln(w, "  tui          interactive terminal dashboard")
	_, _ = fmt.Fprintln(w, "  whoami       show authenticated identity")
	_, _ = fmt.Fprintln(w, "  context      manage local context overrides (show, set, clear)")
	_, _ = fmt.Fprintln(w, "  config       view and initialize config profiles (show, init)")
	_, _ = fmt.Fprintln(w, "  user         manage users (create, list, lookup)")
	_, _ = fmt.Fprintln(w, "  key          manage API keys (create, revoke)")
	_, _ = fmt.Fprintln(w, "  db           database operations (init)")
	_, _ = fmt.Fprintln(w, "  login        store API key for a profile")
	_, _ = fmt.Fprintln(w, "  env          print shell env exports")
	_, _ = fmt.Fprintln(w, "  setup        setup demo environment")
	_, _ = fmt.Fprintln(w, "  demo         seed demo data")
	_, _ = fmt.Fprintln(w, "  version      print version")
}

func parseGlobal(args []string) (globalOpts, []string, error) {
	opts := globalOpts{}
	if len(args) == 0 {
		return opts, nil, nil
	}

	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		a := args[i]
		switch {
		case a == "--profile":
			if i+1 >= len(args) {
				return opts, nil, errors.New("--profile requires a value")
			}
			opts.Profile = args[i+1]
			opts.ProfileSet = true
			i += 2
		case strings.HasPrefix(a, "--profile="):
			opts.Profile = strings.TrimPrefix(a, "--profile=")
			opts.ProfileSet = true
			i++
		case a == "--config":
			if i+1 >= len(args) {
				return opts, nil, errors.New("--config requires a value")
			}
			opts.ConfigPath = args[i+1]
			i += 2
		case strings.HasPrefix(a, "--config="):
			opts.ConfigPath = strings.TrimPrefix(a, "--config=")
			i++
		case a == "--json":
			opts.JSON = true
			i++
		case a == "--quiet", a == "-q":
			opts.Quiet = true
			i++
		default:
			return opts, nil, fmt.Errorf("unknown global flag %q", a)
		}
	}
	return opts, args[i:], nil
}

func resolveDB(rt runtime, flagVal string) string {
	if strings.TrimSpace(flagVal) != "" {
		return flagVal
	}
	if v := strings.TrimSpace(os.Getenv("DOOH_DB")); v != "" {
		return v
	}
	if strings.TrimSpace(rt.context.DB) != "" {
		return strings.TrimSpace(rt.context.DB)
	}
	return rt.profile.DB
}

func resolveTheme(rt runtime, flagVal string) string {
	if strings.TrimSpace(flagVal) != "" {
		return strings.TrimSpace(flagVal)
	}
	if strings.TrimSpace(rt.context.Theme) != "" {
		return strings.TrimSpace(rt.context.Theme)
	}
	return strings.TrimSpace(rt.profile.Theme)
}
