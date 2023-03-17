package main

import (
	"bmstr.ru/rzd-monitor/monitor"
	"bmstr.ru/rzd-monitor/persistence"
	"bmstr.ru/rzd-monitor/rzd"
	"bmstr.ru/rzd-monitor/telegram"
	"context"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/looplab/fsm"
	"time"
)

var currentFSM *fsm.FSM
var mon *monitor.Monitor

func main() {
	defer persistence.WrapUp()

	mainContext := context.Background()
	contexts := make(map[int64]context.Context)

	for update := range telegram.Updates() {
		if update.Message == nil {
			continue
		}

		ctx := contexts[update.Message.From.ID]
		if ctx == nil {
			ctx = context.WithValue(mainContext, "UserId", update.Message.From.ID)
			contexts[update.Message.From.ID] = ctx
		}
		ctx = context.WithValue(ctx, "ChatId", update.Message.Chat.ID)

		if update.Message.IsCommand() {
			commands(ctx, update)
		} else {
			message(ctx, update)
		}
	}
}

func message(ctx context.Context, update tgbotapi.Update) {
	if currentFSM != nil {
		actOnStateMachine(ctx, update.Message.Text)
	} else {
		sendHelp(ctx)
	}
}

func sendHelp(ctx context.Context) {
	sendMessage(ctx, "Привет! Чтобы начать работу со мной запусти одну из команд:\n/newmonitor\n/monitors\n/deleteall")
}

func actOnStateMachine(ctx context.Context, msg string) {
	switch currentFSM.Current() {
	case "get-start":
		processInputStart(ctx, msg)
	case "get-finish":
		processInputFinish(ctx, msg)
	case "get-train":
		processInputTrain(ctx, msg)
	case "get-date":
		processInputDate(ctx, msg)
	case "default":
	}
}

func commands(ctx context.Context, update tgbotapi.Update) {
	switch update.Message.Command() {
	case "newmonitor":
		startNewMonitorInput(ctx)
	case "monitors":
		printMonitors(ctx)
	case "deleteall":
		deleteMonitors(ctx)
	default:
		sendHelp(ctx)
	}
}

func deleteMonitors(ctx context.Context) {
	persistence.DeleteMonitors(ctx.Value("ChatId").(int64))
	sendMessage(ctx, "Ваши отслеживания успешно удалены")
}

func printMonitors(ctx context.Context) {
	monitors := persistence.FindMonitors(ctx.Value("ChatId").(int64))
	if len(monitors) == 0 {
		sendMessage(ctx, "Вы пока не отслеживаете билеты. Чтобы начать, введите команду /newmonitor")
		return
	}
	for _, monitor := range monitors {
		sendMessage(ctx, monitor.ToString())
	}
}

func startNewMonitorInput(ctx context.Context) {
	mon = persistence.NewMonitor(ctx.Value("ChatId").(int64))
	currentFSM = fsm.NewFSM(
		"initial",
		fsm.Events{
			{Name: "prepare", Src: []string{"initial"}, Dst: "get-start"},
			{Name: "input-start", Src: []string{"get-start"}, Dst: "get-finish"},
			{Name: "input-finish", Src: []string{"get-finish"}, Dst: "get-train"},
			{Name: "input-train", Src: []string{"get-train"}, Dst: "get-date"},
			{Name: "input-date", Src: []string{"get-date"}, Dst: "end"},
		},
		fsm.Callbacks{
			"enter_get-start":  requestInputStart,
			"enter_get-finish": requestInputFinish,
			"enter_get-train":  requestInputTrain,
			"enter_get-date":   requestInputDate,
			"enter_end":        printMonitor,

			"before_input-start":  inputStart,
			"before_input-finish": inputFinish,
			"before_input-train":  inputTrain,
			"before_input-date":   inputDate,
		},
	)

	currentFSM.Event(ctx, "prepare")
}

func requestInputStart(ctx context.Context, event *fsm.Event) {
	sendMessage(ctx, "Введите станцию отправления")
}

func requestInputFinish(ctx context.Context, event *fsm.Event) {
	sendMessage(ctx, "Введите станцию прибытия")
}

func requestInputTrain(ctx context.Context, event *fsm.Event) {
	sendMessage(ctx, "Введите номер поезда")
}

func requestInputDate(ctx context.Context, event *fsm.Event) {
	sendMessage(ctx, "Введите дату отправления поезда (ГГГГ/ММ/ДД)")
}

func printMonitor(ctx context.Context, event *fsm.Event) {
	persistence.StoreMonitor(mon)
	sendMessage(ctx, "Спасибо! Ваше отслеживание:\n"+mon.ToString())
	mon = nil
}

func inputStart(ctx context.Context, event *fsm.Event) {
	mon.From = event.Args[0].(*monitor.Station)
}

func inputFinish(ctx context.Context, event *fsm.Event) {
	mon.To = event.Args[0].(*monitor.Station)
}

func inputTrain(ctx context.Context, event *fsm.Event) {
	mon.Train = event.Args[0].(*monitor.Train)
}

func inputDate(ctx context.Context, event *fsm.Event) {
	date := event.Args[0].(time.Time)
	mon.Date = monitor.Date{
		Day:   date.Day(),
		Month: int(date.Month()),
		Year:  date.Year(),
	}
}

func processInputStart(ctx context.Context, stationName string) {
	processInputStation(ctx, stationName, requestInputStart, "input-start")
}

func processInputFinish(ctx context.Context, stationName string) {
	processInputStation(ctx, stationName, requestInputFinish, "input-finish")
}

func processInputStation(ctx context.Context, stationName string, callback fsm.Callback, successEvent string) {
	stations, err := rzd.FindStation(stationName)
	if err != nil {
		sendMessage(ctx, "Ой, что-то пошло не так\nПопробуйте ввечти название станции ещё раз")
		panic(err)
	}
	if stations == nil {
		sendMessage(ctx, "Не смог найти такую станцию :(")
		callback(ctx, nil)
	} else if len(stations) > 1 {
		foundNames := ""
		for _, station := range stations {
			foundNames += station.Name + "\n"
		}
		sendMessage(ctx, "Нашёл несколько похожих станций:\n"+foundNames)
		sendMessage(ctx, "Попробуйте ввести более точное название станции")
	} else {
		currentFSM.Event(ctx, successEvent, stations[0])
	}
}

func processInputDate(ctx context.Context, inputDate string) {
	date, err := time.Parse("2006/01/02", inputDate)
	if err != nil {
		sendMessage(ctx, "Неправильный формат даты, попробуйте ещё раз")
		requestInputDate(ctx, nil)
		return
	}
	currentFSM.Event(ctx, "input-date", date)
}

func processInputTrain(ctx context.Context, trainNumber string) {
	train := monitor.Train{Number: trainNumber}
	currentFSM.Event(ctx, "input-train", &train)
}

func sendMessage(ctx context.Context, msg string) {
	telegram.Send(ctx.Value("ChatId").(int64), msg)
}
