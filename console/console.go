package console

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/curtisnewbie/gocommon/common"
	"github.com/curtisnewbie/gocommon/sqlite"
	"golang.org/x/term"
)

const (
	STATUS_NONE        = 0
	STATUS_IN_PROGRESS = 1
	STATUS_FINISHED    = 2
	STATUS_CANCELLED   = 3

	PGE_CONSOLE    = 1
	PGE_LIST_TASKS = 2

	CMD_GOTO_CREATE_TASK            = 1
	CMD_GOTO_DELETE_TASK            = 2
	CMD_GOTO_UPDATE_TASK            = 3
	CMD_GOTO_PGE_LIST_TASKS         = 4
	CMD_GOTO_CONSOLE                = 5
	CMD_LIST_TASK                   = 6
	CMD_LIST_TAKS_NEXT_PAGE         = 7
	CMD_LIST_TAKS_PREV_PAGE         = 8
	CMD_CREATE_TASK                 = 9
	CMD_LIST_TAKS_FILTER_NAME       = 10
	CMD_LIST_TAKS_FILTER_STATUS     = 11
	CMD_LIST_TAKS_FILTER_CURR_WEEK  = 12
	CMD_LIST_TAKS_FILTER_CURR_MONTH = 13

	CMD_IGNORE = -1
	CMD_EXIT   = -2
)

var (
	listPage  = 1
	listLimit = 10
	filter    = ListTaskFilter{}

	onBootstrap []ConsoleLifecycleCallback = []ConsoleLifecycleCallback{}
	onShutdown  []ConsoleLifecycleCallback = []ConsoleLifecycleCallback{}

	timeFormats = []string{
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		"2006-01-02",
		"2006/01/02",
	}
)

type ConsoleLifecycleCallback func() error
type Task struct {
	Id          int
	Name        string
	Status      int
	Ctime       time.Time
	ActualStart *time.Time
	ExpectedEnd *time.Time
	ActualEnd   *time.Time
}

type ListTaskFilter struct {
	Name             string
	Status           int
	CtimeOpen        time.Time
	CtimeClose       time.Time
	ActualStartOpen  time.Time
	ActualStartClose time.Time
	ExpectedEndOpen  time.Time
	ExpectedEndClose time.Time
	ActualEndOpen    time.Time
	ActualEndClose   time.Time
}

type Command struct {
	Cmd     int
	Payload any
}

func init() {
	runOnBootstrap(func() error {
		if e := sqlite.GetSqlite().Exec(`
			CREATE TABLE IF NOT EXISTS task (
				id INTEGER PRIMARY KEY AUTOINCREMENT, 
				name VARCHAR(128) NOT NULL, 
				status TINYINT NOT NULL, 
				ctime TIMESTAMP NOT NULL,	
				actual_start TIMESTAMP,
				expected_end TIMESTAMP, 
				actual_end TIMESTAMP 
			);
		`).Error; e != nil {
			return fmt.Errorf("failed to execute schema, %v", e)
		}

		if e := sqlite.GetSqlite().Exec(`
			CREATE INDEX IF NOT EXISTS name_idx ON task (name);
		`).Error; e != nil {
			return fmt.Errorf("failed to create index, %v", e)
		}

		log.Print("Schema Executed")
		return nil
	})

	runOnShutdown(func() error {
		log.Print("Bye!")
		return nil
	})
}

func saveTask(t Task) error {
	tx := sqlite.GetSqlite().Table("task").Omit("Id").Create(&t)
	return tx.Error
}

func parseCmd(in_page int, input string) Command {
	if in_page == PGE_CONSOLE {
		switch input {
		case "0":
			return Command{Cmd: CMD_EXIT}
		case "1":
			return Command{Cmd: CMD_GOTO_PGE_LIST_TASKS}
		case "2":
			return Command{Cmd: CMD_CREATE_TASK}
		case "3":
			return Command{Cmd: CMD_GOTO_UPDATE_TASK}
		case "4":
			return Command{Cmd: CMD_GOTO_DELETE_TASK}
		}
	} else if in_page == PGE_LIST_TASKS {
		switch input {
		case "0":
			return Command{Cmd: CMD_GOTO_CONSOLE}
		case "1":
			return Command{Cmd: CMD_LIST_TAKS_PREV_PAGE}
		case "2":
			return Command{Cmd: CMD_LIST_TAKS_NEXT_PAGE}
		case "3":
			return Command{Cmd: CMD_LIST_TAKS_FILTER_NAME}
		case "4":
			return Command{Cmd: CMD_LIST_TAKS_FILTER_STATUS}
		case "5":
			return Command{Cmd: CMD_LIST_TAKS_FILTER_CURR_WEEK}
		case "6":
			return Command{Cmd: CMD_LIST_TAKS_FILTER_CURR_MONTH}
		default:
			return Command{Cmd: CMD_GOTO_PGE_LIST_TASKS}
		}
	}
	return Command{Cmd: CMD_IGNORE}
}

