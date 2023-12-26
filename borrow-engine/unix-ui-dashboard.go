package borrow_engine

import (
	"encoding/json"
	ui "github.com/gizak/termui/v3"
	"log"
	"sync"
	"time"
)
import "github.com/gizak/termui/v3/widgets"

func (ie *InferenceEngine) ui(jobsBuffer map[JobPriority][]*ComputeJob, lock *sync.RWMutex) {
	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()

	p0 := widgets.NewParagraph()
	p0.Title = "[ core ]"
	p0.Text = ""
	x2, y2 := ui.TerminalDimensions()
	p0.SetRect(0, 0, x2, 2)
	p0.Border = true

	uiEvents := ui.PollEvents()
	lastLogLines := make([]string, 0, 100)
	//rounds := 0
	selectedComputeNode := 0
	for {
		//rounds++
		topInfo := ie.buildTopString(jobsBuffer, lock, true)
		p0.Text = topInfo.topLines

		// p0.Text = topInfo.topString
		computeTable := widgets.NewTable()
		computeTable.Title = "[ AgencyOS Compute scheduler ]"
		computeTable.Rows = topInfo.computeEngines
		computeTable.TextStyle = ui.NewStyle(ui.ColorWhite)
		computeTable.RowSeparator = false
		computeTable.FillRow = true
		computeTable.RowStyles[0] = ui.NewStyle(ui.ColorWhite, ui.ColorBlack, ui.ModifierBold)
		computeTable.TextAlignment = ui.AlignCenter
		computeTable.RowStyles[selectedComputeNode] = ui.Style{
			Fg:       ui.ColorYellow,
			Bg:       ui.ColorBlue,
			Modifier: ui.ModifierUnderline,
		}

		processesTable := widgets.NewTable()
		processesTable.Title = "[ Processes ]"
		processesTable.RowSeparator = false
		processesTable.Rows = topInfo.processesLines
		processesTable.FillRow = true
		processesTable.RowStyles[0] = ui.NewStyle(ui.ColorWhite, ui.ColorBlack, ui.ModifierBold)

		logPane := widgets.NewTable()
		logPane.Title = "[ Logs ]"
		logPane.RowSeparator = false
		logPane.ColumnWidths = []int{10, x2 - 10}
		logPane.FillRow = true
		logPane.RowStyles[0] = ui.NewStyle(ui.ColorWhite, ui.ColorBlack, ui.ModifierBold)
		// size of screen allows as to show ((y2/2+% - y2) - 2) lines
		logLinesToShow := make([][]string, 0)
		// now add line sfrom lastLogLines to logLinesToShow, starting with the last one
		logLinesToShow = append(logLinesToShow, []string{"level", "message"})
		for i := len(lastLogLines) - 1; i >= 0; i-- {
			type logLine struct {
				Level   string `json:"level"`
				Message string `json:"message"`
			}
			logLineData := &logLine{}
			_ = json.Unmarshal([]byte(lastLogLines[i]), logLineData)
			logLinesToShow = append(logLinesToShow, []string{logLineData.Level, logLineData.Message})
			if len(logLinesToShow) >= ((y2/2+y2)/2)-2 {
				break
			}
		}

		logPane.Rows = logLinesToShow

		p0.SetRect(0, 0, x2, 4)
		computeEnds := 4 + len(topInfo.computeEngines) + 2
		computeTable.SetRect(0, 4, x2, computeEnds)

		processesTable.SetRect(0, computeEnds, x2, computeEnds+2+min(len(topInfo.processesLines), max(5, len(topInfo.processesLines))))
		logPane.SetRect(0, computeEnds+2+min(len(topInfo.processesLines), max(5, len(topInfo.processesLines))), x2, y2)

		ui.Render(p0, computeTable, logPane, processesTable)

		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-timer.C:
		case logLine := <-ie.settings.LogChan:
			lastLogLines = append(lastLogLines, logLine)
			if len(lastLogLines) > 100 {
				lastLogLines = lastLogLines[1:]
			}
		case e := <-uiEvents:
			if e.Type == ui.ResizeEvent {
				x2, y2 = e.Payload.(ui.Resize).Width, e.Payload.(ui.Resize).Height
			}
			switch e.ID {
			case "<Up>":
				// move cursor up
				if selectedComputeNode > 0 {
					selectedComputeNode--
				}
			case "<Down>":
				// move cursor down
				if selectedComputeNode < len(topInfo.computeEngines)-1 {
					selectedComputeNode++
				}
			case "e":
				// disable embeddings processing on current node
				if selectedComputeNode > 0 && selectedComputeNode <= len(ie.Nodes) {
					if len(ie.Nodes[selectedComputeNode-1].JobTypes) == 1 {
						ie.Nodes[selectedComputeNode-1].JobTypes = []JobType{JT_Completion, JT_Embeddings}
					} else {
						ie.Nodes[selectedComputeNode-1].JobTypes = []JobType{JT_Completion}
					}
				}
			case "q", "<C-c>":
				return
			}
		}
	}
}
