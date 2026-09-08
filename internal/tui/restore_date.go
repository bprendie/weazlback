package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"time"
)

func parseRestoreDate(value string, now time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02"} {
		if date, err := time.ParseInLocation(layout, value, now.Location()); err == nil {
			return date, nil
		}
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch strings.ToLower(value) {
	case "today":
		return today, nil
	case "yesterday":
		return today.AddDate(0, 0, -1), nil
	}
	for day := time.Sunday; day <= time.Saturday; day++ {
		if strings.EqualFold(value, day.String()) {
			return today.AddDate(0, 0, -(int(today.Weekday())-int(day)+7)%7), nil
		}
	}
	return time.Time{}, fmt.Errorf("use a weekday, today, yesterday, or YYYY-MM-DD [HH:MM]")
}

func (m Model) restoreDateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.restoreStage = "points"
		return m, nil
	}
	if msg.String() == "enter" {
		requested, err := parseRestoreDate(m.restoreInput.Value(), time.Now())
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		best := time.Duration(1<<63 - 1)
		for i, index := range m.pointChoices {
			distance := m.snapshots[index].Time.Sub(requested)
			if distance < 0 {
				distance = -distance
			}
			if distance < best {
				best = distance
				m.pointIndex = i
			}
		}
		m.err = ""
		m.restoreStage = "points"
		m.status = "Requested " + requested.Format("2006-01-02 15:04") + "; nearest available point highlighted. Enter approves that exact point."
		return m, nil
	}
	var cmd tea.Cmd
	m.restoreInput, cmd = m.restoreInput.Update(msg)
	return m, cmd
}
