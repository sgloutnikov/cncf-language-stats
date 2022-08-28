package main

import (
	"context"
	"encoding/json"
	"flag"
	"github.com/google/go-github/v47/github"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"sort"
	"strings"
	"time"
)

var GitHubToken string

func init() {
	token, ok := os.LookupEnv("GITHUB_TOKEN")
	if !ok {
		log.Fatal("GITHUB_TOKEN ENV variable required")
	}
	GitHubToken = token
}

type Repos struct {
	Graduated  map[string]string `yaml:"Graduated"`
	Incubating map[string]string `yaml:"Incubating"`
	Sandbox    map[string]string `yaml:"Sandbox"`
}

type RepoStats struct {
	GitHubClient *github.Client
	Throttle     time.Duration
	JSONResult
}

type JSONResult struct {
	TopLanguage map[string]int `json:"topLanguage"`
	TotalLines  map[string]int `json:"totalLines"`
}

type LanguageLines struct {
	Language string
	Lines    int
}

// LanguageLinesList A slice of LanguageLinesList that implements sort.Interface to sort by values
type LanguageLinesList []LanguageLines

func (l LanguageLinesList) Len() int           { return len(l) }
func (l LanguageLinesList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l LanguageLinesList) Less(i, j int) bool { return l[i].Lines < l[j].Lines }

func main() {
	var graduated, incubating, sandbox bool
	flag.BoolVar(&graduated, "graduated", false, "Process graduated projects")
	flag.BoolVar(&incubating, "incubating", false, "Process incubating projects")
	flag.BoolVar(&sandbox, "sandbox", false, "Process sandbox projects")
	flag.Parse()

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: GitHubToken},
	)
	tc := oauth2.NewClient(context.Background(), ts)
	results := RepoStats{
		GitHubClient: github.NewClient(tc),
		Throttle:     3 * time.Second,
	}

	var repos Repos
	f, err := os.ReadFile("repos.yaml")
	if err != nil {
		log.Fatal(err)
	}
	if err := yaml.Unmarshal(f, &repos); err != nil {
		log.Fatal(err)
	}

	if graduated {
		results.ProcessProjects(repos.Graduated)
		results.SaveResultsToFile("graduated")
	}
	if incubating {
		results.ProcessProjects(repos.Incubating)
		results.SaveResultsToFile("incubating")
	}
	if sandbox {
		results.ProcessProjects(repos.Sandbox)
		results.SaveResultsToFile("sandbox")
	}
}

func (r *RepoStats) ProcessProjects(projects map[string]string) {
	// Reset counts for each project group
	r.JSONResult = JSONResult{
		TopLanguage: make(map[string]int),
		TotalLines:  make(map[string]int),
	}
	for name, ghUrl := range projects {
		log.Println("Getting language stats for", name)
		owner, repo := getOwnerAndRepo(ghUrl)
		repoLanguages, _, err := r.GitHubClient.Repositories.ListLanguages(context.Background(), owner, repo)
		if err != nil {
			log.Fatal(err)
		}

		if len(repoLanguages) == 0 {
			log.Println(name, "does not contain any language stats")
			continue
		}

		l := sortLanguageMap(repoLanguages)

		// Process repo language statistics
		r.processTopLanguageStats(l)
		r.processTotalLinesStats(l)

		// Some sort of throttle
		time.Sleep(r.Throttle)
	}
}

func (r *RepoStats) SaveResultsToFile(repoGroup string) {
	jsonResult, err := json.MarshalIndent(r.JSONResult, "", " ")
	if err != nil {
		log.Println(err)
	}
	os.WriteFile(getResultFilePath(repoGroup), jsonResult, 0644)
}

func (r *RepoStats) processTopLanguageStats(l LanguageLinesList) {
	r.TopLanguage[l[0].Language]++
}

func (r *RepoStats) processTotalLinesStats(l LanguageLinesList) {
	for _, language := range l {
		r.TotalLines[language.Language] += language.Lines
	}
}

func getOwnerAndRepo(repoUrl string) (string, string) {
	// https://github.com/containerd/containerd
	ownerRepo := strings.Split(repoUrl, "/")
	return ownerRepo[len(ownerRepo)-2], ownerRepo[len(ownerRepo)-1]
}

func sortLanguageMap(repoLanguages map[string]int) LanguageLinesList {
	l := make(LanguageLinesList, len(repoLanguages))
	var i int
	for lang, lines := range repoLanguages {
		l[i] = LanguageLines{Language: lang, Lines: lines}
		i++
	}
	// Sort descending by number of lines
	sort.Sort(sort.Reverse(l))
	return l
}

func getResultFilePath(repoGroup string) string {
	currentTime := time.Now().UTC()
	date := currentTime.Format("2006-01-02")
	basePath := "results/"
	filename := date + "-" + repoGroup + ".json"
	return basePath + filename
}
