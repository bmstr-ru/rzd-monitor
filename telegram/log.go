package telegram

import "log"

var chatId int64 = -903806832

func Error(msg string) {
	log.Println(msg)
	Send(chatId, "❌ "+msg)
}

func Panic(err error) {
	Send(chatId, "❌ "+err.Error())
	panic(err)
}
