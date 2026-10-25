package schedule

import (
	"log"
	"strconv"
	"time"

	"bmstr.ru/rzd-monitor/max"
	"bmstr.ru/rzd-monitor/persistence"
	"bmstr.ru/rzd-monitor/rzd"
)

func Start() {
	ticker := time.NewTicker(1 * time.Minute)
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
	log.Println("Scheduler ticked")
	monitors := persistence.GetScheduledMonitors()
	for _, monitor := range monitors {
		date := time.Date(monitor.Date.Year, time.Month(monitor.Date.Month), monitor.Date.Day, 0, 0, 0, 0, time.UTC)
		log.Println("Requesting free seats\n" + monitor.ToString())
		freeSeats, err := rzd.FindFreeSeats(monitor.Train.Number, monitor.From.Id, monitor.To.Id, date)
		if err != nil {
			max.Error(err.Error())
			continue
		}
		log.Println("Tickets: lower=" + strconv.Itoa(freeSeats.Lower) + ", upper=" + strconv.Itoa(freeSeats.Upper))
		if freeSeats.Lower > 0 || freeSeats.Upper > 0 {
			max.Send(monitor.ChatId, "Нашли билеты по вашему отслеживанию:\n"+monitor.ToString()+
				"\nНижние: "+strconv.Itoa(freeSeats.Lower)+" шт."+
				"\nВерхние: "+strconv.Itoa(freeSeats.Upper)+" шт."+
				"\nСпасибо, что воспользовались нашим сервисом!")
			//persistence.Deactivate(monitor.Id)
		}
	}
}
