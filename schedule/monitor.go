package schedule

import (
	"bmstr.ru/rzd-monitor/persistence"
	"bmstr.ru/rzd-monitor/rzd"
	"bmstr.ru/rzd-monitor/telegram"
	"log"
	"strconv"
	"time"
)

func init() {
	ticker := time.NewTicker(5 * time.Minute)
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
	monitors := persistence.GetScheduledMonitors()
	for _, monitor := range monitors {
		date := time.Date(monitor.Date.Day, time.Month(monitor.Date.Month), monitor.Date.Day, 0, 0, 0, 0, time.UTC)
		freeSeats, err := rzd.FindFreeSeats(monitor.Train.Number, monitor.From.Id, monitor.To.Id, date)
		if err != nil {
			log.Println(err)
			continue
		}
		if freeSeats.Lower > 0 || freeSeats.Upper > 0 {
			telegram.Send(monitor.ChatId, "Нашли билеты по вашему отслеживанию:\n"+monitor.ToString())
			telegram.Send(monitor.ChatId, "Нижние: "+strconv.Itoa(freeSeats.Lower)+" шт.")
			telegram.Send(monitor.ChatId, "Верхние: "+strconv.Itoa(freeSeats.Upper)+" шт.")
			telegram.Send(monitor.ChatId, "Спасибо, что воспользовались нашим сервисом!")
			persistence.Deactivate(monitor.Id)
		}
	}
}
