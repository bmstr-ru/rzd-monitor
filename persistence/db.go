package persistence

import (
	"bmstr.ru/rzd-monitor/max"
	"bmstr.ru/rzd-monitor/monitor"
	"bufio"
	"encoding/csv"
	"log"
	"os"
	"strconv"
	"sync"
)

var storageMu sync.Mutex

var storedMonitors map[int64][]*monitor.Monitor

func init() {
	storedMonitors = make(map[int64][]*monitor.Monitor)
}

func FindMonitors(chatId int64) []*monitor.Monitor {
	storageMu.Lock()
	defer storageMu.Unlock()
	log.Println("Find monitors: chatId=" + strconv.FormatInt(chatId, 10))
	result := make([]*monitor.Monitor, 0, len(storedMonitors[chatId]))
	for _, mon := range storedMonitors[chatId] {
		copy := *mon
		result = append(result, &copy)
	}
	return result
}

func GetScheduledMonitors() []*monitor.Monitor {
	storageMu.Lock()
	defer storageMu.Unlock()
	log.Println("Get scheduled monitors")
	monitors := []*monitor.Monitor{}
	for _, mons := range storedMonitors {
		for _, mon := range mons {
			if mon.Scheduled {
				copy := *mon
				monitors = append(monitors, &copy)
			}
		}
	}
	log.Println(strconv.Itoa(len(monitors)) + " active monitors")
	return monitors
}

func DeleteMonitors(chatId int64) {
	storageMu.Lock()
	defer storageMu.Unlock()
	log.Println("Delete monitors: chatId=" + strconv.FormatInt(chatId, 10))
	delete(storedMonitors, chatId)
	flush()
}

func Deactivate(id int64) {
	storageMu.Lock()
	defer storageMu.Unlock()
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
	storageMu.Lock()
	defer storageMu.Unlock()
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
	f, err := os.Create("max-monitors.csv")
	if err != nil {
		max.Error(err.Error())
		return
	}
	defer f.Close()
	buf := bufio.NewWriter(f)

	w := csv.NewWriter(buf)
	err = w.WriteAll(rows)
	if err != nil {
		max.Error(err.Error())
	}
	if err := w.Error(); err != nil {
		max.Error(err.Error())
	}
	err = buf.Flush()
	if err != nil {
		max.Error(err.Error())
	}
}
