package trader

import (
    "context"
    "fmt"
    "math/rand"
    "strconv"
    "time"
    "github.com/weex/ai_trading/bot/internal/logger"
    "github.com/weex/ai_trading/bot/internal/weex"
)

type WeexTrader struct {
    client *weex.Client
    log    *logger.Logger
}

func NewWeex(client *weex.Client, log *logger.Logger) *WeexTrader {
    return &WeexTrader{client: client, log: log}
}

func (w *WeexTrader) PlaceOrder(symbol string, side Side, orderType string, price, size float64) Order {
    ctx := context.Background()
    req := weex.PlaceOrderReq{
        Symbol:     symbol,
        ClientOID:  w.newClientOID(),
        Size:       strconv.FormatFloat(size, 'f', 8, 64),
        OrderType:  "0",
        MatchPrice: "0",
    }
    if orderType == "market" {
        req.MatchPrice = "1"
    } else {
        req.Price = fmt.Sprintf("%.8f", price)
    }
    if side == Buy {
        req.Type = "1"
    } else {
        req.Type = "2"
    }
    resp, err := w.client.PlaceOrder(ctx, req)
    if err != nil {
        w.log.Error("真实_开仓错误", "币对", symbol, "错误", err.Error())
        return Order{ID: "", Symbol: symbol, Side: side, OrderType: orderType, Price: price, Size: size, Status: "error", CreatedAt: time.Now()}
    }
    w.log.Trade("真实_开仓委托", "币对", symbol, "委托ID", resp.OrderID, "方向", string(side), "类型", orderType)
    return Order{ID: resp.OrderID, Symbol: symbol, Side: side, OrderType: orderType, Price: price, Size: size, Status: "new", CreatedAt: time.Now()}
}

func (w *WeexTrader) ClosePosition(symbol string, side Side, orderType string, price, size float64) Order {
    ctx := context.Background()
    req := weex.PlaceOrderReq{
        Symbol:     symbol,
        ClientOID:  w.newClientOID(),
        Size:       strconv.FormatFloat(size, 'f', 8, 64),
        OrderType:  "0",
        MatchPrice: "0",
    }
    if orderType == "market" {
        req.MatchPrice = "1"
    } else {
        req.Price = fmt.Sprintf("%.8f", price)
    }
    if side == Buy {
        req.Type = "3"
    } else {
        req.Type = "4"
    }
    resp, err := w.client.PlaceOrder(ctx, req)
    if err != nil {
        w.log.Error("真实_平仓错误", "币对", symbol, "错误", err.Error())
        return Order{ID: "", Symbol: symbol, Side: side, OrderType: orderType, Price: price, Size: size, Status: "error", CreatedAt: time.Now()}
    }
    w.log.Trade("真实_平仓委托", "币对", symbol, "委托ID", resp.OrderID, "方向", string(side), "类型", orderType)
    return Order{ID: resp.OrderID, Symbol: symbol, Side: side, OrderType: orderType, Price: price, Size: size, Status: "new", CreatedAt: time.Now()}
}

func (w *WeexTrader) newClientOID() string {
    return time.Now().Format("20060102T150405") + "-" + strconv.FormatInt(rand.Int63(), 10)
}
