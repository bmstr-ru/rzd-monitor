package monitor

import (
	"strconv"
	"sync"
)

type Station struct {
	Name string
	Id   string
}

type Date struct {
	Day   int
	Month int
	Year  int
}

type Train struct {
	Number string
}

type autoInc struct {
	sync.Mutex // ensures autoInc is goroutine-safe
	id         int64
}

var monitorSequence = autoInc{id: 1}

func ID() (id int64) {
	monitorSequence.Lock()
	defer monitorSequence.Unlock()

	id = monitorSequence.id
	monitorSequence.id++
	return
}

type Monitor struct {
	Id        int64
	ChatId    int64
	From      *Station
	To        *Station
	Train     *Train
	Date      Date
	Scheduled bool
}

func (m *Monitor) ToString() string {
	return "Номер отслеживания: " + strconv.FormatInt(m.Id, 10) + "\n" +
		"От: " + m.From.Name + "\n" +
		"До: " + m.To.Name + "\n" +
		"Дата: " + formatDate(m.Date) + "\n" +
		"Поезд: " + m.Train.Number + "\n"
}

func (m *Monitor) ToCsvRow() []string {
	return []string{strconv.FormatInt(m.Id, 10),
		strconv.FormatInt(m.ChatId, 10),
		m.From.Id,
		m.From.Name,
		m.To.Id,
		m.To.Name,
		m.Train.Number,
		strconv.Itoa(m.Date.Year),
		strconv.Itoa(m.Date.Month),
		strconv.Itoa(m.Date.Day),
		strconv.FormatBool(m.Scheduled),
	}
}

func formatDate(d Date) string {
	return strconv.Itoa(d.Day) + " " + month(d.Month) + " " + strconv.Itoa(d.Year) + " г."
}

func month(month int) string {
	switch month {
	case 1:
		return "января"
	case 2:
		return "февраля"
	case 3:
		return "марта"
	case 4:
		return "апреля"
	case 5:
		return "мая"
	case 6:
		return "июня"
	case 7:
		return "июля"
	case 8:
		return "августа"
	case 9:
		return "сентября"
	case 10:
		return "октября"
	case 11:
		return "ноября"
	case 12:
		return "декабря"
	}
	return ""
}
