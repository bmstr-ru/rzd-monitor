package schedule

import (
	"bmstr.ru/rzd-monitor/persistence"
	"bmstr.ru/rzd-monitor/rzd"
	"bmstr.ru/rzd-monitor/telegram"
	"strconv"
	"time"
)

func Start() {
	ticker := time.NewTicker(10 * time.Minute)
	go func() {
		for {
			select {
			case <-ticker.C:
				processMonitors()
			}
		}
	}()
}

func processMonitors() {
	telegram.Info("Scheduler ticked")
	monitors := persistence.GetScheduledMonitors()
	for _, monitor := range monitors {
		date := time.Date(monitor.Date.Year, time.Month(monitor.Date.Month), monitor.Date.Day, 0, 0, 0, 0, time.UTC)
		telegram.Info("Requesting free seats\n" + monitor.ToString())
		freeSeats, err := rzd.FindFreeSeats(monitor.Train.Number, monitor.From.Id, monitor.To.Id, date)
		if err != nil {
			telegram.Error(err.Error())
			continue
		}
		telegram.Info("Tickets: lower=" + strconv.Itoa(freeSeats.Lower) + ", upper=" + strconv.Itoa(freeSeats.Upper))
		if freeSeats.Lower > 0 {
			telegram.Send(monitor.ChatId, "Нашли билеты по вашему отслеживанию:\n"+monitor.ToString())
			telegram.Send(monitor.ChatId, "Нижние: "+strconv.Itoa(freeSeats.Lower)+" шт.")
			telegram.Send(monitor.ChatId, "Верхние: "+strconv.Itoa(freeSeats.Upper)+" шт.")
			telegram.Send(monitor.ChatId, "Спасибо, что воспользовались нашим сервисом!")
			persistence.Deactivate(monitor.Id)
		}
	}
}
