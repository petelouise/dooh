package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dooh/internal/auth"
	"dooh/internal/db"
)

type principal struct {
	UserID     string
	UserName   string
	KeyID      string
	KeyPrefix  string
	Actor      string
	ClientType string
	Scopes     map[string]bool
}

var errNoAuthContext = errors.New("No authenticated user context. Provide a valid key (via --api-key, stored login key, or env key).")

func mustReadAuth(rt runtime, sqlite db.SQLite, keyFromFlag string, neededScopes ...string) (principal, error) {
	return mustAuth(rt, sqlite, keyFromFlag, false, neededScopes...)
}

func mustAuth(rt runtime, sqlite db.SQLite, keyFromFlag string, requireHumanTTY bool, neededScopes ...string) (principal, error) {
	var p principal
	key := strings.TrimSpace(keyFromFlag)
	if key != "" {
		p, err := principalFromKey(sqlite, key)
		if err != nil {
			return principal{}, err
		}
		for _, need := range neededScopes {
			if !p.Scopes[need] {
				return principal{}, fmt.Errorf("missing required scope %q", need)
			}
		}
		return p, nil
	}

	mode := normalizeActor(strings.TrimSpace(os.Getenv("DOOH_MODE")))
	if mode == "agent" {
		key = firstNonEmpty(strings.TrimSpace(os.Getenv("DOOH_AI_KEY")), envKeyValue(rt.profile.APIKeyEnv))
		if key == "" {
			stored, _, err := readStoredKey(rt.opts.Profile, "agent")
			if err != nil {
				return p, err
			}
			key = stored
		}
		if key == "" {
			return p, errNoAuthContext
		}
	} else if mode == "human" {
		if key == "" {
			stored, _, err := readStoredKey(rt.opts.Profile, "human")
			if err != nil {
				return p, err
			}
			key = stored
		}
		if key == "" {
			return p, errNoAuthContext
		}
	} else {
		key = firstNonEmpty(strings.TrimSpace(os.Getenv("DOOH_AI_KEY")), envKeyValue(rt.profile.APIKeyEnv))
		if key == "" {
			if stored, _, err := readStoredKey(rt.opts.Profile, "human"); err != nil {
				return p, err
			} else if stored != "" {
				key = stored
			}
		}
		if key == "" {
			if stored, _, err := readStoredKey(rt.opts.Profile, "agent"); err != nil {
				return p, err
			} else if stored != "" {
				key = stored
			}
		}
		if key == "" {
			return p, errNoAuthContext
		}
	}
	p, err := principalFromKey(sqlite, key)
	if err != nil {
		return principal{}, err
	}
	if requireHumanTTY && p.Actor == "human" {
		if fi, err := os.Stdin.Stat(); err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
			return p, errors.New("human actor requires interactive terminal")
		}
	}
	for _, need := range neededScopes {
		if !p.Scopes[need] {
			return principal{}, fmt.Errorf("missing required scope %q", need)
		}
	}
	return p, nil
}

func principalFromKey(sqlite db.SQLite, key string) (principal, error) {
	var p principal
	hash := auth.HashAPIKey(key)
	rows, err := sqlite.QueryTSV(fmt.Sprintf("SELECT k.id,k.user_id,k.key_prefix,k.scopes,k.client_type,u.name FROM api_keys k JOIN users u ON u.id=k.user_id WHERE k.key_hash=%s AND k.revoked_at IS NULL AND u.status='active' LIMIT 1;", db.Quote(hash)))
	if err != nil {
		return p, err
	}
	if len(rows) == 0 || len(rows[0]) < 6 {
		return p, errNoAuthContext
	}
	clientType := strings.TrimSpace(rows[0][4])
	actor, ok := actorFromClientType(clientType)
	if !ok {
		return principal{}, fmt.Errorf("key client_type %s is not interactive", clientType)
	}
	p = principal{UserID: rows[0][1], UserName: rows[0][5], KeyID: rows[0][0], KeyPrefix: rows[0][2], Actor: actor, ClientType: clientType, Scopes: parseScopes(rows[0][3])}
	return p, nil
}

