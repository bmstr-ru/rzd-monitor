package persistence

import (
	"bmstr.ru/rzd-monitor/monitor"
	"bufio"
	"encoding/csv"
	"os"
)

var storedMonitors map[int64][]monitor.Monitor

func init() {
	storedMonitors = make(map[int64][]monitor.Monitor)
}

func FindMonitors(chatId int64) []monitor.Monitor {
	return storedMonitors[chatId]
}

func GetScheduledMonitors() []monitor.Monitor {
	monitors := []monitor.Monitor{}
	for _, mons := range storedMonitors {
		for _, mon := range mons {
			if mon.Scheduled {
				monitors = append(monitors, mon)
			}
		}
	}
	return monitors
}

func DeleteMonitors(chatId int64) {
	delete(storedMonitors, chatId)
	flush()
}

func Deactivate(id int64) {
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
	if storedMonitors[mon.ChatId] == nil {
		storedMonitors[mon.ChatId] = []monitor.Monitor{}
	}
	storedMonitors[mon.ChatId] = append(storedMonitors[mon.ChatId], *mon)
	flush()
}

func flush() {
	rows := [][]string{}
	for _, mons := range storedMonitors {
		for _, mon := range mons {
			rows = append(rows, mon.ToCsvRow())
		}
	}
	f, err := os.Create("monitors.csv")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	buf := bufio.NewWriter(f)

	w := csv.NewWriter(buf)
	w.WriteAll(rows)
	if err := w.Error(); err != nil {
		panic(err)
	}
	buf.Flush()
}