func joinOptions(title string, options []string) string {
	msg := title + "\n\n"
	for i, s := range options {
		msg += fmt.Sprintf(" %d. %s\n", i, s)
	}
	return msg + "\n"
}

func nextMsg(in_page int) (msg string, listenMultiple bool) {
	listenMultiple = false
	if in_page == PGE_CONSOLE {
		msg = joinOptions("What to do next?", []string{
			"Exit",
			"List Tasks",
			"Create Task",
			"Update Task",
			"Delete Task",
		})
	} else if in_page == PGE_LIST_TASKS {
		msg = joinOptions("What to do next?", []string{
			"Exit",
			"Prev Page",
			"Next Page",
			"Filter Name",
			"Filter Status",
			"Filter Current Week",
			"Filter Current Month",
		})
	}
	return
}

// TODO: try to identify whether the characters are half-width or full-wdith
// https://github.com/golang/text
// https://github.com/golang/text/blob/master/width/width.go
func strWidth(s string) int {
	return utf8.RuneCountInString(s)
}

func input(msg string, multiInput bool, required bool) string {
	if msg != "" {
		log.Print(msg)
	}

	var text string
	var err error
	firstTime := true

	for firstTime || (required && err == nil && text == "") {
		firstTime = false

		if multiInput {
			reader := bufio.NewReader(os.Stdin)
			text, err = reader.ReadString('\n')
			if err == nil {
				text = strings.TrimSpace(text)
			}
		} else {
			// switch stdin into 'raw' mode, must be switched everytime we read from it, it's not a one-time configuration
			// without restoring the mode, the terminal may behave weiredly
			var oldState *term.State
			oldState, err = term.MakeRaw(int(os.Stdin.Fd()))
			defer term.Restore(int(os.Stdin.Fd()), oldState)

			if err != nil {
				err = fmt.Errorf("failed to switch stdin to raw mode, %v", err)
			} else {
				text = ""
				b := make([]byte, 1)

				_, err = os.Stdin.Read(b)
				text = strings.TrimSpace(string(b))
			}
		}
	}

	if err != nil {
		log.Fatalf("failed to read from console, %v", err)
	}
	return strings.TrimSpace(text)
}

func translateStatus(status int) string {
	switch status {
	case STATUS_CANCELLED:
		return "Cancelled"
	case STATUS_IN_PROGRESS:
		return "In Progress"
	case STATUS_FINISHED:
		return "Finished"
	}
	return "Unknown"
}

func taskCols() []string {
	return []string{"Id", "Name", "Status", "Create Time", "Actual Start", "Expected End", "Actual End"}
}

func taskToRow(t Task) []string {
	s := []string{}

	s = append(s, strconv.Itoa(t.Id))
	s = append(s, t.Name)
	s = append(s, translateStatus(t.Status))
	s = append(s, formatTime(t.Ctime))

	actualStart := ""
	if t.ActualStart != nil {
		actualStart = formatTime(*t.ActualStart)
	}
	s = append(s, actualStart)

	expectedEnd := ""
	if t.ExpectedEnd != nil {
		expectedEnd = formatTime(*t.ExpectedEnd)
	}
	s = append(s, expectedEnd)

	actualEnd := ""
	if t.ActualEnd != nil {
		actualEnd = formatTime(*t.ActualEnd)
	}
	s = append(s, actualEnd)
	return s
}

func formatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

func printTasks(page int, limit int, tasks []Task, filter ListTaskFilter, total int64) {
	cols := taskCols()
	rows := [][]string{}

	for _, v := range tasks {
		rows = append(rows, taskToRow(v))
	}
	printRows(cols, rows)

	log.Printf("\nTotal: %d", total)
	log.Printf("Page:  %d", page)
	if filter.Name != "" {
		log.Printf("Filtered: name like '%s'", filter.Name)
	}
	if filter.Status > STATUS_NONE {
		log.Printf("Filtered: status is '%s'", translateStatus(filter.Status))
	}
	if !filter.ActualStartOpen.IsZero() {
		log.Printf("Filtered: actual start >= '%s'", formatTime(filter.ActualStartOpen))
	}
	log.Print("\n")
}

