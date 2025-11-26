package main

import (
    "context"
    "strings"
    "time"
    "github.com/weex/ai_trading/bot/internal/config"
    "github.com/weex/ai_trading/bot/internal/logger"
    "github.com/weex/ai_trading/bot/internal/ratelimit"
    "github.com/weex/ai_trading/bot/internal/strategy"
    "github.com/weex/ai_trading/bot/internal/trader"
    "github.com/weex/ai_trading/bot/internal/weex"
)

func main() {
    cfg := config.Load()

    log := logger.New(cfg.LogDir)
    defer log.Close()

    rl := ratelimit.New(ratelimit.Config{
        IPCapacity: 500,
        UIDCapacity: 500,
        Window: 10 * time.Second,
    })

    client := weex.NewClient(cfg, log, rl)

    ctx := context.Background()

    if err := client.SyncServerTime(ctx); err != nil {
        log.Error("sync_time", "err", err.Error())
    }

    if err := client.PingPrivate(ctx); err != nil {
        log.Error("account_ping", "err", err.Error())
    } else {
        log.Info("account_ping", "msg", "private API reachable")
    }

    var tr trader.Trader
    if strings.ToLower(cfg.TraderMode) == "real" {
        tr = trader.NewWeex(client, log)
        log.Info("trader_mode", "mode", "real")
    } else {
        tr = trader.NewMock(log)
        log.Info("trader_mode", "mode", "mock")
    }
    eng := strategy.NewEngine(cfg, client, tr, log)
    eng.Run(ctx)
}
