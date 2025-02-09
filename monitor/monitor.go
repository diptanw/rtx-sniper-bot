package monitor

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/diptanw/rtx-sniper-bot/async"
	"github.com/diptanw/rtx-sniper-bot/nvidia"
	"github.com/diptanw/rtx-sniper-bot/storage"
)

type (
	MonitoringRequest struct {
		Products  []string `json:"products"`
		Countries []string `json:"countries"`
	}

	Monitor struct {
		store       *storage.Storage[MonitoringRequest]
		scheduler   *async.Scheduler
		pool        async.Pool
		api         *nvidia.Client
		notCh       chan<- Notification
		activeSKUs  map[string]sku
		activeSKUmu sync.Mutex
		log         *slog.Logger
	}

	Notification struct {
		UserID  int64
		Message string
		URLs    map[string]string
	}

	sku struct {
		prod    nvidia.Product
		country nvidia.Country
		users   []int64
	}
)

func NewMonitor(log *slog.Logger, store *storage.Storage[MonitoringRequest], sch *async.Scheduler, pool async.Pool, api *nvidia.Client, notCh chan<- Notification) *Monitor {
	return &Monitor{
		store:      store,
		scheduler:  sch,
		pool:       pool,
		api:        api,
		notCh:      notCh,
		activeSKUs: make(map[string]sku),
		log:        log,
	}
}

func (m *Monitor) Start(ctx context.Context, interval time.Duration, workers int) {
	m.scheduler.Schedule(ctx, interval, func(ctx context.Context) error {
		m.updateActiveSKUs()
		for _, sku := range m.activeSKUs {
			m.pool.Enqueue(func(ctx context.Context) error {
				m.checkStock(ctx, sku)
				return nil
			})
		}
		return nil
	})

	go func() {
		m.pool.Run(ctx, workers)
	}()
}

func (m *Monitor) updateActiveSKUs() {
	m.activeSKUmu.Lock()
	defer m.activeSKUmu.Unlock()

	m.activeSKUs = make(map[string]sku)

	for userID, req := range m.store.All() {
		for _, p := range req.Products {
			sku := sku{
				prod: nvidia.Product(p),
			}

			for _, c := range req.Countries {
				sku.country = nvidia.Country(c)

				skuCode := sku.prod.SKU(sku.country)
				if skuCode == "" {
					m.log.Warn("No SKU code found.", "product", sku.prod, "country", sku.country)

					continue
				}

				uID, _ := strconv.ParseInt(userID, 10, 64)
				sku.users = append(m.activeSKUs[skuCode].users, uID)

				m.activeSKUs[skuCode] = sku
			}
		}
	}
}

func (m *Monitor) checkStock(ctx context.Context, sku sku) {
	links, err := m.api.BuyNow(ctx, sku.prod, sku.country)
	if err == nvidia.ErrNotAvailable {
		return
	}

	if err != nil {
		m.log.Error("Failed to get buy now links.", "product", sku.prod, "country", sku.country, "error", err)
		return
	}

	for _, userID := range sku.users {
		m.notCh <- Notification{
			UserID:  userID,
			Message: "Product " + sku.prod.String() + " is now available! Use /monitor to subscribe again.",
			URLs:    links,
		}

		m.Unmonitor(strconv.FormatInt(userID, 10))
	}
}

func (m *Monitor) Monitor(userID string, products []string, countries []string) {
	m.store.Add(userID, MonitoringRequest{
		Products:  products,
		Countries: countries,
	})

	m.updateActiveSKUs()
}

func (m *Monitor) Unmonitor(userID string) {
	m.store.Remove(userID)
	m.updateActiveSKUs()
}
