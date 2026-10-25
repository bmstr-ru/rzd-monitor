package schedule

import (
	"bmstr.ru/rzd-monitor/max"
	"context"
	"log/slog"
	"time"

	"bmstr.ru/rzd-monitor/persistence"
	"bmstr.ru/rzd-monitor/rzd"
)

// Start returns a stop function that waits for the worker before the DB closes.
func Start(parent context.Context, store *persistence.Store) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				processMonitors(ctx, store)
			}
		}
	}()
	return func() { cancel(); <-done }
}

func processMonitors(ctx context.Context, store *persistence.Store) {
	slog.Info("scheduler.tick")
	monitors, err := store.GetScheduledMonitors(ctx)
	if err != nil {
		slog.Error("scheduler.load_failed", "error", err)
		return
	}
	for _, monitor := range monitors {
		if ctx.Err() != nil {
			return
		}
		date := time.Date(monitor.Date.Year, time.Month(monitor.Date.Month), monitor.Date.Day, 0, 0, 0, 0, time.UTC)
		slog.Info("monitor.check", "monitor_id", monitor.Id, "chat_id", monitor.ChatId, "train", monitor.Train.Number, "date", date.Format("2006-01-02"))
		availability, err := rzd.FindFamilyAvailability(monitor.Train.Number, monitor.From.Id, monitor.To.Id, date)
		if err != nil {
			slog.Error("monitor.check_failed", "monitor_id", monitor.Id, "error", err)
			continue
		}
		slog.Info("monitor.result", "monitor_id", monitor.Id,
			"status", availability.Status, "ordinary_compartment_places", availability.Total,
			"lower", availability.Lower, "upper", availability.Upper,
			"empty_cabin_quantity_sum", availability.EmptyCabinQuantitySum)
		if availability.Status == rzd.FamilyAvailable {
			max.Send(monitor.ChatId, "Нашли пустое купе по вашему отслеживанию:\n"+monitor.ToString())
			slog.Info("monitor.notification", "monitor_id", monitor.Id, "chat_id", monitor.ChatId, "empty_cabin_quantity_sum", availability.EmptyCabinQuantitySum)
		}
	}
}
