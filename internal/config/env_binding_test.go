package config

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

// validSecret satisfies AuthConfig.validate so Load() reaches the assertion instead of
// failing on the secret in every case.
const validSecret = "0123456789abcdef0123456789abcdef0123456789abcdef"

// envVarFor mirrors the SetEnvKeyReplacer in Load.
func envVarFor(key string) string {
	return "CHQ_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
}

// fieldForKey walks a dotted config key through the Config struct using mapstructure tags,
// so the test needs no hand-written key→field table and covers new keys automatically.
func fieldForKey(t *testing.T, cfg *Config, key string) reflect.Value {
	t.Helper()

	v := reflect.ValueOf(cfg).Elem()
	for _, segment := range strings.Split(key, ".") {
		found := false
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).Tag.Get("mapstructure") == segment {
				v = v.Field(i)
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("config key %q: no struct field tagged mapstructure:%q", key, segment)
		}
	}
	return v
}

// testValueFor produces an env value appropriate to the destination field's type, along with
// the string the loaded field should render as.
func testValueFor(t *testing.T, key string, field reflect.Value) (envValue string, want string) {
	t.Helper()

	// auth.jwt_secret is validated, so it cannot take an arbitrary marker.
	if key == "auth.jwt_secret" {
		return validSecret, validSecret
	}
	// trusted_proxies is validated as IPs or CIDRs.
	if key == "server.trusted_proxies" {
		return "10.0.0.1,192.168.0.0/24", "[10.0.0.1 192.168.0.0/24]"
	}
	// database.driver is validated against the supported set.
	if key == "database.driver" {
		return "postgres", "postgres"
	}

	switch field.Interface().(type) {
	case time.Duration:
		return "13m", "13m0s"
	case string:
		return "env-marker-" + strings.ReplaceAll(key, ".", "-"), "env-marker-" + strings.ReplaceAll(key, ".", "-")
	case int:
		return "4321", "4321"
	case int64:
		return "7654321", "7654321"
	case bool:
		return "true", "true"
	case []string:
		return "alpha,beta", "[alpha beta]"
	default:
		t.Fatalf("config key %q has unhandled field type %s — extend testValueFor", key, field.Type())
		return "", ""
	}
}

// Every key in envBoundKeys must actually arrive from its CHQ_ environment variable.
//
// viper's AutomaticEnv only sees keys it already knows about, so a key with neither a default
// nor a config-file entry is silently ignored — which is exactly how CHQ_AUTH_JWT_SECRET and
// all of CHQ_GOOGLE_* were dead on env-only deployments until BindEnv was added for each.
func TestEnvBinding_EveryBoundKeyReachesConfig(t *testing.T) {
	for _, key := range envBoundKeys {
		t.Run(key, func(t *testing.T) {
			// A valid secret is a precondition for Load() returning at all.
			t.Setenv(envVarFor("auth.jwt_secret"), validSecret)

			var probe Config
			field := fieldForKey(t, &probe, key)
			envValue, want := testValueFor(t, key, field)
			t.Setenv(envVarFor(key), envValue)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			got := fmt.Sprint(fieldForKey(t, cfg, key).Interface())
			if got != want {
				t.Errorf("%s = %q via %s, want %q", key, got, envVarFor(key), want)
			}
		})
	}
}

// Checking the list is not enough on its own: it catches a deleted line but not a key added
// to setDefaults and forgotten in envBoundKeys — the direction the project has actually been
// bitten in. Every key viper knows about must be bound.
func TestEnvBinding_NoDefaultedKeyIsLeftUnbound(t *testing.T) {
	v := viper.New()
	setDefaults(v)

	bound := make(map[string]bool, len(envBoundKeys))
	for _, key := range envBoundKeys {
		bound[key] = true
	}

	for _, key := range v.AllKeys() {
		if !bound[key] {
			t.Errorf("config key %q has a default but no entry in envBoundKeys — %s would be ignored",
				key, envVarFor(key))
		}
	}
}
