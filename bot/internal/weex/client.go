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
	"strings"
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
	if symbol != "" {
		q.Set("symbol", symbol)
	}
	var out []Contract
	err := c.doPublic(ctx, epContracts, q, nil, &out)
	return out, err
}

type AccountsResp struct {
	Account struct {
		ContractLeverage map[string]struct {
			IsolatedLong  string `json:"isolated_long_leverage"`
			IsolatedShort string `json:"isolated_short_leverage"`
			Cross         string `json:"cross_leverage"`
			Shared        string `json:"shared_leverage"`
		} `json:"contract_id_to_leverage_setting"`
	} `json:"account"`
	Collateral []struct {
		CoinID    int    `json:"coin_id"`
		Amount    string `json:"amount"`
		Equity    string `json:"equity"`
		Available string `json:"available"`
	} `json:"collateral"`
	Position []struct {
		ContractID int    `json:"contract_id"`
		Side       string `json:"side"`
		MarginMode string `json:"margin_mode"`
		Leverage   string `json:"leverage"`
		Size       string `json:"size"`
	} `json:"position"`
}

type PositionInfo struct {
	Symbol   string
	Side     string
	Leverage float64
	Size     float64
}

func (c *Client) GetPositions(ctx context.Context) ([]PositionInfo, error) {
	var acc AccountsResp
	if err := c.doPrivate(ctx, epAccounts, url.Values{}, nil, &acc); err != nil {
		return nil, err
	}
	cs, _ := c.GetContracts(ctx, "")
	id2sym := make(map[int]string, len(cs))
	for _, ct := range cs {
		if ct.ContractID != 0 && ct.Symbol != "" {
			id2sym[ct.ContractID] = ct.Symbol
		}
	}
	out := make([]PositionInfo, 0, len(acc.Position))
	for _, p := range acc.Position {
		sym := id2sym[p.ContractID]
		if sym == "" {
			continue
		}
		var levF float64
		if p.Leverage != "" {
			levF, _ = strconv.ParseFloat(p.Leverage, 64)
		} else {
			key := strconv.Itoa(p.ContractID)
			if lv, ok := acc.Account.ContractLeverage[key]; ok {
				var lvStr string
				switch strings.ToUpper(p.MarginMode) {
				case "CROSS":
					lvStr = lv.Cross
				case "SHARED":
					lvStr = lv.Shared
				case "ISOLATED":
					if strings.ToUpper(p.Side) == "LONG" {
						lvStr = lv.IsolatedLong
					} else {
						lvStr = lv.IsolatedShort
					}
				default:
					lvStr = lv.Shared
				}
				levF, _ = strconv.ParseFloat(lvStr, 64)
			}
		}
		sz, _ := strconv.ParseFloat(p.Size, 64)
		out = append(out, PositionInfo{Symbol: sym, Side: strings.ToLower(p.Side), Leverage: levF, Size: sz})
	}
	return out, nil
}

func (c *Client) GetCollateralUSDT(ctx context.Context) (available float64, equity float64, err error) {
	var acc AccountsResp
	if err = c.doPrivate(ctx, epAccounts, url.Values{}, nil, &acc); err != nil {
		return 0, 0, err
	}
	for _, col := range acc.Collateral {
		if col.CoinID == 2 { // USDT
			// some fields may be missing depending on env; fallback to amount
			if col.Available != "" {
				available, _ = strconv.ParseFloat(col.Available, 64)
			}
			if col.Equity != "" {
				equity, _ = strconv.ParseFloat(col.Equity, 64)
			}
			if available == 0 && col.Amount != "" {
				available, _ = strconv.ParseFloat(col.Amount, 64)
			}
			if equity == 0 && col.Amount != "" {
				equity, _ = strconv.ParseFloat(col.Amount, 64)
			}
			break
		}
	}
	return
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

type PlaceOrderReq struct {
	Symbol     string `json:"symbol"`
	ClientOID  string `json:"client_oid"`
	Size       string `json:"size"`
	Type       string `json:"type"`
	OrderType  string `json:"order_type"`
	MatchPrice string `json:"match_price"`
	Price      string `json:"price,omitempty"`
}

type PlaceOrderResp struct {
	ClientOID string `json:"client_oid"`
	OrderID   string `json:"order_id"`
}

func (c *Client) PlaceOrder(ctx context.Context, req PlaceOrderReq) (PlaceOrderResp, error) {
	var out PlaceOrderResp
	err := c.doPrivate(ctx, epPlaceOrder, url.Values{}, req, &out)
	if err != nil {
		return PlaceOrderResp{}, err
	}
	return out, nil
}
