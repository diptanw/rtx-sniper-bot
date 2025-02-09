# RTX Sniper Bot

RTX Sniper Bot is a Go-based bot backend designed to monitor the availability of NVIDIA RTX graphics cards across various countries and notify users via Telegram when the products become available.

Bot can be found in Telegram as [@RTXSniperBot](https://t.me/RTXSniperBot)

## Features

- Monitors NVIDIA RTX graphic cards availability.
- Supports multiple countries.
- Sends notifications to users via Telegram when the products are available.
- Allows users to start and stop monitoring through Telegram commands.

## Usage

### Telegram Commands

- `/start`: Start the bot and get a welcome message.
- `/monitor`: Start monitoring product availability.
- `/unmonitor`: Stop monitoring product availability.

### Example

1. Start the bot and send the `/monitor` command.
2. Select the products and countries you want to monitor.
3. Receive notifications when the products become available.

## Docker

You can use Docker Compose to run the RTX Sniper Bot. Here is an example `docker-compose.yml` file:

```yaml
version: '3.8'
services:
  rtx-sniper-bot:
    image: docker.io/diptanw/rtx-sniper-bot:latest
    environment:
      - TELEGRAM_BOT_TOKEN=your_telegram_bot_token
    ports:
      - "8080:8080"
    restart: always
```

## Contributing

Contributions are welcome! Please open an issue or submit a pull request for any improvements or bug fixes.

## License

This project is licensed under the MIT License. See the LICENSE file for details.
