// hwt is the TUI client: a live sensors panel (grouped by chip, with
// current/min/max/avg since reset) fed by the hwtd subscription stream.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zen66ten/HW-T/internal/core"
	"github.com/zen66ten/HW-T/pkg/client"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("25")).Padding(0, 1)
	deviceStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("45"))
	headerStyle = lipgloss.NewStyle().Faint(true)
	footerStyle = lipgloss.NewStyle().Faint(true)
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	critStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	errStyle    = lipgloss.NewStyle().Faint(true)
)

type sensorsMsg []client.Sensor

type streamErrMsg struct{ err error }

type model struct {
	socket  string
	sensors []client.Sensor
	updates chan []client.Sensor
	errs    chan error
	offset  int
	width   int
	height  int
	err     error
}

func main() {
	socket := flag.String("socket", client.DefaultSocket(), "hwtd unix socket")
	flag.Parse()

	m := model{
		socket:  *socket,
		updates: make(chan []client.Sensor, 1),
		errs:    make(chan error, 1),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		err := client.Subscribe(ctx, m.socket, time.Second, func(s []client.Sensor) {
			select {
			case m.updates <- s:
			default:
			}
		})
		if err != nil {
			m.errs <- err
		}
	}()

	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (m model) wait() tea.Cmd {
	return func() tea.Msg {
		select {
		case s := <-m.updates:
			return sensorsMsg(s)
		case err := <-m.errs:
			return streamErrMsg{err}
		}
	}
}

func (m model) Init() tea.Cmd { return m.wait() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sensorsMsg:
		m.sensors = msg
		return m, m.wait()
	case streamErrMsg:
		m.err = msg.err
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "r":
			socket := m.socket
			return m, func() tea.Msg {
				if c, err := client.Dial(socket); err == nil {
					defer c.Close()
					c.Reset("")
				}
				return nil
			}
		case "up", "k":
			m.offset = max(0, m.offset-1)
		case "down", "j":
			m.offset++
		case "pgup":
			m.offset = max(0, m.offset-m.pageSize())
		case "pgdown":
			m.offset += m.pageSize()
		case "home":
			m.offset = 0
		}
	}
	return m, nil
}

func (m model) pageSize() int { return max(1, m.height-4) }

func (m model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("HW-T sensors"))
	b.WriteString("\n")

	if m.err != nil {
		b.WriteString(fmt.Sprintf("\n  %v\n\n  Start the daemon first:  hwtd\n", m.err))
		b.WriteString(footerStyle.Render("\n  [q] quit"))
		return b.String()
	}
	if len(m.sensors) == 0 {
		b.WriteString("\n  connecting to hwtd...\n")
		b.WriteString(footerStyle.Render("\n  [q] quit"))
		return b.String()
	}

	b.WriteString(headerStyle.Render(fmt.Sprintf("%-26s%12s%12s%12s%12s", "Sensor", "Current", "Min", "Max", "Avg")))
	b.WriteString("\n")

	lines := m.buildLines()
	visible := m.pageSize()
	offset := min(m.offset, max(0, len(lines)-visible))
	end := min(len(lines), offset+visible)
	b.WriteString(strings.Join(lines[offset:end], "\n"))

	b.WriteString("\n")
	b.WriteString(footerStyle.Render(fmt.Sprintf("[q] quit  [r] reset stats  [↑/↓] scroll   %d sensors", len(m.sensors))))
	return b.String()
}

func (m model) buildLines() []string {
	var lines []string
	lastDev := ""
	for _, s := range m.sensors {
		if s.Device != lastDev {
			lastDev = s.Device
			path := core.ShortenPath(strings.TrimPrefix(s.Device, s.Provider+":"), 40)
			lines = append(lines, deviceStyle.Render(core.DisplayName(s.DeviceName))+headerStyle.Render("  "+path))
		}
		lines = append(lines, sensorLine(s))
	}
	return lines
}

func sensorLine(s client.Sensor) string {
	label := fmt.Sprintf("  %-24s", s.Label)
	if s.Err != "" {
		return label + errStyle.Render(fmt.Sprintf("%12s", "N/A"))
	}
	kind := core.Kind(s.Kind)
	cur := fmt.Sprintf("%12s", core.FormatValue(kind, s.Cur))
	if kind == core.KindTemp {
		if crit, ok := s.Limits["crit"]; ok && s.Cur >= crit {
			cur = critStyle.Render(cur)
		} else if high, ok := s.Limits["max"]; ok && s.Cur >= high {
			cur = warnStyle.Render(cur)
		}
	}
	return label + cur + fmt.Sprintf("%12s%12s%12s",
		core.FormatValue(kind, s.Min),
		core.FormatValue(kind, s.Max),
		core.FormatValue(kind, s.Avg))
}
