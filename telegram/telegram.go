package telegram

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
)

var bot *tgbotapi.BotAPI
var prod = "6259981758:AAHBrzw35fqa7QkInwgrLb5V6yCUObYBq2c"
var test = "5879092419:AAEv78NzNoBxLrE6OeLhr9KKNy5EisePzwE"

var apiKey = test

func init() {
	newBot, err := tgbotapi.NewBotAPI(prod)
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
