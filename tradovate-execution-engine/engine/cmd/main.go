package main

import (
	"fmt"
	"tradovate-execution-engine/engine/UI"
	_ "tradovate-execution-engine/engine/strategies"
	"tradovate-execution-engine/engine/tests"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {

	tests.RunAllTests()

	p := tea.NewProgram(UI.InitialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}

}
