package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/diptanw/rtx-sniper-bot/async"
	"github.com/diptanw/rtx-sniper-bot/monitor"
	"github.com/diptanw/rtx-sniper-bot/nvidia"
	"github.com/diptanw/rtx-sniper-bot/proxy"
	"github.com/diptanw/rtx-sniper-bot/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type config struct {
	TelegramToken  string
	StorageFile    string
	UpdateInterval time.Duration
	Workers        int
	ProxyServers   []string
}

func main() {
	log := slog.Default()

	if os.Getenv("DEBUG") == "true" {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Error("Failed to load configuration.", "error", err)
		os.Exit(1)
	}

	httpClient := http.DefaultClient

	if len(cfg.ProxyServers) > 0 {
		proxyTransport, err := proxy.NewRotatingTransport(cfg.ProxyServers)
		if err != nil {
			log.Error("Failed to initialize proxy transport.", "error", err)
			os.Exit(1)
		}

		httpClient = &http.Client{
			Transport: proxyTransport,
		}
	}

	bot, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		log.Error("Failed to initialize Telegram bot.", "error", err)
		os.Exit(1)
	}

	file, err := os.OpenFile(cfg.StorageFile, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Error("Failed to open storage file.", "error", err)
		os.Exit(1)
	}

	defer file.Close()

	store, err := storage.Load[monitor.Request](file)
	if err != nil {
		log.Error("Failed to initialize storage.", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	baseURL, err := url.Parse("https://api.nvidia.partners")
	if err != nil {
		log.Error("Failed to parse base URL.", "error", err)
		os.Exit(1)
	}

	notificationCh := make(chan monitor.Notification)
	defer close(notificationCh)

	apiClient := nvidia.NewClient(baseURL, nvidia.WithHTTPClient(httpClient))
	mon := monitor.New(log, store, async.NewScheduler(log), async.NewPool(), apiClient, notificationCh)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mon.Start(ctx, cfg.UpdateInterval, cfg.Workers)
	log.Info("Monitoring service started", "interval", cfg.UpdateInterval, "workers", cfg.Workers)

	updatesCh, err := bot.GetUpdatesChan(tgbotapi.NewUpdate(0))
	if err != nil {
		log.Error("Failed to get updates.", "error", err)
		os.Exit(1)
	}

	var (
		availableProducts = []string{
			string(nvidia.ProductRTX5080),
			string(nvidia.ProductRTX5090),
		}
		availableCountries = []string{
			string(nvidia.CountrySweden),
			string(nvidia.CountryDenmark),
			string(nvidia.CountryFinland),
			string(nvidia.CountryGermany),
			string(nvidia.CountryNetherlands),
		}
		userSelections = make(map[int64]struct {
			Products  []string
			Countries []string
		})
	)

	go func() {
		for notif := range notificationCh {
			buttons := make([]tgbotapi.InlineKeyboardButton, 0, len(notif.URLs))

			for name, u := range notif.URLs {
				buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonURL(name, u))
			}

			msg := tgbotapi.NewMessage(notif.UserID, notif.Message)
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(buttons...),
			)

			if _, err := bot.Send(msg); err != nil {
				log.Error("Failed to send message.", "error", err)
			}
		}
	}()

	for update := range updatesCh {
		if update.Message == nil {
			continue
		}

		userID := update.Message.Chat.ID
		text := update.Message.Text

		if text == "/start" {
			if _, err := bot.Send(tgbotapi.NewMessage(userID, "Welcome! Use /monitor to track product availability.")); err != nil {
				log.Error("Failed to send message.", "error", err)
			}

			continue
		}

		if text == "/monitor" {
			msg := tgbotapi.NewMessage(userID, "Select products:")
			msg.ReplyMarkup = selection(availableProducts, userSelections[userID].Products, "Confirm Products")

			if _, err := bot.Send(msg); err != nil {
				log.Error("Failed to send message.", "error", err)
			}

			userSelections[userID] = struct {
				Products  []string
				Countries []string
			}{}

			continue
		}

		if text == "Confirm Products" && len(userSelections[userID].Products) > 0 {
			msg := tgbotapi.NewMessage(userID, "Select countries:")
			msg.ReplyMarkup = selection(availableCountries, nil, "Confirm Countries")

			if _, err := bot.Send(msg); err != nil {
				log.Error("Failed to send message.", "error", err)
			}

			continue
		}

		if text == "Confirm Countries" && len(userSelections[userID].Countries) > 0 {
			sel := userSelections[userID]
			mon.Monitor(fmt.Sprintf("%d", userID), sel.Products, sel.Countries)

			msg := tgbotapi.NewMessage(userID, fmt.Sprintf("Monitoring started for %s in %s. /unmonitor to stop",
				strings.Join(sel.Products, ", "),
				strings.Join(sel.Countries, ", "),
			))
			msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)

			if _, err := bot.Send(msg); err != nil {
				log.Error("Failed to send message.", "error", err)
			}

			log.Info("New monitor added", "userID", userID, "products", sel.Products, "countries", sel.Countries)
			delete(userSelections, userID)

			continue
		}

		if slices.Contains(availableProducts, text) {
			sel := userSelections[userID]

			// Avoid duplicate selection
			if !slices.Contains(sel.Products, text) {
				sel.Products = append(sel.Products, text)
				userSelections[userID] = sel
			}

			// Send updated menu with remaining options
			msg := tgbotapi.NewMessage(userID, fmt.Sprintf("Selected product: %s", text))
			msg.ReplyMarkup = selection(availableProducts, sel.Products, "Confirm Products")

			if _, err := bot.Send(msg); err != nil {
				log.Error("Failed to send message.", "error", err)
			}

			continue
		}

		if slices.Contains(availableCountries, text) {
			sel := userSelections[userID]

			// Avoid duplicate selection
			if !slices.Contains(sel.Countries, text) {
				sel.Countries = append(sel.Countries, text)
				userSelections[userID] = sel
			}

			// Send updated menu with remaining options
			msg := tgbotapi.NewMessage(userID, fmt.Sprintf("Selected country: %s", text))
			msg.ReplyMarkup = selection(availableCountries, sel.Countries, "Confirm Countries")

			if _, err := bot.Send(msg); err != nil {
				log.Error("Failed to send message.", "error", err)
			}

			continue
		}

		if text == "/unmonitor" {
			mon.Unmonitor(fmt.Sprintf("%d", userID))

			if _, err := bot.Send(tgbotapi.NewMessage(userID, "Monitoring stopped. Use /monitor to start again.")); err != nil {
				log.Error("Failed to send message.", "error", err)
			}

			log.Info("Monitor removed", "userID", userID)
			continue
		}

		if _, err := bot.Send(tgbotapi.NewMessage(userID, "Unknown command. Use /monitor or /unmonitor.")); err != nil {
			log.Error("Failed to send message.", "error", err)
		}
	}
}

