package tui

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type themeTickMsg struct{}

func themeTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return themeTickMsg{} })
}

func loadOmarchyPalette() {
	setFallbackPalette()
	name, err := exec.Command("omarchy-theme-current").Output()
	if err != nil {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	path := filepath.Join(home, ".config", "omarchy", "themes", strings.ToLower(strings.TrimSpace(string(name))), "colors.toml")
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	colors := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "=", 2)
		if len(parts) == 2 {
			colors[strings.TrimSpace(parts[0])] = strings.Trim(strings.TrimSpace(parts[1]), "\"")
		}
	}
	use := func(key string, target *lipgloss.Color) {
		if value := colors[key]; strings.HasPrefix(value, "#") && len(value) == 7 {
			*target = lipgloss.Color(value)
		}
	}
	use("accent", &accent)
	use("magenta", &secondary)
	use("green", &success)
	use("yellow", &warning)
	use("foreground", &foreground)
	use("muted", &muted)
	use("lighter_background", &border)
	use("background", &canvas)
	use("dark_background", &panel)
}

func setFallbackPalette() {
	accent, secondary = "#7aa2f7", "#bb9af7"
	success, warning = "#9ece6a", "#e0af68"
	foreground, muted = "#c0caf5", "#565f89"
	border, canvas, panel = "#3b4261", "#1a1b26", "#16161e"
}
