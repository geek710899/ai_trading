package main

import (
    "context"
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

    m := trader.NewMock(log)
    eng := strategy.NewEngine(cfg, client, m, log)
    eng.Run(ctx)
}

