package weex

import (
    "bytes"
    "context"
    "crypto/hmac"
    "crypto/sha256"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strconv"
    "time"
    "github.com/weex/ai_trading/bot/internal/config"
    "github.com/weex/ai_trading/bot/internal/logger"
    "github.com/weex/ai_trading/bot/internal/ratelimit"
)

type Client struct {
    cfg   config.Config
    log   *logger.Logger
    rl    *ratelimit.RateLimiter
    hc    *http.Client
    drift int64
}

func NewClient(cfg config.Config, log *logger.Logger, rl *ratelimit.RateLimiter) *Client {
    return &Client{
        cfg: cfg,
        log: log,
        rl:  rl,
        hc:  &http.Client{Timeout: 10 * time.Second},
    }
}

func (c *Client) serverTimestamp() string {
    now := time.Now().UnixMilli() + c.drift
    return strconv.FormatInt(now, 10)
}

func (c *Client) SyncServerTime(ctx context.Context) error {
    var resp struct {
        Timestamp int64 `json:"timestamp"`
    }
    if err := c.doPublic(ctx, epServerTime, url.Values{}, nil, &resp); err != nil {
        return err
    }
    c.drift = resp.Timestamp - time.Now().UnixMilli()
    c.log.Info("sync_time", "server_ts", strconv.FormatInt(resp.Timestamp, 10), "drift_ms", strconv.FormatInt(c.drift, 10))
    return nil
}

func (c *Client) PingPrivate(ctx context.Context) error {
    var out map[string]any
    return c.doPrivate(ctx, epAccounts, url.Values{}, nil, &out)
}

func (c *Client) GetTicker(ctx context.Context, symbol string) (Ticker, error) {
    q := url.Values{"symbol": []string{symbol}}
    var out Ticker
    err := c.doPublic(ctx, epTicker, q, nil, &out)
    return out, err
}

func (c *Client) GetIndex(ctx context.Context, symbol string) (IndexResp, error) {
    q := url.Values{"symbol": []string{symbol}}
    var out IndexResp
    err := c.doPublic(ctx, epIndex, q, nil, &out)
    return out, err
}

func (c *Client) GetDepth(ctx context.Context, symbol string, limit int) (DepthResp, error) {
    q := url.Values{"symbol": []string{symbol}}
    if limit > 0 {
        q.Set("limit", strconv.Itoa(limit))
    }
    var out DepthResp
    err := c.doPublic(ctx, epDepth, q, nil, &out)
    return out, err
}

func (c *Client) GetCurrentFundRate(ctx context.Context, symbol string) ([]FundRate, error) {
    q := url.Values{}
    if symbol != "" {
        q.Set("symbol", symbol)
    }
    var out []FundRate
    err := c.doPublic(ctx, epFundRate, q, nil, &out)
    return out, err
}

func (c *Client) GetContracts(ctx context.Context, symbol string) ([]Contract, error) {
    q := url.Values{}
    if symbol != "" { q.Set("symbol", symbol) }
    var out []Contract
    err := c.doPublic(ctx, epContracts, q, nil, &out)
    return out, err
}

func (c *Client) doPublic(ctx context.Context, ep endpoint, query url.Values, body any, out any) error {
    c.rl.Acquire(ep.domain, ep.weight)
    u := c.cfg.BaseURL + ep.path
    if len(query) > 0 {
        u += "?" + query.Encode()
    }
    var reqBody io.Reader
    if body != nil {
        b, _ := json.Marshal(body)
        reqBody = bytes.NewReader(b)
    }
    req, err := http.NewRequestWithContext(ctx, ep.method, u, reqBody)
    if err != nil {
        return err
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("locale", "zh-CN")
    resp, err := c.hc.Do(req)
    if err != nil {
        c.log.Error("http_public", "path", ep.path, "err", err.Error())
        return err
    }
    defer resp.Body.Close()
    b, _ := io.ReadAll(resp.Body)
    if resp.StatusCode != 200 {
        c.log.Error("http_public", "path", ep.path, "code", strconv.Itoa(resp.StatusCode), "body", string(b))
        return fmt.Errorf("status %d", resp.StatusCode)
    }
    if out != nil {
        if err := json.Unmarshal(b, out); err != nil {
            c.log.Error("json_public", "path", ep.path, "err", err.Error())
            return err
        }
    }
    return nil
}

func (c *Client) doPrivate(ctx context.Context, ep endpoint, query url.Values, body any, out any) error {
    c.rl.Acquire(ep.domain, ep.weight)
    requestPath := ep.path
    var queryString string
    if len(query) > 0 {
        queryString = query.Encode()
    }
    u := c.cfg.BaseURL + requestPath
    if queryString != "" {
        u += "?" + queryString
    }
    var bodyBytes []byte
    if body != nil {
        bodyBytes, _ = json.Marshal(body)
    }
    method := ep.method
    ts := c.serverTimestamp()
    signPayload := ts + method + requestPath
    if queryString != "" {
        signPayload += "?" + queryString
    }
    if len(bodyBytes) > 0 {
        signPayload += string(bodyBytes)
    }
    mac := hmac.New(sha256.New, []byte(c.cfg.APISecret))
    mac.Write([]byte(signPayload))
    signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

    var reqBody io.Reader
    if len(bodyBytes) > 0 {
        reqBody = bytes.NewReader(bodyBytes)
    }
    req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
    if err != nil {
        return err
    }
    req.Header.Set("ACCESS-KEY", c.cfg.APIKey)
    req.Header.Set("ACCESS-SIGN", signature)
    req.Header.Set("ACCESS-TIMESTAMP", ts)
    req.Header.Set("ACCESS-PASSPHRASE", c.cfg.Passphrase)
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("locale", "zh-CN")

    resp, err := c.hc.Do(req)
    if err != nil {
        c.log.Error("http_private", "path", ep.path, "err", err.Error())
        return err
    }
    defer resp.Body.Close()
    b, _ := io.ReadAll(resp.Body)
    if resp.StatusCode != 200 {
        c.log.Error("http_private", "path", ep.path, "code", strconv.Itoa(resp.StatusCode), "body", string(b))
        return fmt.Errorf("status %d", resp.StatusCode)
    }
    if out != nil {
        if err := json.Unmarshal(b, out); err != nil {
            c.log.Error("json_private", "path", ep.path, "err", err.Error())
            return err
        }
    }
    return nil
}
