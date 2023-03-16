package monitor

import (
	"strconv"
)

type Station struct {
	Name string
	Id   int64
}

type Date struct {
	Day   int
	Month int
	Year  int
}

type Train struct {
	Number string
}

type Monitor struct {
	Id    int64
	From  *Station
	To    *Station
	Train *Train
	Date  Date
}

func (m *Monitor) ToString() string {
	return "Номер отслеживания: " + strconv.FormatInt(m.Id, 10) + ":\n" +
		"От: " + m.From.Name + "\n" +
		"До: " + m.To.Name + "\n" +
		"Дата: " + formatDate(m.Date) + "\n" +
		"Поезд: " + m.Train.Number + "\n"
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
