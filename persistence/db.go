package persistence

import (
	"bmstr.ru/rzd-monitor/monitor"
	"bmstr.ru/rzd-monitor/telegram"
	"bufio"
	"encoding/csv"
	"log"
	"os"
	"strconv"
)

var storedMonitors map[int64][]*monitor.Monitor

func init() {
	storedMonitors = make(map[int64][]*monitor.Monitor)
}

func FindMonitors(chatId int64) []*monitor.Monitor {
	log.Println("Find monitors: chatId=" + strconv.FormatInt(chatId, 10))
	return storedMonitors[chatId]
}

func GetScheduledMonitors() []*monitor.Monitor {
	log.Println("Get scheduled monitors")
	monitors := []*monitor.Monitor{}
	for _, mons := range storedMonitors {
		for _, mon := range mons {
			if mon.Scheduled {
				monitors = append(monitors, mon)
			}
		}
	}
	log.Println(strconv.Itoa(len(monitors)) + " active monitors")
	return monitors
}

func DeleteMonitors(chatId int64) {
	log.Println("Delete monitors: chatId=" + strconv.FormatInt(chatId, 10))
	delete(storedMonitors, chatId)
	flush()
}

func Deactivate(id int64) {
	log.Println("Deactivate monitor: id=" + strconv.FormatInt(id, 10))
	for _, mons := range storedMonitors {
		for _, mon := range mons {
			if mon.Id == id {
				mon.Scheduled = false
			}
		}
	}
	flush()
}

func StoreMonitor(mon *monitor.Monitor) {
	log.Println("Store monitor:\n" + mon.ToString())
	mon.Scheduled = true
	if storedMonitors[mon.ChatId] == nil {
		storedMonitors[mon.ChatId] = []*monitor.Monitor{}
	}
	storedMonitors[mon.ChatId] = append(storedMonitors[mon.ChatId], mon)
	flush()
}

func flush() {
	log.Println("Flush monitors")

	rows := [][]string{}
	for _, mons := range storedMonitors {
		for _, mon := range mons {
			rows = append(rows, mon.ToCsvRow())
		}
	}
	f, err := os.Create("monitors.csv")
	if err != nil {
		telegram.Error(err.Error())
	}
	defer f.Close()
	buf := bufio.NewWriter(f)

	w := csv.NewWriter(buf)
	err = w.WriteAll(rows)
	if err != nil {
		telegram.Error(err.Error())
	}
	if err := w.Error(); err != nil {
		telegram.Error(err.Error())
	}
	err = buf.Flush()
	if err != nil {
		telegram.Error(err.Error())
	}
}
