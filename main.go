package main

import (
	"errors"
	"fmt"
	"github.com/namsral/flag"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

func main() {
	log.Println("================= Initializing Agent =================")

	var groups string
	parser := flag.NewFlagSetWithEnvPrefix(os.Args[0], "SLICK_AGENT", 0)
	parser.StringVar(&ProgramOptions.ConfigurationLocation, "conf", "", "configuration location")
	parser.StringVar(&groups, "groups", "", "comma separated list of groups")
	parser.BoolVar(&ProgramOptions.Debug, "debug", false, "Enable debug logging for extra info.")
	err := parser.Parse(os.Args[1:])
	if err != nil {
		log.Fatalf("Unable to parse command line arguments: %s", err.Error())
	}

	ProgramOptions.Groups = regexp.MustCompile(",[ ]?").Split(groups, -1)

	debug("Program Options: \n%+v", ProgramOptions)
	log.Printf("Loading Configuration from %s", ProgramOptions.ConfigurationLocation)

	agent := Agent{}
	agent.Config, agent.Cache, err = LoadConfiguration()
	if err != nil {
		log.Fatalf("Error loading configuration: %s", err.Error())
	}
	agent.LastConfigurationCheck = time.Now()
	output, _ := yaml.Marshal(agent.Config)
	log.Printf("Configuration:\n%s", string(output))

	for !agent.Status.ShouldExit {
		debugln("Top of loop, initializing status.")
		agent.Status = DefaultStatus()
		agent.CheckConfiguration()
		agent.HandleLoopStart()
		agent.HandleCheckForAction()
		if agent.Status.Action != "" {
			agent.HandlePerformAction()
		}
		agent.HandleDiscovery()
		agent.HandleStatusUpdate()
		agent.HandleBrokenDiscovery()
		agent.HandleStatusUpdate()
		agent.HandleGetCurrentStatus()
		agent.HandleStatusUpdate()
		if agent.Status.RunStatus == "IDLE" {
			agent.HandleGetTest()
			agent.HandleStatusUpdate()
			if agent.Status.ResultToRun != nil {
				agent.RanTest = true
				agent.HandleRunTest()
			} else {
				agent.HandleNoTest()
			}
			agent.HandleStatusUpdate()
		}
		agent.HandleCleanup()
		agent.HandleSleep()
	}
	log.Println("Agent requested to exit!")
}

var (
	ProgramOptions struct {
		ConfigurationLocation string
		Groups []string
		Debug bool
	}
)

type AgentStatus struct {
	Provides []string `json:"provides"`
	BrokenProvides []string `json:"broken"`
	RunStatus string `json:"runStatus"`
	Project string `json:"project,omitempty"`
	Release string `json:"release,omitempty"`
	Build string `json:"build,omitempty"`
	Versions map[string]string `json:"versions,omitempty"`
	Hardware string `json:"hardware,omitempty"`
	RequiredTestAttributes map[string]string `json:"requiredAttrs,omitempty"`
	RanTest bool `json:"ranTest"`
	Action string `json:"action,omitempty"`
	ActionParameter string `json:"actionParameter,omitempty"`
	IP string `json:"IP,omitempty"`
	Attributes map[string]string `json:"attributes"`
	ResultToRun map[string]interface{} `json:"testcase"`
	Groups []string `json:"groups"`
	ShouldExit bool `json:"shouldExit"`
}

type AgentConfiguration struct {
	LoopStart []PhaseConfiguration `yaml:"loop-start,omitempty"`
	CheckForAction []PhaseConfiguration `yaml:"check-for-action,omitempty"`
	Discovery []PhaseConfiguration `yaml:"discovery,omitempty"`
	BrokenDiscovery []PhaseConfiguration `yaml:"broke-discovery,omitempty"`
	GetStatus []PhaseConfiguration `yaml:"get-status,omitempty"`
	UpdateStatus []PhaseConfiguration `yaml:"update-status,omitempty"`
	RunTest []PhaseConfiguration `yaml:"run-test,omitempty"`
	NoTest []PhaseConfiguration `yaml:"no-test,omitempty"`
	Cleanup []PhaseConfiguration `yaml:"cleanup,omitempty"`
	ActionMap map[string]PhaseConfiguration `yaml:"action-map,omitempty"`
	GetTest []PhaseConfiguration `yaml:"get-test,omitempty"`
	Slick SlickConfiguration `yaml:"slick,omitempty"`
	CheckForConfigurationEvery string `yaml:"check-for-configuration-every,omitempty"`
	Sleep SleepConfiguration `yaml:"sleep,omitempty"`
}

type ParsedConfigurationOptions struct {
	Sleep ParsedSleepOptions
	CheckForConfigurationEvery time.Duration
}

type ParsedSleepOptions struct {
	AfterTest time.Duration
	NoTest time.Duration
}

type Agent struct {
	Config AgentConfiguration
	Status AgentStatus
	LastConfigurationCheck time.Time
	RanTest bool
	Cache ParsedConfigurationOptions
}

type SlickConfiguration struct {
	BaseUrl string `yaml:"base-url"`
}

type PhaseConfiguration struct {
	HttpUrl string `yaml:"http-url,omitempty"`
	Command string `yaml:"command,omitempty"`
	WriteFile string `yaml:"write-file,omitempty"`
	ReadFile string `yaml:"read-file,omitempty"`
	StaticList []string `yaml:"static-list,omitempty,flow"`
	StaticMap map[string]string `yaml:"static-map,omitempty"`
	StaticValue string `yaml:"static-value,omitempty"`
}

type SleepConfiguration struct {
	AfterTest string `yaml:"after-test,omitempty"`
	NoTest string `yaml:"no-test,omitempty"`
}

func DefaultConfiguration() (AgentConfiguration, ParsedConfigurationOptions) {
	return AgentConfiguration{
		CheckForConfigurationEvery: "5s",
		Sleep: SleepConfiguration{
			AfterTest: "500ms",
			NoTest: "2s",
		},
	}, ParsedConfigurationOptions{
		CheckForConfigurationEvery: 5 * time.Second,
		Sleep: ParsedSleepOptions{
			AfterTest: 500 * time.Millisecond,
			NoTest: 2 * time.Second,
		},
	}
}

func LoadConfiguration() (AgentConfiguration, ParsedConfigurationOptions, error) {
	config, parsed := DefaultConfiguration()
	var err error

	if strings.HasPrefix(ProgramOptions.ConfigurationLocation, "http") {
		debug("Determined configuration is a url, fetching %#v", ProgramOptions.ConfigurationLocation)
		response, err := http.Get(ProgramOptions.ConfigurationLocation)
		if err == nil {
			if response.StatusCode == 200 {
				debug("Reading %d bytes from body of response from %#v", response.ContentLength, ProgramOptions.ConfigurationLocation)
				buf, err := ioutil.ReadAll(response.Body)
				if err == nil {
					err = yaml.Unmarshal(buf, &config)
				}
			} else {
				debug("Response had a bad status code of %d.  Full response:\n%+v", response)
				err = errors.New(fmt.Sprintf("http status code was %d", response.StatusCode))
			}
		}
	} else {
		buf, err := ioutil.ReadFile(ProgramOptions.ConfigurationLocation)
		if err == nil {
			err = yaml.Unmarshal(buf, &config)
		}
	}

	if err == nil {
		d, err := time.ParseDuration(config.CheckForConfigurationEvery)
		if err == nil {
			parsed.CheckForConfigurationEvery = d
		} else {
			log.Printf("Using default of 5 seconds, Error in check-for-configuration-every %#v: %s", config.CheckForConfigurationEvery, err.Error())
		}

		d, err = time.ParseDuration(config.Sleep.AfterTest)
		if err == nil {
			parsed.Sleep.AfterTest = d
		} else {
			log.Printf("Using default of 500 milliseconds, Error in sleep.after-test %#v: %s", config.Sleep.AfterTest, err.Error())
		}
		d, err = time.ParseDuration(config.Sleep.NoTest)
		if err == nil {
			parsed.Sleep.NoTest = d
		} else {
			log.Printf("Using default of 2 seconds, Error in sleep.no-test %#v: %s", config.Sleep.NoTest, err.Error())
		}
		// hide parsing errors since we use defaults
		err = nil
	}
	debug("Loaded Configuration:\n%+v\nParsed Configuration: %+v, Error: %+v", config, parsed, err)
	return config, parsed, err
}

func DefaultStatus() AgentStatus {
	groups := make([]string, len(ProgramOptions.Groups))
	copy(groups, ProgramOptions.Groups)
	return AgentStatus{
		RunStatus: "IDLE",
		RanTest: false,
		Groups: groups,
	}
}

func (agent *Agent) CheckConfiguration() {
	debug("Checking to see if we need to reload config.  Last check happened at %s", agent.LastConfigurationCheck.String())
	if time.Now().After(agent.LastConfigurationCheck.Add(agent.Cache.CheckForConfigurationEvery)) {
		config, cache, err := LoadConfiguration()
		if err == nil {
			agent.Config = config
			agent.Cache = cache
		} else {
			log.Printf("Error loading configuration, using old configuration: %s", err.Error())
		}
		agent.LastConfigurationCheck = time.Now()
	}
}

func (agent *Agent) HandleLoopStart() {
	debug("Inside HandleLoopStart, there are %d configs to process.", len(agent.Config.LoopStart))
}

func (agent *Agent) HandleCheckForAction() {
	debug("Inside HandleCheckForAction, there are %d configs to process.", len(agent.Config.CheckForAction))
}

func (agent *Agent) HandlePerformAction() {
	debug("Inside HandlePerformAction, Action: %#v Parameter: %#v", agent.Status.Action, agent.Status.ActionParameter)
}

func (agent *Agent) HandleDiscovery() {
	debug("Inside HandleDiscovery, there are %d configs to process.", len(agent.Config.Discovery))
}

func (agent *Agent) HandleBrokenDiscovery() {
	debug("Inside HandleBrokenDiscovery, there are %d configs to process.", len(agent.Config.BrokenDiscovery))
}

func (agent *Agent) HandleStatusUpdate() {
	debug("Inside HandleStatusUpdate, there are %d configs to process.", len(agent.Config.UpdateStatus))
}

func (agent *Agent) HandleGetCurrentStatus() {
	debug("Inside HandleGetCurrentStatus, there are %d configs to process.", len(agent.Config.GetStatus))
}

func (agent *Agent) HandleGetTest() {
	debug("Inside HandleGetTest, there are %d configs to process.", len(agent.Config.GetTest))
}

func (agent *Agent) HandleRunTest() {
	debug("Inside HandleRunTest, there are %d configs to process.  Current Test:\n%+v", len(agent.Config.RunTest), agent.Status.ResultToRun)
}

func (agent *Agent) HandleNoTest() {
	debug("Inside HandleNoTest, there are %d configs to process.", len(agent.Config.NoTest))
}

func (agent *Agent) HandleCleanup() {
	debug("Inside HandleCleanup, there are %d configs to process.", len(agent.Config.Cleanup))
}

func (agent *Agent) HandleSleep() {
	if agent.RanTest {
		debug("HandleSleep: After a test, sleeping %s", agent.Cache.Sleep.AfterTest)
		time.Sleep(agent.Cache.Sleep.AfterTest)
	} else {
		time.Sleep(agent.Cache.Sleep.NoTest)
		debug("HandleSleep: No test ran, sleeping %s", agent.Cache.Sleep.NoTest)
	}
}

func (agent *Agent) RunCommand(command string) ([]byte, error) {
	return []byte{}, nil
}

func (conf *PhaseConfiguration) ApplyToStatus(status *AgentStatus, statusContext string) error {

	return nil
}

func debug(format string, v ...interface{}) {
	if ProgramOptions.Debug {
		log.Printf(format, v...)
	}
}

func debugln(v ...interface{}) {
	if ProgramOptions.Debug {
		log.Println(v...)
	}
}