func listTasks(page int, limit int, filter ListTaskFilter) ([]Task, int64, error) {
	tasks := []Task{}
	tx := sqlite.GetSqlite().Debug().
		Table("task").
		Select("*").
		Offset((page - 1) * limit).
		Limit(limit).
		Order("id desc")

	if filter.Name != "" {
		tx = tx.Where("name like ?", "%"+filter.Name+"%")
	}
	if filter.Status > 0 {
		tx = tx.Where("status = ?", filter.Status)
	}
	if !filter.ActualStartOpen.IsZero() {
		tx = tx.Where("actual_start >= ?", filter.ActualStartOpen)
	}
	if !filter.ActualStartClose.IsZero() {
		tx = tx.Where("actual_start <= ?", filter.ActualStartClose)
	}
	if !filter.ActualEndOpen.IsZero() {
		tx = tx.Where("actual_end >= ?", filter.ActualEndOpen)
	}
	if !filter.ActualEndClose.IsZero() {
		tx = tx.Where("actual_end <= ?", filter.ActualEndClose)
	}
	if !filter.ExpectedEndOpen.IsZero() {
		tx = tx.Where("expected_end >= ?", filter.ExpectedEndOpen)
	}
	if !filter.ExpectedEndClose.IsZero() {
		tx = tx.Where("expected_end <= ?", filter.ExpectedEndClose)
	}

	tx = tx.Scan(&tasks)
	if tx.Error != nil {
		return nil, 0, fmt.Errorf("failed to list tasks, %v", tx.Error)
	}
	var total int64

	tx = sqlite.GetSqlite().Debug().
		Table("task")

	if filter.Name != "" {
		tx = tx.Where("name like ?", "%"+filter.Name+"%")
	}
	if filter.Status > 0 {
		tx = tx.Where("status = ?", filter.Status)
	}
	if !filter.ActualStartOpen.IsZero() {
		tx = tx.Where("actual_start >= ?", filter.ActualStartOpen)
	}
	if !filter.ActualStartClose.IsZero() {
		tx = tx.Where("actual_start <= ?", filter.ActualStartClose)
	}
	if !filter.ActualEndOpen.IsZero() {
		tx = tx.Where("actual_end >= ?", filter.ActualEndOpen)
	}
	if !filter.ActualEndClose.IsZero() {
		tx = tx.Where("actual_end <= ?", filter.ActualEndClose)
	}
	if !filter.ExpectedEndOpen.IsZero() {
		tx = tx.Where("expected_end >= ?", filter.ExpectedEndOpen)
	}
	if !filter.ExpectedEndClose.IsZero() {
		tx = tx.Where("expected_end <= ?", filter.ExpectedEndClose)
	}
	tx = tx.Count(&total)
	if tx.Error != nil {
		return nil, 0, fmt.Errorf("failed to list tasks, %v", tx.Error)
	}

	return tasks, total, nil
}

func sjoin(cnt int, token string) string {
	s := ""
	for i := 0; i < cnt; i++ {
		s += token
	}
	return s
}

func spaces(cnt int) string {
	return sjoin(cnt, " ")
}

func parseStatus(s string, def int) int {
	s = strings.ToUpper(s)
	switch s {
	case "IN_PROGRESS":
		return STATUS_IN_PROGRESS
	case "FINISHED":
		return STATUS_FINISHED
	case "CANCELLED":
		return STATUS_CANCELLED
	}

	return def
}

func printRows(cols []string, rows [][]string) {
	// max length among the rows
	indent := make(map[int]int, len(cols))
	for i, v := range cols {
		indent[i] = strWidth(v)
	}
	for _, r := range rows {
		for i := range cols {
			indent[i] = max(indent[i], strWidth(r[i]))
		}
	}
	colTitle := "| "
	colSep := "|-"
	for i := range cols {
		colTitle += cols[i] + spaces(indent[i]-strWidth(cols[i])+1) + " | "
		colSep += sjoin(indent[i]+1, "-") + "-|"
		if i < len(cols)-1 {
			colSep += "-"
		}
	}
	log.Print(colSep + "\n" + colTitle + "\n" + colSep)

	for _, r := range rows {
		rowCtn := "| "
		for i := range cols {
			rowCtn += r[i] + spaces(1+indent[i]-strWidth(r[i])) + " | "
		}
		log.Print(rowCtn)
	}
	log.Print(colSep)
}

func max(a int, b int) int {
	if a >= b {
		return a
	}
	return b
}

func parseTime(s string) *time.Time {
	var t time.Time
	if s == "" {
		return nil
	}

	for _, f := range timeFormats {
		pt, err := time.Parse(f, s)
		if err == nil {
			t = time.Time(pt)
			return &t
		}
	}
	t = time.Now()
	return &t
}

func taskFromInput() (bool, Task) {
	ok := true
	t := Task{}
	t.Name = input("Name:", true, true)
	t.Status = parseStatus(input("Status [IN_PROGRESS | FINISHED | CANCELLED]:", true, false), STATUS_IN_PROGRESS)
	t.Ctime = time.Now()
	t.ActualStart = parseTime(input("Actual Start:", true, false))
	t.ExpectedEnd = parseTime(input("Expected End:", true, false))
	t.ActualEnd = parseTime(input("Actual End:", true, false))
	return ok, t
}

