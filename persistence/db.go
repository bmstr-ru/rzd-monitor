package persistence

import (
	"bmstr.ru/rzd-monitor/monitor"
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

func init() {
	localDb, err := sql.Open("sqlite3", "./db.sqlite")
	if err != nil {
		panic(err)
	}
	db = localDb
}

func FindMonitors(chatId int64) []monitor.Monitor {
	rows, err := db.Query("select id, chat_id, from_station_id, from_station_name, to_station_id, to_station_name, departure_day, departure_month, departure_year, train_number from monitors where chat_id = ? and scheduled = 1", chatId)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	return mapToMonitor(rows)
}

func GetScheduledMonitors() []monitor.Monitor {
	rows, err := db.Query("select id, chat_id, from_station_id, from_station_name, to_station_id, to_station_name, departure_day, departure_month, departure_year, train_number from monitors where scheduled = 1")
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	return mapToMonitor(rows)
}

func mapToMonitor(rows *sql.Rows) []monitor.Monitor {
	var monitors []monitor.Monitor
	for rows.Next() {
		monitor := monitor.Monitor{
			From:  &monitor.Station{},
			To:    &monitor.Station{},
			Train: &monitor.Train{},
			Date:  monitor.Date{},
		}

		err := rows.Scan(
			&monitor.Id,
			&monitor.ChatId,
			&monitor.From.Id,
			&monitor.From.Name,
			&monitor.To.Id,
			&monitor.To.Name,
			&monitor.Date.Day,
			&monitor.Date.Month,
			&monitor.Date.Year,
			&monitor.Train.Number,
		)
		if err != nil {
			panic(err)
		}
		monitors = append(monitors, monitor)
	}
	return monitors
}

func NewMonitor(chatId int64) *monitor.Monitor {
	var monId int64

	inTx(func(tx *sql.Tx) {
		res, err := tx.Exec("insert into monitors (chat_id) values (?)", chatId)
		if err != nil {
			panic(err)
		}
		monId, _ = res.LastInsertId()
	})
	return &monitor.Monitor{Id: monId}
}

func DeleteMonitors(chatId int64) {
	inTx(func(tx *sql.Tx) {
		_, err := tx.Exec("delete from monitors where chat_id = ?", chatId)
		if err != nil {
			panic(err)
		}
	})
}

func Deactivate(id int64) {
	inTx(func(tx *sql.Tx) {
		_, err := tx.Exec("update monitors set scheduled = 0 where id = ?", id)
		if err != nil {
			panic(err)
		}
	})
}

func StoreMonitor(mon *monitor.Monitor) {
	inTx(func(tx *sql.Tx) {
		stmt, err := tx.Prepare("update monitors set from_station_id = ?, from_station_name = ?, to_station_id = ?, to_station_name = ?, departure_day = ?, departure_month = ?, departure_year = ?, train_number = ?, scheduled = 1 where id = ?")
		if err != nil {
			panic(err)
		}
		defer stmt.Close()

		_, err = stmt.Exec(
			mon.From.Id,
			mon.From.Name,
			mon.To.Id,
			mon.To.Name,
			mon.Date.Day,
			mon.Date.Month,
			mon.Date.Year,
			mon.Train.Number,
			mon.Id,
		)
		if err != nil {
			panic(err)
		}
	})
}

func inTx(block func(tx *sql.Tx)) {
	tx, err := db.Begin()
	if err != nil {
		panic(err)
	}

	block(tx)

	err = tx.Commit()
	if err != nil {
		panic(err)
	}
}

func WrapUp() {
	db.Close()
}
