package strategy

import (
    "context"
    "math"
    "strconv"
    "time"
    "github.com/weex/ai_trading/bot/internal/config"
    "github.com/weex/ai_trading/bot/internal/logger"
    "github.com/weex/ai_trading/bot/internal/trader"
    "github.com/weex/ai_trading/bot/internal/weex"
)

type Engine struct {
    cfg    config.Config
    client *weex.Client
    mock   *trader.Mock
    log    *logger.Logger
    states map[string]*symbolState
    positions map[string][]position
}

func NewEngine(cfg config.Config, client *weex.Client, mock *trader.Mock, log *logger.Logger) *Engine {
    e := &Engine{cfg: cfg, client: client, mock: mock, log: log, states: make(map[string]*symbolState), positions: make(map[string][]position)}
    for _, s := range cfg.Symbols { e.states[s] = newSymbolState() }
    return e
}

func (e *Engine) Run(ctx context.Context) {
    ticker := time.NewTicker(e.cfg.QueryInterval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            e.tick(ctx)
        }
    }
}

func (e *Engine) tick(ctx context.Context) {
    for _, s := range e.cfg.Symbols {
        e.processSymbol(ctx, s)
    }
}

func (e *Engine) processSymbol(ctx context.Context, symbol string) {
    t, err := e.client.GetTicker(ctx, symbol)
    if err != nil {
        e.log.Error("query_ticker", "symbol", symbol, "err", err.Error())
        return
    }
    e.log.Info("query_ticker", "symbol", symbol, "last", t.Last, "bid", t.BestBid, "ask", t.BestAsk, "mark", t.MarkPrice, "index", t.IndexPrice)

    idx, err := e.client.GetIndex(ctx, symbol)
    if err != nil {
        e.log.Error("query_index", "symbol", symbol, "err", err.Error())
        return
    }
    e.log.Info("query_index", "symbol", symbol, "index", idx.Index)

    d, err := e.client.GetDepth(ctx, symbol, 15)
    if err != nil {
        e.log.Error("query_depth", "symbol", symbol, "err", err.Error())
        return
    }
    e.log.Info("query_depth", "symbol", symbol, "asks", strconv.Itoa(len(d.Asks)), "bids", strconv.Itoa(len(d.Bids)))

    frs, err := e.client.GetCurrentFundRate(ctx, symbol)
    if err != nil {
        e.log.Error("query_fund_rate", "symbol", symbol, "err", err.Error())
        return
    }
    var fr string
    if len(frs) > 0 {
        fr = frs[0].FundingRate
        e.log.Info("query_fund_rate", "symbol", symbol, "fundingRate", fr)
    }
    e.evaluateAndMockTrade(symbol, t, idx, d, fr)
    e.evaluatePnL(symbol, t)
}

func parseFloat(s string) float64 {
    v, _ := strconv.ParseFloat(s, 64)
    return v
}

func (e *Engine) evaluateAndMockTrade(symbol string, t weex.Ticker, idx weex.IndexResp, d weex.DepthResp, fundingRate string) {
    mark := parseFloat(t.MarkPrice)
    index := parseFloat(idx.Index)
    last := parseFloat(t.Last)
    if mark == 0 || index == 0 || last == 0 {
        return
    }
    dev := (mark - index) / index
    st := e.states[symbol]
    if st == nil { st = newSymbolState(); e.states[symbol] = st }
    st.basis.push(dev)
    m, s := st.basis.meanStd()
    z := 0.0
    if s > 0 { z = (dev - m) / s }
    // Volatility-adjusted threshold
    zThreshold := 2.0
    if math.Abs(z) < zThreshold { return }
    // Cooldown
    if time.Since(st.lastTrigger) < st.cooldown { return }
    // Funding filter
    fr := parseFloat(fundingRate)
    if math.Abs(fr) > 0.002 { return }
    // Slippage estimate using top of book
    var askP, bidP float64
    if len(d.Asks) > 0 {
        askP = parseFloat(d.Asks[0][0])
    }
    if len(d.Bids) > 0 {
        bidP = parseFloat(d.Bids[0][0])
    }
    spread := askP - bidP
    if spread/index > 0.002 {
        return
    }
    // Position size proportional to z-score, capped
    size := 0.001 * math.Min(3, math.Abs(z))
    var side trader.Side
    orderType := "limit"
    // Prefer carry: short when fundingRate>0, long when fundingRate<0
    if dev > 0 {
        side = trader.Sell
        if fr < 0 { return }
    } else {
        side = trader.Buy
        if fr > 0 { return }
    }
    o := e.mock.PlaceOrder(symbol, side, orderType, last, size)
    st.lastTrigger = time.Now()
    e.positions[symbol] = append(e.positions[symbol], position{orderID: o.ID, side: side, entryPrice: last, entryTime: time.Now(), orderType: orderType})
    e.log.Info("strategy_trigger", "symbol", symbol, "action", mapSide(side), "dev", strconv.FormatFloat(dev, 'f', 6, 64), "z", strconv.FormatFloat(z, 'f', 3, 64), "size", strconv.FormatFloat(size, 'f', 6, 64), "orderId", o.ID, "type", orderType)
}

type position struct {
    orderID   string
    side      trader.Side
    entryPrice float64
    entryTime time.Time
    orderType string
}

func mapSide(s trader.Side) string { if s == trader.Buy { return "long" } ; return "short" }

func (e *Engine) evaluatePnL(symbol string, t weex.Ticker) {
    last := parseFloat(t.Last)
    hold := 10 * time.Minute
    ps := e.positions[symbol]
    kept := ps[:0]
    for _, p := range ps {
        if time.Since(p.entryTime) < hold { kept = append(kept, p); continue }
        pnl := 0.0
        if p.side == trader.Buy { pnl = (last - p.entryPrice) } else { pnl = (p.entryPrice - last) }
        // fee: maker vs taker (assume limit maker by default, conservative)
        feeRate := e.feeRate(symbol, p.orderType)
        fee := feeRate * p.entryPrice * pSize(p)
        pnlNet := pnl - fee
        e.log.Info("mock_pnl_close", "symbol", symbol, "side", mapSide(p.side), "entry", strconv.FormatFloat(p.entryPrice, 'f', 6, 64), "exit", strconv.FormatFloat(last, 'f', 6, 64), "pnl", strconv.FormatFloat(pnl, 'f', 6, 64), "fee", strconv.FormatFloat(fee, 'f', 6, 64), "pnlNet", strconv.FormatFloat(pnlNet, 'f', 6, 64), "type", p.orderType)
    }
    e.positions[symbol] = kept
}

func pSize(p position) float64 { return 1.0 } // unit notional approximation for mock fee

func (e *Engine) feeRate(symbol, orderType string) float64 {
    // try get metadata from contracts endpoint once per symbol
    if cs, err := e.client.GetContracts(context.Background(), symbol); err == nil && len(cs) > 0 {
        // rates are strings; parse
        mf := parseFloat(cs[0].MakerFeeRate)
        tf := parseFloat(cs[0].TakerFeeRate)
        if orderType == "market" {
            if tf > 0 { return tf }
        } else {
            if mf > 0 { return mf }
        }
    }
    if orderType == "market" { return 0.0006 }
    return 0.0002
}
