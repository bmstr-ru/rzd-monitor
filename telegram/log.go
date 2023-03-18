package telegram

import "log"

var chatId int64 = -881739772

func Info(msg string) {
	log.Println(msg)
	Send(chatId, "ℹ️ "+msg)
}

func Error(msg string) {
	log.Println(msg)
	Send(chatId, "❌ "+msg)
}

func Panic(err error) {
	Send(chatId, "❌ "+err.Error())
	panic(err)
}
