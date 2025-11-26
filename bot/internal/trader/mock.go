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
    m.log.Info("mock_order_create", "id", id, "symbol", symbol, "side", string(side))
    // Simulate fill after delay
    go func() {
        time.Sleep(time.Second * 2)
        m.mu.Lock()
        o := m.orders[id]
        o.Status = "filled"
        m.orders[id] = o
        m.mu.Unlock()
        m.log.Info("mock_order_fill", "id", id, "symbol", symbol, "type", orderType)
    }()
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
