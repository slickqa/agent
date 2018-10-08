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
	"strings"
	"time"
)

func main() {
	log.Println("=========== Initializing Agent ===========")

	parser := flag.NewFlagSetWithEnvPrefix(os.Args[0], "SLICK_AGENT", 0)
	parser.StringVar(&ProgramOptions.ConfigurationLocation, "conf", "", "configuration location")
	err := parser.Parse(os.Args[1:])
	if err != nil {
		log.Fatalf("Unable to parse command line arguments: %s", err.Error())
	}

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
	}
)

type AgentStatus struct {
	Provides []string
	BrokenProvides []string
	RunStatus string
	Project string
	Release string
	Build string
	Versions map[string]string
	Hardware string
	RequiredTestAttributes map[string]string
	RanTest bool
	Action string
	ActionParameter string
	IP string
	Attributes map[string]string
	ResultToRun *map[string]interface{}
	Groups []string
	ShouldExit bool
}

type AgentConfiguration struct {
	LoopStart []PhaseConfiguration `yaml:"loop-start,omitempty"`
	CheckForAction []PhaseConfiguration `yaml:"check-for-action,omitempty"`
	Discovery []PhaseConfiguration `yaml:"discovery,omitempty"`
	BrokenDiscovery []PhaseConfiguration `yaml:"broke-discovery,omitempty"`
	GetStatus []PhaseConfiguration `yaml:"get-status,omitempty"`
	UpdateStatus []PhaseConfiguration `yaml:"update-status,omitempty"`
	RunTest Action `yaml:"run-test,omitempty"`
	NoTest []PhaseConfiguration `yaml:"no-test,omitempty"`
	Cleanup []PhaseConfiguration `yaml:"cleanup,omitempty"`
	ActionMap map[string]Action `yaml:"action-map,omitempty"`
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

type Action struct {
	HttpUrl string `yaml:"http-url,omitempty"`
	Command string `yaml:"command,omitempty"`
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
		response, err := http.Get(ProgramOptions.ConfigurationLocation)
		if err == nil {
			if response.StatusCode == 200 {
				buf, err := ioutil.ReadAll(response.Body)
				if err == nil {
					err = yaml.Unmarshal(buf, &config)
				}
			} else {
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
	return config, parsed, err
}

func DefaultStatus() AgentStatus {
	return AgentStatus{
		RunStatus: "IDLE",
		RanTest: false,
	}
}

func (agent *Agent) CheckConfiguration() {
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
}

func (agent *Agent) HandleCheckForAction() {
}

func (agent *Agent) HandlePerformAction() {
}

func (agent *Agent) HandleDiscovery() {
}

func (agent *Agent) HandleBrokenDiscovery() {
}

func (agent *Agent) HandleStatusUpdate() {
}

func (agent *Agent) HandleGetCurrentStatus() {
}

func (agent *Agent) HandleGetTest() {
}

func (agent *Agent) HandleRunTest() {
}

func (agent *Agent) HandleNoTest() {
}

func (agent *Agent) HandleCleanup() {
}

func (agent *Agent) HandleSleep() {
	if agent.RanTest {
		time.Sleep(agent.Cache.Sleep.AfterTest)
	} else {
		time.Sleep(agent.Cache.Sleep.NoTest)
	}
}

