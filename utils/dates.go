// utils/dates.go
package utils

import "time"

func BeginningOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

func DaysBetween(start, end time.Time) int {
	start = BeginningOfDay(start)
	end = BeginningOfDay(end)
	return int(end.Sub(start).Hours() / 24)
}