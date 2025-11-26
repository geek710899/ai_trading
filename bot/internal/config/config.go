package config

import (
    "os"
    "strings"
    "time"
)

type Config struct {
    BaseURL        string
    APIKey         string
    APISecret      string
    Passphrase     string
    Symbols        []string
    QueryInterval  time.Duration
    LogDir         string
}

func Load() Config {
    baseURL := getenv("WEEX_BASE_URL", "https://api-contract.weex.com")
    apiKey := getenv("WEEX_API_KEY", "")
    apiSecret := getenv("WEEX_API_SECRET", "")
    pass := getenv("WEEX_API_PASSPHRASE", "")
    syms := []string{
        "cmt_btcusdt", "cmt_ethusdt", "cmt_solusdt", "cmt_dogeusdt",
        "cmt_xrpusdt", "cmt_adausdt", "cmt_bnbusdt", "cmt_suiusdt",
        "cmt_ltcusdt", "cmt_linkusdt", "cmt_trxusdt", "cmt_hbarusdt",
        "cmt_tonusdt", "cmt_shibusdt", "cmt_uniusdt", "cmt_enausdt",
    }
    if v := os.Getenv("WEEX_SYMBOLS"); v != "" {
        parts := strings.Split(v, ",")
        syms = make([]string, 0, len(parts))
        for _, p := range parts {
            p = strings.TrimSpace(p)
            if p != "" {
                syms = append(syms, p)
            }
        }
    }
    qi := getenvDuration("WEEX_QUERY_INTERVAL", 5*time.Second)
    logDir := getenv("WEEX_LOG_DIR", "../log")

    return Config{
        BaseURL:       baseURL,
        APIKey:        apiKey,
        APISecret:     apiSecret,
        Passphrase:    pass,
        Symbols:       syms,
        QueryInterval: qi,
        LogDir:        logDir,
    }
}

func getenv(key, def string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return def
}

func getenvDuration(key string, def time.Duration) time.Duration {
    if v := os.Getenv(key); v != "" {
        if d, err := time.ParseDuration(v); err == nil {
            return d
        }
    }
    return def
}

