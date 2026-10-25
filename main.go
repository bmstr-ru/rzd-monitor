package main

import (
	"bmstr.ru/rzd-monitor/max"
	"bmstr.ru/rzd-monitor/monitor"
	"bmstr.ru/rzd-monitor/persistence"
	"bmstr.ru/rzd-monitor/rzd"
	"context"
	"github.com/looplab/fsm"
	"strings"
	"time"
)

type dialogKey struct{ chatID, userID int64 }
type dialog struct {
	machine *fsm.FSM
	mon     *monitor.Monitor
}
type dialogContextKey struct{}

var dialogs = make(map[dialogKey]*dialog)

func session(ctx context.Context) *dialog { return ctx.Value(dialogContextKey{}).(*dialog) }

func handleUpdate(update max.Update) {
	chatID, userID, text, ok := update.Input()
	if !ok {
		return
	}
	key := dialogKey{chatID, userID}
	d := dialogs[key]
	if d == nil {
		d = &dialog{}
		dialogs[key] = d
	}
	ctx := context.WithValue(context.Background(), "ChatId", chatID)
	ctx = context.WithValue(ctx, dialogContextKey{}, d)
	if strings.HasPrefix(text, "/") {
		commands(ctx, strings.TrimPrefix(strings.Fields(text)[0], "/"))
	} else if d.machine != nil {
		actOnStateMachine(ctx, text)
	} else {
		sendHelp(ctx)
	}
}

func sendHelp(ctx context.Context) {
	sendMessage(ctx, "Привет! Чтобы начать работу со мной запусти одну из команд:\n/newmonitor\n/monitors\n/deleteall\n/cancel")
}

func actOnStateMachine(ctx context.Context, msg string) {
	switch session(ctx).machine.Current() {
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

func commands(ctx context.Context, command string) {
	switch command {
	case "newmonitor":
		startNewMonitorInput(ctx)
	case "monitors":
		printMonitors(ctx)
	case "deleteall":
		deleteMonitors(ctx)
	case "cancel":
		session(ctx).machine = nil
		session(ctx).mon = nil
		sendMessage(ctx, "Создание отслеживания отменено")
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
	session(ctx).mon = &monitor.Monitor{Id: monitor.ID(), ChatId: ctx.Value("ChatId").(int64)}
	session(ctx).machine = fsm.NewFSM(
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

	session(ctx).machine.Event(ctx, "prepare")
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
	persistence.StoreMonitor(session(ctx).mon)
	sendMessage(ctx, "Спасибо! Ваше отслеживание:\n"+session(ctx).mon.ToString())
	session(ctx).mon = nil
	session(ctx).machine = nil
}

func inputStart(ctx context.Context, event *fsm.Event) {
	session(ctx).mon.From = event.Args[0].(*monitor.Station)
}

func inputFinish(ctx context.Context, event *fsm.Event) {
	session(ctx).mon.To = event.Args[0].(*monitor.Station)
}

func inputTrain(ctx context.Context, event *fsm.Event) {
	session(ctx).mon.Train = event.Args[0].(*monitor.Train)
}

func inputDate(ctx context.Context, event *fsm.Event) {
	date := event.Args[0].(time.Time)
	session(ctx).mon.Date = monitor.Date{
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
		sendMessage(ctx, "Ой, что-то пошло не так\nПопробуйте ввести название станции ещё раз")
		max.Error(err.Error())
		return
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
		session(ctx).machine.Event(ctx, successEvent, stations[0])
	}
}

func processInputDate(ctx context.Context, inputDate string) {
	date, err := time.Parse("2006/01/02", inputDate)
	if err != nil {
		sendMessage(ctx, "Неправильный формат даты, попробуйте ещё раз")
		requestInputDate(ctx, nil)
		max.Error(err.Error())
		return
	}
	session(ctx).machine.Event(ctx, "input-date", date)
}

func processInputTrain(ctx context.Context, trainNumber string) {
	train := monitor.Train{Number: trainNumber}
	session(ctx).machine.Event(ctx, "input-train", &train)
}

func sendMessage(ctx context.Context, msg string) {
	max.Send(ctx.Value("ChatId").(int64), msg)
}
