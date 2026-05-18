package billingcycle

import (
	"strconv"
	"strings"
	"time"

	"frps-status-app.local/status/src/model"
)

// ResolveStartDay 返回有效计费周期起始日（1-31）。
// traffic_cycle_start_day 为 0 时使用 deploy_date 的日；否则回退为 1 号。
func ResolveStartDay(settings model.PublicSettings) int {
	if settings.TrafficCycleStartDay >= 1 && settings.TrafficCycleStartDay <= 31 {
		return settings.TrafficCycleStartDay
	}
	deployDay := strings.TrimSpace(settings.DeployDate)
	if len(deployDay) >= 10 {
		if day, err := strconv.Atoi(deployDay[8:10]); err == nil && day >= 1 && day <= 31 {
			return day
		}
	}
	return 1
}

func clampDay(year int, month time.Month, day int, loc *time.Location) time.Time {
	if day < 1 {
		day = 1
	}
	first := time.Date(year, month, 1, 0, 0, 0, 0, loc)
	lastDay := first.AddDate(0, 1, -1).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(year, month, day, 0, 0, 0, 0, loc)
}

// CurrentRange 返回当前计费周期的起止日期（均为当天 0 点，含首尾）。
func CurrentRange(now time.Time, startDay int) (time.Time, time.Time) {
	if startDay < 1 || startDay > 31 {
		startDay = 1
	}
	loc := now.Location()
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, loc)

	if startDay == 1 {
		return time.Date(y, m, 1, 0, 0, 0, 0, loc), today
	}

	thisStart := clampDay(y, m, startDay, loc)
	if !today.Before(thisStart) {
		return thisStart, today
	}
	prevY, prevM := y, m-1
	if prevM < 1 {
		prevM = 12
		prevY--
	}
	return clampDay(prevY, prevM, startDay, loc), today
}

func FormatDay(t time.Time) string {
	return t.Format("2006-01-02")
}

func EnrichSettings(out *model.PublicSettings, now time.Time) {
	startDay := ResolveStartDay(*out)
	from, to := CurrentRange(now, startDay)
	out.TrafficCycleEffectiveStartDay = startDay
	out.TrafficCycleFrom = FormatDay(from)
	out.TrafficCycleTo = FormatDay(to)
}
