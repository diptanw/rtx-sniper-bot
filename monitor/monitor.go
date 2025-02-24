package monitor

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/dyptan-io/rtx-sniper-bot/async"
	"github.com/dyptan-io/rtx-sniper-bot/nvidia"
	"github.com/dyptan-io/rtx-sniper-bot/storage"
)

var ErrNotAvailable = errors.New("product not available")

type (
	Request struct {
		Products  []string `json:"products"`
		Countries []string `json:"countries"`
	}

	Monitor struct {
		store       *storage.Storage[Request]
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

func New(log *slog.Logger, store *storage.Storage[Request], sch *async.Scheduler, pool async.Pool, api *nvidia.Client, notCh chan<- Notification) *Monitor {
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

		numSKUs := len(m.activeSKUs)
		if numSKUs == 0 {
			return nil
		}

		delay := interval / time.Duration(numSKUs)

		var count int

		for _, s := range m.activeSKUs {
			count++

			go func(ctx context.Context, s sku, delay time.Duration) {
				select {
				case <-ctx.Done():
					return
				case <-time.After(delay):
					m.pool.Enqueue(func(ctx context.Context) error {
						err := m.checkStock(ctx, s)
						if errors.Is(err, ErrNotAvailable) {
							m.log.Debug("Product not available.", "product", s.prod, "country", s.country)
							return nil
						}

						if err != nil {
							m.log.Error("Failed to get buy now links.", "product", s.prod, "country", s.country, "error", err)
						}

						return nil
					})
				}
			}(ctx, s, delay*time.Duration(count))
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

func (m *Monitor) checkStock(ctx context.Context, sku sku) error {
	stocks, err := m.api.BuyNow(ctx, sku.prod, sku.country)
	if err != nil {
		return err
	}

	const (
		nvidiaID      = "111"
		nvidiaStoreID = "9595"
	)

	links := make(map[string]string)

	for _, s := range stocks {
		// Needs to be other than Nvidia partner and store.
		if s.PartnerID != nvidiaID && s.StoreID != nvidiaStoreID {
			purchaiseLink := s.DirectPurchaseLink

			if purchaiseLink == "" {
				purchaiseLink = s.PurchaseLink
			}

			links[s.RetailerName] = s.PurchaseLink
		}
	}

	if len(links) == 0 {
		return ErrNotAvailable
	}

	for _, userID := range sku.users {
		m.notCh <- Notification{
			UserID:  userID,
			Message: "Product " + sku.prod.String() + " is now available! Unsubscribed, use /monitor to subscribe again.",
			URLs:    links,
		}

		m.Unmonitor(strconv.FormatInt(userID, 10))
	}

	return nil
}

func (m *Monitor) Monitor(userID string, products []string, countries []string) {
	if err := m.store.Add(userID, Request{
		Products:  products,
		Countries: countries,
	}); err != nil {
		m.log.Error("Failed to add user to store.", "error", err)
		return
	}

	m.updateActiveSKUs()
}

func (m *Monitor) Unmonitor(userID string) {
	if err := m.store.Remove(userID); err != nil {
		m.log.Error("Failed to remove user from store.", "error", err)
		return
	}

	m.updateActiveSKUs()
}
