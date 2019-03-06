package nagiosfoundation

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

func getPidNameWithHandler(readFile func(string) ([]byte, error), pid int) (string, error) {
	procFile := fmt.Sprintf("/proc/%d/stat", pid)
	procDataBytes, err := readFile(procFile)
	if err != nil {
		return "", err
	}

	procData := string(procDataBytes)

	procNameStart := strings.IndexRune(procData, '(') + 1
	procNameEnd := strings.IndexRune(procData, ')')

	if procNameStart >= procNameEnd {
		return "", errors.New("Could not parse process name")
	}

	procName := procData[procNameStart:procNameEnd]

	return procName, nil
}

func getPidName(pid int) (string, error) {
	return getPidNameWithHandler(ioutil.ReadFile, pid)
}

type processByNameHandlers struct {
	open       func(string) (*os.File, error)
	close      func(*os.File) error
	readDir    func(*os.File, int) ([]os.FileInfo, error)
	getPidName func(readFile func(string) ([]byte, error), pid int) (string, error)
	readFile   func(string) ([]byte, error)
}

func getProcessesByNameWithHandlers(svc processByNameHandlers, name string) ([]os.FileInfo, error) {
	var errorReturn error
	matchingEntries := make([]os.FileInfo, 0)

	dir, err := svc.open("/proc")
	if err != nil {
		matchingEntries = nil
		errorReturn = err
	}

	defer svc.close(dir)

	var procEntries []os.FileInfo
	if errorReturn == nil {
		procEntries, err = svc.readDir(dir, 0)

		if err != nil {
			matchingEntries = nil
			errorReturn = err
		}
	}

	if errorReturn == nil {
		for _, procEntry := range procEntries {
			// Skip entries that aren't directories
			if !procEntry.IsDir() {
				continue
			}

			// Skip entries that aren't numbers
			pid, err := strconv.Atoi(procEntry.Name())
			if err != nil {
				continue
			}

			if procName, _ := svc.getPidName(svc.readFile, pid); procName == name {
				matchingEntries = append(matchingEntries, procEntry)
			}
		}
	}

	return matchingEntries, errorReturn
}

func getProcessesByName(name string) ([]os.FileInfo, error) {
	svc := processByNameHandlers{
		open: os.Open,
		close: func(f *os.File) error {
			return f.Close()
		},
		readDir: func(f *os.File, entries int) ([]os.FileInfo, error) {
			return f.Readdir(entries)
		},
		getPidName: getPidNameWithHandler,
		readFile:   ioutil.ReadFile,
	}

	return getProcessesByNameWithHandlers(svc, name)
}

// ProcessService is an interface required by ProcessCheck.
//
// The given a process name, the method IsProcessRunning()
// must return true if the named process is running, otherwise
// false. Note the code will be different for each OS.
type ProcessService interface {
	IsProcessRunning(string) bool
}

type processHandler struct{}

func (p processHandler) IsProcessRunning(name string) bool {
	return isProcessRunningOsConstrained(name)
}

// ProcessCheck is used to encapsulate a named process
// along with the methods used to get information about
// that process. Currently the only check is for the named
// process running.
type ProcessCheck struct {
	ProcessName string

	ProcessCheckHandler ProcessService
}

// IsProcessRunning interrogates the OS for the named
// process to check if it's running. Note this function
// calls IsProcessRunning in the injected service and
// in this implementation will ultimately call an OS
// constrained function.
func (p ProcessCheck) IsProcessRunning() bool {
	return p.ProcessCheckHandler.IsProcessRunning(p.ProcessName)
}

func showHelp() {
	fmt.Printf(
		`check_process -name <process name> [ other options ]
  Perform various checks for a process. These checks depend on the -check-type
  flag which defaults to "running". The -name option is always required.

	-name <process name>: Required. The name of the process to check
	-type <check type>: Defaults to "running". Supported types are "running"
	  "notrunning".
`)

	showHelpOsConstrained()
}

func checkRunning(processCheck ProcessCheck, invert bool) (string, int) {
	var msg string
	var retcode int
	var responseStateText string
	var checkInfo string

	result := processCheck.IsProcessRunning()
	if result != invert {
		retcode = 0
		responseStateText = "OK"
	} else {
		retcode = 2
		responseStateText = "CRITICAL"
	}

	if result == true {
		checkInfo = ""
	} else {
		checkInfo = "not "
	}

	msg = fmt.Sprintf("CheckProcess %s - Process %s is %srunning", responseStateText, processCheck.ProcessName, checkInfo)

	return msg, retcode
}

// CheckProcessWithService provides a way to inject a custom
// service for interrogating the OS for the named process.
// This is mainly used for testing but can also be used for any
// application wishing to override the normal interrogations.
func CheckProcessWithService(name string, checkType string, processService ProcessService) (string, int) {
	pc := ProcessCheck{
		ProcessName:         name,
		ProcessCheckHandler: processService,
	}

	var msg string
	var retcode int

	switch checkType {
	case "running":
		msg, retcode = checkRunning(pc, false)
	case "notrunning":
		msg, retcode = checkRunning(pc, true)
	default:
		msg = fmt.Sprintf("Invalid check type: %s", checkType)
		retcode = 3
	}

	return msg, retcode
}

// CheckProcessFlags provides an injection entry point for
// a check process function and a service. Command line flags
// are used to determine the process and check type to execute.
//
// Returns are a text description of the response and an integer
// return code indicating the response.
func CheckProcessFlags(checkProcess func(string, string, ProcessService) (string, int), processService ProcessService) (string, int) {
	var msg string
	var retcode int
	var invalidCmdMsg string

	if len(os.Args) <= 2 {
		showHelp()
		retcode = 2
	} else {
		namePtr := flag.String("name", "", "process name")
		checkTypePtr := flag.String("type", "running", "type of check (currently only \"running\" is supported")
		flag.Parse()

		*checkTypePtr = strings.ToLower(*checkTypePtr)

		invalidCmdMsg = ""

		if *namePtr == "" {
			invalidCmdMsg = invalidCmdMsg +
				"A process name must be specified with the -name option."
		} else if *checkTypePtr != "running" && *checkTypePtr != "notrunning" {
			invalidCmdMsg = invalidCmdMsg +
				fmt.Sprintf("Invalid check type (%s). Only \"running\" and \"notrunning\" are supported.",
					*checkTypePtr)
		}

		if invalidCmdMsg != "" {
			msg = fmt.Sprintf("CheckProcess CRITICAL - %s", invalidCmdMsg)
			retcode = 2
		} else {
			msg, retcode = checkProcess(*namePtr, *checkTypePtr, processService)
		}
	}

	return msg, retcode
}

// CheckProcess will interrogate the OS for details on
// a named process. The details of the interrogation
// depend on the check type.
func CheckProcess() {
	msg, retcode := CheckProcessFlags(CheckProcessWithService, new(processHandler))

	if retcode >= 0 {
		fmt.Println(msg)
	}

	os.Exit(retcode)
}
