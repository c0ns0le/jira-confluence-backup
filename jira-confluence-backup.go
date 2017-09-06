package main

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/pflag"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// Environment vars
	ENV_USER = "ATL_USER"
	ENV_PASS = "ATL_PASS"

	// Url where we can login for jira and confluence
	LOGIN_URI = "/rest/auth/1/session"

	// Confluence specific urls
	CONFLUENCE_BACKUP_URI   = "/wiki/rest/obm/1.0/runbackup.json"
	CONFLUENCE_PROGRESS_URI = "/wiki/rest/obm/1.0/getprogress.json"
	CONFLUENCE_DOWLOAD_URI  = "/wiki/download"

	// Jira specific urls
	JIRA_BACKUP_URI    = "/rest/backup/1/export/runbackup"
	JIRA_LAST_TASK_URI = "/rest/backup/1/export/lastTaskId"
	JIRA_PROGRESS_URI  = "/rest/internal/2/task/progress"
	JIRA_DOWLOAD_URI   = "/plugins/servlet/export/download"
)

type Config struct {
	Confluence    bool
	Jira          bool
	Url           *url.URL
	User          string
	Password      string
	File          string
	Timeout       time.Duration
	Attachments   bool
	ExportToCloud bool
}

func LoadConfig() *Config {

	config := &Config{}

	baseUrl := pflag.String("url", "", "Url of the jira/confuence instance")
	pflag.BoolVar(&config.Jira, "jira", false, "Perform a backup of JIRA")
	pflag.BoolVar(&config.Confluence, "confluence", false, "Perform a backup of Confluence")
	pflag.StringVar(&config.User, "user", "", "User to authenticate against atlassian")
	pflag.StringVar(&config.Password, "pass", "", "Password to authenticate with")
	pflag.StringVar(&config.File, "file", "./backup.zip", "File to store the backup in")
	pflag.DurationVar(&config.Timeout, "timeout", 3*60*60*1000000000, "Timeout wait for the backup, eg: 2h45m")
	pflag.BoolVar(&config.Attachments, "attachments", true, "Backup attachments")
	pflag.BoolVar(&config.ExportToCloud, "exporttocloud", true, "Perform a backup that can be restored in the cloud")
	pflag.Parse()

	if config.Jira == false && config.Confluence == false {
		fmt.Print("Please specify if you want to backup jira or confluence\n\n")
		pflag.Usage()
		os.Exit(1)
	}

	if config.User == "" {
		if user, ok := os.LookupEnv(ENV_USER); ok {
			config.User = user
		} else {
			fmt.Printf("Please specify a user, or declare the '%s' environment variable\n\n", ENV_USER)
			pflag.Usage()
			os.Exit(1)
		}
	}

	if config.Password == "" {
		if pass, ok := os.LookupEnv(ENV_PASS); ok {
			config.Password = pass
		} else {
			fmt.Printf("Please specify a password, or declare the '%s' environment variable\n\n", ENV_PASS)
			pflag.Usage()
			os.Exit(1)
		}
	}

	parsedUrl, url_err := url.Parse(*baseUrl)
	if url_err != nil {
		fmt.Printf("Please specify a correct url: %v\n\n", url_err)
		pflag.Usage()
		os.Exit(1)
	}
	config.Url = parsedUrl

	return config
}

type Atlassian struct {
	config         *Config
	httpClient     *http.Client
	httpDownloader *http.Client
	startTime      int64
	errors         int64
}

