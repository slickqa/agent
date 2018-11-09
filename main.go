package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kbinani/screenshot"
	"github.com/minio/minio-go"
	"github.com/namsral/flag"
	"github.com/slickqa/slick-agent/slickClient"
	"github.com/slickqa/slick/slickqa"
	"golang.org/x/net/context"
	"gopkg.in/yaml.v2"
	"image/png"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

func main() {
	log.Println("================= Initializing Agent =================")

	var groups string

	if runtime.GOOS == "windows" {
		ProgramOptions.ShellCommand = "cmd.exe"
		ProgramOptions.ShellOpt = "/C"
	} else {
		ProgramOptions.ShellCommand = "/bin/bash"
		ProgramOptions.ShellOpt = "-c"
	}

	parser := flag.NewFlagSetWithEnvPrefix(os.Args[0], "SLICK_AGENT", 0)
	parser.StringVar(&ProgramOptions.ConfigurationLocation, "conf", "", "configuration location")
	parser.StringVar(&groups, "groups", "", "comma separated list of groups")
	parser.StringVar(&ProgramOptions.ShellCommand, "shell", ProgramOptions.ShellCommand, "Shell to use for command execution.")
	parser.StringVar(&ProgramOptions.ShellOpt, "shell-arg", ProgramOptions.ShellOpt, "Option to pass to shell for command execution.")
	parser.BoolVar(&ProgramOptions.Debug, "debug", false, "Enable debug logging for extra info.")
	err := parser.Parse(os.Args[1:])
	if err != nil {
		log.Fatalf("Unable to parse command line arguments: %s", err.Error())
	}

	if groups != "" {
		ProgramOptions.Groups = regexp.MustCompile(",[ ]?").Split(groups, -1)
	} else {
		ProgramOptions.Groups = make([]string, 0)
	}

	debug("Program Options: \n%+v", ProgramOptions)
	log.Printf("Loading Configuration from %s", ProgramOptions.ConfigurationLocation)

	agent := Agent{}
	agent.Config, agent.Cache, err = LoadConfiguration()
	if err != nil {
		log.Fatalf("Error loading configuration: %s", err.Error())
	}

	if agent.Config.Slick.GrpcUrl != "" {
		//TODO: get token dynamically
		agent.Slick, err = slickClient.CreateClient(agent.Config.Slick.GrpcUrl, "yomamasofat")
		if err != nil {
			log.Printf("Error creating slick client: %s", err)
		}
	}

	// Configure S3 Storage
	//TODO: add command line argument to turn on
	var s3Options = S3CompatibleStorageOptions{
		Endpoint:        os.Getenv("S3ENDPOINT"),
		AccessKeyID:     "leeard",
		SecretAccessKey: os.Getenv("S3SECRETKEY"),
		BucketName:      "agentscreenshots",
		Location:        "utah-higg-trailer",
	}
	agent.S3Storage = s3Options

	agent.LastConfigurationCheck = time.Now()
	output, _ := yaml.Marshal(agent.Config)
	log.Printf("Configuration:\n%s", string(output))

	go agent.startScreenShots()
	for !agent.Status.ShouldExit {
		debugln("Top of loop, initializing status.")
		agent.Status = agent.DefaultStatus()
		agent.CheckConfiguration()
		agent.HandleLoopStart()
		agent.HandleCheckForAction()
		if agent.Status.Action != "" {
			agent.HandlePerformAction()
		}
		agent.HandleDiscoverTestAttributes()
		agent.HandleDiscovery()
		agent.HandleStatusUpdate()
		agent.HandleBrokenDiscovery()
		agent.HandleStatusUpdate()
		agent.HandleGetCurrentStatus()
		agent.HandleStatusUpdate()
		if agent.Status.RunStatus == "IDLE" {
			agent.HandleBeforeGetTest()
			agent.HandleGetTest()
			agent.HandleStatusUpdate()
			if agent.Status.ResultToRun != nil {
				agent.RanTest = true
				agent.Status.RunStatus = "RUNNING"
				agent.HandleStatusUpdate()
				agent.HandleRunTest()
			} else {
				agent.RanTest = false
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
		Groups                []string
		Debug                 bool
		ShellCommand          string
		ShellOpt              string
	}
)

type AgentStatus struct {
	Provides               []string                           `json:"provides"`
	BrokenProvides         []string                           `json:"broken"`
	RunStatus              string                             `json:"runStatus"`
	Projects               []*slickqa.ProjectReleaseBuildInfo //[]ProjectReleaseBuild `json:"projects,omitempty"`
	Versions               map[string]string                  `json:"versions,omitempty"`
	Hardware               string                             `json:"hardware,omitempty"`
	RequiredTestAttributes map[string]string                  `json:"requiredAttrs,omitempty"`
	RanTest                bool                               `json:"ranTest"`
	Action                 string                             `json:"action,omitempty"`
	ActionParameter        string                             `json:"actionParameter,omitempty"`
	IP                     string                             `json:"IP,omitempty"`
	Attributes             map[string]string                  `json:"attributes"`
	ResultToRun            map[string]interface{}             `json:"testcase"`
	Groups                 []string                           `json:"groups"`
	ShouldExit             bool                               `json:"shouldExit"`
	AgentName              string                             `json:"agentName"`
}

type ProjectReleaseBuild struct {
	Name    string `json:"name" yaml:"name"`
	Release string `json:"release,omitempty" yaml:"release,omitempty"`
	Build   string `json:"build,omitempty" yaml:"build,omitempty"`
}

type AgentConfiguration struct {
	Company                    string                        `yaml:"company,omitempty"`
	Projects                   []ProjectReleaseBuild         `yaml:"projects,omitempty"`
	LoopStart                  []PhaseConfiguration          `yaml:"loop-start,omitempty"`
	CheckForAction             []PhaseConfiguration          `yaml:"check-for-action,omitempty"`
	TestAttributeDiscovery     []PhaseConfiguration          `yaml:"test-attribute-discovery,omitempty"`
	Discovery                  []PhaseConfiguration          `yaml:"discovery,omitempty"`
	BrokenDiscovery            []PhaseConfiguration          `yaml:"broke-discovery,omitempty"`
	GetStatus                  []PhaseConfiguration          `yaml:"get-status,omitempty"`
	UpdateStatus               []PhaseConfiguration          `yaml:"update-status,omitempty"`
	RunTest                    []PhaseConfiguration          `yaml:"run-test,omitempty"`
	NoTest                     []PhaseConfiguration          `yaml:"no-test,omitempty"`
	Cleanup                    []PhaseConfiguration          `yaml:"cleanup,omitempty"`
	ActionMap                  map[string]PhaseConfiguration `yaml:"action-map,omitempty"`
	BeforeGetTest              []PhaseConfiguration          `yaml:"before-get-test,omitempty"`
	GetTest                    []PhaseConfiguration          `yaml:"get-test,omitempty"`
	Slick                      SlickConfiguration            `yaml:"slick,omitempty"`
	CheckForConfigurationEvery string                        `yaml:"check-for-configuration-every,omitempty"`
	Sleep                      SleepConfiguration            `yaml:"sleep,omitempty"`
}

type ParsedConfigurationOptions struct {
	Sleep                      ParsedSleepOptions
	CheckForConfigurationEvery time.Duration
}

type ParsedSleepOptions struct {
	AfterTest time.Duration
	NoTest    time.Duration
}

type S3CompatibleStorageOptions struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	BucketName      string
	Location        string
}

type Agent struct {
	Config                 AgentConfiguration
	S3Storage              S3CompatibleStorageOptions
	Status                 AgentStatus
	LastConfigurationCheck time.Time
	RanTest                bool
	Cache                  ParsedConfigurationOptions
	Slick                  *slickClient.SlickClient
}

type SlickConfiguration struct {
	BaseUrl   string `yaml:"base-url"`
	GrpcUrl   string `yaml:"grpc-url"`
	AgentName string `yaml:"agent-name"`
}

type PhaseConfiguration struct {
	HttpUrl     string            `yaml:"http-url,omitempty"`
	Command     string            `yaml:"command,omitempty"`
	WriteFile   string            `yaml:"write-file,omitempty"`
	ReadFile    string            `yaml:"read-file,omitempty"`
	StaticList  []string          `yaml:"static-list,omitempty,flow"`
	StaticMap   map[string]string `yaml:"static-map,omitempty"`
	StaticValue string            `yaml:"static-value,omitempty"`
}

type SleepConfiguration struct {
	AfterTest string `yaml:"after-test,omitempty"`
	NoTest    string `yaml:"no-test,omitempty"`
}

type TestcaseInfo struct {
	Id           string
	Name         string
	AutomationId string
	TestrunId    string
}

func DefaultConfiguration() (AgentConfiguration, ParsedConfigurationOptions) {
	return AgentConfiguration{
			CheckForConfigurationEvery: "5s",
			Sleep: SleepConfiguration{
				AfterTest: "500ms",
				NoTest:    "2s",
			},
			Slick: SlickConfiguration{},
		}, ParsedConfigurationOptions{
			CheckForConfigurationEvery: 5 * time.Second,
			Sleep: ParsedSleepOptions{
				AfterTest: 500 * time.Millisecond,
				NoTest:    2 * time.Second,
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
		if config.Slick.AgentName == "" {
			config.Slick.AgentName, _ = os.Hostname()
		}
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

func (agent *Agent) DefaultStatus() AgentStatus {
	groups := make([]string, len(ProgramOptions.Groups))
	projects := make([]*slickqa.ProjectReleaseBuildInfo, 0)
	copy(groups, ProgramOptions.Groups)
	for _, p := range agent.Config.Projects {
		projects = append(projects, &slickqa.ProjectReleaseBuildInfo{Project: p.Name,
			Build:   p.Build,
			Release: p.Release})
	}
	return AgentStatus{
		RunStatus:              "IDLE",
		RanTest:                false,
		Groups:                 groups,
		Provides:               make([]string, 0),
		BrokenProvides:         make([]string, 0),
		Attributes:             make(map[string]string),
		RequiredTestAttributes: make(map[string]string),
		Projects:               projects,
		AgentName:              agent.Config.Slick.AgentName,
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
	for _, phase := range agent.Config.LoopStart {
		phase.ApplyToStatus(&agent.Status, nil, nil, nil)
	}
}

func (agent *Agent) HandleCheckForAction() {
	debug("Inside HandleCheckForAction, there are %d configs to process.", len(agent.Config.CheckForAction))
	for _, phase := range agent.Config.CheckForAction {
		phase.ApplyToStatus(&agent.Status, &agent.Status.Action, nil, nil)
	}
}

func (agent *Agent) HandlePerformAction() {
	debug("Inside HandlePerformAction, Action: %#v Parameter: %#v", agent.Status.Action, agent.Status.ActionParameter)
	config, ok := agent.Config.ActionMap[agent.Status.Action]
	if !ok {
		log.Printf("Unable to find action %#v in action map %+v from configuration value action-map from %s", agent.Status.Action, agent.Config.ActionMap, ProgramOptions.ConfigurationLocation)
		return
	}
	config.ApplyToStatus(&agent.Status, nil, nil, nil)

	// TODO handler for successful and unsuccessful action
}

func (agent *Agent) HandleDiscoverTestAttributes() {
	debug("Inside HandleDiscoverTestAttributes, there are %d configs to process.", len(agent.Config.TestAttributeDiscovery))
	for _, phase := range agent.Config.TestAttributeDiscovery {
		phase.ApplyToStatus(&agent.Status, nil, nil, &agent.Status.RequiredTestAttributes)
	}
}

func (agent *Agent) HandleDiscovery() {
	debug("Inside HandleDiscovery, there are %d configs to process.", len(agent.Config.Discovery))
	for _, phase := range agent.Config.Discovery {
		phase.ApplyToStatus(&agent.Status, nil, &agent.Status.Provides, nil)
	}
}

func (agent *Agent) HandleBrokenDiscovery() {
	debug("Inside HandleBrokenDiscovery, there are %d configs to process.", len(agent.Config.BrokenDiscovery))
	for _, phase := range agent.Config.BrokenDiscovery {
		phase.ApplyToStatus(&agent.Status, nil, &agent.Status.BrokenProvides, nil)
	}
}

func (agent *Agent) HandleStatusUpdate() {
	debug("Inside HandleStatusUpdate, there are %d configs to process.", len(agent.Config.UpdateStatus))
	for _, phase := range agent.Config.UpdateStatus {
		phase.ApplyToStatus(&agent.Status, nil, nil, nil)
	}
	// update slick
	if agent.Slick != nil {
		var currentTest slickqa.AgentCurrentTest
		if agent.Status.ResultToRun != nil {
			testInfo := GetTestInfo(agent.Status.ResultToRun)
			testUrl := agent.Config.Slick.BaseUrl + "/testruns/" + testInfo.TestrunId + "?result=" + testInfo.Id
			currentTest = slickqa.AgentCurrentTest{
				Name:         testInfo.Name,
				AutomationId: testInfo.AutomationId,
				Url:          testUrl,
			}

		}
		_, err := agent.Slick.Agents.UpdateStatus(context.Background(), &slickqa.AgentStatusUpdate{
			Id: &slickqa.AgentId{Company: agent.Config.Company, Name: agent.Status.AgentName},
			Status: &slickqa.AgentStatus{
				Projects:    agent.Status.Projects,
				RunStatus:   agent.Status.RunStatus,
				CurrentTest: &currentTest,
			},
		})
		if err != nil {
			// re-connect?
			agent.Slick.Close()
			log.Printf("Trying to re-connect to slick")
			//TODO: get token dynamically
			slickClient, err := slickClient.CreateClient(agent.Config.Slick.GrpcUrl, "yomamasofat")
			if err != nil {
				log.Printf("Error re-connecting to slick: %s", err)
			} else {
				agent.Slick = slickClient
			}
		}
	}
}

func (agent *Agent) HandleGetCurrentStatus() {
	debug("Inside HandleGetCurrentStatus, there are %d configs to process.", len(agent.Config.GetStatus))
	for _, phase := range agent.Config.GetStatus {
		phase.ApplyToStatus(&agent.Status, &agent.Status.RunStatus, nil, nil)
	}
}

func (agent *Agent) HandleBeforeGetTest() {
	debug("Inside HandleBeforeGetTest, there are %d configs to process.", len(agent.Config.BeforeGetTest))
	for _, phase := range agent.Config.BeforeGetTest {
		phase.ApplyToStatus(&agent.Status, nil, nil, nil)
	}
}

func (status *AgentStatus) getNonBrokenProvides() []string {
	var exists = struct{}{}
	providesSet := make(map[string]struct{})
	for _, provide := range status.Provides {
		providesSet[provide] = exists
	}
	for _, broken := range status.BrokenProvides {
		delete(providesSet, broken)
	}

	provides := make([]string, len(providesSet))
	i := 0
	for provide := range providesSet {
		provides[i] = provide
		i++
	}
	return provides
}

func (agent *Agent) RequestResultFromSlickQueue(query map[string]interface{}) map[string]interface{} {
	var result map[string]interface{} = nil
	jsonContentBody, err := json.Marshal(query)
	if err != nil {
		log.Printf("Error building query to slick for test: %s", err.Error())
		return nil
	}
	url := agent.Config.Slick.BaseUrl + "/api/results/queue/" + agent.Config.Slick.AgentName
	debug("URL: %s", url)
	debug("JSON: %s", string(jsonContentBody))
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonContentBody))
	if err != nil {
		log.Printf("Error making request to slick for test to run: %s", err.Error())
		return nil
	}
	if resp.StatusCode == 200 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error occurred while trying to read response from slick: %s", err.Error())
			return nil
		}
		err = json.Unmarshal(body, &result)
		if err != nil {
			log.Printf("Error occurred while trying to parse json from slick: %s", err.Error())
			return nil
		}
		return result
	}
	debug("Response code from slick when requesting result from queue: %d", resp.StatusCode)
	debug("Response:\n%+v", resp)
	return nil
}

func (agent *Agent) HandleGetTest() {
	debug("Inside HandleGetTest, there are %d configs to process.", len(agent.Config.GetTest))
	// first get the test from slick, then call everything else
	query := make(map[string]interface{})
	query["provides"] = agent.Status.getNonBrokenProvides()
	for key, value := range agent.Status.RequiredTestAttributes {
		query[key] = value
	}

	if len(agent.Status.Projects) > 0 {
		for _, project := range agent.Status.Projects {
			projectQuery := query
			projectQuery["project"] = project.Project
			if project.Release != "" {
				projectQuery["release"] = project.Release
			}
			if project.Build != "" {
				projectQuery["build"] = project.Build
			}
			agent.Status.ResultToRun = agent.RequestResultFromSlickQueue(projectQuery)
			if agent.Status.ResultToRun != nil {
				break
			}
		}
	} else {
		agent.Status.ResultToRun = agent.RequestResultFromSlickQueue(query)
	}

	// TODO Handle the new go version of slick, when it's finished
	for _, phase := range agent.Config.GetTest {
		phase.ApplyToStatus(&agent.Status, nil, nil, nil)
	}
}

func (agent *Agent) HandleRunTest() {
	debug("Inside HandleRunTest, there are %d configs to process.  Current Test:\n%+v", len(agent.Config.RunTest), agent.Status.ResultToRun)
	log.Printf("Running result: %+v", GetTestInfo(agent.Status.ResultToRun))
	for _, phase := range agent.Config.RunTest {
		phase.ApplyToStatus(&agent.Status, nil, nil, nil)
	}
	status := GetTestResult(agent.Status.ResultToRun)
	if status == "" || status == "NO_RESULT" {
		status = "UNKNOWN"
	}
	log.Printf("Result of test: %s", status)
}

func (agent *Agent) HandleNoTest() {
	debug("Inside HandleNoTest, there are %d configs to process.", len(agent.Config.NoTest))
	for _, phase := range agent.Config.NoTest {
		phase.ApplyToStatus(&agent.Status, nil, nil, nil)
	}
}

func (agent *Agent) HandleCleanup() {
	debug("Inside HandleCleanup, there are %d configs to process.", len(agent.Config.Cleanup))
	for _, phase := range agent.Config.Cleanup {
		phase.ApplyToStatus(&agent.Status, nil, nil, nil)
	}
}

func (agent *Agent) HandleSleep() {
	if agent.RanTest {
		debug("HandleSleep: After a test, sleeping %s", agent.Cache.Sleep.AfterTest)
		time.Sleep(agent.Cache.Sleep.AfterTest)
	} else {
		debug("HandleSleep: No test ran, sleeping %s", agent.Cache.Sleep.NoTest)
		time.Sleep(agent.Cache.Sleep.NoTest)
	}
}

func (a *Agent) startScreenShots() {
	useSSL := true
	endPoint := a.S3Storage.Endpoint
	accessKey := a.S3Storage.AccessKeyID
	secret := a.S3Storage.SecretAccessKey
	bucket := a.S3Storage.BucketName
	location := a.S3Storage.Location

	// Initialize minio client object.
	minioClient, err := minio.New(endPoint, accessKey, secret, useSSL)
	if err != nil {
		log.Fatalln(err)
	}
	// Make the bucket

	exists, err := minioClient.BucketExists(bucket)
	if err != nil {
		log.Printf("error when checking if bucket %s exists\n", bucket)
	}
	if !exists {
		err = minioClient.MakeBucket(bucket, location)
		if err != nil {
			log.Printf("error creating bucket %s\n", err)
		} else {
			log.Printf("Successfully created %s\n", bucket)
		}
	}

	bounds := screenshot.GetDisplayBounds(0)

	fmt.Printf("Starting screenshot loop\n")
	for {
		img, err := screenshot.CaptureRect(bounds)
		if err != nil {
			fmt.Printf("error grabbing screenshot %s\n", err)
		}
		fileName := a.Config.Slick.AgentName + "-screenshot.png"
		file, _ := os.Create(fileName)
		png.Encode(file, img)
		file.Close()

		// Upload the screenshot
		objectName := fileName
		filePath := fileName
		contentType := "image/png"

		// Upload the zip file with FPutObject
		_, err = minioClient.FPutObject(bucket, objectName, filePath, minio.PutObjectOptions{ContentType: contentType})
		if err != nil {
			log.Printf("error uploading screenshot %s\n", err)
		} else {
			//TODO: update timestamp
		}
		time.Sleep(4 * time.Second)
	}

}

func (conf *PhaseConfiguration) ApplyToStatus(status *AgentStatus, staticVar *string, staticArray *[]string, staticMap *map[string]string) error {
	if conf.Command != "" {
		tmpfile, err := ioutil.TempFile("", "slick-agent-status-*.yml")
		tmpFilename := tmpfile.Name()
		if err != nil {
			log.Printf("Unable to write temp file with status before running command %s: %s", conf.Command, err.Error())
			return err
		}
		defer os.Remove(tmpFilename)

		debug("Writing agent status to %s", tmpFilename)
		content, err := json.Marshal(status)
		if err != nil {
			log.Printf("Unable to marshal status to json before running command %s: %s", conf.Command, err.Error())
			return err
		}
		_, err = tmpfile.Write(content)
		if err != nil {
			log.Printf("Error writing temp file %s before running %s: %s", tmpFilename, conf.Command, err.Error())
			return err
		}
		tmpfile.Close()

		debug("Running Command: %s %s %#v", ProgramOptions.ShellCommand, ProgramOptions.ShellOpt, conf.Command)
		cmd := exec.Command(ProgramOptions.ShellCommand, ProgramOptions.ShellOpt, conf.Command)
		cmd.Env = append(os.Environ(), fmt.Sprintf("SLICK_AGENT_STATUS=%s", tmpFilename))
		if err := cmd.Run(); err != nil {
			log.Printf("Command %#v encountered an error: %s", conf.Command, err.Error())
			return err
		}
		debug("Reading status back in from %s", tmpFilename)
		content, err = ioutil.ReadFile(tmpFilename)
		if err != nil {
			log.Printf("Unable to read state from %s after running command %#v: %s", tmpFilename, conf.Command, err.Error())
			return err
		}

		newStatus := *status
		err = json.Unmarshal(content, &newStatus)
		if err != nil {
			log.Printf("Problem parsing state from %s after running command %#v: %s", tmpFilename, conf.Command, err.Error())
			return err
		}
		debug("Status after command:\n%+v", newStatus)
		*status = newStatus
		return nil
	} else if conf.WriteFile != "" {
		content, err := json.Marshal(status)
		if err != nil {
			log.Printf("Error serializing agent status to json before writing to file %s: %s", conf.WriteFile, err.Error())
			return err
		}
		err = ioutil.WriteFile(conf.WriteFile, content, 0644)
		if err != nil {
			log.Printf("Error writing agent status to %s: %s", conf.WriteFile, err.Error())
			return err
		}
	} else if conf.HttpUrl != "" {
		//TODO handle URL posting
	} else if conf.StaticValue != "" {
		if staticVar != nil {
			*staticVar = conf.StaticValue
		} else if staticArray != nil {
			*staticArray = append(*staticArray, conf.StaticValue)
		} else {
			log.Printf("Attempted to set static value %#v but that is invalid during this phase, ignoring.", conf.StaticValue)
			return fmt.Errorf("nil staticVar and staticArray during this phase, cannot set %#v", conf.StaticValue)
		}
	} else if len(conf.StaticList) > 0 {
		if staticArray != nil {
			*staticArray = append(*staticArray, conf.StaticList...)
		} else {
			log.Printf("Attempted to set static list %+v but that is invalid during this phase, ignoring.", conf.StaticList)
			return fmt.Errorf("nil staticArray, can't set value %#v during this phase", conf.StaticList)
		}
	} else if len(conf.StaticMap) > 0 && staticMap != nil {
		for k, v := range conf.StaticMap {
			(*staticMap)[k] = v
		}
	}

	return nil
}

func GetTestResult(test map[string]interface{}) string {
	status, ok := test["status"]
	if !ok {
		return ""
	}
	result, ok := status.(string)
	if !ok {
		return ""
	}
	return result
}

func GetTestInfo(test map[string]interface{}) TestcaseInfo {
	var retval TestcaseInfo
	testcase, ok := test["testcase"]
	if !ok {
		return TestcaseInfo{}
	}
	testref, ok := testcase.(map[string]interface{})
	if !ok {
		return TestcaseInfo{}
	}
	testrun, ok := test["testrun"]
	if !ok {
		return TestcaseInfo{}
	}
	testrunRef, ok := testrun.(map[string]interface{})
	if !ok {
		retval = TestcaseInfo{
			Id:           test["id"].(string),
			Name:         testref["name"].(string),
			AutomationId: testref["automationId"].(string),
		}
	} else {
		retval = TestcaseInfo{
			Id:           test["id"].(string),
			Name:         testref["name"].(string),
			AutomationId: testref["automationId"].(string),
			TestrunId:    testrunRef["testrunId"].(string),
		}
	}
	return retval
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
