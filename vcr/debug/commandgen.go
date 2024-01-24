package debug

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/shirou/gopsutil/process"
)

type CommandGenerator struct {
	entrypoint        []string
	cwd               string
	instanceID        string
	serviceName       string
	apiKey            string
	apiSecret         string
	applicationID     string
	applicationPort   int
	debuggerPort      int
	privateKey        string
	regionAlias       string
	publicURL         string
	endpointURLScheme string
	debuggerURLScheme string

	commandName string
	commandArgs []string
}

func NewCommandGenerator(
	entrypoint []string,
	cwd string,
	instanceID string,
	serviceName string,
	apiKey string,
	apiSecret string,
	applicationID string,
	applicationPort int,
	debuggerPort int,
	privateKey string,
	regionAlias string,
	publicURL string,
	endpointURLScheme string,
	debuggerURLScheme string,
) (*CommandGenerator, error) {
	g := &CommandGenerator{
		entrypoint:        entrypoint,
		cwd:               cwd,
		instanceID:        instanceID,
		serviceName:       serviceName,
		apiKey:            apiKey,
		apiSecret:         apiSecret,
		applicationID:     applicationID,
		applicationPort:   applicationPort,
		debuggerPort:      debuggerPort,
		privateKey:        privateKey,
		regionAlias:       regionAlias,
		publicURL:         publicURL,
		endpointURLScheme: endpointURLScheme,
		debuggerURLScheme: debuggerURLScheme,
	}
	if err := g.parseCommand(); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *CommandGenerator) generateCmd() *exec.Cmd {
	command := exec.Command(g.commandName, g.commandArgs...)
	command = setProcessGroup(command)
	command.Env = append(os.Environ(),
		"DEBUG=true",
		"INSTANCE_SERVICE_NAME="+g.serviceName,
		"API_ACCOUNT_ID="+g.apiKey,
		"API_APPLICATION_ID="+g.applicationID,
		"API_ACCOUNT_SECRET="+g.apiSecret,
		"PRIVATE_KEY="+g.privateKey,
		"CODE_DIR="+g.cwd,
		"ENDPOINT_URL_SCHEME="+g.endpointURLScheme,
		"DEBUGGER_URL_SCHEME="+g.debuggerURLScheme,
		"REGION="+g.regionAlias,
		"NERU_APP_PORT="+strconv.Itoa(g.applicationPort),
		"VCR_DEBUG=true",
		"VCR_INSTANCE_SERVICE_NAME="+g.serviceName,
		"VCR_INSTANCE_PUBLIC_URL="+g.publicURL,
		"VCR_API_ACCOUNT_ID="+g.apiKey,
		"VCR_API_ACCOUNT_SECRET="+g.apiSecret,
		"VCR_API_APPLICATION_ID="+g.applicationID,
		"VCR_PRIVATE_KEY="+g.privateKey,
		"VCR_CODE_DIR="+g.cwd,
		"VCR_ENDPOINT_URL_SCHEME="+g.endpointURLScheme,
		"VCR_DEBUGGER_URL_SCHEME="+g.debuggerURLScheme,
		"VCR_REGION="+g.regionAlias,
		"VCR_PORT="+strconv.Itoa(g.applicationPort),
		"FORCE_COLOR=1",
	)
	if g.instanceID != "" {
		command.Env = append(command.Env, "INSTANCE_ID="+g.instanceID)
	}
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command
}

func (g *CommandGenerator) parseCommand() error {
	var args []string
	if len(g.entrypoint) >= 2 {
		args = g.entrypoint[1:]
	}
	g.commandName = g.entrypoint[0]
	g.commandArgs = args
	return nil
}

func killProcess(cmd *exec.Cmd) error {
	p, err := process.NewProcess(int32(cmd.Process.Pid))
	if err != nil {
		return fmt.Errorf("failed to get process information for pid %v: %w", cmd.Process.Pid, err)
	}
	killProcessRecursive(p)
	return nil
}

func killProcessRecursive(p *process.Process) {
	childProcs, _ := p.Children()
	if len(childProcs) != 0 {
		for _, c := range childProcs {
			killProcessRecursive(c)
		}
	}
	p.Kill()
}
