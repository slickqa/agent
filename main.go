package main

import (
	"log"
	"time"
)

func main() {
	log.Println("=========== Initializing Agent ===========")
	agent := Agent{}
	err := agent.LoadConfiguration()
	if err != nil {
		log.Fatalf("Error loading configuration: %s", err.Error())
	}
	for !agent.Status.ShouldExit {
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
				agent.HandleRunTest()
			} else {
				agent.HandleNoTest()
			}
			agent.HandleStatusUpdate()
		}
		agent.HandleCleanUp()
		agent.HandleSleep()
	}
	log.Println("Agent requested to exit!")
}

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

type Integration interface {
	call(*AgentStatus) error
}

type AgentConfiguration struct {
	LoopStart []PhaseConfiguration
	CheckForAction []PhaseConfiguration
	Discovery []PhaseConfiguration
	BrokenDiscovery []PhaseConfiguration
	GetStatus []PhaseConfiguration
	UpdateStatus []PhaseConfiguration
	RunTest *Action
	NoTest []PhaseConfiguration
	CleanUp []PhaseConfiguration
	ActionMap map[string]Action
	GetTest []PhaseConfiguration
	Slick *SlickConfiguration
	CheckForConfigurationEvery *string
}

type Agent struct {
	Config AgentConfiguration
	Status AgentStatus
	LastConfigurationCheck time.Time
}

type SlickConfiguration struct {
	BaseUrl string
}

type Action struct {
	HttpUrl *string
	Command *string
}

type PhaseConfiguration struct {
	HttpUrl *string
	Command *string
	WriteFile *string
	ReadFile *string
	StaticList *[]string
	StaticMap *map[string]string
	StaticValue *string
}

func (agent *Agent) LoadConfiguration() (error) {
	return nil
}

func (agent *Agent) CheckConfiguration() {
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

func (agent *Agent) HandleCleanUp() {
}

func (agent *Agent) HandleSleep() {
}

