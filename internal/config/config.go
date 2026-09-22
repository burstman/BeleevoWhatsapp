package config

import (
	"crypto/sha256"
	"encoding/hex"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Env  string
	Name string

	// HTTPAddr is the address the web service binds to.
	// On Render this must come from the PORT environment variable.
	HTTPAddr string
	AppURL   string

	DatabaseURL string
	RedisURL    string

	SuperkitSecret string

	// Converty OAuth integration (Phase 2).
	ConvertyClientID     string
	ConvertyClientSecret string
	ConvertyBaseURL      string
	ConvertyAPIURL       string
	ConvertyRedirectURI  string

	// ConvertyEncryptionKey encrypts Converty tokens at rest (AES-256-GCM).
	ConvertyEncryptionKey string

	// MetaGraphURL is the base of the Facebook Graph API. Overridable so
	// tests and mirrors can point elsewhere.
	MetaGraphURL string

	// MetaSystemUserToken is the platform's long-lived Meta system-user
	// access token. Under the BSP model the platform holds it centrally and
	// provisions connections for every shop "through mine" — clients never
	// paste their own token.
	MetaSystemUserToken string
}

// Load reads configuration from the environment.
// A local ".env" file is loaded first when present; real environment
// variables always take precedence.
func Load() Config {
	_ = godotenv.Load()

	port := getenv("PORT", "3000")

	return Config{
		Env:            getenv("APP_ENV", "development"),
		Name:           getenv("APP_NAME", "Converty WhatsApp"),
		HTTPAddr:       getenv("HTTP_LISTEN_ADDR", ":"+port),
		AppURL:         getenv("APP_URL", "http://localhost:"+port),
		DatabaseURL:    getenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/converty_whatsapp?sslmode=disable"),
		RedisURL:       getenv("REDIS_URL", "redis://localhost:6379/0"),
		SuperkitSecret: getenv("SUPERKIT_SECRET", "dev-only-change-me-please-32-bytes"),

		ConvertyClientID:      getenv("CONVERTY_CLIENT_ID", ""),
		ConvertyClientSecret:  getenv("CONVERTY_CLIENT_SECRET", ""),
		ConvertyBaseURL:       getenv("CONVERTY_BASE_URL", "https://partner.converty.shop"),
		ConvertyAPIURL:        getenv("CONVERTY_API_URL", "https://api.converty.shop"),
		ConvertyRedirectURI:   getenv("CONVERTY_REDIRECT_URI", "http://localhost:"+port+"/auth/converty/callback"),
		ConvertyEncryptionKey: getenv("CONVERTY_ENCRYPTION_KEY", ""),
		MetaGraphURL:          getenv("META_GRAPH_URL", "https://graph.facebook.com"),
		MetaSystemUserToken:   getenv("META_SYSTEM_USER_TOKEN", ""),
	}
}

// EncryptionKeyResolved returns the encryption key, falling back to a
// deterministic derivation of SUPERKIT_SECRET in development when
// CONVERTY_ENCRYPTION_KEY is not set. A 64-character hex value (e.g. the
// output of `openssl rand -hex 32`) is decoded to its 32 bytes.
func (c Config) EncryptionKeyResolved() []byte {
	if key := c.ConvertyEncryptionKey; key != "" {
		if decoded, err := hex.DecodeString(key); err == nil && len(decoded) == 32 {
			return decoded
		}
		return []byte(key)
	}
	sum := sha256.Sum256([]byte(c.SuperkitSecret))
	return sum[:]
}

func getenv(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func (c Config) IsDevelopment() bool {
	return c.Env == "development"
}

func (c Config) IsProduction() bool {
	return c.Env == "production"
}
