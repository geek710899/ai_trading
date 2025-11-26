package strategy

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/weex/ai_trading/bot/internal/config"
	"github.com/weex/ai_trading/bot/internal/logger"
	"github.com/weex/ai_trading/bot/internal/trader"
	"github.com/weex/ai_trading/bot/internal/weex"
)

type Engine struct {
	cfg         config.Config
	client      *weex.Client
	tr          trader.Trader
	log         *logger.Logger
	states      map[string]*symbolState
	positions   map[string][]position
	realizedPnL map[string]float64
	closedCount map[string]int
}

func NewEngine(cfg config.Config, client *weex.Client, tr trader.Trader, log *logger.Logger) *Engine {
	e := &Engine{cfg: cfg, client: client, tr: tr, log: log, states: make(map[string]*symbolState), positions: make(map[string][]position), realizedPnL: make(map[string]float64), closedCount: make(map[string]int)}
	for _, s := range cfg.Symbols {
		e.states[s] = newSymbolState(cfg.Cooldown)
	}
	return e
}

func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(e.cfg.QueryInterval)
	summaryTicker := time.NewTicker(e.cfg.MetricsInterval)
	defer ticker.Stop()
	defer summaryTicker.Stop()
	e.logStartupSnapshot(ctx)
	if e.cfg.FlattenOnStart {
		e.flattenExistingPositions(ctx)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.tick(ctx)
		case <-summaryTicker.C:
			e.printSummary()
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
	e.evaluateAndTrade(symbol, t, idx, d, fr)
	e.evaluatePnL(symbol, t)
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func (e *Engine) evaluateAndTrade(symbol string, t weex.Ticker, idx weex.IndexResp, d weex.DepthResp, fundingRate string) {
	mark := parseFloat(t.MarkPrice)
	index := parseFloat(idx.Index)
	last := parseFloat(t.Last)
	if mark == 0 || index == 0 || last == 0 {
		return
	}
	dev := (mark - index) / index
	st := e.states[symbol]
	if st == nil {
		st = newSymbolState(e.cfg.Cooldown)
		e.states[symbol] = st
	}
	st.basis.push(dev)
	m, s := st.basis.meanStd()
	z := 0.0
	if s > 0 {
		z = (dev - m) / s
	}
	zThreshold := e.cfg.ZThreshold
	if math.Abs(z) < zThreshold {
		return
	}
	// Cooldown
	if time.Since(st.lastTrigger) < st.cooldown {
		return
	}
	fr := parseFloat(fundingRate)
	if math.Abs(fr) > e.cfg.FundingAbsMax {
		return
	}
	// Slippage estimate using top of book
	var askP, bidP float64
	if len(d.Asks) > 0 {
		askP = parseFloat(d.Asks[0][0])
	}
	if len(d.Bids) > 0 {
		bidP = parseFloat(d.Bids[0][0])
	}
	spread := askP - bidP
	if spread/index > e.cfg.SpreadMaxRatio {
		return
	}
	// Position size proportional to z-score, capped
	size := 0.001 * math.Min(3, math.Abs(z))
	size = e.adjustOrderSize(symbol, size)
	var side trader.Side
	orderType := "limit"
	// Prefer carry: short when fundingRate>0, long when fundingRate<0
	if dev > 0 {
		side = trader.Sell
		if fr < 0 {
			return
		}
	} else {
		side = trader.Buy
		if fr > 0 {
			return
		}
	}
	// choose executable price near top of book
	price := last
	if side == trader.Sell {
		if askP > 0 {
			price = askP
		}
	} else {
		if bidP > 0 {
			price = bidP
		}
	}
	// notional cap to avoid oversized orders
	if price*size > e.cfg.MaxNotionalUSD {
		e.log.Info("跳过下单_名义金额上限", "币对", symbol, "方向", mapSide(side), "数量", strconv.FormatFloat(size, 'f', 6, 64), "价格", strconv.FormatFloat(price, 'f', 6, 64), "名义金额", strconv.FormatFloat(price*size, 'f', 2, 64))
		return
	}
	o := e.tr.PlaceOrder(symbol, side, orderType, price, size)
	st.lastTrigger = time.Now()
	e.positions[symbol] = append(e.positions[symbol], position{orderID: o.ID, side: side, entryPrice: last, entryTime: time.Now(), orderType: orderType, size: size})
	e.log.Info("strategy_trigger", "symbol", symbol, "action", mapSide(side), "dev", strconv.FormatFloat(dev, 'f', 6, 64), "z", strconv.FormatFloat(z, 'f', 3, 64), "size", strconv.FormatFloat(size, 'f', 6, 64), "orderId", o.ID, "type", orderType)
}

type position struct {
	orderID    string
	side       trader.Side
	entryPrice float64
	entryTime  time.Time
	orderType  string
	size       float64
}

func mapSide(s trader.Side) string {
	if s == trader.Buy {
		return "long"
	}
	return "short"
}

func (e *Engine) evaluatePnL(symbol string, t weex.Ticker) {
	last := parseFloat(t.Last)
	hold := e.cfg.HoldDuration
	ps := e.positions[symbol]
	kept := ps[:0]
	for _, p := range ps {
		if time.Since(p.entryTime) < hold {
			kept = append(kept, p)
			continue
		}
		pnl := 0.0
		if p.side == trader.Buy {
			pnl = (last - p.entryPrice)
		} else {
			pnl = (p.entryPrice - last)
		}
		feeRate := e.feeRate(symbol, p.orderType)
		fee := feeRate * p.entryPrice * p.size
		pnlNet := pnl - fee
		_ = e.tr.ClosePosition(symbol, p.side, "market", 0, p.size)
		e.log.PnL("平仓收益", "币对", symbol, "方向", mapSide(p.side), "入场价", strconv.FormatFloat(p.entryPrice, 'f', 6, 64), "平仓价", strconv.FormatFloat(last, 'f', 6, 64), "毛利润", strconv.FormatFloat(pnl, 'f', 6, 64), "手续费", strconv.FormatFloat(fee, 'f', 6, 64), "净利润", strconv.FormatFloat(pnlNet, 'f', 6, 64), "类型", p.orderType)
		e.realizedPnL[symbol] += pnlNet
		e.closedCount[symbol]++
	}
	e.positions[symbol] = kept
}

func pSize(p position) float64 { return p.size }

func (e *Engine) feeRate(symbol, orderType string) float64 {
	// try get metadata from contracts endpoint once per symbol
	if cs, err := e.client.GetContracts(context.Background(), symbol); err == nil && len(cs) > 0 {
		// rates are strings; parse
		mf := parseFloat(cs[0].MakerFeeRate)
		tf := parseFloat(cs[0].TakerFeeRate)
		if orderType == "market" {
			if tf > 0 {
				return tf
			}
		} else {
			if mf > 0 {
				return mf
			}
		}
	}
	if orderType == "market" {
		return 0.0006
	}
	return 0.0002
}

func (e *Engine) adjustOrderSize(symbol string, suggested float64) float64 {
	inc := 0.0
	if cs, err := e.client.GetContracts(context.Background(), symbol); err == nil && len(cs) > 0 {
		inc = parseFloat(cs[0].SizeIncrement)
	}
	if inc <= 0 {
		inc = 1.0
	}
	units := math.Max(1, math.Round(suggested/inc))
	size := units * inc
	min := e.cfg.MinSizeMap[symbol]
	if min > 0 && size < min {
		units = math.Ceil(min / inc)
		size = units * inc
	}
	e.log.Info("数量调整", "币对", symbol, "建议数量", strconv.FormatFloat(suggested, 'f', 6, 64), "步长", strconv.FormatFloat(inc, 'f', 6, 64), "最小数量", strconv.FormatFloat(min, 'f', 6, 64), "最终数量", strconv.FormatFloat(size, 'f', 6, 64))
	return size
}

func (e *Engine) printSummary() {
	open := 0
	for _, ps := range e.positions {
		open += len(ps)
	}
	total := 0.0
	for _, v := range e.realizedPnL {
		total += v
	}
	e.log.Metrics("汇总", "持仓数", strconv.Itoa(open), "累计净收益", strconv.FormatFloat(total, 'f', 6, 64))
	ctx := context.Background()
	if pos, err := e.client.GetPositions(ctx); err == nil && len(pos) > 0 {
		for _, p := range pos {
			e.log.Metrics("持仓明细", "币对", p.Symbol, "方向", p.Side, "仓位数量", strconv.FormatFloat(p.Size, 'f', 6, 64), "杠杆", strconv.FormatFloat(p.Leverage, 'f', 2, 64))
		}
	} else {
		for sym, ps := range e.positions {
			longSize := 0.0
			shortSize := 0.0
			for _, p := range ps {
				if p.side == trader.Buy {
					longSize += p.size
				} else {
					shortSize += p.size
				}
			}
			if longSize > 0 {
				e.log.Metrics("持仓明细", "币对", sym, "方向", "long", "仓位数量", strconv.FormatFloat(longSize, 'f', 6, 64), "杠杆", "n/a")
			}
			if shortSize > 0 {
				e.log.Metrics("持仓明细", "币对", sym, "方向", "short", "仓位数量", strconv.FormatFloat(shortSize, 'f', 6, 64), "杠杆", "n/a")
			}
		}
	}
}
func (e *Engine) flattenExistingPositions(ctx context.Context) {
	pos, err := e.client.GetPositions(ctx)
	if err != nil || len(pos) == 0 {
		return
	}
	for _, p := range pos {
		var side trader.Side
		if strings.ToUpper(p.Side) == "LONG" {
			side = trader.Buy
		} else {
			side = trader.Sell
		}
		_ = e.tr.ClosePosition(p.Symbol, side, "market", 0, p.Size)
		e.log.Trade("启动_一键平仓", "币对", p.Symbol, "方向", strings.ToLower(p.Side), "数量", strconv.FormatFloat(p.Size, 'f', 6, 64))
	}
}

func (e *Engine) logStartupSnapshot(ctx context.Context) {
	avail, eq, err := e.client.GetCollateralUSDT(ctx)
	if err == nil {
		if avail > 0 || eq > 0 {
			e.log.Metrics("metrics_start", "equity_usdt", strconv.FormatFloat(eq, 'f', 6, 64), "available_usdt", strconv.FormatFloat(avail, 'f', 6, 64))
		} else {
			e.log.Metrics("metrics_start", "equity_usdt", "unknown", "available_usdt", "unknown")
		}
	}
	if pos, err := e.client.GetPositions(ctx); err == nil {
		for _, p := range pos {
			e.log.Metrics("metrics_start_position", "symbol", p.Symbol, "side", p.Side, "size", strconv.FormatFloat(p.Size, 'f', 6, 64), "leverage", strconv.FormatFloat(p.Leverage, 'f', 2, 64))
		}
	}
}