func weekBegin() time.Time {
	now := time.Now()
	weekday := int(now.Weekday())
	if weekday < 1 { // SUNDAY
		return now.AddDate(0, 0, -6)
	}
	if weekday == 1 {
		return now
	}
	return now.AddDate(0, 0, -(weekday - 1))
}

func monthBegin() time.Time {
	now := time.Now()
	day := int(now.Day())
	if day == 1 { // first day of the month
		return now
	}
	return now.AddDate(0, 0, -day + 1)
}

func execute(in_page int, c Command) (int, error) {
	if c.Cmd == CMD_GOTO_CONSOLE {
		filter = ListTaskFilter{}
		return PGE_CONSOLE, nil
	} else if c.Cmd == CMD_EXIT {
		log.Print("Bye!")
		os.Exit(0)
	} else if c.Cmd == CMD_GOTO_PGE_LIST_TASKS {
		l, c, e := listTasks(listPage, listLimit, filter)
		if e == nil {
			printTasks(listPage, listLimit, l, filter, c)
		}
		return PGE_LIST_TASKS, e
	} else if c.Cmd == CMD_LIST_TAKS_NEXT_PAGE {
		l, c, e := listTasks(listPage+1, listLimit, filter)
		if e == nil {
			printTasks(listPage, listLimit, l, filter, c)
			if len(l) > 0 {
				listPage += 1
			}
		}
		return PGE_LIST_TASKS, e
	} else if c.Cmd == CMD_LIST_TAKS_PREV_PAGE {
		if listPage > 1 {
			listPage -= 1
		}
		l, c, e := listTasks(listPage, listLimit, filter)
		if e == nil {
			printTasks(listPage, listLimit, l, filter, c)
		}
		return PGE_LIST_TASKS, e
	} else if c.Cmd == CMD_LIST_TAKS_FILTER_NAME {
		filter.Name = input("Filtering by name", true, false)
		l, c, e := listTasks(listPage, listLimit, filter)
		if e == nil {
			printTasks(listPage, listLimit, l, filter, c)
		}
		return PGE_LIST_TASKS, e
	} else if c.Cmd == CMD_LIST_TAKS_FILTER_STATUS {
		filter.Status = parseStatus(input("Filter by status [IN_PROGRESS | FINISHED | CANCELLED]:", true, false), STATUS_NONE)
		l, c, e := listTasks(listPage, listLimit, filter)
		if e == nil {
			printTasks(listPage, listLimit, l, filter, c)
		}
		return PGE_LIST_TASKS, e
	} else if c.Cmd == CMD_LIST_TAKS_FILTER_CURR_WEEK {
		filter.ActualStartOpen = weekBegin()
		l, c, e := listTasks(listPage, listLimit, filter)
		if e == nil {
			printTasks(listPage, listLimit, l, filter, c)
		}
		return PGE_LIST_TASKS, e
	} else if c.Cmd == CMD_LIST_TAKS_FILTER_CURR_MONTH {
		filter.ActualStartOpen = monthBegin()
		l, c, e := listTasks(listPage, listLimit, filter)
		if e == nil {
			printTasks(listPage, listLimit, l, filter, c)
		}
		return PGE_LIST_TASKS, e
	} else if c.Cmd == CMD_CREATE_TASK {
		ok, t := taskFromInput()
		var e error
		if ok {
			// error is recoverable
			if ve := saveTask(t); ve != nil {
				log.Printf("Failed to saveTask, %v", e)
			}
		}
		return in_page, nil
	}

	return in_page, nil // stay at original page
}

func isExit(cmd Command) bool {
	return cmd.Cmd == CMD_EXIT
}

func LaunchConsole() error {
	if e := _invokeOnBoostrap(); e != nil {
		return e
	}

	var e error
	page := PGE_CONSOLE
	log.Printf("Peanut %s launched", common.GetPropStr("version"))

	for {
		msg, multi := nextMsg(page)
		input := input("\n"+msg, multi, true)
		cmd := parseCmd(page, input)
		if isExit(cmd) {
			break
		}
		page, e = execute(page, cmd)
		if e != nil {
			return fmt.Errorf("failed to execute command, %v", e)
		}
	}

	if e := _invokeOnShutdown(); e != nil {
		return e
	}
	return nil
}

func _invokeOnBoostrap() error {
	return _invokeCallbacks(onBootstrap)
}

func _invokeOnShutdown() error {
	return _invokeCallbacks(onShutdown)
}

func _invokeCallbacks(callbacks []ConsoleLifecycleCallback) error {
	for _, callback := range callbacks {
		if e := callback(); e != nil {
			return e
		}
	}
	return nil
}

func runOnBootstrap(f ConsoleLifecycleCallback) {
	onBootstrap = append(onBootstrap, f)
}

func runOnShutdown(f ConsoleLifecycleCallback) {
	onShutdown = append(onShutdown, f)
}
