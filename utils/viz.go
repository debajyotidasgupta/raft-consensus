package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
)

// Entry is a single log entry emitted by a raft server.
type Entry struct {
	timestamp string
	id        string
	msg       string
}

// TestLog is a whole log for a single test, containing many entries.
type TestLog struct {
	name    string
	status  string
	entries []Entry

	// ids is a set of all IDs seen emitting entries in this test.
	ids map[string]bool
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func format(msg string) string {
	// break the message in multiple lines to display only 30 characters per line
	newMsg := ""
	numChars := 30

	for i := 0; i < len(msg); i += numChars {
		newMsg += msg[i:min(i+numChars, len(msg))] + "\n"
	}

	return newMsg
}

func TableViz(tl TestLog) {
	t := table.NewWriter()
	nservers := len(tl.ids)

	t.SetOutputMirror(os.Stdout)

	headers := table.Row{"Time"}
	for i := 0; i < nservers; i++ {
		headers = append(headers, strconv.Itoa(i))
	}
	t.AppendHeader(headers)

	for _, entry := range tl.entries {
		row := table.Row{entry.timestamp}
		idInt, err := strconv.Atoi(entry.id)
		if err != nil {
			log.Fatal(err)
		}

		for i := 0; i < nservers; i++ {
			if i == idInt {
				row = append(row, format(entry.msg))
			} else {
				row = append(row, "")
			}
		}
		t.AppendSeparator()
		t.AppendRow(row)
	}
	t.SetStyle(table.StyleLight)
	// t.SetStyle(table.StyleColoredBlueWhiteOnBlack)
	t.Render()
}

func parseTestLogs(rd io.Reader) []TestLog {
	var testlogs []TestLog

	statusRE := regexp.MustCompile(`--- (\w+):\s+(\w+)`)
	entryRE := regexp.MustCompile(`([0-9:.]+) \[(\d+)\] (.*)`)

	scanner := bufio.NewScanner(bufio.NewReader(rd))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "=== RUN") {
			testlogs = append(testlogs, TestLog{ids: make(map[string]bool)})
			testlogs[len(testlogs)-1].name = strings.TrimSpace(line[7:])
		} else {
			if len(testlogs) == 0 {
				continue
			}
			curlog := &testlogs[len(testlogs)-1]

			statusMatch := statusRE.FindStringSubmatch(line)
			if len(statusMatch) > 0 {
				if statusMatch[2] != curlog.name {
					log.Fatalf("name on line %q mismatch with test name: got %s", line, curlog.name)
				}
				curlog.status = statusMatch[1]
				continue
			}

			entryMatch := entryRE.FindStringSubmatch(line)
			if len(entryMatch) > 0 {
				entry := Entry{
					timestamp: entryMatch[1],
					id:        entryMatch[2],
					msg:       entryMatch[3],
				}
				curlog.entries = append(curlog.entries, entry)
				curlog.ids[entry.id] = true
				continue
			}
		}
	}
	return testlogs
}

func main() {
	testlogs := parseTestLogs(os.Stdin)
	tnames := make(map[string]int)

	/**
	 * We deduplicate the repeated test case names, so that in  case  the
	 * test case name is repeated, we can generate a unique table for it.
	 */

	for i, tl := range testlogs {
		if count, ok := tnames[tl.name]; ok {
			testlogs[i].name = fmt.Sprintf("%s_%d", tl.name, count)
		}
		tnames[tl.name] += 1
	}

	statusSummary := "PASS"

	for _, tl := range testlogs {
		fmt.Println(tl.status, tl.name, tl.ids, "; entries:", len(tl.entries))
		if tl.status != "PASS" {
			statusSummary = tl.status
		}
		TableViz(tl)
		fmt.Println("")
	}

	fmt.Println(statusSummary)
}