func selection(opts []string, selected []string, confirmText string) tgbotapi.ReplyKeyboardMarkup {
	var (
		rows         [][]tgbotapi.KeyboardButton
		filteredOpts []string
	)

	for _, opt := range opts {
		if !slices.Contains(selected, opt) {
			filteredOpts = append(filteredOpts, opt)
		}
	}

	for _, option := range filteredOpts {
		rows = append(rows, tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(option)))
	}

	if len(selected) > 0 {
		rows = append(rows, tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButton(confirmText)))
	}

	return tgbotapi.NewReplyKeyboard(rows...)
}

func loadConfig() (*config, error) {
	telegramToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if telegramToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is not set")
	}

	storageFile := os.Getenv("STORAGE_FILE")
	if storageFile == "" {
		storageFile = "db.json"
	}

	intervalStr := os.Getenv("UPDATE_INTERVAL")
	if intervalStr == "" {
		intervalStr = "60s"
	}

	updateInterval, err := time.ParseDuration(intervalStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse UPDATE_INTERVAL: %w", err)
	}

	workersStr := os.Getenv("WORKERS")
	if workersStr == "" {
		workersStr = "1"
	}

	workers, err := strconv.Atoi(workersStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse WORKERS: %w", err)
	}

	proxyServers := strings.Split(os.Getenv("PROXY_SERVERS"), ",")

	return &config{
		TelegramToken:  telegramToken,
		StorageFile:    storageFile,
		UpdateInterval: updateInterval,
		Workers:        workers,
		ProxyServers:   proxyServers,
	}, nil
}
