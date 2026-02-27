package cron

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseCronExpression parses a cron expression and returns the next occurrence
// Supports standard 5-field and optional 6-field (with seconds) formats:
//   sec min hour dom mon dow
//   min hour dom mon dow (seconds default to 0)
func parseCronExpression(expr string, from time.Time) (time.Time, error) {
	fields := strings.Fields(expr)
	if len(fields) < 5 || len(fields) > 6 {
		return time.Time{}, fmt.Errorf("invalid cron expression: expected 5 or 6 fields, got %d", len(fields))
	}

	// Parse based on field count
	var seconds, minutes, hours, dom, month, dow string

	if len(fields) == 6 {
		// sec min hour dom mon dow
		seconds = fields[0]
		minutes = fields[1]
		hours = fields[2]
		dom = fields[3]
		month = fields[4]
		dow = fields[5]
	} else {
		// min hour dom mon dow (seconds default to 0)
		seconds = "0"
		minutes = fields[0]
		hours = fields[1]
		dom = fields[2]
		month = fields[3]
		dow = fields[4]
	}

	// Parse each field
	secVals, err := parseField(seconds, 0, 59)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid seconds field: %w", err)
	}

	minVals, err := parseField(minutes, 0, 59)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid minutes field: %w", err)
	}

	hourVals, err := parseField(hours, 0, 23)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid hours field: %w", err)
	}

	domVals, err := parseField(dom, 1, 31)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid day-of-month field: %w", err)
	}

	monthVals, err := parseField(month, 1, 12)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid month field: %w", err)
	}

	dowVals, err := parseField(dow, 0, 6)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid day-of-week field: %w", err)
	}

	// Find next occurrence
	return findNextTime(from, secVals, minVals, hourVals, domVals, monthVals, dowVals), nil
}

// parseField parses a cron field and returns valid values
func parseField(field string, min, max int) ([]int, error) {
	var values []int

	// Handle * (all values)
	if field == "*" {
		for i := min; i <= max; i++ {
			values = append(values, i)
		}
		return values, nil
	}

	// Handle */n (every n)
	if strings.HasPrefix(field, "*/") {
		step, err := strconv.Atoi(field[2:])
		if err != nil {
			return nil, fmt.Errorf("invalid step: %s", field[2:])
		}
		for i := min; i <= max; i += step {
			values = append(values, i)
		}
		return values, nil
	}

	// Handle n/m (from n, every m)
	if strings.Contains(field, "/") {
		parts := strings.Split(field, "/")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid range/step: %s", field)
		}
		baseVals, err := parseField(parts[0], min, max)
		if err != nil {
			return nil, err
		}
		step, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid step: %s", parts[1])
		}
		for i := 0; i < len(baseVals); i += step {
			values = append(values, baseVals[i])
		}
		return values, nil
	}

	// Handle n-m (range)
	if strings.Contains(field, "-") {
		parts := strings.Split(field, "-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid range: %s", field)
		}
		start, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid range start: %s", parts[0])
		}
		end, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid range end: %s", parts[1])
		}
		if start < min || end > max {
			return nil, fmt.Errorf("range out of bounds: %s", field)
		}
		for i := start; i <= end; i++ {
			values = append(values, i)
		}
		return values, nil
	}

	// Handle comma-separated list
	if strings.Contains(field, ",") {
		parts := strings.Split(field, ",")
		for _, part := range parts {
			val, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil {
				return nil, fmt.Errorf("invalid value: %s", part)
			}
			if val < min || val > max {
				return nil, fmt.Errorf("value out of bounds: %d", val)
			}
			values = append(values, val)
		}
		return values, nil
	}

	// Single value
	val, err := strconv.Atoi(field)
	if err != nil {
		return nil, fmt.Errorf("invalid value: %s", field)
	}
	if val < min || val > max {
		return nil, fmt.Errorf("value out of bounds: %d", val)
	}
	return []int{val}, nil
}

// findNextTime finds the next time that matches all field constraints
func findNextTime(from time.Time, secs, mins, hours, doms, months, dows []int) time.Time {
	// Start from the next second
	t := from.Add(1 * time.Second)

	// Limit iterations to prevent infinite loops
	maxIterations := 4 * 365 * 24 * 60 // ~4 years worth of minutes
	for i := 0; i < maxIterations; i++ {
		// Check month
		if !contains(months, int(t.Month())) {
			t = nextMonth(t)
			t = beginningOfMonth(t)
			continue
		}

		// Check day of month OR day of week
		domMatch := contains(doms, t.Day())
		dowMatch := contains(dows, int(t.Weekday()))

		if !domMatch && !dowMatch {
			t = t.Add(24 * time.Hour)
			t = beginningOfDay(t)
			continue
		}

		// Check hour
		if !contains(hours, t.Hour()) {
			t = nextHour(t)
			continue
		}

		// Check minute
		if !contains(mins, t.Minute()) {
			t = nextMinute(t)
			continue
		}

		// Check second
		if !contains(secs, t.Second()) {
			t = nextSecond(t)
			continue
		}

		return t
	}

	// Should never reach here with valid cron expressions
	return time.Time{}
}

// Helper functions

func contains(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

func nextMonth(t time.Time) time.Time {
	return t.AddDate(0, 1, 0)
}

func beginningOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

func beginningOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func nextHour(t time.Time) time.Time {
	if t.Hour() == 23 {
		return t.Add(24 * time.Hour)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
}

func nextMinute(t time.Time) time.Time {
	if t.Minute() == 59 {
		return nextHour(t)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute()+1, 0, 0, t.Location())
}

func nextSecond(t time.Time) time.Time {
	if t.Second() == 59 {
		return nextMinute(t)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second()+1, 0, t.Location())
}

// ParseHumanDuration parses human-readable duration strings
// Supports: "30s", "5m", "2h", "1d", "1w"
func ParseHumanDuration(s string) (time.Duration, error) {
	s = strings.ToLower(strings.TrimSpace(s))

	// Handle numeric only (assume minutes)
	if val, err := strconv.Atoi(s); err == nil {
		return time.Duration(val) * time.Minute, nil
	}

	re := regexp.MustCompile(`^(\d+)([smhdw])$`)
	matches := re.FindStringSubmatch(s)
	if len(matches) != 3 {
		return 0, fmt.Errorf("invalid duration format: %s (expected: 30s, 5m, 2h, 1d, 1w)", s)
	}

	val, _ := strconv.Atoi(matches[1])
	unit := matches[2]

	switch unit {
	case "s":
		return time.Duration(val) * time.Second, nil
	case "m":
		return time.Duration(val) * time.Minute, nil
	case "h":
		return time.Duration(val) * time.Hour, nil
	case "d":
		return time.Duration(val) * 24 * time.Hour, nil
	case "w":
		return time.Duration(val) * 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown duration unit: %s", unit)
	}
}

// FormatDuration formats a duration in human-readable format
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return d.String()
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	return fmt.Sprintf("%.1fd", d.Hours()/24)
}
