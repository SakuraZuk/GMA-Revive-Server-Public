package config

import (
	"os"
	"strconv"
)

type Config struct {
	SDKBind              string
	LoginBind            string
	GameBind             string
	DebugBind            string
	AdminBind            string
	AdminToken           string
	HotfixBind           string
	HotfixTLSBind        string
	DNSBind              string
	DNSAnswer            string
	GameDNSAnswer        string
	DataDir              string
	TLSCert              string
	TLSKey               string
	GameDomain           string
	GameAddress          string
	HotfixAddress        string
	HotfixVersions       string
	MaxSessions          int
	MaxFramePayload      int
	DatabaseURL          string
	DatabaseMaxConns     int
	RSAKeyPath           string
	ActivityScheduleFile string
}

func Load() Config {
	return Config{
		SDKBind:              env("HS_SDK_BIND", ":8080"),
		LoginBind:            env("HS_LOGIN_BIND", ":8081"),
		GameBind:             env("HS_GAME_BIND", "0.0.0.0:9000"),
		DebugBind:            os.Getenv("HS_GAME_DEBUG_BIND"),
		AdminBind:            os.Getenv("HS_GAME_ADMIN_BIND"),
		AdminToken:           os.Getenv("HS_GAME_ADMIN_TOKEN"),
		HotfixBind:           env("HS_HOTFIX_BIND", ":8082"),
		HotfixTLSBind:        os.Getenv("HS_HOTFIX_TLS_BIND"),
		DNSBind:              os.Getenv("HS_DNS_BIND"),
		DNSAnswer:            env("HS_DNS_ANSWER", "192.0.2.20"),
		GameDNSAnswer:        env("HS_GAME_DNS_ANSWER", "192.0.2.10"),
		DataDir:              env("HS_DATA_DIR", "deploy/data"),
		TLSCert:              os.Getenv("HS_TLS_CERT"),
		TLSKey:               os.Getenv("HS_TLS_KEY"),
		GameDomain:           env("HS_GAME_DOMAIN", "r18sex.net"),
		GameAddress:          env("HS_GAME_ADDRESS", "127.0.0.1:9000"),
		HotfixAddress:        env("HS_HOTFIX_ADDRESS", "192.0.2.20:443"),
		HotfixVersions:       env("HS_HOTFIX_VERSIONS", "1.0.125"),
		MaxSessions:          envInt("HS_MAX_SESSIONS", 500),
		MaxFramePayload:      envInt("HS_MAX_FRAME_PAYLOAD", 1<<20),
		DatabaseURL:          os.Getenv("HS_DATABASE_URL"),
		DatabaseMaxConns:     envInt("HS_DATABASE_MAX_CONNS", 32),
		RSAKeyPath:           os.Getenv("HS_GAME_RSA_KEY"),
		ActivityScheduleFile: os.Getenv("HS_ACTIVITY_SCHEDULE_FILE"),
	}
}

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

func envInt(k string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(k))
	if err != nil || v < 1 {
		return fallback
	}
	return v
}