func (atl *Atlassian) TriggerBackup() {

	uri := CONFLUENCE_BACKUP_URI
	if atl.config.Jira {
		uri = JIRA_BACKUP_URI
	}

	attachments, toCloud := "false", "false"
	if atl.config.Attachments {
		attachments = "true"
	}
	if atl.config.ExportToCloud {
		toCloud = "true"
	}

	body := fmt.Sprintf(`{"cbAttachments": "%s", "exportToCloud": "%s" }`, attachments, toCloud)
	resp, err := atl.doRequest(http.MethodPost, uri, &body, false)

	if err != nil {
		log.Fatalf("Unable to trigger the backup, error: %v", err)
	}

	if resp.StatusCode != 200 {
		log.Fatalf("Unable to trigger backup, status: %v, body: %s", resp.Status, getBody(resp))
	}

	log.Print("OUTPUT from trigger backup:")
	log.Print(getBody(resp))
	log.Print("Successfully started backup process")

}

func (atl *Atlassian) GetFileName() string {
	if atl.config.Confluence {
		return atl.confluenceProgress()
	} else {
		return atl.jiraProgress()
	}
}

func (atl *Atlassian) confluenceProgress() string {

	type JsProgress struct {
		Completed                  bool   `json:"-"`
		FileName                   string `json:"fileName,omitempty"`
		Size                       int64  `json:"size"`
		Status                     string `json:"currentStatus"`
		Percentage                 string `json:"alternativePercentage"`
		ConcurrentBackupInProgress bool   `json:"concurrentBackupInProgress"`
	}

	var filename string

	for atl.sleepAndWait() {

		resp, err := atl.doRequest(
			http.MethodGet,
			CONFLUENCE_PROGRESS_URI+fmt.Sprintf("?_=%d", time.Now().Unix()),
			nil,
			false)

		if err != nil {
			log.Printf("Error retrieving backup progress, %v", err)
			atl.errors++
			continue
		}

		if resp.StatusCode != 200 {
			log.Printf("Unexpected http status while retrieving backup progress, %d", resp.StatusCode)
			atl.errors++
			continue
		}

		var jsProgress = JsProgress{}
		decoder := json.NewDecoder(resp.Body)
		defer resp.Body.Close()

		err = decoder.Decode(&jsProgress)
		if err != nil {
			log.Printf("Unable to decode json response, %v, %v", err, getBody(resp))
			atl.errors++
			continue
		} else {
			log.Printf("Progress update: %s, %s", jsProgress.Status, jsProgress.Percentage)
		}

		if jsProgress.FileName != "" {
			filename = jsProgress.FileName
			log.Printf("Backup task completed, filename: %s", filename)
			break
		}
	}

	return filename
}

func (atl *Atlassian) jiraProgress() string {

	type JsProgress struct {
		Status      string
		Progress    int64
		Description string
		Result      string // yes, just embedded json string... we will unmarshal it in JsProgressResult
	}

	type JsProgressResult struct {
		MediaFileId string
		FileName    string
	}

	var filename string

	for atl.sleepAndWait() {

		// Fetch the last task id
		resp, err := atl.doRequest(
			http.MethodGet,
			JIRA_LAST_TASK_URI+fmt.Sprintf("?_=%d", time.Now().Unix()),
			nil,
			false)

		if err != nil {
			log.Printf("Unable to fetch last task id: %v, (body: %v)", err, getBody(resp))
			atl.errors++
			continue
		}
		taskId := getBody(resp)

		// Fetch the progress for the last task id
		progressUrl := fmt.Sprintf("%s/%s?_=%d", JIRA_PROGRESS_URI, taskId, time.Now().Unix())
		resp, err = atl.doRequest(http.MethodGet, progressUrl, nil, false)

		if err != nil {
			log.Printf("Error retrieving backup progress, %v", err)
			atl.errors++
			continue
		}

		if resp.StatusCode != 200 {
			log.Printf("Unexpected http status while retrieving backup progress, %d", resp.StatusCode)
			atl.errors++
			continue
		}

		var jsProgress = JsProgress{}
		decoder := json.NewDecoder(resp.Body)
		err = decoder.Decode(&jsProgress)
		resp.Body.Close()

		if err != nil {
			log.Printf("Unable to decode json response, %v", err)
			atl.errors++
			continue
		} else {
			log.Printf("Progress update, status: %s, progress %d pct", jsProgress.Status, jsProgress.Progress)
		}

		if jsProgress.Status == "Success" {

			var jsProgressResult JsProgressResult
			err = json.Unmarshal([]byte(jsProgress.Result), &jsProgressResult)
			if err != nil {
				log.Printf("Unable to decode inner json string, %v", err)
				atl.errors++
				continue
			} else {
				filename = jsProgressResult.MediaFileId + "/" + jsProgressResult.FileName
				log.Printf("Progress file available: %s", filename)
				break
			}
		}
	}
	return filename
}

