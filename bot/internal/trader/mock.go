package trader

import (
    "math/rand"
    "strconv"
    "sync"
    "time"
    "github.com/weex/ai_trading/bot/internal/logger"
)

type Side string

const (
    Buy  Side = "buy"
    Sell Side = "sell"
)

type Trader interface {
    PlaceOrder(symbol string, side Side, orderType string, price, size float64) Order
    ClosePosition(symbol string, side Side, orderType string, price, size float64) Order
}

type Order struct {
    ID        string
    Symbol    string
    Side      Side
    OrderType string
    Price     float64
    Size      float64
    Status    string
    CreatedAt time.Time
}

type Mock struct {
    mu     sync.Mutex
    orders map[string]Order
    log    *logger.Logger
}

func NewMock(log *logger.Logger) *Mock {
    return &Mock{orders: make(map[string]Order), log: log}
}

func (m *Mock) PlaceOrder(symbol string, side Side, orderType string, price, size float64) Order {
    m.mu.Lock()
    defer m.mu.Unlock()
    id := m.newID()
    o := Order{
        ID:        id,
        Symbol:    symbol,
        Side:      side,
        OrderType: orderType,
        Price:     price,
        Size:      size,
        Status:    "new",
        CreatedAt: time.Now(),
    }
    m.orders[id] = o
    m.log.Trade("模拟_开仓委托", "委托ID", id, "币对", symbol, "方向", string(side))
    go func() {
        time.Sleep(time.Second * 2)
        m.mu.Lock()
        o := m.orders[id]
        o.Status = "filled"
        m.orders[id] = o
        m.mu.Unlock()
        m.log.Trade("模拟_委托成交", "委托ID", id, "币对", symbol, "类型", orderType)
    }()
    return o
}

func (m *Mock) ClosePosition(symbol string, side Side, orderType string, price, size float64) Order {
    var closeSide Side
    if side == Buy { closeSide = Sell } else { closeSide = Buy }
    id := m.newID()
    o := Order{ID: id, Symbol: symbol, Side: closeSide, OrderType: orderType, Price: price, Size: size, Status: "filled", CreatedAt: time.Now()}
    m.log.Trade("模拟_平仓委托", "委托ID", id, "币对", symbol, "方向", string(closeSide))
    return o
}

func (m *Mock) GetOrder(id string) (Order, bool) {
    m.mu.Lock()
    defer m.mu.Unlock()
    o, ok := m.orders[id]
    return o, ok
}

func (m *Mock) newID() string {
    return time.Now().Format("20060102T150405") + "-" + strconv.FormatInt(rand.Int63(), 10)
}
