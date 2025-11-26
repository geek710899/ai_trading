package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BaseURL         string
	APIKey          string
	APISecret       string
	Passphrase      string
	Symbols         []string
	QueryInterval   time.Duration
	LogDir          string
	ZThreshold      float64
	FundingAbsMax   float64
	SpreadMaxRatio  float64
	Cooldown        time.Duration
	HoldDuration    time.Duration
	TraderMode      string
	MetricsInterval time.Duration
	MinSizeMap      map[string]float64
	MaxNotionalUSD  float64
	FlattenOnStart  bool
}

func Load() Config {
	baseURL := getenv("WEEX_BASE_URL", "https://api-contract.weex.com")
	apiKey := getenv("WEEX_API_KEY", "")
	apiSecret := getenv("WEEX_API_SECRET", "")
	pass := getenv("WEEX_API_PASSPHRASE", "")
	syms := []string{
		"cmt_btcusdt", "cmt_ethusdt", "cmt_solusdt", "cmt_bnbusdt",
		"cmt_xrpusdt", "cmt_adausdt", "cmt_ltcusdt", "cmt_linkusdt",
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
	qi := getenvDuration("WEEX_QUERY_INTERVAL", 1*time.Second)
	logDir := getenv("WEEX_LOG_DIR", "../log")
	z := getenvFloat("WEEX_Z_THRESHOLD", 1.2)
	frMax := getenvFloat("WEEX_FUND_RATE_MAX_ABS", 0.01)
	spMax := getenvFloat("WEEX_SPREAD_MAX_RATIO", 0.005)
	cd := getenvDuration("WEEX_COOLDOWN", 1*time.Minute)
	hd := getenvDuration("WEEX_HOLD_DURATION", 3*time.Minute)
	tm := getenv("WEEX_TRADER_MODE", "mock")
	mi := getenvDuration("WEEX_METRICS_INTERVAL", 10*time.Second)
	msm := getenvFloatMap("WEEX_MIN_SIZE_MAP")
	mnu := getenvFloat("WEEX_MAX_NOTIONAL_USD", 300)
	fos := getenv("WEEX_FLATTEN_ON_START", "false") == "true"

	return Config{
		BaseURL:         baseURL,
		APIKey:          apiKey,
		APISecret:       apiSecret,
		Passphrase:      pass,
		Symbols:         syms,
		QueryInterval:   qi,
		LogDir:          logDir,
		ZThreshold:      z,
		FundingAbsMax:   frMax,
		SpreadMaxRatio:  spMax,
		Cooldown:        cd,
		HoldDuration:    hd,
		TraderMode:      tm,
		MetricsInterval: mi,
		MinSizeMap:      msm,
		MaxNotionalUSD:  mnu,
		FlattenOnStart:  fos,
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

func getenvFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func getenvFloatMap(key string) map[string]float64 {
	out := make(map[string]float64)
	v := os.Getenv(key)
	if v == "" {
		return out
	}
	parts := strings.Split(v, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		kv := strings.SplitN(p, ":", 2)
		if len(kv) != 2 {
			continue
		}
		sym := strings.TrimSpace(kv[0])
		f, err := strconv.ParseFloat(strings.TrimSpace(kv[1]), 64)
		if err != nil {
			continue
		}
		if sym != "" && f > 0 {
			out[sym] = f
		}
	}
	return out
}
