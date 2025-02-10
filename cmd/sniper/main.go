package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/diptanw/rtx-sniper-bot/async"
	"github.com/diptanw/rtx-sniper-bot/monitor"
	"github.com/diptanw/rtx-sniper-bot/nvidia"
	"github.com/diptanw/rtx-sniper-bot/storage"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

func main() {
	log := slog.Default()

	if os.Getenv("DEBUG") == "true" {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	telegramToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if telegramToken == "" {
		log.Error("TELEGRAM_BOT_TOKEN is not setN")
		os.Exit(1)
	}

	fileName := os.Getenv("STORAGE_FILE")
	if fileName == "" {
		fileName = "db.json"
	}

	intervalStr := os.Getenv("UPDATE_INTERVAL")
	if intervalStr == "" {
		intervalStr = "60s"
	}

	workersStr := os.Getenv("WORKERS")
	if workersStr == "" {
		workersStr = "1"
	}

	bot, err := tgbotapi.NewBotAPI(telegramToken)
	if err != nil {
		log.Error("Failed to initialize Telegram bot.", "error", err)
		os.Exit(1)
	}

	file, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0644)
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

	apiClient := nvidia.NewClient(baseURL)
	mon := monitor.New(log, store, async.NewScheduler(log), async.NewPool(), apiClient, notificationCh)

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		log.Error("Failed to parse update interval.", "error", err)
		os.Exit(1)
	}

	workers, err := strconv.Atoi(workersStr)
	if err != nil {
		log.Error("Failed to parse number of workers.", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mon.Start(ctx, interval, workers)
	log.Info("Monitoring service started", "interval", interval, "workers", workers)

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

			if _, err := bot.Send(tgbotapi.NewMessage(userID, "Monitoring stopped.")); err != nil {
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
