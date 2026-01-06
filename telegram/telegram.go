package telegram

import (
	"log"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var bot *tgbotapi.BotAPI
var apiKey = os.Getenv("BOT_TOKEN")

func init() {
	newBot, err := tgbotapi.NewBotAPI(apiKey)
	if err != nil {
		panic(err)
	}
	bot = newBot
	bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)
}

func Updates() tgbotapi.UpdatesChannel {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	return bot.GetUpdatesChan(u)
}

func Send(chatId int64, msg string) {
	if _, err := bot.Send(tgbotapi.NewMessage(chatId, msg)); err != nil {
		log.Println(err)
	}
}