func (atl *Atlassian) sleepAndWait() bool {

	if atl.errors > 4 {
		log.Fatal("Too many http errors while retrieving backup progress")
	}

	if atl.startTime+atl.config.Timeout.Nanoseconds() < time.Now().UnixNano() {
		log.Fatal("Timeout while waiting for backup completion")
	}

	time.Sleep(time.Second * 10)
	return true
}

func (atl *Atlassian) Download(fileName string) {

	uri := CONFLUENCE_DOWLOAD_URI
	if atl.config.Jira {
		uri = JIRA_DOWLOAD_URI
	}
	downloadUrl := uri + "/" + fileName

	log.Print("Starting download, this might take some time....")

	resp, err := atl.doRequest(http.MethodGet, downloadUrl, nil, true)

	if err != nil {
		log.Fatalf("Unable to download backup: %v", err)
	}

	if resp.StatusCode != 200 {
		log.Fatalf("Unable to download, got http status %d for %s", resp.StatusCode, downloadUrl)
	}

	file, err := os.Create(atl.config.File)
	if err != nil {
		log.Fatalf("Unable to open file %s for writing: %v", atl.config.File, err)
	}

	defer file.Close()
	written, err := io.Copy(file, resp.Body)
	if err != nil {
		log.Fatalf("Error during write to file %s: %v", atl.config.File, err)
	}

	log.Printf("Sucessfully downloaded: %s, %d bytes", atl.config.File, written)
}

func Login(config *Config) *Atlassian {

	jar, err := cookiejar.New(&cookiejar.Options{})
	if err != nil {
		log.Fatalf("Unable to create cookie jar, error: %v", err)
	}

	atl := &Atlassian{
		config:         config,
		httpClient:     &http.Client{Jar: jar, Timeout: time.Second * 30},
		httpDownloader: &http.Client{Jar: jar, Timeout: time.Hour * 3},
		startTime:      time.Now().UnixNano(),
		errors:         0,
	}

	body := fmt.Sprintf(`{"username": "%s", "password": "%s"}`, config.User, config.Password)
	resp, err := atl.doRequest(http.MethodPost, LOGIN_URI, &body, false)

	if err != nil {
		log.Fatalf("Unable to login, error: %v", err)
	}

	if resp.StatusCode != 200 {
		log.Fatalf("Unable to login, status: %v", resp.Status)
	}

	return atl
}

func (atl *Atlassian) doRequest(method string, uri string, body *string, longTimeOut bool) (*http.Response, error) {

	var bodyRdr io.Reader
	if body != nil {
		bodyRdr = strings.NewReader(*body)
	}
	req, err := http.NewRequest(method, atl.config.Url.String()+uri, bodyRdr)

	if err != nil {
		log.Fatalf("Unable to create http request, error: %v", err)
	}
	log.Printf("Performing %s => %s", method, req.URL.String())

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Atlassian-Token", "no-check")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	if longTimeOut {
		return atl.httpDownloader.Do(req)
	} else {
		return atl.httpClient.Do(req)
	}
}

func getBody(resp *http.Response) string {
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return string(body)
}

func main() {
	config := LoadConfig()
	atlassian := Login(config)
	atlassian.TriggerBackup()
	fileName := atlassian.GetFileName()
	atlassian.Download(fileName)
}