func resolvePrincipalForShow(rt runtime, sqlite db.SQLite) (principal, string, bool) {
	if k := strings.TrimSpace(os.Getenv("DOOH_AI_KEY")); k != "" {
		if p, err := principalFromKey(sqlite, k); err == nil {
			return p, "env:DOOH_AI_KEY", true
		}
	}
	if envName := strings.TrimSpace(rt.profile.APIKeyEnv); envName != "" {
		if k := strings.TrimSpace(os.Getenv(envName)); k != "" {
			if p, err := principalFromKey(sqlite, k); err == nil {
				return p, "env:" + envName, true
			}
		}
	}
	if k := strings.TrimSpace(os.Getenv("DOOH_API_KEY")); k != "" {
		if p, err := principalFromKey(sqlite, k); err == nil {
			return p, "env:DOOH_API_KEY", true
		}
	}
	if k, _, err := readStoredKey(rt.opts.Profile, "human"); err == nil && k != "" {
		if p, err := principalFromKey(sqlite, k); err == nil {
			return p, "stored:human", true
		}
	}
	if k, _, err := readStoredKey(rt.opts.Profile, "agent"); err == nil && k != "" {
		if p, err := principalFromKey(sqlite, k); err == nil {
			return p, "stored:ai", true
		}
	}
	return principal{}, "", false
}

func normalizeActor(v string) string {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "ai":
		return "agent"
	case "human":
		return "human"
	case "agent":
		return "agent"
	default:
		return ""
	}
}

func displayActor(v string) string {
	if normalizeActor(v) == "agent" {
		return "ai"
	}
	return "human"
}

func actorFromClientType(clientType string) (string, bool) {
	switch strings.TrimSpace(clientType) {
	case "human_cli":
		return "human", true
	case "agent_cli":
		return "agent", true
	default:
		return "", false
	}
}

func requireHumanLifecycleAdmin(p principal, allowSystemAdmin bool) error {
	if p.Actor == "human" {
		return nil
	}
	if p.ClientType == "system" && allowSystemAdmin {
		return nil
	}
	return errors.New("lifecycle admin actions require human actor (or --allow-system-admin with system key)")
}

func parseScopes(v string) map[string]bool {
	out := map[string]bool{}
	for _, s := range strings.Split(v, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			out[s] = true
		}
	}
	return out
}

func hasAIEnvKey() bool {
	return strings.TrimSpace(os.Getenv("DOOH_AI_KEY")) != "" || strings.TrimSpace(os.Getenv("DOOH_API_KEY")) != ""
}

func envKeyValue(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		n = "DOOH_API_KEY"
	}
	return strings.TrimSpace(os.Getenv(n))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func appHomeDir() (string, error) {
	if v := strings.TrimSpace(os.Getenv("DOOH_HOME")); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "dooh"), nil
}

func authStoreDir() (string, error) {
	base, err := appHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "auth"), nil
}

func keyFilePath(profile string, actor string) (string, error) {
	actor = normalizeActor(actor)
	if actor != "human" && actor != "agent" {
		return "", fmt.Errorf("invalid actor %q", actor)
	}
	p := strings.TrimSpace(profile)
	if p == "" {
		p = "default"
	}
	dir, err := authStoreDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("%s.%s.key", p, actor)), nil
}

func writeStoredKey(profile string, actor string, key string) (string, error) {
	path, err := keyFilePath(profile, actor)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(key)+"\n"), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func readStoredKey(profile string, actor string) (key string, path string, err error) {
	path, err = keyFilePath(profile, actor)
	if err != nil {
		return "", "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", path, nil
		}
		return "", path, err
	}
	return strings.TrimSpace(string(b)), path, nil
}
