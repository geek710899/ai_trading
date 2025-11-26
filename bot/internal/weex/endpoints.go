package weex

import "github.com/weex/ai_trading/bot/internal/ratelimit"

type endpoint struct {
    path   string
    method string
    weight int
    domain ratelimit.Domain
}

var (
    epServerTime = endpoint{"/capi/v2/market/time", "GET", 1, ratelimit.IP}
    epTicker     = endpoint{"/capi/v2/market/ticker", "GET", 1, ratelimit.IP}
    epIndex      = endpoint{"/capi/v2/market/index", "GET", 1, ratelimit.IP}
    epDepth      = endpoint{"/capi/v2/market/depth", "GET", 1, ratelimit.IP}
    epFundRate   = endpoint{"/capi/v2/market/currentFundRate", "GET", 1, ratelimit.IP}
    epAccounts   = endpoint{"/capi/v2/account/accounts", "GET", 5, ratelimit.UID}
    epContracts  = endpoint{"/capi/v2/market/contracts", "GET", 10, ratelimit.IP}
)

type Ticker struct {
    Symbol            string  `json:"symbol"`
    Last              string  `json:"last"`
    BestAsk           string  `json:"best_ask"`
    BestBid           string  `json:"best_bid"`
    High24h           string  `json:"high_24h"`
    Low24h            string  `json:"low_24h"`
    Volume24h         string  `json:"volume_24h"`
    PriceChangePct    string  `json:"priceChangePercent"`
    BaseVolume        string  `json:"base_volume"`
    MarkPrice         string  `json:"markPrice"`
    IndexPrice        string  `json:"indexPrice"`
    Timestamp         string  `json:"timestamp"`
}

type IndexResp struct {
    Symbol    string `json:"symbol"`
    Index     string `json:"index"`
    Timestamp string `json:"timestamp"`
}

type DepthResp struct {
    Asks      [][]string `json:"asks"`
    Bids      [][]string `json:"bids"`
    Timestamp string     `json:"timestamp"`
}

type FundRate struct {
    Symbol      string `json:"symbol"`
    FundingRate string `json:"fundingRate"`
    CollectCycle int64 `json:"collectCycle"`
    Timestamp   int64  `json:"timestamp"`
}

type Contract struct {
    Symbol        string `json:"symbol"`
    TickSize      string `json:"tick_size"`
    SizeIncrement string `json:"size_increment"`
    MakerFeeRate  string `json:"makerFeeRate"`
    TakerFeeRate  string `json:"takerFeeRate"`
}
